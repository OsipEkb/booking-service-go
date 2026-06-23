package service_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spyOutboxSaver захватывает вызовы SaveTx.
type spyOutboxSaver struct {
	events []messaging.BookingStatusChangedEvent
}

func (s *spyOutboxSaver) SaveTx(_ context.Context, _ pgx.Tx, event messaging.BookingStatusChangedEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestConfirm_SavesEventToOutboxInsteadOfPublishingDirectly(t *testing.T) {
	evRepo := newStubProcessedEventRepo()
	repo := &stubBookingRepo{booking: newAwaitingBooking(t)}
	spy := &spyOutboxSaver{}

	svc := service.NewBookingsService(repo, &stubHistoryRepo{}, evRepo, nil, spy, zap.NewNop())

	err := svc.Confirm(context.Background(), 1, "")
	require.NoError(t, err)

	require.Len(t, spy.events, 1, "OutboxSaver.SaveTx должен быть вызван ровно 1 раз")
	evt := spy.events[0]
	assert.Equal(t, int64(1), evt.BookingId)
	assert.Equal(t, "awaits_confirmation", evt.OldStatus)
	assert.Equal(t, "confirmed", evt.NewStatus)
	assert.NotEmpty(t, evt.EventId)
}

func TestCancel_SavesEventToOutboxInsteadOfPublishingDirectly(t *testing.T) {
	evRepo := newStubProcessedEventRepo()
	repo := &stubBookingRepo{booking: newAwaitingBooking(t)}
	spy := &spyOutboxSaver{}

	svc := service.NewBookingsService(repo, &stubHistoryRepo{}, evRepo, nil, spy, zap.NewNop())

	err := svc.Cancel(context.Background(), 1)
	require.NoError(t, err)

	require.Len(t, spy.events, 1, "OutboxSaver.SaveTx должен быть вызван ровно 1 раз")
	evt := spy.events[0]
	assert.Equal(t, int64(1), evt.BookingId)
	assert.Equal(t, "awaits_confirmation", evt.OldStatus)
	assert.Equal(t, "cancellation_pending", evt.NewStatus)
	assert.NotEmpty(t, evt.EventId)
}
