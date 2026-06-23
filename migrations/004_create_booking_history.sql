-- +goose Up
CREATE TABLE IF NOT EXISTS booking_history (
    id           BIGSERIAL    PRIMARY KEY,
    booking_id   BIGINT       NOT NULL REFERENCES bookings(id),
    old_status   VARCHAR(30)  NULL,
    new_status   VARCHAR(30)  NOT NULL,
    changed_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    reason       TEXT         NULL,
    initiated_by VARCHAR(255) NOT NULL
);

CREATE INDEX idx_booking_history_booking_id_changed_at
    ON booking_history (booking_id, changed_at DESC);

-- +goose Down
DROP TABLE IF EXISTS booking_history;
