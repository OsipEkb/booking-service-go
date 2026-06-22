package models

import "time"

type BookingAuditLog struct {
	id         int64
	bookingID  int64
	fromStatus BookingStatus
	toStatus   BookingStatus
	changedAt  time.Time
	initiator  string
	reason     string
}

// Конструктор для создания нового лога в бизнес-логике
func NewBookingAuditLog(bookingID int64, fromStatus, toStatus BookingStatus, initiator, reason string) *BookingAuditLog {
	return &BookingAuditLog{
		bookingID:  bookingID,
		fromStatus: fromStatus,
		toStatus:   toStatus,
		changedAt:  time.Now(),
		initiator:  initiator,
		reason:     reason,
	}
}

func (l *BookingAuditLog) ID() int64                 { return l.id }
func (l *BookingAuditLog) BookingID() int64          { return l.bookingID }
func (l *BookingAuditLog) FromStatus() BookingStatus { return l.fromStatus }
func (l *BookingAuditLog) ToStatus() BookingStatus   { return l.toStatus }
func (l *BookingAuditLog) ChangedAt() time.Time      { return l.changedAt }
func (l *BookingAuditLog) Initiator() string         { return l.initiator }
func (l *BookingAuditLog) Reason() string            { return l.reason }

func RestoreBookingAuditLog(id, bookingID int64, fromStatus, toStatus BookingStatus, changedAt time.Time, initiator, reason string) *BookingAuditLog {
	return &BookingAuditLog{
		id:         id,
		bookingID:  bookingID,
		fromStatus: fromStatus,
		toStatus:   toStatus,
		changedAt:  changedAt,
		initiator:  initiator,
		reason:     reason,
	}
}
