package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"booking-service/app/models"
)

// BookingHistoryRepository реализует models.BookingHistoryRepository.
type BookingHistoryRepository struct {
	pool *pgxpool.Pool
}

func NewBookingHistoryRepository(pool *pgxpool.Pool) *BookingHistoryRepository {
	return &BookingHistoryRepository{pool: pool}
}

func (r *BookingHistoryRepository) AddTx(ctx context.Context, tx pgx.Tx, entry models.BookingHistoryEntry) error {
	var oldStatus *string
	if entry.OldStatus != nil {
		s := string(*entry.OldStatus)
		oldStatus = &s
	}
	_, err := tx.Exec(ctx, queryInsertBookingHistory,
		entry.BookingID,
		oldStatus,
		string(entry.NewStatus),
		entry.ChangedAt,
		entry.Reason,
		entry.InitiatedBy,
	)
	if err != nil {
		return fmt.Errorf("запись истории бронирования: %w", err)
	}
	return nil
}

func (r *BookingHistoryRepository) GetByBookingID(ctx context.Context, bookingID int64, page, pageSize int) ([]models.BookingHistoryEntry, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, queryCountBookingHistory, bookingID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("подсчёт истории: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx, queryGetBookingHistory, bookingID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение истории: %w", err)
	}
	defer rows.Close()

	var entries []models.BookingHistoryEntry
	for rows.Next() {
		var e models.BookingHistoryEntry
		var oldStatus *string
		var changedAt time.Time
		if err := rows.Scan(&e.ID, &e.BookingID, &oldStatus, &e.NewStatus, &changedAt, &e.Reason, &e.InitiatedBy); err != nil {
			return nil, 0, fmt.Errorf("сканирование истории: %w", err)
		}
		if oldStatus != nil {
			s := models.BookingStatus(*oldStatus)
			e.OldStatus = &s
		}
		e.ChangedAt = changedAt
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}
