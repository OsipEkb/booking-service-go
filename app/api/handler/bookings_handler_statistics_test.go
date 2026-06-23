package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"booking-service/app/api/dto"
	"booking-service/app/api/handler"
	"booking-service/app/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// stubQueries реализует handler.BookingQueries для тестов.
type stubQueries struct {
	statsResult dto.BookingStatisticsResponse
	statsErr    error
}

func (s *stubQueries) GetByID(_ context.Context, _ int64) (dto.BookingResponse, error) {
	return dto.BookingResponse{}, nil
}
func (s *stubQueries) GetByFilter(_ context.Context, _ dto.GetBookingsByFilterRequest) (dto.PagedResponse[dto.BookingResponse], error) {
	return dto.PagedResponse[dto.BookingResponse]{}, nil
}
func (s *stubQueries) GetStatus(_ context.Context, _ int64) (models.BookingStatus, error) {
	return "", nil
}
func (s *stubQueries) GetStatistics(_ context.Context, _, _ time.Time) (dto.BookingStatisticsResponse, error) {
	return s.statsResult, s.statsErr
}
func (s *stubQueries) GetHistory(_ context.Context, _ int64, _, _ int) (dto.BookingHistoryResponse, error) {
	return dto.BookingHistoryResponse{}, nil
}

// stubService реализует handler.BookingService для тестов.
type stubService struct{}

func (s *stubService) Create(_ context.Context, _ dto.CreateBookingRequest) (int64, error) {
	return 0, nil
}
func (s *stubService) Cancel(_ context.Context, _ int64) error { return nil }

func newTestHandler(q *stubQueries) *handler.BookingsHandler {
	return handler.NewBookingsHandler(&stubService{}, q, zap.NewNop())
}

func TestGetStatistics_MissingBothParams(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetStatistics_MissingDateTo(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=2024-01-01", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetStatistics_MissingDateFrom(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateTo=2024-12-31", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetStatistics_InvalidDateFormat(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=01-01-2024&dateTo=2024-12-31", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetStatistics_DateToBeforeDateFrom(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=2024-12-31&dateTo=2024-01-01", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetStatistics_Success(t *testing.T) {
	expected := dto.BookingStatisticsResponse{
		TotalBookings: 10,
		ByStatus: dto.BookingStatusStats{
			Confirmed: 7,
			Cancelled: 3,
		},
		TopResources: []dto.ResourceBookingCount{
			{ResourceID: 1, BookingsCount: 5},
		},
	}
	q := &stubQueries{statsResult: expected}
	h := newTestHandler(q)
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=2024-01-01&dateTo=2024-12-31", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"totalBookings":10`)
}

func TestGetStatistics_SameDateRange(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=2024-06-15&dateTo=2024-06-15", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	// dateFrom == dateTo допустимо
	assert.Equal(t, http.StatusOK, w.Code)
}
