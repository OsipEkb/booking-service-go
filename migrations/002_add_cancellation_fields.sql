-- +goose Up
ALTER TABLE bookings ADD COLUMN prev_status VARCHAR(50);
ALTER TABLE bookings ADD COLUMN canceled_at TIMESTAMP WITH TIME ZONE;

CREATE INDEX idx_bookings_created_at ON bookings(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_bookings_created_at;

ALTER TABLE bookings DROP COLUMN IF EXISTS prev_status;
ALTER TABLE bookings DROP COLUMN IF EXISTS canceled_at;