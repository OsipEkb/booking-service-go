package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"booking-service/app/models"
)

// BookingsRepository реализует models.BookingRepository.
type BookingsRepository struct {
	pool *pgxpool.Pool
}

// NewBookingsRepository создаёт новый экземпляр BookingsRepository.
func NewBookingsRepository(pool *pgxpool.Pool) *BookingsRepository {
	return &BookingsRepository{pool: pool}
}

// Create сохраняет новое бронирование.
func (r *BookingsRepository) Create(ctx context.Context, booking *models.Booking) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, queryInsertBooking,
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
	booking, err := r.scanBooking(r.pool.QueryRow(ctx, queryGetBookingByID, id))
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
	var previousStatus *string
	if ps := booking.PreviousStatus(); ps != "" {
		s := string(ps)
		previousStatus = &s
	}

	tag, err := r.pool.Exec(ctx, queryUpdateBookingStatus,
		string(booking.Status()),
		previousStatus,
		booking.CancellationSentAt(),
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
	err := r.pool.QueryRow(ctx, queryCountBookingsByFilter, userID, resourceID, status).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчёт бронирований: %w", err)
	}

	// Получение данных
	rows, err := r.pool.Query(ctx, queryGetBookingsByFilter, userID, resourceID, status, filter.Size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение бронирований по фильтру: %w", err)
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
	rows, err := r.pool.Query(ctx, queryGetAwaitingConfirmation, limit)
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

// GetStatistics возвращает агрегированную статистику бронирований за период.
func (r *BookingsRepository) GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (models.BookingStatistics, error) {
	stats := models.BookingStatistics{
		ByStatus:     make(map[models.BookingStatus]int64),
		TopResources: []models.ResourceBookingStats{},
	}

	// Статистика по статусам (и суммарное количество)
	rows, err := r.pool.Query(ctx, queryGetStatsByStatus, dateFrom, dateTo)
	if err != nil {
		return stats, fmt.Errorf("получение статистики по статусам: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var cnt int64
		if err := rows.Scan(&status, &cnt); err != nil {
			return stats, fmt.Errorf("сканирование статистики по статусам: %w", err)
		}
		stats.ByStatus[models.BookingStatus(status)] = cnt
		stats.TotalBookings += cnt
	}
	if err := rows.Err(); err != nil {
		return stats, fmt.Errorf("итерация статистики по статусам: %w", err)
	}

	// Топ ресурсов
	topRows, err := r.pool.Query(ctx, queryGetTopResources, dateFrom, dateTo)
	if err != nil {
		return stats, fmt.Errorf("получение топ ресурсов: %w", err)
	}
	defer topRows.Close()

	for topRows.Next() {
		var rs models.ResourceBookingStats
		if err := topRows.Scan(&rs.ResourceID, &rs.BookingsCount); err != nil {
			return stats, fmt.Errorf("сканирование топ ресурсов: %w", err)
		}
		stats.TopResources = append(stats.TopResources, rs)
	}
	if err := topRows.Err(); err != nil {
		return stats, fmt.Errorf("итерация топ ресурсов: %w", err)
	}

	return stats, nil
}

// GetPendingCancellations возвращает зависшие отмены (cancellation_pending старше cutoff).
func (r *BookingsRepository) GetPendingCancellations(ctx context.Context, before time.Time) ([]models.Booking, error) {
	rows, err := r.pool.Query(ctx, queryGetPendingCancellations, before)
	if err != nil {
		return nil, fmt.Errorf("получение зависших отмен: %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("сканирование зависшей отмены: %w", err)
		}
		bookings = append(bookings, *booking)
	}

	return bookings, rows.Err()
}

// BeginTx начинает новую транзакцию.
func (r *BookingsRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// CreateTx сохраняет новое бронирование в рамках транзакции.
func (r *BookingsRepository) CreateTx(ctx context.Context, tx pgx.Tx, booking *models.Booking) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, queryInsertBooking,
		string(booking.Status()),
		booking.UserID(),
		booking.ResourceID(),
		booking.StartDate(),
		booking.EndDate(),
		booking.CreatedAt(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("создание бронирования в транзакции: %w", err)
	}
	return id, nil
}

// UpdateTx обновляет бронирование в рамках транзакции.
func (r *BookingsRepository) UpdateTx(ctx context.Context, tx pgx.Tx, booking *models.Booking) error {
	var previousStatus *string
	if ps := booking.PreviousStatus(); ps != "" {
		s := string(ps)
		previousStatus = &s
	}
	tag, err := tx.Exec(ctx, queryUpdateBookingStatus,
		string(booking.Status()),
		previousStatus,
		booking.CancellationSentAt(),
		booking.ID(),
	)
	if err != nil {
		return fmt.Errorf("обновление бронирования в транзакции id=%d: %w", booking.ID(), err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrBookingNotFound
	}
	return nil
}

// scanBooking сканирует одну строку в доменный объект Booking.
func (r *BookingsRepository) scanBooking(row pgx.Row) (*models.Booking, error) {
	var (
		id                 int64
		status             string
		userID             int64
		resourceID         int64
		startDate          time.Time
		endDate            time.Time
		createdAt          time.Time
		previousStatus     *string
		cancellationSentAt *time.Time
	)

	err := row.Scan(&id, &status, &userID, &resourceID, &startDate, &endDate, &createdAt, &previousStatus, &cancellationSentAt)
	if err != nil {
		return nil, err
	}

	var ps models.BookingStatus
	if previousStatus != nil {
		ps = models.BookingStatus(*previousStatus)
	}

	return models.RestoreBooking(id, models.BookingStatus(status), userID, resourceID, startDate, endDate, createdAt, ps, cancellationSentAt), nil
}

// scanBookingFromRows сканирует строку из pgx.Rows.
func (r *BookingsRepository) scanBookingFromRows(rows pgx.Rows) (*models.Booking, error) {
	var (
		id                 int64
		status             string
		userID             int64
		resourceID         int64
		startDate          time.Time
		endDate            time.Time
		createdAt          time.Time
		previousStatus     *string
		cancellationSentAt *time.Time
	)

	err := rows.Scan(&id, &status, &userID, &resourceID, &startDate, &endDate, &createdAt, &previousStatus, &cancellationSentAt)
	if err != nil {
		return nil, err
	}

	var ps models.BookingStatus
	if previousStatus != nil {
		ps = models.BookingStatus(*previousStatus)
	}

	return models.RestoreBooking(id, models.BookingStatus(status), userID, resourceID, startDate, endDate, createdAt, ps, cancellationSentAt), nil
}
