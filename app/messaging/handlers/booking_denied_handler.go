package handlers

import (
	"booking-service/app/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/service"
)

// BookingDeniedHandler обрабатывает события BookingJobDenied.
type BookingDeniedHandler struct {
	service *service.BookingsService
	repo    models.BookingRepository
	logger  *zap.Logger
}

// NewBookingDeniedHandler создаёт новый обработчик.
func NewBookingDeniedHandler(svc *service.BookingsService, repo models.BookingRepository, logger *zap.Logger) *BookingDeniedHandler {
	return &BookingDeniedHandler{
		service: svc,
		repo:    repo,
		logger:  logger,
	}
}

// Handle обрабатывает событие отклонения бронирования.
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
		if err := h.repo.RegisterEvent(txCtx, eventIDStr, "BookingDenied"); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				h.logger.Warn("событие уже обработано (дубликат)", zap.String("eventId", eventIDStr))
				return nil
			}
			return err
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
