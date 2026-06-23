-- +goose Up
CREATE TABLE processed_events (
                                  event_id VARCHAR(255) PRIMARY KEY,
                                  event_type VARCHAR(100) NOT NULL,
                                  processed_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS processed_events;