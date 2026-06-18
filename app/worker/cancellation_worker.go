package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

type CancellationWorker struct {
	repo      models.BookingRepository
	publisher *messaging.Publisher
	interval  time.Duration
	timeout   time.Duration
	batchSize int
	logger    *zap.Logger
}

func NewCancellationWorker(
	repo models.BookingRepository,
	publisher *messaging.Publisher,
	interval time.Duration,
	timeout time.Duration,
	batchSize int,
	logger *zap.Logger,
) *CancellationWorker {
	return &CancellationWorker{
		repo:      repo,
		publisher: publisher,
		interval:  interval,
		timeout:   timeout,
		batchSize: batchSize,
		logger:    logger,
	}
}

func (w *CancellationWorker) Run(ctx context.Context) {
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

func (w *CancellationWorker) processBatch(ctx context.Context) {
	status := models.BookingStatusCancellationPending

	filter := models.BookingFilter{
		Status: &status,
		Page:   1,
		Size:   w.batchSize,
	}

	bookings, _, err := w.repo.GetByFilter(ctx, filter)
	if err != nil {
		w.logger.Error("ошибка получения отмен через фильтр", zap.Error(err))
		return
	}

	if len(bookings) == 0 {
		return
	}

	stuckThreshold := time.Now().Add(-w.timeout)

	for _, booking := range bookings {
		if booking.CanceledAt() != nil && booking.CanceledAt().After(stuckThreshold) {
			continue
		}

		w.processBooking(ctx, &booking)
	}
}

func (w *CancellationWorker) processBooking(ctx context.Context, booking *models.Booking) {
	bookingID := booking.ID()
	logger := w.logger.With(zap.Int64("bookingId", bookingID))

	requestID := messaging.BookingIDToRequestID(bookingID)

	err := w.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
		EventId:   "",
		RequestId: requestID,
	})

	if err != nil {
		logger.Error("не удалось повторно отправить команду отмены в очередь", zap.Error(err))
		return
	}

	logger.Info("повторная команда отмены успешно отправлена в очередь")
}
