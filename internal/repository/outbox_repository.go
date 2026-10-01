package repository

import (
	"context"
	"time"

	"tasklattice/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxRepository persists transactional outbox events and consumer
// deduplication records.
type OutboxRepository interface {
	// Insert writes an event using db (pool or an open transaction).
	Insert(ctx context.Context, db DBTX, e *models.OutboxEvent) error
	// ClaimBatch atomically takes up to limit due unpublished events,
	// bumping attempts so a crashed publisher never double-claims.
	ClaimBatch(ctx context.Context, limit int) ([]models.OutboxEvent, error)
	MarkPublished(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id, lastErr string, nextAttempt time.Time) error
	// TryMarkProcessed records (event, consumer); false means duplicate.
	TryMarkProcessed(ctx context.Context, eventID, consumer string) (bool, error)
}

// PostgresOutboxRepository is the PostgreSQL implementation.
type PostgresOutboxRepository struct {
	pool *pgxpool.Pool
}

// NewOutboxRepository wires an OutboxRepository to a pool.
func NewOutboxRepository(pool *pgxpool.Pool) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{pool: pool}
}

// Insert writes one outbox event.
func (r *PostgresOutboxRepository) Insert(ctx context.Context, db DBTX, e *models.OutboxEvent) error {
	return insertOutboxEvent(ctx, db, e)
}

func insertOutboxEvent(ctx context.Context, db DBTX, e *models.OutboxEvent) error {
	const query = `
		INSERT INTO outbox_events (event_type, aggregate_type, aggregate_id, organization_id, payload)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING id, created_at, attempts
	`
	return db.QueryRow(ctx, query, e.EventType, e.AggregateType, e.AggregateID, e.OrganizationID, e.Payload).Scan(
		&e.ID, &e.CreatedAt, &e.Attempts,
	)
}

// ClaimBatch takes due unpublished events in one atomic statement.
func (r *PostgresOutboxRepository) ClaimBatch(ctx context.Context, limit int) ([]models.OutboxEvent, error) {
	const query = `
		UPDATE outbox_events SET attempts = attempts + 1, last_error = NULL
		WHERE id IN (
			SELECT id FROM outbox_events
			WHERE published_at IS NULL AND next_attempt_at <= CURRENT_TIMESTAMP
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, event_type, aggregate_type, aggregate_id, organization_id,
			payload::text, created_at, attempts
	`
	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []models.OutboxEvent{}
	for rows.Next() {
		var e models.OutboxEvent
		if err = rows.Scan(
			&e.ID, &e.EventType, &e.AggregateType, &e.AggregateID,
			&e.OrganizationID, &e.Payload, &e.CreatedAt, &e.Attempts,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// MarkPublished flags an event delivered.
func (r *PostgresOutboxRepository) MarkPublished(ctx context.Context, id string) error {
	const query = `
		UPDATE outbox_events
		SET published_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// MarkFailed records an attempt failure and schedules the next try.
func (r *PostgresOutboxRepository) MarkFailed(ctx context.Context, id, lastErr string, nextAttempt time.Time) error {
	const query = `
		UPDATE outbox_events
		SET last_error = $1, next_attempt_at = $2
		WHERE id = $3
	`
	_, err := r.pool.Exec(ctx, query, lastErr, nextAttempt, id)
	return err
}

// TryMarkProcessed inserts a dedupe record; false means already processed.
func (r *PostgresOutboxRepository) TryMarkProcessed(ctx context.Context, eventID, consumer string) (bool, error) {
	const query = `
		INSERT INTO processed_events (event_id, consumer)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	tag, err := r.pool.Exec(ctx, query, eventID, consumer)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
