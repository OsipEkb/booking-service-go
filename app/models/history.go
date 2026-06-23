package models

import "time"

// BookingHistoryEntry — запись в истории изменений бронирования.
type BookingHistoryEntry struct {
	ID          int64
	BookingID   int64
	OldStatus   *BookingStatus
	NewStatus   BookingStatus
	ChangedAt   time.Time
	Reason      *string
	InitiatedBy string
}
