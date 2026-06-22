package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

type mockBookingRepository struct {
	models.BookingRepository
	registerEventFunc func(ctx context.Context, eventID string, eventType string) (bool, error)
	withTxFunc        func(ctx context.Context, fn func(ctx context.Context) error) error
}

func (m *mockBookingRepository) RegisterEvent(ctx context.Context, eventID string, eventType string) (bool, error) {
	if m.registerEventFunc != nil {
		return m.registerEventFunc(ctx, eventID, eventType)
	}
	return true, nil
}

func (m *mockBookingRepository) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if m.withTxFunc != nil {
		return m.withTxFunc(ctx, fn)
	}
	return fn(ctx)
}

func TestBookingConfirmedHandler_Handle_Idempotency(t *testing.T) {
	logger := zap.NewNop()

	mockRepo := &mockBookingRepository{
		registerEventFunc: func(ctx context.Context, eventID string, eventType string) (bool, error) {
			return false, nil // Имитируем, что ON CONFLICT сработал и вернул false (дубликат!)
		},
		withTxFunc: func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		},
	}

	handler := NewBookingConfirmedHandler(nil, nil, mockRepo, logger)

	event := messaging.BookingJobConfirmed{
		EventId:   "unique-msg-uuid-123",
		RequestId: "00000000-0000-0000-0000-00000000007b", // Наш валидный UUID-HEX
		Id:        456,
	}
	body, _ := json.Marshal(event)

	err := handler.Handle(context.Background(), body)

	if err != nil {
		t.Fatalf("Ожидался успешный пропуск дубликата (nil) через ON CONFLICT, но получена ошибка: %v", err)
	}
}
