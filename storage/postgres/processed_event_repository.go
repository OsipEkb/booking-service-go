package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProcessedEventRepository реализует models.ProcessedEventRepository поверх PostgreSQL.
type ProcessedEventRepository struct {
	pool *pgxpool.Pool
}

// NewProcessedEventRepository создаёт новый репозиторий обработанных событий.
func NewProcessedEventRepository(pool *pgxpool.Pool) *ProcessedEventRepository {
	return &ProcessedEventRepository{pool: pool}
}

// IsProcessed проверяет, было ли событие с данным eventID уже обработано.
func (r *ProcessedEventRepository) IsProcessed(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)`,
		eventID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// MarkProcessedTx записывает eventID в таблицу в рамках переданной транзакции.
func (r *ProcessedEventRepository) MarkProcessedTx(ctx context.Context, tx pgx.Tx, eventID string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO processed_events (event_id) VALUES ($1)`,
		eventID,
	)
	return err
}
