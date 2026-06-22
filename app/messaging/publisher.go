package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// Publisher публикует сообщения в RabbitMQ.
type Publisher struct {
	conn                  *Connection
	exchangeName          string
	publisherExchangeName string
	logger                *zap.Logger
}

// NewPublisher создаёт новый Publisher.
// exchangeName — exchange для получения ответов (consumer side).
// publisherExchangeName — exchange для отправки команд в Catalog.
func NewPublisher(conn *Connection, exchangeName, publisherExchangeName string, logger *zap.Logger) *Publisher {
	return &Publisher{
		conn:                  conn,
		exchangeName:          exchangeName,
		publisherExchangeName: publisherExchangeName,
		logger:                logger,
	}
}

// Publish публикует сообщение с указанным routing key.
func (p *Publisher) Publish(ctx context.Context, routingKey string, message any) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("сериализация сообщения: %w", err)
	}

	err = p.conn.Channel().PublishWithContext(
		ctx,
		p.exchangeName, // exchange
		routingKey,     // routing key
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("публикация сообщения (routing key: %s): %w", routingKey, err)
	}

	p.logger.Debug("сообщение опубликовано",
		zap.String("routingKey", routingKey),
		zap.String("body", string(body)),
	)

	return nil
}

// PublishCreateBookingJob публикует команду на создание задания бронирования.
func (p *Publisher) PublishCreateBookingJob(ctx context.Context, cmd CreateBookingJobCommand) error {
	return p.publishToCatalog(ctx, RoutingKeyCreateBookingJob, cmd)
}

// PublishCancelBookingJob публикует команду на отмену задания бронирования.
func (p *Publisher) PublishCancelBookingJob(ctx context.Context, cmd CancelBookingJobCommand) error {
	return p.publishToCatalog(ctx, RoutingKeyCancelBookingJob, cmd)
}

// publishToCatalog публикует сообщение в Catalog через Rebus-совместимый exchange.
// Добавляет заголовки, необходимые для Rebus (rbs2-*).
func (p *Publisher) publishToCatalog(ctx context.Context, routingKey string, message any) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("сериализация сообщения: %w", err)
	}

	msgID := NewMessageID()
	headers := amqp.Table{
		"rbs2-msgid":        msgID,
		"rbs2-corr-id":      msgID,
		"rbs2-corr-seq":     int64(0),
		"rbs2-sent-time":    time.Now().UTC().Format(time.RFC3339),
		"rbs2-msg-type":     routingKey,
		"rbs2-content-type": "application/json;charset=utf-8",
	}

	err = p.conn.Channel().PublishWithContext(
		ctx,
		p.publisherExchangeName,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json;charset=utf-8",
			DeliveryMode: amqp.Persistent,
			Body:         body,
			Headers:      headers,
		},
	)
	if err != nil {
		return fmt.Errorf("публикация сообщения в Catalog (routing key: %s): %w", routingKey, err)
	}

	p.logger.Debug("сообщение опубликовано в Catalog",
		zap.String("routingKey", routingKey),
		zap.String("exchange", p.publisherExchangeName),
		zap.String("body", string(body)),
	)

	return nil
}

func (p *Publisher) PublishBookingStatusChanged(ctx context.Context, event BookingStatusChangedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("сериализация BookingStatusChangedEvent: %w", err)
	}

	err = p.conn.Channel().PublishWithContext(
		ctx,
		"booking-domain-events",
		RoutingKeyBookingStatusChanged,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("публикация доменного события (bookingId: %d): %w", event.BookingId, err)
	}

	p.logger.Debug("доменное событие изменения статуса опубликовано",
		zap.Int64("bookingId", event.BookingId),
		zap.String("routingKey", RoutingKeyBookingStatusChanged),
		zap.String("body", string(body)),
	)

	return nil
}
