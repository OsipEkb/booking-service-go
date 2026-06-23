package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/models"
)

// BookingsQueries обрабатывает запросы (чтение данных) для бронирований.
type BookingsQueries struct {
	repo        models.BookingRepository
	historyRepo models.BookingHistoryRepository
	logger      *zap.Logger
}

// NewBookingsQueries создаёт новый BookingsQueries.
func NewBookingsQueries(repo models.BookingRepository, historyRepo models.BookingHistoryRepository, logger *zap.Logger) *BookingsQueries {
	return &BookingsQueries{
		repo:        repo,
		historyRepo: historyRepo,
		logger:      logger,
	}
}

// GetByID возвращает бронирование по ID.
func (q *BookingsQueries) GetByID(ctx context.Context, id int64) (dto.BookingResponse, error) {
	booking, err := q.repo.GetByID(ctx, id)
	if err != nil {
		return dto.BookingResponse{}, err
	}

	return mapBookingToResponse(booking), nil
}

// GetStatus возвращает статус бронирования по ID.
func (q *BookingsQueries) GetStatus(ctx context.Context, id int64) (models.BookingStatus, error) {
	booking, err := q.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return booking.Status(), nil
}

// GetByFilter возвращает список бронирований с пагинацией.
func (q *BookingsQueries) GetByFilter(ctx context.Context, req dto.GetBookingsByFilterRequest) (dto.PagedResponse[dto.BookingResponse], error) {
	filter := models.NewDefaultFilter()

	if req.Page > 0 {
		filter.Page = req.Page
	}
	if req.Size > 0 {
		filter.Size = req.Size
	}
	if req.UserID != nil {
		filter.UserID = req.UserID
	}
	if req.ResourceID != nil {
		filter.ResourceID = req.ResourceID
	}
	if req.Status != nil {
		status := models.BookingStatus(*req.Status)
		if !status.IsValid() {
			return dto.PagedResponse[dto.BookingResponse]{}, fmt.Errorf("некорректный статус: %s", *req.Status)
		}
		filter.Status = &status
	}

	bookings, totalCount, err := q.repo.GetByFilter(ctx, filter)
	if err != nil {
		return dto.PagedResponse[dto.BookingResponse]{}, fmt.Errorf("получение бронирований: %w", err)
	}

	items := make([]dto.BookingResponse, 0, len(bookings))
	for i := range bookings {
		items = append(items, mapBookingToResponse(&bookings[i]))
	}

	return dto.PagedResponse[dto.BookingResponse]{
		Items:      items,
		TotalCount: totalCount,
		Page:       filter.Page,
		Size:       filter.Size,
	}, nil
}

// GetStatistics возвращает агрегированную статистику бронирований за период.
func (q *BookingsQueries) GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (dto.BookingStatisticsResponse, error) {
	stats, err := q.repo.GetStatistics(ctx, dateFrom, dateTo)
	if err != nil {
		return dto.BookingStatisticsResponse{}, fmt.Errorf("получение статистики: %w", err)
	}

	response := dto.BookingStatisticsResponse{
		TotalBookings: stats.TotalBookings,
		ByStatus: dto.BookingStatusStats{
			AwaitConfirmation:   stats.ByStatus[models.BookingStatusAwaitsConfirmation],
			Confirmed:           stats.ByStatus[models.BookingStatusConfirmed],
			Cancelled:           stats.ByStatus[models.BookingStatusCancelled],
			CancellationPending: stats.ByStatus[models.BookingStatusCancellationPending],
		},
		TopResources: make([]dto.ResourceBookingCount, 0, len(stats.TopResources)),
	}

	for _, r := range stats.TopResources {
		response.TopResources = append(response.TopResources, dto.ResourceBookingCount{
			ResourceID:    r.ResourceID,
			BookingsCount: r.BookingsCount,
		})
	}

	return response, nil
}

// GetHistory возвращает историю изменений бронирования с пагинацией.
func (q *BookingsQueries) GetHistory(ctx context.Context, bookingID int64, page, pageSize int) (dto.BookingHistoryResponse, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	entries, total, err := q.historyRepo.GetByBookingID(ctx, bookingID, page, pageSize)
	if err != nil {
		return dto.BookingHistoryResponse{}, fmt.Errorf("получение истории бронирования: %w", err)
	}

	items := make([]dto.BookingHistoryItem, 0, len(entries))
	for _, e := range entries {
		item := dto.BookingHistoryItem{
			ID:          e.ID,
			NewStatus:   string(e.NewStatus),
			ChangedAt:   e.ChangedAt.Format("2006-01-02T15:04:05Z07:00"),
			Reason:      e.Reason,
			InitiatedBy: e.InitiatedBy,
		}
		if e.OldStatus != nil {
			s := string(*e.OldStatus)
			item.OldStatus = &s
		}
		items = append(items, item)
	}

	return dto.BookingHistoryResponse{
		BookingID:  bookingID,
		TotalCount: total,
		Items:      items,
	}, nil
}

// mapBookingToResponse конвертирует доменный объект в DTO ответа.
func mapBookingToResponse(b *models.Booking) dto.BookingResponse {
	return dto.BookingResponse{
		ID:         b.ID(),
		Status:     string(b.Status()),
		UserID:     b.UserID(),
		ResourceID: b.ResourceID(),
		StartDate:  b.StartDate().Format(dto.DateFormat),
		EndDate:    b.EndDate().Format(dto.DateFormat),
		CreatedAt:  b.CreatedAt().Format("2006-01-02T15:04:05Z07:00"),
	}
}
