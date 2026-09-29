package repository

import (
	"context"

	"Todo-App/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionRepository persists refresh-token sessions as hashes.
type SessionRepository interface {
	Create(ctx context.Context, s *models.RefreshSession) error
	GetByHash(ctx context.Context, hash string) (*models.RefreshSession, error)
	Revoke(ctx context.Context, id string) error
	RevokeAllForUser(ctx context.Context, userID string) error
	// Rotate revokes oldID and inserts next atomically.
	Rotate(ctx context.Context, oldID string, next *models.RefreshSession) error
}

// PostgresSessionRepository is the PostgreSQL SessionRepository implementation.
type PostgresSessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository wires a SessionRepository to a pool.
func NewSessionRepository(pool *pgxpool.Pool) *PostgresSessionRepository {
	return &PostgresSessionRepository{pool: pool}
}

// Create inserts a session and fills its generated ID and timestamp.
func (r *PostgresSessionRepository) Create(ctx context.Context, s *models.RefreshSession) error {
	const query = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`
	return r.pool.QueryRow(ctx, query, s.UserID, s.TokenHash, s.ExpiresAt).Scan(&s.ID, &s.CreatedAt)
}

// GetByHash fetches the session behind a token hash.
func (r *PostgresSessionRepository) GetByHash(ctx context.Context, hash string) (*models.RefreshSession, error) {
	const query = `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`
	var s models.RefreshSession
	err := r.pool.QueryRow(ctx, query, hash).Scan(
		&s.ID,
		&s.UserID,
		&s.TokenHash,
		&s.ExpiresAt,
		&s.RevokedAt,
		&s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Revoke marks one session revoked (logout, rotation). Idempotent.
func (r *PostgresSessionRepository) Revoke(ctx context.Context, id string) error {
	const query = `
		UPDATE refresh_tokens
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND revoked_at IS NULL
	`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// RevokeAllForUser marks every live session of a user revoked
// (logout-all, refresh-token reuse).
func (r *PostgresSessionRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	const query = `
		UPDATE refresh_tokens
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND revoked_at IS NULL
	`
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}

// Rotate revokes oldID and inserts next in one transaction so a refresh
// either fully rotates or leaves the old session untouched.
func (r *PostgresSessionRepository) Rotate(ctx context.Context, oldID string, next *models.RefreshSession) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}

	const revoke = `
		UPDATE refresh_tokens
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND revoked_at IS NULL
	`
	if _, err = tx.Exec(ctx, revoke, oldID); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	const insert = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`
	if err = tx.QueryRow(ctx, insert, next.UserID, next.TokenHash, next.ExpiresAt).Scan(&next.ID, &next.CreatedAt); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}
