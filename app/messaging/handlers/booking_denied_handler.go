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

type BookingDeniedHandler struct {
	service *service.BookingsService
	repo    models.BookingRepository
	logger  *zap.Logger
}

func NewBookingDeniedHandler(svc *service.BookingsService, repo models.BookingRepository, logger *zap.Logger) *BookingDeniedHandler {
	return &BookingDeniedHandler{
		service: svc,
		repo:    repo,
		logger:  logger,
	}
}

func (h *BookingDeniedHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.BookingJobDenied
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация BookingJobDenied: %w", err)
	}

	bookingID, err := messaging.RequestIDToBookingID(event.RequestId)
	if err != nil {
		return fmt.Errorf("извлечение bookingId из RequestId: %w", err)
	}

	h.logger.Info("получено событие BookingJobDenied",
		zap.Int64("bookingId", bookingID),
		zap.String("eventId", event.EventId),
	)

	eventIDStr := event.EventId

	err = h.repo.WithTx(ctx, func(txCtx context.Context) error {
		isNew, err := h.repo.RegisterEvent(txCtx, eventIDStr, "BookingDenied")
		if err != nil {
			return err
		}
		if !isNew {
			h.logger.Warn("событие уже обработано (дубликат), пропускаем", zap.String("eventId", eventIDStr))
			return nil
		}

		if err := h.service.Cancel(txCtx, bookingID); err != nil {
			return fmt.Errorf("отмена бронирования %d: %w", bookingID, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("ошибка транзакции при отмене бронирования: %w", err)
	}

	h.logger.Info("бронирование отменено через событие",
		zap.Int64("bookingId", bookingID),
		zap.String("reason", event.Reason),
	)
	return nil
}
