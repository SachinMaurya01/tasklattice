package repository

import (
	"context"

	"tasklattice/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX abstracts the query surface shared by *pgxpool.Pool and pgx.Tx so
// outbox and audit rows can join a mutation's transaction atomically.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// applyChangeSet writes a mutation's outbox events and audit row using the
// caller's transaction. A nil audit writes no audit row.
func applyChangeSet(ctx context.Context, db DBTX, cs models.ChangeSet) error {	for _, e := range cs.Events {
		if e == nil {
			continue
		}
		if err := insertOutboxEvent(ctx, db, e); err != nil {
			return err
		}
	}
	if cs.Audit != nil {
		if err := insertAuditLog(ctx, db, cs.Audit); err != nil {
			return err
		}
	}
	return nil
}

// adoptCreatedID points side effects with no target at the entity just
// created by the same mutation. Only organization creation leaves the
// organization ID empty; everything else sets it explicitly.
func adoptCreatedID(id string, cs *ChangeSet) {
	for _, e := range cs.Events {
		if e == nil {
			continue
		}
		if e.AggregateID == "" {
			e.AggregateID = id
		}
		if e.OrganizationID == "" {
			e.OrganizationID = id
		}
	}
	if cs.Audit != nil {
		if cs.Audit.EntityID == nil {
			eid := id
			cs.Audit.EntityID = &eid
		}
		if cs.Audit.OrganizationID == "" {
			cs.Audit.OrganizationID = id
		}
	}
}
