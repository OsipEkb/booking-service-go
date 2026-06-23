package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

// OutboxService сохраняет доменные события в outbox для гарантированной доставки.
type OutboxService struct {
	repo   models.OutboxRepository
	logger *zap.Logger
}

// NewOutboxService создаёт новый OutboxService.
func NewOutboxService(repo models.OutboxRepository, logger *zap.Logger) *OutboxService {
	return &OutboxService{repo: repo, logger: logger}
}

// SaveTx сохраняет BookingStatusChangedEvent в Outbox в рамках переданной транзакции.
//
// ВАЖНО: метод НЕ начинает собственную транзакцию —
// он должен участвовать в транзакции вызывающего метода BookingsService,
// чтобы INSERT в outbox_messages и UPDATE bookings были атомарны.
func (s *OutboxService) SaveTx(ctx context.Context, tx pgx.Tx, event messaging.BookingStatusChangedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("сериализация события для outbox: %w", err)
	}

	msg := models.NewOutboxMessage(messaging.RoutingKeyBookingStatusChanged, string(payload))
	if err := s.repo.SaveTx(ctx, tx, msg); err != nil {
		return fmt.Errorf("сохранение события в outbox: %w", err)
	}

	s.logger.Debug("событие сохранено в outbox", zap.Int64("bookingId", event.BookingId))
	return nil
}
