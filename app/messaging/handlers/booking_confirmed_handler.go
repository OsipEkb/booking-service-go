package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
	"booking-service/app/service"
)

// BookingConfirmedHandler обрабатывает события BookingJobConfirmed.
type BookingConfirmedHandler struct {
	service *service.BookingsService
	queries *service.BookingsQueries
	repo    models.BookingRepository
	logger  *zap.Logger
}

// NewBookingConfirmedHandler создаёт новый обработчик.
func NewBookingConfirmedHandler(svc *service.BookingsService, queries *service.BookingsQueries, repo models.BookingRepository, logger *zap.Logger) *BookingConfirmedHandler {
	return &BookingConfirmedHandler{
		service: svc,
		queries: queries,
		repo:    repo,
		logger:  logger,
	}
}

// Handle обрабатывает событие подтверждения бронирования.
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
		zap.Int64("catalogJobId", event.Id),
	)

	eventIDStr := strconv.FormatInt(event.Id, 10)

	err = h.repo.WithTx(ctx, func(txCtx context.Context) error {

		if err := h.repo.RegisterEvent(txCtx, eventIDStr, "BookingConfirmed"); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				h.logger.Warn("событие уже обработано (дубликат)", zap.String("eventId", eventIDStr))
				return nil
			}
			return err
		}

		currentStatus, err := h.queries.GetStatus(txCtx, bookingID)
		if err == nil && currentStatus == models.BookingStatusCancellationPending {
			h.logger.Warn("Обнаружен Race Condition: Catalog подтвердил бронирование, находящееся в статусе отмены",
				zap.Int64("bookingId", bookingID),
			)
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
