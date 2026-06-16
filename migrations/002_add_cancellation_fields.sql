-- +goose Up
ALTER TABLE bookings ADD COLUMN prev_status VARCHAR(30);
ALTER TABLE bookings ADD COLUMN canceled_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE bookings DROP COLUMN prev_status;
ALTER TABLE bookings DROP COLUMN canceled_at;