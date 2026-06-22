package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/messaging"
	"booking-service/app/models"
)

// BookingsService обрабатывает команды (изменение состояния) для бронирований.
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

	var id int64

	// Оборачиваем создание и самый первый лог аудита в транзакцию
	err = s.repo.WithTx(ctx, func(txCtx context.Context) error {
		var txErr error
		id, txErr = s.repo.Create(txCtx, booking)
		if txErr != nil {
			return fmt.Errorf("сохранение бронирования: %w", txErr)
		}

		// Для нового бронирования начального статуса не было — передаем пустую строку ""
		log := models.NewBookingAuditLog(
			id,
			"",
			booking.Status(),
			strconv.FormatInt(req.UserID, 10),
			"Initial booking creation",
		)

		if txErr := s.repo.SaveAuditLog(txCtx, log); txErr != nil {
			return fmt.Errorf("сохранение начального лога аудита: %w", txErr)
		}

		return nil
	})

	if err != nil {
		return 0, err
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
	}

	return id, nil
}

// Cancel отменяет бронирование по ID.
func (s *BookingsService) Cancel(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	oldStatus := booking.Status()

	if err := booking.StartCancellation(time.Now()); err != nil {
		return err
	}

	err = s.repo.WithTx(ctx, func(txCtx context.Context) error {
		if txErr := s.repo.Update(txCtx, booking); txErr != nil {
			return fmt.Errorf("обновление бронирования: %w", txErr)
		}

		log := models.NewBookingAuditLog(
			booking.ID(),
			oldStatus,
			booking.Status(),
			"User",
			"Cancellation requested by user",
		)

		if txErr := s.repo.SaveAuditLog(txCtx, log); txErr != nil {
			return fmt.Errorf("сохранение лога аудита отмены: %w", txErr)
		}

		return nil
	})

	if err != nil {
		return err
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

	oldStatus := booking.Status()

	if err := booking.Confirm(); err != nil {
		return err
	}

	err = s.repo.WithTx(ctx, func(txCtx context.Context) error {
		if txErr := s.repo.Update(txCtx, booking); txErr != nil {
			return fmt.Errorf("обновление статуса бронирования при подтверждении создания: %w", txErr)
		}

		log := models.NewBookingAuditLog(
			booking.ID(),
			oldStatus,
			booking.Status(),
			"System",
			"Booking confirmed by system",
		)

		if txErr := s.repo.SaveAuditLog(txCtx, log); txErr != nil {
			return fmt.Errorf("сохранение лога аудита подтверждения: %w", txErr)
		}

		return nil
	})

	if err != nil {
		return err
	}

	s.logger.Info("бронирование успешно подтверждено (создано)", zap.Int64("id", id))
	return nil
}

// HandleCancelError откатывает отмену при сбоях в других сервисах (DLQ)
func (s *BookingsService) HandleCancelError(ctx context.Context, requestID string) error {
	bookingID, err := messaging.RequestIDToBookingID(requestID)
	if err != nil {
		s.logger.Error("критическая ошибка: не удалось распарсить requestID в DLQ", zap.String("requestId", requestID), zap.Error(err))
		return nil
	}

	booking, err := s.repo.GetByID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, models.ErrBookingNotFound) {
			s.logger.Warn("бронирование для отката не найдено в БД, пропускаем сообщение", zap.Int64("id", bookingID))
			return nil
		}
		return fmt.Errorf("получение бронирования из БД id=%d: %w", bookingID, err)
	}

	oldStatus := booking.Status()

	if err := booking.RollbackCancellation(); err != nil {
		return err
	}

	err = s.repo.WithTx(ctx, func(txCtx context.Context) error {
		if txErr := s.repo.Update(txCtx, booking); txErr != nil {
			return fmt.Errorf("откат отмены бронирования в БД id=%d: %w", bookingID, txErr)
		}

		log := models.NewBookingAuditLog(
			booking.ID(),
			oldStatus,
			booking.Status(),
			"System",
			"Rollback cancellation due to external processing failure",
		)

		if txErr := s.repo.SaveAuditLog(txCtx, log); txErr != nil {
			return fmt.Errorf("сохранение лога аудита отката отмены: %w", txErr)
		}

		return nil
	})

	if err != nil {
		return err
	}

	s.logger.Info("отмена бронирования откатана назад", zap.Int64("id", bookingID))
	return nil
}
