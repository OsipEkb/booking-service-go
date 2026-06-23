package service_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/models"
	"booking-service/app/service"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stubs ---

type stubBookingRepo struct {
	booking *models.Booking
	updated *models.Booking
}

func (r *stubBookingRepo) Create(_ context.Context, _ *models.Booking) (int64, error) {
	return 1, nil
}
func (r *stubBookingRepo) GetByID(_ context.Context, _ int64) (*models.Booking, error) {
	return r.booking, nil
}
func (r *stubBookingRepo) Update(_ context.Context, b *models.Booking) error {
	r.updated = b
	return nil
}
func (r *stubBookingRepo) GetByFilter(_ context.Context, _ models.BookingFilter) ([]models.Booking, int64, error) {
	return nil, 0, nil
}
func (r *stubBookingRepo) GetAwaitingConfirmation(_ context.Context, _ int) ([]models.Booking, error) {
	return nil, nil
}
func (r *stubBookingRepo) GetStatistics(_ context.Context, _, _ time.Time) (models.BookingStatistics, error) {
	return models.BookingStatistics{}, nil
}
func (r *stubBookingRepo) GetPendingCancellations(_ context.Context, _ time.Time) ([]models.Booking, error) {
	return nil, nil
}
func (r *stubBookingRepo) BeginTx(_ context.Context) (pgx.Tx, error) {
	return &stubTx{}, nil
}
func (r *stubBookingRepo) CreateTx(_ context.Context, _ pgx.Tx, _ *models.Booking) (int64, error) {
	return 1, nil
}
func (r *stubBookingRepo) UpdateTx(_ context.Context, _ pgx.Tx, b *models.Booking) error {
	r.updated = b
	return nil
}

type stubTx struct{}

func (t *stubTx) Begin(_ context.Context) (pgx.Tx, error) { return t, nil }
func (t *stubTx) Commit(_ context.Context) error          { return nil }
func (t *stubTx) Rollback(_ context.Context) error        { return nil }
func (t *stubTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (t *stubTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults { return nil }
func (t *stubTx) LargeObjects() pgx.LargeObjects                             { return pgx.LargeObjects{} }
func (t *stubTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (t *stubTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (t *stubTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) { return nil, nil }
func (t *stubTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row        { return nil }
func (t *stubTx) Conn() *pgx.Conn                                               { return nil }

type stubHistoryRepo struct{}

func (r *stubHistoryRepo) AddTx(_ context.Context, _ pgx.Tx, _ models.BookingHistoryEntry) error {
	return nil
}
func (r *stubHistoryRepo) GetByBookingID(_ context.Context, _ int64, _, _ int) ([]models.BookingHistoryEntry, int64, error) {
	return nil, 0, nil
}

type stubProcessedEventRepo struct {
	processed map[string]bool
	marked    []string
}

func newStubProcessedEventRepo() *stubProcessedEventRepo {
	return &stubProcessedEventRepo{processed: make(map[string]bool)}
}

func (r *stubProcessedEventRepo) IsProcessed(_ context.Context, eventID string) (bool, error) {
	return r.processed[eventID], nil
}
func (r *stubProcessedEventRepo) MarkProcessedTx(_ context.Context, _ pgx.Tx, eventID string) error {
	r.marked = append(r.marked, eventID)
	return nil
}

// --- helpers ---

func newConfirmedBooking(t *testing.T) *models.Booking {
	t.Helper()
	b, err := models.NewBooking(1, 1, time.Now().Add(24*time.Hour), time.Now().Add(48*time.Hour))
	require.NoError(t, err)
	require.NoError(t, b.Confirm())
	return b
}

func newAwaitingBooking(t *testing.T) *models.Booking {
	t.Helper()
	b, err := models.NewBooking(1, 1, time.Now().Add(24*time.Hour), time.Now().Add(48*time.Hour))
	require.NoError(t, err)
	return b
}

// --- tests ---

func TestConfirm_DuplicateEvent_IsIgnored(t *testing.T) {
	evRepo := newStubProcessedEventRepo()
	evRepo.processed["evt-123"] = true

	repo := &stubBookingRepo{booking: newConfirmedBooking(t)}
	svc := service.NewBookingsService(repo, &stubHistoryRepo{}, evRepo, nil, nil, zap.NewNop())

	err := svc.Confirm(context.Background(), 1, "evt-123")

	assert.NoError(t, err, "дубликат события должен игнорироваться без ошибки")
	assert.Nil(t, repo.updated, "бронирование не должно обновляться при дубликате")
}

func TestConfirm_NewEvent_MarksProcessed(t *testing.T) {
	evRepo := newStubProcessedEventRepo()
	repo := &stubBookingRepo{booking: newAwaitingBooking(t)}
	svc := service.NewBookingsService(repo, &stubHistoryRepo{}, evRepo, nil, nil, zap.NewNop())

	err := svc.Confirm(context.Background(), 1, "evt-456")

	require.NoError(t, err)
	assert.Contains(t, evRepo.marked, "evt-456", "eventID должен быть записан в processed_events")
}

func TestConfirm_EmptyEventID_SkipsIdempotencyCheck(t *testing.T) {
	evRepo := newStubProcessedEventRepo()
	repo := &stubBookingRepo{booking: newAwaitingBooking(t)}
	svc := service.NewBookingsService(repo, &stubHistoryRepo{}, evRepo, nil, nil, zap.NewNop())

	// Worker polling — eventID пустой, идемпотентность не применяется
	err := svc.Confirm(context.Background(), 1, "")

	require.NoError(t, err)
	assert.Empty(t, evRepo.marked, "при пустом eventID ничего не должно записываться")
}

func TestCancelFromEvent_DuplicateEvent_IsIgnored(t *testing.T) {
	evRepo := newStubProcessedEventRepo()
	evRepo.processed["evt-cancel-dup"] = true

	repo := &stubBookingRepo{booking: newAwaitingBooking(t)}
	svc := service.NewBookingsService(repo, &stubHistoryRepo{}, evRepo, nil, nil, zap.NewNop())

	err := svc.CancelFromEvent(context.Background(), 1, "some reason", "evt-cancel-dup")

	assert.NoError(t, err)
	assert.Nil(t, repo.updated, "бронирование не должно обновляться при дубликате")
}

func TestHandleCancelError_DuplicateEvent_IsIgnored(t *testing.T) {
	evRepo := newStubProcessedEventRepo()
	evRepo.processed["evt-dlq-dup"] = true

	repo := &stubBookingRepo{booking: newAwaitingBooking(t)}
	svc := service.NewBookingsService(repo, &stubHistoryRepo{}, evRepo, nil, nil, zap.NewNop())

	err := svc.HandleCancelError(context.Background(), 1, "evt-dlq-dup")

	assert.NoError(t, err)
	assert.Nil(t, repo.updated)
}

// Проверяем dto чтобы не было ошибки импорта
var _ = dto.DateFormat
