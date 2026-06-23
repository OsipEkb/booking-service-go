package worker

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

type OutboxWorker struct {
	repo        models.BookingRepository
	publisher   *messaging.Publisher
	interval    time.Duration
	batchSize   int
	maxAttempts int
	logger      *zap.Logger
}

func NewOutboxWorker(
	repo models.BookingRepository,
	publisher *messaging.Publisher,
	interval time.Duration,
	batchSize int,
	maxAttempts int,
	logger *zap.Logger,
) *OutboxWorker {
	return &OutboxWorker{
		repo:        repo,
		publisher:   publisher,
		interval:    interval,
		batchSize:   batchSize,
		maxAttempts: maxAttempts,
		logger:      logger,
	}
}

func (w *OutboxWorker) Run(ctx context.Context) {
	w.logger.Info("Outbox воркер успешно запущен",
		zap.Duration("interval", w.interval),
		zap.Int("batchSize", w.batchSize),
		zap.Int("maxAttempts", w.maxAttempts),
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Outbox воркер остановлен")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *OutboxWorker) processBatch(ctx context.Context) {
	messages, err := w.repo.GetPendingOutboxMessages(ctx, w.batchSize)
	if err != nil {
		w.logger.Error("ошибка получения сообщений из outbox", zap.Error(err))
		return
	}

	if len(messages) == 0 {
		return
	}

	for _, msg := range messages {
		w.processMessage(ctx, msg)
	}
}

func (w *OutboxWorker) processMessage(ctx context.Context, msg *models.OutboxMessage) {
	logger := w.logger.With(zap.Int64("outboxId", msg.ID), zap.String("eventId", msg.EventID))

	if msg.EventType != "BookingStatusChangedEvent" {
		logger.Warn("пропущено неизвестное доменное событие", zap.String("type", msg.EventType))
		msg.Status = "failed"
		_ = w.repo.UpdateOutboxMessage(ctx, msg)
		return
	}

	var event messaging.BookingStatusChangedEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		logger.Error("ошибка десериализации payload события", zap.Error(err))
		msg.Status = "failed"
		_ = w.repo.UpdateOutboxMessage(ctx, msg)
		return
	}

	msg.Attempts++

	err := w.publisher.PublishBookingStatusChanged(ctx, event)
	if err != nil {
		logger.Error("не удалось опубликовать доменное событие из outbox, ретрай", zap.Error(err), zap.Int("attempt", msg.Attempts))

		if msg.Attempts >= w.maxAttempts {
			logger.Error("достигнут лимит попыток отправки outbox события, помечаем как failed")
			msg.Status = "failed"
		}

		if updateErr := w.repo.UpdateOutboxMessage(ctx, msg); updateErr != nil {
			logger.Error("не удалось обновить статус ошибки в outbox таблице", zap.Error(updateErr))
		}
		return
	}

	now := time.Now()
	msg.Status = "processed"
	msg.ProcessedAt = &now

	if updateErr := w.repo.UpdateOutboxMessage(ctx, msg); updateErr != nil {
		logger.Error("не удалось обновить статус отправленного сообщения в outbox", zap.Error(updateErr))
		return
	}

	logger.Info("событие из outbox успешно опубликовано в RabbitMQ")
}
