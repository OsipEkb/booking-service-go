package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
	"booking-service/app/service"
)

type CancelBookingErrorHandler struct {
	service *service.BookingsService
	repo    models.BookingRepository
	logger  *zap.Logger
}

func NewCancelBookingErrorHandler(svc *service.BookingsService, repo models.BookingRepository, logger *zap.Logger) *CancelBookingErrorHandler {
	return &CancelBookingErrorHandler{
		service: svc,
		repo:    repo,
		logger:  logger,
	}
}

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
		isNew, err := h.repo.RegisterEvent(txCtx, event.EventId, "CancelBookingError")
		if err != nil {
			return err
		}
		if !isNew {
			h.logger.Warn("событие ошибки отмены уже обработано (дубликат), пропускаем", zap.String("eventId", event.EventId))
			return nil
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
