-- +goose Up
CREATE TABLE IF NOT EXISTS booking_audit_logs (
    id          BIGSERIAL    PRIMARY KEY,
    booking_id  BIGINT       NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    from_status VARCHAR(50),
    to_status   VARCHAR(50)  NOT NULL,
    changed_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    initiator   VARCHAR(100) NOT NULL,
    reason      VARCHAR(255) NOT NULL
    );

-- Составной индекс для быстрой пагинации и сортировки по истории конкретной брони
CREATE INDEX IF NOT EXISTS idx_booking_audit_logs_booking_id_changed_at
    ON booking_audit_logs (booking_id, changed_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_booking_audit_logs_booking_id_changed_at;
DROP TABLE IF EXISTS booking_audit_logs;