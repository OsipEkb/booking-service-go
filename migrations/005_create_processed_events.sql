-- +goose Up
CREATE TABLE processed_events (
    id           BIGSERIAL PRIMARY KEY,
    event_id     TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_processed_events_event_id UNIQUE (event_id)
);

-- +goose Down
DROP TABLE IF EXISTS processed_events;
