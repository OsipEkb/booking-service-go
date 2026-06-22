package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"booking-service/app/models"
)

// BookingsRepository реализует models.BookingRepository.
type BookingsRepository struct {
	pool *pgxpool.Pool
}

type txKey struct{}

// pgxDB описывает общие методы для *pgxpool.Pool и pgx.Tx
type pgxDB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
}

func (r *BookingsRepository) getExecutor(ctx context.Context) pgxDB {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return r.pool
}

// NewBookingsRepository создаёт новый экземпляр BookingsRepository.
func NewBookingsRepository(pool *pgxpool.Pool) *BookingsRepository {
	return &BookingsRepository{pool: pool}
}

// WithTx запускает переданную функцию внутри ACID транзакции базы данных
func (r *BookingsRepository) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)

	err = fn(txCtx)
	if err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// Create сохраняет новое бронирование.
func (r *BookingsRepository) Create(ctx context.Context, booking *models.Booking) (int64, error) {
	var id int64
	// Используем getExecutor(ctx) вместо r.pool, чтобы поддерживать транзакции
	err := r.getExecutor(ctx).QueryRow(ctx, queryInsertBooking,
		string(booking.Status()),
		booking.UserID(),
		booking.ResourceID(),
		booking.StartDate(),
		booking.EndDate(),
		booking.CreatedAt(),
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("создание бронирования: %w", err)
	}
	return id, nil
}

// GetByID возвращает бронирование по ID.
func (r *BookingsRepository) GetByID(ctx context.Context, id int64) (*models.Booking, error) {
	// Используем getExecutor(ctx) вместо r.pool, чтобы поддерживать транзакции
	booking, err := r.scanBooking(r.getExecutor(ctx).QueryRow(ctx, queryGetBookingByID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrBookingNotFound
		}
		return nil, fmt.Errorf("получение бронирования id=%d: %w", id, err)
	}
	return booking, nil
}

// Update обновляет статус бронирования.
func (r *BookingsRepository) Update(ctx context.Context, booking *models.Booking) error {
	var prevStatusPtr *string
	if booking.PrevStatus() != "" {
		str := string(booking.PrevStatus())
		prevStatusPtr = &str
	}

	var canceledAtPtr *time.Time
	if booking.CanceledAt() != nil && !booking.CanceledAt().IsZero() {
		canceledAtPtr = booking.CanceledAt()
	}

	// Используем getExecutor(ctx) вместо r.pool, чтобы поддерживать транзакции
	tag, err := r.getExecutor(ctx).Exec(ctx, queryUpdateBookingStatus,
		string(booking.Status()),
		prevStatusPtr,
		canceledAtPtr,
		booking.ID(),
	)
	if err != nil {
		return fmt.Errorf("обновление бронирования id=%d: %w", booking.ID(), err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrBookingNotFound
	}
	return nil
}

// GetByFilter возвращает бронирования с фильтрацией и пагинацией.
func (r *BookingsRepository) GetByFilter(ctx context.Context, filter models.BookingFilter) ([]models.Booking, int64, error) {
	offset := (filter.Page - 1) * filter.Size

	var userID, resourceID *int64
	var status *string
	if filter.UserID != nil {
		userID = filter.UserID
	}
	if filter.ResourceID != nil {
		resourceID = filter.ResourceID
	}
	if filter.Status != nil {
		s := string(*filter.Status)
		status = &s
	}

	// Получение общего количества
	var totalCount int64
	err := r.getExecutor(ctx).QueryRow(ctx, queryCountBookingsByFilter, userID, resourceID, status).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчёт бронирований: %w", err)
	}

	// Получение данных
	rows, err := r.getExecutor(ctx).Query(ctx, queryGetBookingsByFilter, userID, resourceID, status, filter.Size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение бронирования по фильтру: %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("сканирование бронирования: %w", err)
		}
		bookings = append(bookings, *booking)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("итерация по строкам: %w", err)
	}

	return bookings, totalCount, nil
}

// GetAwaitingConfirmation возвращает бронирования, ожидающие подтверждения,
// с пессимистичной блокировкой FOR UPDATE SKIP LOCKED.
func (r *BookingsRepository) GetAwaitingConfirmation(ctx context.Context, limit int) ([]models.Booking, error) {
	rows, err := r.getExecutor(ctx).Query(ctx, queryGetAwaitingConfirmation, limit)
	if err != nil {
		return nil, fmt.Errorf("получение бронирований для подтверждения: %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("сканирование бронирования: %w", err)
		}
		bookings = append(bookings, *booking)
	}

	return bookings, rows.Err()
}

// scanBooking сканирует одну строку в доменный объект Booking.
func (r *BookingsRepository) scanBooking(row pgx.Row) (*models.Booking, error) {
	var (
		id         int64
		status     string
		userID     int64
		resourceID int64
		startDate  time.Time
		endDate    time.Time
		createdAt  time.Time
		prevStatus *string
		canceledAt *time.Time
	)

	err := row.Scan(&id, &status, &userID, &resourceID, &startDate, &endDate, &createdAt, &prevStatus, &canceledAt)
	if err != nil {
		return nil, err
	}

	var pStatus string
	if prevStatus != nil {
		pStatus = *prevStatus
	}

	return models.RestoreBooking(id, models.BookingStatus(status), models.BookingStatus(pStatus), userID, resourceID, startDate, endDate, createdAt, canceledAt), nil
}

