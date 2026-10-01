package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"tasklattice/internal/models"
	"tasklattice/internal/repository"
)

// Envelope is the SQS message body for one domain event.
type Envelope struct {
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	AggregateType  string          `json:"aggregate_type"`
	AggregateID    string          `json:"aggregate_id"`
	OrganizationID string          `json:"organization_id"`
	OccurredAt     time.Time       `json:"occurred_at"`
	Payload        json.RawMessage `json:"payload"`
}

// maxBackoff caps publisher retries; attempts continue indefinitely so no
// event is silently dropped.
const maxBackoff = 5 * time.Minute

// backoff returns exponential delay by attempt count, capped.
func backoff(attempts int) time.Duration {
	shift := attempts
	if shift < 0 {
		shift = 0
	}
	if shift > 8 {
		shift = 8
	}
	d := time.Duration(1<<uint(shift)) * time.Second
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func truncateErr(err error, n int) string {
	msg := err.Error()
	if len(msg) > n {
		return msg[:n]
	}
	return msg
}

// Publisher relays claimed outbox events to SQS.
type Publisher struct {
	outbox *repository.PostgresOutboxRepository
	queue  *Queue
	batch  int
	poll   time.Duration
}

// NewPublisher wires the outbox publisher.
func NewPublisher(outbox *repository.PostgresOutboxRepository, queue *Queue, batch int, poll time.Duration) *Publisher {
	return &Publisher{outbox: outbox, queue: queue, batch: batch, poll: poll}
}

// Run publishes due events until ctx ends.
func (p *Publisher) Run(ctx context.Context) {
	p.publishBatch(ctx)
	t := time.NewTicker(p.poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.publishBatch(ctx)
		}
	}
}

func (p *Publisher) publishBatch(ctx context.Context) {
	events, err := p.outbox.ClaimBatch(ctx, p.batch)
	if err != nil {
		log.Printf("outbox claim failed: %v", err)
		return
	}
	for _, e := range events {
		p.publishOne(ctx, e)
	}
}

func (p *Publisher) publishOne(ctx context.Context, e models.OutboxEvent) {
	env := Envelope{
		EventID:        e.ID,
		EventType:      e.EventType,
		AggregateType:  e.AggregateType,
		AggregateID:    e.AggregateID,
		OrganizationID: e.OrganizationID,
		OccurredAt:     e.CreatedAt,
		Payload:        json.RawMessage(e.Payload),
	}
	body, err := json.Marshal(env)
	if err != nil {
		log.Printf("outbox event %s unserializable: %v", e.ID, err)
		return
	}
	if err = p.queue.Send(ctx, string(body), e.EventType, e.ID); err != nil {
		next := time.Now().Add(backoff(e.Attempts))
		log.Printf("outbox event %s publish failed (attempt %d): %v", e.ID, e.Attempts, err)
		if derr := p.outbox.MarkFailed(ctx, e.ID, truncateErr(err, 500), next); derr != nil {
			log.Printf("outbox event %s mark-failed failed: %v", e.ID, derr)
		}
		return
	}
	if err = p.outbox.MarkPublished(ctx, e.ID); err != nil {
		log.Printf("outbox event %s mark-published failed: %v", e.ID, err)
	}
}
