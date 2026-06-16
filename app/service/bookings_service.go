package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/messaging"
	"booking-service/app/models"
)

// BookingsService обрабатывает команды (изменение состояния) для бронирований.
//
// Этот сервис -- оркестратор: он координирует домен и репозиторий,
// но НЕ содержит бизнес-правила (они в models.Booking).
type BookingsService struct {
	repo      models.BookingRepository
	publisher *messaging.Publisher
	logger    *zap.Logger
}

// NewBookingsService создаёт новый BookingsService.
func NewBookingsService(repo models.BookingRepository, publisher *messaging.Publisher, logger *zap.Logger) *BookingsService {
	return &BookingsService{
		repo:      repo,
		publisher: publisher,
		logger:    logger,
	}
}

// Create создаёт новое бронирование.
//
// Шаги:
//  1. Парсинг дат из строкового формата
//  2. Создание доменного объекта (валидация в конструкторе)
//  3. Сохранение в БД
//  4. Публикация команды в Catalog
//  5. Возврат ID
func (s *BookingsService) Create(ctx context.Context, req dto.CreateBookingRequest) (int64, error) {
	startDate, err := time.Parse(dto.DateFormat, req.StartDate)
	if err != nil {
		return 0, fmt.Errorf("некорректный формат startDate: %w", err)
	}

	endDate, err := time.Parse(dto.DateFormat, req.EndDate)
	if err != nil {
		return 0, fmt.Errorf("некорректный формат endDate: %w", err)
	}

	booking, err := models.NewBooking(req.UserID, req.ResourceID, startDate, endDate)
	if err != nil {
		return 0, err
	}

	id, err := s.repo.Create(ctx, booking)
	if err != nil {
		return 0, fmt.Errorf("сохранение бронирования: %w", err)
	}

	s.logger.Info("бронирование создано",
		zap.Int64("id", id),
		zap.Int64("userId", req.UserID),
		zap.Int64("resourceId", req.ResourceID),
	)

	if err := s.publisher.PublishCreateBookingJob(ctx, messaging.CreateBookingJobCommand{
		EventId:    messaging.NewMessageID(),
		RequestId:  messaging.BookingIDToRequestID(id),
		ResourceId: req.ResourceID,
		StartDate:  req.StartDate,
		EndDate:    req.EndDate,
	}); err != nil {
		s.logger.Error("ошибка публикации CreateBookingJob", zap.Error(err), zap.Int64("bookingId", id))
		// Не возвращаем ошибку -- бронирование уже создано, команда может быть обработана позже
	}

	return id, nil
}

// Cancel отменяет бронирование по ID.
//
// Шаги:
//  1. Загрузка бронирования из БД
//  2. Вызов доменного метода Cancel() (валидация перехода статуса)
//  3. Сохранение обновлённого состояния
//  4. Публикация команды в Catalog
func (s *BookingsService) Cancel(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := booking.StartCancellation(time.Now()); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, booking); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("бронирование отменено", zap.Int64("id", id))

	if err := s.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
		EventId:   messaging.NewMessageID(),
		RequestId: messaging.BookingIDToRequestID(id),
	}); err != nil {
		s.logger.Error("ошибка публикации CancelBookingJob", zap.Error(err), zap.Int64("bookingId", id))
	}

	return nil
}

// Confirm подтверждает бронирование по ID.
func (s *BookingsService) Confirm(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := booking.Confirm(); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, booking); err != nil {
		return fmt.Errorf("обновление статуса бронирования при подтверждении создания: %w", err)
	}
	s.logger.Info("бронирование успешно подтверждено (создано)", zap.Int64("id", id))
	return nil
}

func (s *BookingsService) HandleCancellationConfirmed(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := booking.ConfirmCancellation(); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, booking); err != nil {
		return fmt.Errorf("обновление статуса бронирования при подтверждении отмены: %w", err)
	}
	s.logger.Info("отмена бронирования успешно подтверждена", zap.Int64("id", id))
	return nil
}

func (s *BookingsService) HandleCancelError(ctx context.Context, requestID string) error {
	bookingID, err := messaging.RequestIDToBookingID(requestID)
	if err != nil {
		// Если мы даже не смогли распарсить ID, то и искать в базе нечего.
		// Залогируем это и вернем nil, чтобы битое сообщение удалилось из очереди.
		s.logger.Error("критическая ошибка: не удалось распарсить requestID в DLQ", zap.String("requestId", requestID), zap.Error(err))
		return nil
	}

	booking, err := s.repo.GetByID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, models.ErrBookingNotFound) {
			s.logger.Warn("бронирование для отката не найдено в БД, пропускаем сообщение", zap.Int64("id", bookingID))
			return nil // Возвращаем nil, чтобы сообщение ушло из очереди
		}
		return fmt.Errorf("получение бронирования из БД id=%d: %w", bookingID, err)
	}
	if err := booking.RollbackCancellation(); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, booking); err != nil {
		return fmt.Errorf("откат отмены бронирования в БД id=%d: %w", bookingID, err)
	}
	s.logger.Info("отмена бронирования откатана назад", zap.Int64("id", bookingID))
	return nil
}
func (s *BookingsService) ConfirmCancellation(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := booking.ConfirmCancellation(); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, booking); err != nil {
		return fmt.Errorf("подтверждение отмены бронирования в БД id=%d: %w", id, err)
	}
	s.logger.Info("отмена бронирования успешно подтверждена", zap.Int64("id", id))
	return nil
}
