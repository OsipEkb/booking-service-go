-- +goose Up
-- Индекс для фильтрации по created_at (используется в статистике)
CREATE INDEX idx_bookings_created_at ON bookings (created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_bookings_created_at;
