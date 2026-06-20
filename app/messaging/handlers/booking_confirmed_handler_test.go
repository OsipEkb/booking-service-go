package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

type mockBookingRepository struct {
	models.BookingRepository
	registerEventFunc func(ctx context.Context, eventID string, eventType string) error
	withTxFunc        func(ctx context.Context, fn func(ctx context.Context) error) error
}

func (m *mockBookingRepository) RegisterEvent(ctx context.Context, eventID string, eventType string) error {
	if m.registerEventFunc != nil {
		return m.registerEventFunc(ctx, eventID, eventType)
	}
	return nil
}

func (m *mockBookingRepository) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if m.withTxFunc != nil {
		return m.withTxFunc(ctx, fn)
	}
	return fn(ctx)
}

func TestBookingConfirmedHandler_Handle_Idempotency(t *testing.T) {
	logger := zap.NewNop()

	duplicateErr := &pgconn.PgError{
		Code: "23505",
	}

	mockRepo := &mockBookingRepository{
		registerEventFunc: func(ctx context.Context, eventID string, eventType string) error {
			return duplicateErr
		},
		withTxFunc: func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		},
	}

	handler := NewBookingConfirmedHandler(nil, nil, mockRepo, logger)

	event := messaging.BookingJobConfirmed{
		EventId:   "unique-msg-uuid-123",
		RequestId: "00000000-0000-0000-0000-00000000007b",
		Id:        456,
	}
	body, _ := json.Marshal(event)

	err := handler.Handle(context.Background(), body)

	if err != nil {
		t.Fatalf("Ожидался успешный пропуск дубликата (nil), но получена ошибка: %v", err)
	}
}
