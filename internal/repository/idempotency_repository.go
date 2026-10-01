package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Idempotency statuses.
const (
	IdempotencyInProgress = "in_progress"
	IdempotencyCompleted  = "completed"
)

// IdempotencyKey is a stored request fingerprint with its replay.
type IdempotencyKey struct {
	Key          string
	UserID       string
	RequestHash  string
	Status       string
	StatusCode   *int
	ResponseBody *string
	ExpiresAt    time.Time
}

// IdempotencyRepository persists idempotency keys.
type IdempotencyRepository interface {
	// CreatePending inserts a key; false means the key already exists.
	CreatePending(ctx context.Context, key, userID, hash string, expiresAt time.Time) (bool, error)
	Get(ctx context.Context, key, userID string) (*IdempotencyKey, error)
	Complete(ctx context.Context, key, userID string, statusCode int, body string) error
	Delete(ctx context.Context, key, userID string) error
	CleanupExpired(ctx context.Context) (int64, error)
}

// PostgresIdempotencyRepository is the PostgreSQL implementation.
type PostgresIdempotencyRepository struct {
	pool *pgxpool.Pool
}

// NewIdempotencyRepository wires an IdempotencyRepository to a pool.
func NewIdempotencyRepository(pool *pgxpool.Pool) *PostgresIdempotencyRepository {
	return &PostgresIdempotencyRepository{pool: pool}
}

// CreatePending inserts a key; false means a row already exists.
func (r *PostgresIdempotencyRepository) CreatePending(ctx context.Context, key, userID, hash string, expiresAt time.Time) (bool, error) {
	const query = `
		INSERT INTO idempotency_keys (key, user_id, request_hash, status, expires_at)
		VALUES ($1, $2, $3, 'in_progress', $4)
		ON CONFLICT DO NOTHING
	`
	tag, err := r.pool.Exec(ctx, query, key, userID, hash, expiresAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// Get fetches a key, or pgx.ErrNoRows.
func (r *PostgresIdempotencyRepository) Get(ctx context.Context, key, userID string) (*IdempotencyKey, error) {
	const query = `
		SELECT key, user_id, request_hash, status, status_code, response_body, expires_at
		FROM idempotency_keys
		WHERE key = $1 AND user_id = $2
	`
	var k IdempotencyKey
	err := r.pool.QueryRow(ctx, query, key, userID).Scan(
		&k.Key, &k.UserID, &k.RequestHash, &k.Status, &k.StatusCode, &k.ResponseBody, &k.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	// Expired rows behave as absent; cleanup removes them lazily.
	if !k.ExpiresAt.After(time.Now()) {
		return nil, pgx.ErrNoRows
	}
	return &k, nil
}

// Complete stores the replay for a key.
func (r *PostgresIdempotencyRepository) Complete(ctx context.Context, key, userID string, statusCode int, body string) error {
	const query = `
		UPDATE idempotency_keys
		SET status = 'completed', status_code = $1, response_body = $2
		WHERE key = $3 AND user_id = $4
	`
	_, err := r.pool.Exec(ctx, query, statusCode, body, key, userID)
	return err
}

// Delete drops a key so the client may retry (used after 5xx).
func (r *PostgresIdempotencyRepository) Delete(ctx context.Context, key, userID string) error {
	const query = `DELETE FROM idempotency_keys WHERE key = $1 AND user_id = $2`
	_, err := r.pool.Exec(ctx, query, key, userID)
	return err
}

// CleanupExpired removes expired keys, returning the row count.
func (r *PostgresIdempotencyRepository) CleanupExpired(ctx context.Context) (int64, error) {
	const query = `DELETE FROM idempotency_keys WHERE expires_at <= CURRENT_TIMESTAMP`
	tag, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
