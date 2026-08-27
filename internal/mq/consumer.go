package mq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Consumer delivers messages from one durable queue to a handler, reconnecting
// with a fixed backoff until its context is cancelled.
type Consumer struct {
	url   string
	queue string
	tag   string
}

// NewConsumer creates a Consumer for queue, identifying itself to the broker
// as tag.
func NewConsumer(url, queue, tag string) *Consumer {
	return &Consumer{url: url, queue: queue, tag: tag}
}

// Consume blocks, passing each message body to handle and acking it once
// handle returns, until ctx is cancelled. Prefetch is 1: a slow download must
// not make the broker dump the whole backlog onto one worker.
func (c *Consumer) Consume(ctx context.Context, log *slog.Logger, handle func(context.Context, []byte)) {
	const backoff = 3 * time.Second
	for {
		err := c.session(ctx, handle)
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		log.Error("mq: consumer connection lost, retrying", "queue", c.queue, "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (c *Consumer) session(ctx context.Context, handle func(context.Context, []byte)) error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	if err := declareQueue(ch, c.queue); err != nil {
		return err
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	deliveries, err := ch.Consume(c.queue, c.tag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %q: %w", c.queue, err)
	}

	closed := conn.NotifyClose(make(chan *amqp.Error, 1))

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-closed:
			if err != nil {
				return err
			}
			return errors.New("connection closed by broker")
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("delivery channel closed")
			}
			handle(ctx, d.Body)
			if err := d.Ack(false); err != nil {
				return fmt.Errorf("ack: %w", err)
			}
		}
	}
}
