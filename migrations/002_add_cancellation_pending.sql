-- +goose Up
ALTER TABLE bookings
    ADD COLUMN previous_status       VARCHAR(30)  NULL,
    ADD COLUMN cancellation_sent_at  TIMESTAMPTZ  NULL;

-- +goose Down
ALTER TABLE bookings
    DROP COLUMN IF EXISTS previous_status,
    DROP COLUMN IF EXISTS cancellation_sent_at;
