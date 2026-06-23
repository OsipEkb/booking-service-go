package worker

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

// OutboxWorker читает неотправленные сообщения из outbox и доставляет их в RabbitMQ.
type OutboxWorker struct {
	repo      models.OutboxRepository
	publisher *messaging.Publisher
	interval  time.Duration
	maxRetry  int
	batchSize int
	logger    *zap.Logger
}

// NewOutboxWorker создаёт новый OutboxWorker.
func NewOutboxWorker(
	repo models.OutboxRepository,
	publisher *messaging.Publisher,
	interval time.Duration,
	maxRetry int,
	batchSize int,
	logger *zap.Logger,
) *OutboxWorker {
	return &OutboxWorker{
		repo:      repo,
		publisher: publisher,
		interval:  interval,
		maxRetry:  maxRetry,
		batchSize: batchSize,
		logger:    logger,
	}
}

// Run запускает воркер. Блокирует до отмены контекста.
func (w *OutboxWorker) Run(ctx context.Context) {
	w.logger.Info("outbox worker запущен",
		zap.Duration("interval", w.interval),
		zap.Int("maxRetry", w.maxRetry),
		zap.Int("batchSize", w.batchSize),
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("outbox worker остановлен")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *OutboxWorker) processBatch(ctx context.Context) {
	messages, err := w.repo.GetPending(ctx, w.maxRetry, w.batchSize)
	if err != nil {
		w.logger.Error("outbox: ошибка получения сообщений", zap.Error(err))
		return
	}
	if len(messages) == 0 {
		return
	}

	w.logger.Info("outbox: обрабатываем сообщения", zap.Int("count", len(messages)))
	success, failed := 0, 0

	for _, msg := range messages {
		var event messaging.BookingStatusChangedEvent
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
			w.logger.Error("outbox: ошибка десериализации", zap.Int64("msgId", msg.ID), zap.Error(err))
			msg.MarkAsFailed(err.Error())
			w.repo.Update(ctx, msg) //nolint:errcheck
			failed++
			continue
		}

		if err := w.publisher.PublishBookingStatusChanged(ctx, event); err != nil {
			w.logger.Error("outbox: ошибка публикации", zap.Int64("msgId", msg.ID), zap.Error(err))
			msg.MarkAsFailed(err.Error())
			w.repo.Update(ctx, msg) //nolint:errcheck
			failed++
			continue
		}

		msg.MarkAsProcessed()
		w.repo.Update(ctx, msg) //nolint:errcheck
		success++
	}

	w.logger.Info("outbox: обработано", zap.Int("success", success), zap.Int("failed", failed))
}
