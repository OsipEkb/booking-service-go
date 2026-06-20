package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
	"booking-service/app/service"
)

// CancelBookingErrorHandler обрабатывает ошибки отмены бронирования из DLQ.
type CancelBookingErrorHandler struct {
	service *service.BookingsService
	repo    models.BookingRepository
	logger  *zap.Logger
}

// NewCancelBookingErrorHandler создаёт новый обработчик.
func NewCancelBookingErrorHandler(svc *service.BookingsService, repo models.BookingRepository, logger *zap.Logger) *CancelBookingErrorHandler {
	return &CancelBookingErrorHandler{
		service: svc,
		repo:    repo,
		logger:  logger,
	}
}

// Handle обрабатывает событие ошибки отмены бронирования (выполняет rollback).
func (h *CancelBookingErrorHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.CancelBookingJobCommand
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация CancelBookingJobCommand: %w", err)
	}

	h.logger.Info("получена ошибка отмены бронирования, запускаем откат",
		zap.String("requestId", event.RequestId),
		zap.String("eventId", event.EventId),
	)

	err := h.repo.WithTx(ctx, func(txCtx context.Context) error {

		if err := h.repo.RegisterEvent(txCtx, event.EventId, "CancelBookingError"); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				h.logger.Warn("событие ошибки отмены уже обработано (дубликат)", zap.String("eventId", event.EventId))
				return nil
			}
			return err
		}
		
		if err := h.service.HandleCancelError(txCtx, event.RequestId); err != nil {
			return fmt.Errorf("ошибка при выполнении отката для requestId=%s: %w", event.RequestId, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("ошибка транзакции в CancelBookingErrorHandler: %w", err)
	}

	h.logger.Info("откат статуса бронирования успешно завершен", zap.String("requestId", event.RequestId))
	return nil
}
