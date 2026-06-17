package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/service"
)

// CancelBookingErrorHandler обрабатывает ошибки отмены бронирования из DLQ.
type CancelBookingErrorHandler struct {
	service *service.BookingsService
	logger  *zap.Logger
}

// NewCancelBookingErrorHandler создаёт новый обработчик.
func NewCancelBookingErrorHandler(svc *service.BookingsService, logger *zap.Logger) *CancelBookingErrorHandler {
	return &CancelBookingErrorHandler{
		service: svc,
		logger:  logger,
	}
}

// Handle обрабатывает событие ошибки отмены бронирования (выполняет rollback).
func (h *CancelBookingErrorHandler) Handle(ctx context.Context, body []byte) error {
	// 1. Десериализуем событие ошибки
	var event messaging.CancelBookingJobCommand
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация CancelBookingJobCommand: %w", err)
	}

	h.logger.Info("получена ошибка отмены бронирования, запускаем откат",
		zap.String("requestId", event.RequestId),
	)

	// 2. Вызываем метод отката (HandleCancelError), который мы написали в сервисе
	// Он сам внутри распарсит RequestId в числовой ID и вернет статус назад
	if err := h.service.HandleCancelError(ctx, event.RequestId); err != nil {
		return fmt.Errorf("ошибка при выполнении отката для requestId=%s: %w", event.RequestId, err)
	}

	h.logger.Info("откат статуса бронирования успешно завершен", zap.String("requestId", event.RequestId))
	return nil
}
