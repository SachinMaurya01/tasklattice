package repository

import (
	"context"

	"tasklattice/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditRepository persists and reads immutable audit records.
type AuditRepository interface {
	// Insert writes an audit row using db (pool or an open transaction).
	Insert(ctx context.Context, db DBTX, a *models.AuditLog) error
	ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]models.AuditLog, error)
}

// PostgresAuditRepository is the PostgreSQL implementation.
type PostgresAuditRepository struct {
	pool *pgxpool.Pool
}

// NewAuditRepository wires an AuditRepository to a pool.
func NewAuditRepository(pool *pgxpool.Pool) *PostgresAuditRepository {
	return &PostgresAuditRepository{pool: pool}
}

// Insert writes one audit row.
func (r *PostgresAuditRepository) Insert(ctx context.Context, db DBTX, a *models.AuditLog) error {
	return insertAuditLog(ctx, db, a)
}

func insertAuditLog(ctx context.Context, db DBTX, a *models.AuditLog) error {
	const query = `
		INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, old_value, new_value, metadata)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::jsonb)
		RETURNING id, created_at
	`
	return db.QueryRow(ctx, query,
		a.OrganizationID, a.ActorID, a.Action, a.EntityType, a.EntityID,
		nullJSON(a.OldValue), nullJSON(a.NewValue), nullJSON(a.Metadata),
	).Scan(&a.ID, &a.CreatedAt)
}

func nullJSON(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// ListByOrg returns audit rows newest first with limit/offset paging.
func (r *PostgresAuditRepository) ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]models.AuditLog, error) {
	const query = `
		SELECT id, organization_id, actor_id, action, entity_type, entity_id,
			old_value::text, new_value::text, metadata::text, created_at
		FROM audit_logs
		WHERE organization_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []models.AuditLog{}
	for rows.Next() {
		var a models.AuditLog
		var oldVal, newVal, meta *string
		if err = rows.Scan(
			&a.ID, &a.OrganizationID, &a.ActorID, &a.Action, &a.EntityType, &a.EntityID,
			&oldVal, &newVal, &meta, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		a.OldValue, a.NewValue, a.Metadata = oldVal, newVal, meta
		logs = append(logs, a)
	}
	return logs, rows.Err()
}
