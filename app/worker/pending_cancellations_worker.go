package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

// PendingCancellationsWorker -- фоновый воркер для повторной отправки
// команд отмены по зависшим бронированиям в статусе cancellation_pending.
type PendingCancellationsWorker struct {
	repo      models.BookingRepository
	publisher *messaging.Publisher
	timeout   time.Duration
	interval  time.Duration
	logger    *zap.Logger
}

// NewPendingCancellationsWorker создаёт новый воркер зависших отмен.
func NewPendingCancellationsWorker(
	repo models.BookingRepository,
	publisher *messaging.Publisher,
	timeout time.Duration,
	interval time.Duration,
	logger *zap.Logger,
) *PendingCancellationsWorker {
	return &PendingCancellationsWorker{
		repo:      repo,
		publisher: publisher,
		timeout:   timeout,
		interval:  interval,
		logger:    logger,
	}
}

// Run запускает воркер. Блокирует до отмены контекста.
func (w *PendingCancellationsWorker) Run(ctx context.Context) {
	w.logger.Info("воркер зависших отмен запущен",
		zap.Duration("interval", w.interval),
		zap.Duration("timeout", w.timeout),
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("воркер зависших отмен остановлен")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

// processBatch находит зависшие отмены и повторно отправляет команды в Catalog.
func (w *PendingCancellationsWorker) processBatch(ctx context.Context) {
	cutoffTime := time.Now().UTC().Add(-w.timeout)

	bookings, err := w.repo.GetPendingCancellations(ctx, cutoffTime)
	if err != nil {
		w.logger.Error("ошибка получения зависших отмен", zap.Error(err))
		return
	}

	if len(bookings) == 0 {
		return
	}

	w.logger.Info("найдено зависших отмен", zap.Int("count", len(bookings)))

	retryCount, errorCount := 0, 0
	for _, booking := range bookings {
		if err := w.retryCancel(ctx, &booking); err != nil {
			w.logger.Error("ошибка повторной отправки",
				zap.Int64("bookingId", booking.ID()),
				zap.Error(err),
			)
			errorCount++
			continue
		}
		retryCount++
	}

	w.logger.Info("обработано зависших отмен",
		zap.Int("retry", retryCount),
		zap.Int("ошибок", errorCount),
	)
}

// retryCancel повторно отправляет команду отмены для одного бронирования.
func (w *PendingCancellationsWorker) retryCancel(ctx context.Context, booking *models.Booking) error {
	cmd := messaging.CancelBookingJobCommand{
		EventId:   messaging.NewMessageID(),
		RequestId: messaging.BookingIDToRequestID(booking.ID()),
	}

	if err := w.publisher.PublishCancelBookingJob(ctx, cmd); err != nil {
		return err
	}

	w.logger.Info("повторная отправка команды отмены",
		zap.Int64("bookingId", booking.ID()),
		zap.Timep("cancellationSentAt", booking.CancellationSentAt()),
	)

	return nil
}
