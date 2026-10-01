package worker

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"tasklattice/internal/models"
	"tasklattice/internal/repository"

	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// longPollSeconds keeps Receive blocked for near-real-time delivery.
const longPollSeconds = 20

// Handler processes one event; errors trigger redelivery with backoff.
type Handler func(ctx context.Context, env Envelope) error

// Consumer long-polls SQS and dispatches to handlers. Processed events are
// deduplicated per consumer, so at-least-once delivery stays safe.
type Consumer struct {
	queue      *Queue
	outbox     *repository.PostgresOutboxRepository
	name       string
	handlers   map[string]Handler
	visibility int32
	workers    int
}

// NewConsumer wires the SQS consumer.
func NewConsumer(queue *Queue, outbox *repository.PostgresOutboxRepository, name string, visibility int32, workers int) *Consumer {
	return &Consumer{
		queue:      queue,
		outbox:     outbox,
		name:       name,
		handlers:   map[string]Handler{},
		visibility: visibility,
		workers:    workers,
	}
}

// On registers a handler for an event type.
func (c *Consumer) On(eventType string, h Handler) {
	c.handlers[eventType] = h
}

// Run polls until ctx ends, waiting for in-flight messages on shutdown.
func (c *Consumer) Run(ctx context.Context) {
	sem := make(chan struct{}, c.workers)
	var wg sync.WaitGroup
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		default:
		}
		msgs, err := c.queue.Receive(ctx, c.visibility, longPollSeconds)
		if err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				return
			}
			log.Printf("sqs receive failed: %v", err)
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case <-time.After(time.Second):
			}
			continue
		}
		for _, m := range msgs {
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case sem <- struct{}{}:
			}
			wg.Add(1)
			go func(msg sqstypes.Message) {
				defer wg.Done()
				defer func() { <-sem }()
				c.handle(ctx, msg)
			}(m)
		}
	}
}

func receiveCount(m sqstypes.Message) int {
	raw, ok := m.Attributes[string(sqstypes.QueueAttributeNameApproximateReceiveCount)]
	if !ok {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// retryDelay backs off redelivery; the DLQ redrive policy caps attempts.
func retryDelay(receives int) int32 {
	d := 5 * receives
	if d > 300 {
		d = 300
	}
	return int32(d)
}

func (c *Consumer) handle(ctx context.Context, m sqstypes.Message) {
	receipt := ""
	if m.ReceiptHandle != nil {
		receipt = *m.ReceiptHandle
	}
	drop := func(reason string) {
		log.Printf("sqs message dropped (%s): %s", reason, stringValue(m.Body))
		if receipt != "" {
			if err := c.queue.Delete(ctx, receipt); err != nil {
				log.Printf("sqs delete failed: %v", err)
			}
		}
	}

	var env Envelope
	if m.Body == nil || json.Unmarshal([]byte(*m.Body), &env) != nil {
		drop("unparseable body")
		return
	}
	if !models.IsUUID(env.EventID) || env.EventType == "" {
		drop("invalid envelope")
		return
	}
	h, ok := c.handlers[env.EventType]
	if !ok {
		drop("no handler for "+env.EventType)
		return
	}
	// Handlers run first and are idempotent; the dedupe record lands only
	// after success, so failures always redeliver. A crash between the
	// two just replays an idempotent handler.
	if err := h(ctx, env); err != nil {
		log.Printf("handler %s failed for %s: %v", env.EventType, env.EventID, err)
		if receipt != "" {
			_ = c.queue.Defer(ctx, receipt, retryDelay(receiveCount(m)))
		}
		return
	}
	if first, err := c.outbox.TryMarkProcessed(ctx, env.EventID, c.name); err != nil {
		log.Printf("dedupe record failed for %s: %v", env.EventID, err)
	} else if !first {
		log.Printf("duplicate delivery of %s", env.EventID)
	}
	if receipt != "" {
		if err := c.queue.Delete(ctx, receipt); err != nil {
			log.Printf("sqs delete failed for %s: %v", env.EventID, err)
		}
	}
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
