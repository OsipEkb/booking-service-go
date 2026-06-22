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

type BookingConfirmedHandler struct {
	service *service.BookingsService
	queries *service.BookingsQueries
	repo    models.BookingRepository
	logger  *zap.Logger
}

func NewBookingConfirmedHandler(svc *service.BookingsService, queries *service.BookingsQueries, repo models.BookingRepository, logger *zap.Logger) *BookingConfirmedHandler {
	return &BookingConfirmedHandler{
		service: svc,
		queries: queries,
		repo:    repo,
		logger:  logger,
	}
}

func (h *BookingConfirmedHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.BookingJobConfirmed
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация BookingJobConfirmed: %w", err)
	}

	bookingID, err := messaging.RequestIDToBookingID(event.RequestId)
	if err != nil {
		return fmt.Errorf("извлечение bookingId из RequestId: %w", err)
	}

	h.logger.Info("получено событие BookingJobConfirmed",
		zap.Int64("bookingId", bookingID),
		zap.String("eventId", event.EventId),
	)

	eventIDStr := event.EventId

	err = h.repo.WithTx(ctx, func(txCtx context.Context) error {
		isNew, err := h.repo.RegisterEvent(txCtx, eventIDStr, "BookingConfirmed")
		if err != nil {
			return err
		}
		if !isNew {
			h.logger.Warn("событие уже обработано (дубликат), пропускаем", zap.String("eventId", eventIDStr))
			return nil
		}

		if err := h.service.Confirm(txCtx, bookingID); err != nil {
			return fmt.Errorf("подтверждение бронирования %d: %w", bookingID, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("ошибка транзакции обработки события: %w", err)
	}

	h.logger.Info("бронирование подтверждено через событие", zap.Int64("bookingId", bookingID))
	return nil
}
