// Package mq is the messaging layer connecting the three vdownloader services
// over RabbitMQ. It uses two durable queues on the default exchange (the
// routing key is the queue name):
//
//   - QueueJobs      – download job requests (web + telegram publish, worker consumes)
//   - QueueCompleted – job-completion notifications (worker publishes, telegram consumes)
//
// Both Publisher and Consumer redial on their own if the broker connection
// drops, so the order services start in doesn't matter.
package mq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Queue names never vary between deployments, so they're constants rather than
// configuration; only the broker URL (RABBITMQ_URL) is tunable.
const (
	QueueJobs      = "video.jobs"
	QueueCompleted = "video.completed"
)

// declareQueue declares a durable, non-exclusive queue that survives a broker
// restart. Every endpoint declares the queues it touches, so none depends on
// another service having started first.
func declareQueue(ch *amqp.Channel, name string) error {
	if _, err := ch.QueueDeclare(name, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", name, err)
	}
	return nil
}
