package worker

import (
	"context"
	"log"
	"time"

	"tasklattice/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cleanup removes expired transient rows on a schedule.
type Cleanup struct {
	pool     *pgxpool.Pool
	idem     *repository.PostgresIdempotencyRepository
	interval time.Duration
}

// NewCleanup wires the cleanup loop.
func NewCleanup(pool *pgxpool.Pool, idem *repository.PostgresIdempotencyRepository, interval time.Duration) *Cleanup {
	return &Cleanup{pool: pool, idem: idem, interval: interval}
}

// Run sweeps until ctx ends.
func (c *Cleanup) Run(ctx context.Context) {
	c.sweep(ctx)
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.sweep(ctx)
		}
	}
}

func (c *Cleanup) sweep(ctx context.Context) {
	n, err := c.idem.CleanupExpired(ctx)
	if err != nil {
		log.Printf("cleanup idempotency keys failed: %v", err)
	} else if n > 0 {
		log.Printf("cleanup removed %d expired idempotency keys", n)
	}

	const expiredSessions = `
		DELETE FROM refresh_tokens
		WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '30 days'
	`
	if tag, err := c.pool.Exec(ctx, expiredSessions); err != nil {
		log.Printf("cleanup refresh sessions failed: %v", err)
	} else if tag.RowsAffected() > 0 {
		log.Printf("cleanup removed %d stale refresh sessions", tag.RowsAffected())
	}
}
