package mq

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher publishes persistent JSON messages to one durable queue. It is
// safe for concurrent use and transparently redials if the connection has
// dropped since the last publish.
type Publisher struct {
	url   string
	queue string

	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewPublisher dials url and declares queue. The connection is opened eagerly
// so a misconfigured broker URL fails fast at startup.
func NewPublisher(url, queue string) (*Publisher, error) {
	p := &Publisher{url: url, queue: queue}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

// connect (re)establishes the connection and channel. Callers must hold p.mu
// (NewPublisher is still single-goroutine when it calls this).
func (p *Publisher) connect() error {
	if p.conn != nil && !p.conn.IsClosed() {
		p.conn.Close()
	}
	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("open channel: %w", err)
	}
	if err := declareQueue(ch, p.queue); err != nil {
		conn.Close()
		return err
	}
	p.conn, p.ch = conn, ch
	return nil
}

// Publish sends body to the queue as a persistent message. If the channel is
// dead it redials once and retries, so a broker restart between publishes is
// invisible to callers.
func (p *Publisher) Publish(ctx context.Context, body []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if p.ch == nil || p.ch.IsClosed() {
			if err := p.connect(); err != nil {
				lastErr = err
				continue
			}
		}
		err := p.ch.PublishWithContext(ctx, "", p.queue, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		})
		if err == nil {
			return nil
		}
		lastErr = err
		p.ch = nil // force a redial on the next attempt
	}
	return fmt.Errorf("publish to %q: %w", p.queue, lastErr)
}

// Close closes the underlying connection (and its channel).
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil && !p.conn.IsClosed() {
		return p.conn.Close()
	}
	return nil
}
