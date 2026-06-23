package models

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusProcessed = "PROCESSED"
	OutboxStatusFailed    = "FAILED"
)

type OutboxMessage struct {
	ID          int64
	RoutingKey  string
	Payload     string
	Status      string
	RetryCount  int
	CreatedAt   time.Time
	ProcessedAt *time.Time
	LastError   *string
}

func NewOutboxMessage(routingKey, payload string) *OutboxMessage {
	return &OutboxMessage{
		RoutingKey: routingKey,
		Payload:    payload,
		Status:     OutboxStatusPending,
		CreatedAt:  time.Now().UTC(),
	}
}

func (m *OutboxMessage) MarkAsProcessed() {
	now := time.Now().UTC()
	m.Status = OutboxStatusProcessed
	m.ProcessedAt = &now
}

func (m *OutboxMessage) MarkAsFailed(err string) {
	m.Status = OutboxStatusFailed
	m.RetryCount++
	m.LastError = &err
}
