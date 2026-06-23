package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"booking-service/app/models"
)

// OutboxRepository реализует models.OutboxRepository поверх PostgreSQL.
type OutboxRepository struct {
	pool *pgxpool.Pool
}

// NewOutboxRepository создаёт новый репозиторий outbox-сообщений.
func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: pool}
}

// SaveTx сохраняет сообщение в outbox в рамках переданной транзакции.
func (r *OutboxRepository) SaveTx(ctx context.Context, tx pgx.Tx, msg *models.OutboxMessage) error {
	return tx.QueryRow(ctx,
		`INSERT INTO outbox_messages (routing_key, payload, status, created_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		msg.RoutingKey, msg.Payload, msg.Status, msg.CreatedAt,
	).Scan(&msg.ID)
}

// GetPending возвращает сообщения со статусом PENDING или FAILED с retry_count < maxRetries.
func (r *OutboxRepository) GetPending(ctx context.Context, maxRetries, limit int) ([]*models.OutboxMessage, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, routing_key, payload, status, retry_count, created_at, processed_at, last_error
		 FROM outbox_messages
		 WHERE status IN ('PENDING', 'FAILED') AND retry_count < $1
		 ORDER BY created_at
		 LIMIT $2`,
		maxRetries, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*models.OutboxMessage
	for rows.Next() {
		msg := &models.OutboxMessage{}
		if err := rows.Scan(
			&msg.ID, &msg.RoutingKey, &msg.Payload, &msg.Status,
			&msg.RetryCount, &msg.CreatedAt, &msg.ProcessedAt, &msg.LastError,
		); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

// Update обновляет статус сообщения в БД.
func (r *OutboxRepository) Update(ctx context.Context, msg *models.OutboxMessage) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE outbox_messages
		 SET status = $1, retry_count = $2, processed_at = $3, last_error = $4
		 WHERE id = $5`,
		msg.Status, msg.RetryCount, msg.ProcessedAt, msg.LastError, msg.ID,
	)
	return err
}
