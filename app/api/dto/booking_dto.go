package dto

// CreateBookingRequest -- запрос на создание бронирования.
type CreateBookingRequest struct {
	UserID     int64  `json:"userId"`
	ResourceID int64  `json:"resourceId"`
	StartDate  string `json:"startDate"` // формат: "2006-01-02"
	EndDate    string `json:"endDate"`   // формат: "2006-01-02"
}

// CreateBookingResponse -- ответ при создании бронирования.
type CreateBookingResponse struct {
	ID int64 `json:"id"`
}

// BookingResponse -- полные данные бронирования.
type BookingResponse struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	UserID     int64  `json:"userId"`
	ResourceID int64  `json:"resourceId"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
	CreatedAt  string `json:"createdAt"` // формат: RFC3339
}

// BookingStatusResponse -- статус бронирования.
type BookingStatusResponse struct {
	Status string `json:"status"`
}

// GetBookingsByFilterRequest -- запрос с фильтром и пагинацией.
type GetBookingsByFilterRequest struct {
	UserID     *int64  `json:"userId,omitempty"`
	ResourceID *int64  `json:"resourceId,omitempty"`
	Status     *string `json:"status,omitempty"`
	Page       int     `json:"page"`
	Size       int     `json:"size"`
}

// PagedResponse -- ответ с пагинацией.
type PagedResponse[T any] struct {
	Items      []T   `json:"items"`
	TotalCount int64 `json:"totalCount"`
	Page       int   `json:"page"`
	Size       int   `json:"size"`
}

// ProblemDetails -- стандартный формат ошибки RFC 7807.
type ProblemDetails struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// BookingStatisticsResponse -- ответ с агрегированной статистикой бронирований.
type BookingStatisticsResponse struct {
	TotalBookings int64               `json:"totalBookings"`
	ByStatus      BookingStatusStats  `json:"byStatus"`
	TopResources  []ResourceBookingCount `json:"topResources"`
}

// BookingStatusStats -- разбивка количества бронирований по статусам.
type BookingStatusStats struct {
	AwaitConfirmation   int64 `json:"awaitConfirmation"`
	Confirmed           int64 `json:"confirmed"`
	Cancelled           int64 `json:"cancelled"`
	CancellationPending int64 `json:"cancellationPending"`
}

// ResourceBookingCount -- количество бронирований для одного ресурса.
type ResourceBookingCount struct {
	ResourceID    int64 `json:"resourceId"`
	BookingsCount int64 `json:"bookingsCount"`
}

// BookingHistoryResponse — ответ с историей изменений бронирования.
type BookingHistoryResponse struct {
	BookingID  int64                `json:"bookingId"`
	TotalCount int64                `json:"totalCount"`
	Items      []BookingHistoryItem `json:"items"`
}

// BookingHistoryItem — одна запись в истории изменений.
type BookingHistoryItem struct {
	ID          int64   `json:"id"`
	OldStatus   *string `json:"oldStatus"`
	NewStatus   string  `json:"newStatus"`
	ChangedAt   string  `json:"changedAt"`
	Reason      *string `json:"reason"`
	InitiatedBy string  `json:"initiatedBy"`
}

// DateFormat -- формат даты для JSON-сериализации.
const DateFormat = "2006-01-02"
