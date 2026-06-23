package models

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// BookingRepository -- интерфейс репозитория бронирований.
type BookingRepository interface {
	// Create сохраняет новое бронирование и возвращает присвоенный ID.
	Create(ctx context.Context, booking *Booking) (int64, error)

	// GetByID возвращает бронирование по ID.
	GetByID(ctx context.Context, id int64) (*Booking, error)

	// Update обновляет бронирование в хранилище.
	Update(ctx context.Context, booking *Booking) error

	// GetByFilter возвращает список бронирований с пагинацией.
	GetByFilter(ctx context.Context, filter BookingFilter) ([]Booking, int64, error)

	// GetAwaitingConfirmation возвращает бронирования в статусе AwaitsConfirmation
	// с пессимистичной блокировкой (SELECT ... FOR UPDATE SKIP LOCKED).
	GetAwaitingConfirmation(ctx context.Context, limit int) ([]Booking, error)

	// GetStatistics возвращает агрегированную статистику за период.
	// dateFrom и dateTo — включительно по полю created_at.
	GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (BookingStatistics, error)

	// GetPendingCancellations возвращает бронирования в статусе cancellation_pending,
	// у которых cancellation_sent_at раньше указанного времени.
	GetPendingCancellations(ctx context.Context, before time.Time) ([]Booking, error)

	// BeginTx начинает транзакцию.
	BeginTx(ctx context.Context) (pgx.Tx, error)

	// CreateTx сохраняет бронирование в транзакции.
	CreateTx(ctx context.Context, tx pgx.Tx, booking *Booking) (int64, error)

	// UpdateTx обновляет бронирование в транзакции.
	UpdateTx(ctx context.Context, tx pgx.Tx, booking *Booking) error
}

// BookingHistoryRepository — интерфейс репозитория истории изменений.
type BookingHistoryRepository interface {
	AddTx(ctx context.Context, tx pgx.Tx, entry BookingHistoryEntry) error
	GetByBookingID(ctx context.Context, bookingID int64, page, pageSize int) ([]BookingHistoryEntry, int64, error)
}

// ProcessedEventRepository хранит обработанные event_id для идемпотентности.
type ProcessedEventRepository interface {
	IsProcessed(ctx context.Context, eventID string) (bool, error)
	MarkProcessedTx(ctx context.Context, tx pgx.Tx, eventID string) error
}

// OutboxRepository хранит исходящие сообщения для Transactional Outbox Pattern.
type OutboxRepository interface {
	// SaveTx сохраняет сообщение в outbox в рамках переданной транзакции.
	SaveTx(ctx context.Context, tx pgx.Tx, msg *OutboxMessage) error

	// GetPending возвращает сообщения со статусом PENDING или FAILED
	// с retry_count < maxRetries, ограниченно по limit.
	GetPending(ctx context.Context, maxRetries, limit int) ([]*OutboxMessage, error)

	// Update обновляет статус сообщения.
	Update(ctx context.Context, msg *OutboxMessage) error
}

// BookingFilter содержит параметры фильтрации и пагинации.
type BookingFilter struct {
	UserID     *int64
	ResourceID *int64
	Status     *BookingStatus
	Page       int
	Size       int
}

// NewDefaultFilter создаёт фильтр с пагинацией по умолчанию.
func NewDefaultFilter() BookingFilter {
	return BookingFilter{
		Page: 1,
		Size: 25,
	}
}
