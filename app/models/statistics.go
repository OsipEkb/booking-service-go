package models

// BookingStatistics содержит агрегированную статистику бронирований.
type BookingStatistics struct {
	TotalBookings int64
	ByStatus      map[BookingStatus]int64
	TopResources  []ResourceBookingStats
}

// ResourceBookingStats содержит статистику по ресурсу.
type ResourceBookingStats struct {
	ResourceID    int64
	BookingsCount int64
}
