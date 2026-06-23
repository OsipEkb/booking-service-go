package postgres

const (
	queryInsertBooking = `
		INSERT INTO bookings (status, user_id, resource_id, start_date, end_date, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	queryGetBookingByID = `
		SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancellation_sent_at
		FROM bookings
		WHERE id = $1`

	queryUpdateBookingStatus = `
		UPDATE bookings
		SET status = $1, previous_status = $2, cancellation_sent_at = $3
		WHERE id = $4`

	queryGetBookingsByFilter = `
		SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancellation_sent_at
		FROM bookings
		WHERE ($1::BIGINT IS NULL OR user_id = $1)
		  AND ($2::BIGINT IS NULL OR resource_id = $2)
		  AND ($3::VARCHAR IS NULL OR status = $3)
		ORDER BY id DESC
		LIMIT $4 OFFSET $5`

	queryCountBookingsByFilter = `
		SELECT COUNT(*)
		FROM bookings
		WHERE ($1::BIGINT IS NULL OR user_id = $1)
		  AND ($2::BIGINT IS NULL OR resource_id = $2)
		  AND ($3::VARCHAR IS NULL OR status = $3)`

	queryGetAwaitingConfirmation = `
		SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancellation_sent_at
		FROM bookings
		WHERE status = 'awaits_confirmation'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	queryGetStatsByStatus = `
		SELECT status, COUNT(*) AS cnt
		FROM bookings
		WHERE created_at >= $1 AND created_at < ($2::date + INTERVAL '1 day')
		GROUP BY status`

	queryGetTopResources = `
		SELECT resource_id, COUNT(*) AS bookings_count
		FROM bookings
		WHERE created_at >= $1 AND created_at < ($2::date + INTERVAL '1 day')
		GROUP BY resource_id
		ORDER BY bookings_count DESC
		LIMIT 5`

	queryGetPendingCancellations = `
		SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancellation_sent_at
		FROM bookings
		WHERE status = 'cancellation_pending'
		  AND cancellation_sent_at < $1`

	queryInsertBookingHistory = `
    INSERT INTO booking_history (booking_id, old_status, new_status, changed_at, reason, initiated_by)
    VALUES ($1, $2, $3, $4, $5, $6)`

	queryGetBookingHistory = `
    SELECT id, booking_id, old_status, new_status, changed_at, reason, initiated_by
    FROM booking_history
    WHERE booking_id = $1
    ORDER BY changed_at DESC
    LIMIT $2 OFFSET $3`

	queryCountBookingHistory = `
    SELECT COUNT(*) FROM booking_history WHERE booking_id = $1`
)
