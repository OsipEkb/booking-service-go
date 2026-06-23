-- +goose Up
CREATE TABLE IF NOT EXISTS outbox_messages (
    id           BIGSERIAL    PRIMARY KEY,
    routing_key  VARCHAR(255) NOT NULL,
    payload      TEXT         NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'PENDING',
    retry_count  INT          NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ  NULL,
    last_error   TEXT         NULL
);

CREATE INDEX idx_outbox_status_retry_count ON outbox_messages (status, retry_count);
CREATE INDEX idx_outbox_status_created_at  ON outbox_messages (status, created_at);

-- +goose Down
DROP TABLE IF EXISTS outbox_messages;
