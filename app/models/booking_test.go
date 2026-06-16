package models_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"booking-service/app/models"
)

func TestNewBooking_Success(t *testing.T) {
	// Arrange
	userID := int64(1)
	resourceID := int64(10)
	startDate := time.Now().AddDate(0, 0, 7)
	endDate := time.Now().AddDate(0, 0, 14)

	// Act
	booking, err := models.NewBooking(userID, resourceID, startDate, endDate)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusAwaitsConfirmation, booking.Status())
	assert.Equal(t, userID, booking.UserID())
	assert.Equal(t, resourceID, booking.ResourceID())
}

func TestNewBooking_InvalidUserID(t *testing.T) {
	_, err := models.NewBooking(0, 10, time.Now(), time.Now().AddDate(0, 0, 1))
	assert.ErrorIs(t, err, models.ErrInvalidUserID)
}

func TestNewBooking_EndDateBeforeStartDate(t *testing.T) {
	start := time.Now().AddDate(0, 0, 7)
	end := time.Now().AddDate(0, 0, 1)
	_, err := models.NewBooking(1, 10, start, end)
	assert.ErrorIs(t, err, models.ErrEndDateBeforeStartDate)
}

func TestConfirm_FromAwaitsConfirmation(t *testing.T) {
	booking := createTestBooking(t)

	err := booking.Confirm()

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusConfirmed, booking.Status())
}

func TestConfirm_FromConfirmed_Error(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Confirm()

	err := booking.Confirm()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestCancel_FromAwaitsConfirmation(t *testing.T) {
	booking := createTestBooking(t)

	err := booking.Cancel(time.Now())

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancelled, booking.Status())
}

func TestCancel_FromConfirmed_FutureStartDate(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Confirm()
	today := time.Now()

	err := booking.Cancel(today)

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancelled, booking.Status())
}

func TestCancel_FromConfirmed_PastStartDate_Error(t *testing.T) {
	b := models.RestoreBooking(
		1,
		models.BookingStatusConfirmed,
		models.BookingStatus(""),
		1, 10,
		time.Now().AddDate(0, 0, -3),
		time.Now().AddDate(0, 0, -1),
		time.Now().AddDate(0, 0, -5),
		time.Time{},
	)

	err := b.Cancel(time.Now())

	assert.ErrorIs(t, err, models.ErrCannotCancelPastBooking)
}

func TestCancel_FromCancelled_Error(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Cancel(time.Now())

	err := booking.Cancel(time.Now())

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestConfirmCancellation_Success(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.StartCancellation(time.Now())
	err := booking.ConfirmCancellation()
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancelled, booking.Status())
}

func TestConfirmCancellation_InvalidStatus_Error(t *testing.T) {
	booking := createTestBooking(t)
	err := booking.ConfirmCancellation()
	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestRollbackCancellation_Success(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.StartCancellation(time.Now())
	err := booking.RollbackCancellation()
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusAwaitsConfirmation, booking.Status())
	assert.Equal(t, models.BookingStatus(""), booking.PrevStatus())
}

func TestRollbackCancellation_InvalidStatus_Error(t *testing.T) {
	booking := createTestBooking(t)
	err := booking.RollbackCancellation()
	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestStartCancellation_FromAwaitsConfirmation_Success(t *testing.T) {
	booking := createTestBooking(t)
	now := time.Now()
	err := booking.StartCancellation(now)
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancellationPending, booking.Status())
	assert.Equal(t, models.BookingStatusAwaitsConfirmation, booking.PrevStatus())
	assert.Equal(t, now, booking.CanceledAt())
}

func TestStartCancellation_FromAwaitsConfirmation_InvalidStatus_Error(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Cancel(time.Now())
	err := booking.StartCancellation(time.Now())
	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestStartCancellation_FromConfirmed_PastStartDate_Error(t *testing.T) {
	today := time.Now()
	booking := models.RestoreBooking(
		1,
		models.BookingStatusConfirmed,
		models.BookingStatus(""),
		1, 10,
		today.AddDate(0, 0, -3),
		today.AddDate(0, 0, -1),
		today.AddDate(0, 0, -5),
		time.Time{},
	)
	err := booking.StartCancellation(today)
	assert.ErrorIs(t, err, models.ErrCannotCancelPastBooking)
}

func createTestBooking(t *testing.T) *models.Booking {
	t.Helper()
	b, err := models.NewBooking(1, 10, time.Now().AddDate(0, 0, 7), time.Now().AddDate(0, 0, 14))
	require.NoError(t, err)
	return b
}