// scanBookingFromRows сканирует строку из pgx.Rows.
func (r *BookingsRepository) scanBookingFromRows(rows pgx.Rows) (*models.Booking, error) {
	var (
		id         int64
		status     string
		userID     int64
		resourceID int64
		startDate  time.Time
		endDate    time.Time
		createdAt  time.Time
		prevStatus *string
		canceledAt *time.Time
	)

	err := rows.Scan(&id, &status, &userID, &resourceID, &startDate, &endDate, &createdAt, &prevStatus, &canceledAt)
	if err != nil {
		return nil, err
	}

	var pStatus string
	if prevStatus != nil {
		pStatus = *prevStatus
	}

	return models.RestoreBooking(id, models.BookingStatus(status), models.BookingStatus(pStatus), userID, resourceID, startDate, endDate, createdAt, canceledAt), nil
}

func (r *BookingsRepository) GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (*models.BookingStatistics, error) {
	rowsStatuses, err := r.getExecutor(ctx).Query(ctx, queryGetBookingStatusCounts, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("получение статистики по статусам: %w", err)
	}
	defer rowsStatuses.Close()

	statusCounts := make(map[models.BookingStatus]int64)
	var totalCount int64

	for rowsStatuses.Next() {
		var status string
		var count int64
		if err := rowsStatuses.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("сканирование строки статуса: %w", err)
		}

		bookingStatus := models.BookingStatus(status)
		statusCounts[bookingStatus] = count
		totalCount += count
	}

	if err := rowsStatuses.Err(); err != nil {
		return nil, fmt.Errorf("итерация по строкам статусов: %w", err)
	}

	rowsResources, err := r.getExecutor(ctx).Query(ctx, queryGetTopResources, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("получение топ ресурсов: %w", err)
	}
	defer rowsResources.Close()

	var topResources []models.TopResource
	for rowsResources.Next() {
		var resourceID int64
		var count int64
		if err := rowsResources.Scan(&resourceID, &count); err != nil {
			return nil, fmt.Errorf("сканирование строки топ ресурсов: %w", err)
		}
		topResources = append(topResources, models.TopResource{
			ResourceID:   resourceID,
			BookingCount: count,
		})
	}

	if err := rowsResources.Err(); err != nil {
		return nil, fmt.Errorf("итерация по строкам топ ресурсов: %w", err)
	}

	return &models.BookingStatistics{
		TotalCount:   totalCount,
		StatusCounts: statusCounts,
		TopResources: topResources,
	}, nil
}

// SaveAuditLog сохраняет лог аудита в базу данных
func (r *BookingsRepository) SaveAuditLog(ctx context.Context, log *models.BookingAuditLog) error {
	query := `
        INSERT INTO booking_audit_logs (booking_id, from_status, to_status, changed_at, initiator, reason)
        VALUES ($1, $2, $3, $4, $5, $6)`

	var fromStatusPtr *string
	if log.FromStatus() != "" {
		str := string(log.FromStatus())
		fromStatusPtr = &str
	}

	_, err := r.getExecutor(ctx).Exec(ctx, query,
		log.BookingID(),
		fromStatusPtr,
		string(log.ToStatus()),
		log.ChangedAt(),
		log.Initiator(),
		log.Reason(),
	)
	if err != nil {
		return fmt.Errorf("сохранение лога аудита для бронирования id=%d: %w", log.BookingID(), err)
	}

	return nil
}

// GetAuditLogsByBookingID возвращает историю изменений логов по ID бронирования с пагинацией
func (r *BookingsRepository) GetAuditLogsByBookingID(ctx context.Context, bookingID int64, page int, size int) ([]models.BookingAuditLog, int64, error) {
	offset := (page - 1) * size

	var totalCount int64
	countQuery := `SELECT COUNT(*) FROM booking_audit_logs WHERE booking_id = $1`
	err := r.getExecutor(ctx).QueryRow(ctx, countQuery, bookingID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчет логов аудита: %w", err)
	}

	if totalCount == 0 {
		return nil, 0, nil
	}

	selectQuery := `
        SELECT id, booking_id, from_status, to_status, changed_at, initiator, reason 
        FROM booking_audit_logs 
        WHERE booking_id = $1 
        ORDER BY changed_at DESC 
        LIMIT $2 OFFSET $3`

	rows, err := r.getExecutor(ctx).Query(ctx, selectQuery, bookingID, size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение логов аудита: %w", err)
	}
	defer rows.Close()

	var logs []models.BookingAuditLog
	for rows.Next() {
		var (
			id, bID           int64
			fromStrPtr        *string // Наш nullable-указатель из базы
			toStr             string
			changedAt         time.Time
			initiator, reason string
		)

		if err := rows.Scan(&id, &bID, &fromStrPtr, &toStr, &changedAt, &initiator, &reason); err != nil {
			return nil, 0, fmt.Errorf("сканирование лога аудита: %w", err)
		}

		var fromStr string
		if fromStrPtr != nil {
			fromStr = *fromStrPtr
		}

		log := models.RestoreBookingAuditLog(
			id, bID, models.BookingStatus(fromStr), models.BookingStatus(toStr), changedAt, initiator, reason,
		)
		logs = append(logs, *log)
	}

	return logs, totalCount, rows.Err()
}

func (r *BookingsRepository) RegisterEvent(ctx context.Context, eventID string, eventType string) (bool, error) {
	query := `
        INSERT INTO processed_events (event_id, event_type, processed_at)
        VALUES ($1, $2, $3)
        ON CONFLICT (event_id) DO NOTHING`

	tag, err := r.getExecutor(ctx).Exec(ctx, query, eventID, eventType, time.Now())
	if err != nil {
		return false, fmt.Errorf("регистрация события %s: %w", eventID, err)
	}

	return tag.RowsAffected() > 0, nil
}
