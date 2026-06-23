package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/messaging"
	"booking-service/app/models"
)

// OutboxSaver сохраняет доменные события в outbox для гарантированной доставки.
type OutboxSaver interface {
	SaveTx(ctx context.Context, tx pgx.Tx, event messaging.BookingStatusChangedEvent) error
}

// BookingsService обрабатывает команды (изменение состояния) для бронирований.
//
// Этот сервис -- оркестратор: он координирует домен и репозиторий,
// но НЕ содержит бизнес-правила (они в models.Booking).
type BookingsService struct {
	repo               models.BookingRepository
	historyRepo        models.BookingHistoryRepository
	processedEventRepo models.ProcessedEventRepository
	publisher          *messaging.Publisher
	outboxSaver        OutboxSaver
	logger             *zap.Logger
}

// NewBookingsService создаёт новый BookingsService.
func NewBookingsService(
	repo models.BookingRepository,
	historyRepo models.BookingHistoryRepository,
	processedEventRepo models.ProcessedEventRepository,
	publisher *messaging.Publisher,
	outboxSaver OutboxSaver,
	logger *zap.Logger,
) *BookingsService {
	return &BookingsService{
		repo:               repo,
		historyRepo:        historyRepo,
		processedEventRepo: processedEventRepo,
		publisher:          publisher,
		outboxSaver:        outboxSaver,
		logger:             logger,
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

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return 0, fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := s.repo.CreateTx(ctx, tx, booking)
	if err != nil {
		return 0, fmt.Errorf("сохранение бронирования: %w", err)
	}

	reason := "Booking created by user"
	initiatedBy := fmt.Sprintf("%d", req.UserID)
	if err := s.historyRepo.AddTx(ctx, tx, models.BookingHistoryEntry{
		BookingID:   id,
		OldStatus:   nil,
		NewStatus:   models.BookingStatusAwaitsConfirmation,
		ChangedAt:   time.Now(),
		Reason:      &reason,
		InitiatedBy: initiatedBy,
	}); err != nil {
		return 0, fmt.Errorf("запись истории: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("коммит транзакции: %w", err)
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

// Cancel переводит бронирование в статус cancellation_pending.
func (s *BookingsService) Cancel(ctx context.Context, id int64) error {
	return s.cancelWithReason(ctx, id, "Cancelled by user request", "User", "")
}

// cancelWithReason — внутренняя реализация отмены.
// eventID непустой только при вызове из обработчика событий (для идемпотентности).
func (s *BookingsService) cancelWithReason(ctx context.Context, id int64, reason, initiatedBy, eventID string) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	oldStatus := booking.Status()

	if err := booking.StartCancellation(time.Now()); err != nil {
		return err
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.UpdateTx(ctx, tx, booking); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	if err := s.historyRepo.AddTx(ctx, tx, models.BookingHistoryEntry{
		BookingID:   id,
		OldStatus:   &oldStatus,
		NewStatus:   booking.Status(),
		ChangedAt:   time.Now(),
		Reason:      &reason,
		InitiatedBy: initiatedBy,
	}); err != nil {
		return fmt.Errorf("запись истории: %w", err)
	}

	if eventID != "" {
		if err := s.markProcessedTx(ctx, tx, eventID); err != nil {
			return err
		}
	}

	if s.outboxSaver != nil {
		if err := s.outboxSaver.SaveTx(ctx, tx, messaging.BookingStatusChangedEvent{
			EventId:   messaging.NewMessageID(),
			BookingId: id,
			OldStatus: string(oldStatus),
			NewStatus: string(booking.Status()),
			ChangedAt: time.Now().UTC(),
			Reason:    reason,
		}); err != nil {
			return fmt.Errorf("сохранение события в outbox: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("коммит транзакции: %w", err)
	}

	s.logger.Info("бронирование переведено в статус отмены", zap.Int64("id", id))

	if s.publisher != nil {
		if err := s.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
			EventId:   messaging.NewMessageID(),
			RequestId: messaging.BookingIDToRequestID(id),
		}); err != nil {
			s.logger.Error("ошибка публикации CancelBookingJob", zap.Error(err), zap.Int64("bookingId", id))
		}
	}

	return nil
}

// CancelFromEvent отменяет бронирование по событию от Catalog (BookingJobDenied).
// eventID используется для идемпотентности: повторное событие игнорируется.
func (s *BookingsService) CancelFromEvent(ctx context.Context, id int64, reason, eventID string) error {
	if eventID != "" {
		if dup, err := s.isDuplicate(ctx, eventID); err != nil {
			return err
		} else if dup {
			return nil
		}
	}
	return s.cancelWithReason(ctx, id, reason, "System", eventID)
}

// Confirm подтверждает бронирование по ID.
// eventID используется для идемпотентности: повторное событие игнорируется.
// Передайте пустую строку, если вызов не из обработчика событий (например, из worker polling).
func (s *BookingsService) Confirm(ctx context.Context, id int64, eventID string) error {
	if eventID != "" {
		if dup, err := s.isDuplicate(ctx, eventID); err != nil {
			return err
		} else if dup {
			return nil
		}
	}
	return s.confirmWithInitiator(ctx, id, "System", eventID)
}

// confirmWithInitiator — внутренняя реализация подтверждения.
func (s *BookingsService) confirmWithInitiator(ctx context.Context, id int64, initiatedBy, eventID string) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if booking.Status() == models.BookingStatusCancellationPending {
		s.logger.Warn("race condition обнаружен: Catalog подтвердил бронирование, которое уже в процессе отмены; принимаем факт подтверждения для синхронизации с Catalog",
			zap.Int64("id", id),
		)
	}

	oldStatus := booking.Status()

	if err := booking.Confirm(); err != nil {
		return err
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.UpdateTx(ctx, tx, booking); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	reason := "Booking job confirmed by Catalog Service"
	if err := s.historyRepo.AddTx(ctx, tx, models.BookingHistoryEntry{
		BookingID:   id,
		OldStatus:   &oldStatus,
		NewStatus:   booking.Status(),
		ChangedAt:   time.Now(),
		Reason:      &reason,
		InitiatedBy: initiatedBy,
	}); err != nil {
		return fmt.Errorf("запись истории: %w", err)
	}

	if eventID != "" {
		if err := s.markProcessedTx(ctx, tx, eventID); err != nil {
			return err
		}
	}

	if s.outboxSaver != nil {
		if err := s.outboxSaver.SaveTx(ctx, tx, messaging.BookingStatusChangedEvent{
			EventId:   messaging.NewMessageID(),
			BookingId: id,
			OldStatus: string(oldStatus),
			NewStatus: string(booking.Status()),
			ChangedAt: time.Now().UTC(),
			Reason:    "Booking job confirmed by Catalog Service",
		}); err != nil {
			return fmt.Errorf("сохранение события в outbox: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("коммит транзакции: %w", err)
	}

	s.logger.Info("бронирование подтверждено", zap.Int64("id", id))

	return nil
}

// HandleCancelError выполняет rollback отмены при ошибке (DLQ handler).
// eventID используется для идемпотентности: повторное событие игнорируется.
func (s *BookingsService) HandleCancelError(ctx context.Context, id int64, eventID string) error {
	if eventID != "" {
		if dup, err := s.isDuplicate(ctx, eventID); err != nil {
			return err
		} else if dup {
			return nil
		}
	}
	return s.handleCancelErrorWithInitiator(ctx, id, "System", eventID)
}

// handleCancelErrorWithInitiator — внутренняя реализация rollback.
func (s *BookingsService) handleCancelErrorWithInitiator(ctx context.Context, id int64, initiatedBy, eventID string) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrBookingNotFound) {
			s.logger.Warn("бронирование не найдено при откате отмены", zap.Int64("id", id))
			return nil
		}
		return err
	}

	oldStatus := booking.Status()

	if err := booking.RollbackCancellation(); err != nil {
		return fmt.Errorf("откат отмены бронирования %d: %w", id, err)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.UpdateTx(ctx, tx, booking); err != nil {
		return fmt.Errorf("обновление бронирования при откате: %w", err)
	}

	reason := "Cancellation rolled back: DLQ error"
	if err := s.historyRepo.AddTx(ctx, tx, models.BookingHistoryEntry{
		BookingID:   id,
		OldStatus:   &oldStatus,
		NewStatus:   booking.Status(),
		ChangedAt:   time.Now(),
		Reason:      &reason,
		InitiatedBy: initiatedBy,
	}); err != nil {
		return fmt.Errorf("запись истории: %w", err)
	}

	if eventID != "" {
		if err := s.markProcessedTx(ctx, tx, eventID); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("коммит транзакции: %w", err)
	}

	s.logger.Info("отмена не удалась, статус возвращён",
		zap.Int64("id", id),
		zap.String("status", string(booking.Status())),
	)
	return nil
}

// isDuplicate проверяет, обрабатывалось ли событие ранее.
// Возвращает (true, nil) если дубликат — обработчик должен вернуть nil.
func (s *BookingsService) isDuplicate(ctx context.Context, eventID string) (bool, error) {
	processed, err := s.processedEventRepo.IsProcessed(ctx, eventID)
	if err != nil {
		return false, fmt.Errorf("проверка идемпотентности: %w", err)
	}
	if processed {
		s.logger.Warn("дубликат события проигнорирован", zap.String("eventId", eventID))
		return true, nil
	}
	return false, nil
}

// markProcessedTx записывает eventID в рамках транзакции.
// Конфликт UNIQUE (код 23505) означает, что параллельный instance уже обработал событие — не ошибка.
func (s *BookingsService) markProcessedTx(ctx context.Context, tx pgx.Tx, eventID string) error {
	err := s.processedEventRepo.MarkProcessedTx(ctx, tx, eventID)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		s.logger.Warn("duplicate event on concurrent processing, ignoring", zap.String("eventId", eventID))
		return nil
	}
	return fmt.Errorf("запись идемпотентности: %w", err)
}

