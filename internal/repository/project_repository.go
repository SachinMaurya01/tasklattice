package repository

import (
	"context"

	"tasklattice/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProjectRepository persists projects scoped to organizations.
type ProjectRepository interface {
	Create(ctx context.Context, p *models.Project, cs models.ChangeSet) error
	GetByID(ctx context.Context, id string) (*models.Project, error)
	ListByOrg(ctx context.Context, orgID string) ([]models.Project, error)
	Update(ctx context.Context, p *models.Project) (*models.Project, error)
	SoftDelete(ctx context.Context, id string) error
}

// PostgresProjectRepository is the PostgreSQL implementation.
type PostgresProjectRepository struct {
	pool *pgxpool.Pool
}

// NewProjectRepository wires a ProjectRepository to a pool.
func NewProjectRepository(pool *pgxpool.Pool) *PostgresProjectRepository {
	return &PostgresProjectRepository{pool: pool}
}

// Create inserts a project and its side effects, atomically.
func (r *PostgresProjectRepository) Create(ctx context.Context, p *models.Project, cs models.ChangeSet) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO projects (organization_id, name, description, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	if err = tx.QueryRow(ctx, query, p.OrganizationID, p.Name, p.Description, p.CreatedBy).Scan(
		&p.ID, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	adoptCreatedID(p.ID, &cs)
	if err = applyChangeSet(ctx, tx, cs); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// GetByID fetches one non-deleted project.
func (r *PostgresProjectRepository) GetByID(ctx context.Context, id string) (*models.Project, error) {
	const query = `
		SELECT id, organization_id, name, description, created_by, created_at, updated_at
		FROM projects
		WHERE id = $1 AND deleted_at IS NULL
	`
	var p models.Project
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.Description, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListByOrg returns non-deleted projects of an org, newest first.
func (r *PostgresProjectRepository) ListByOrg(ctx context.Context, orgID string) ([]models.Project, error) {
	const query = `
		SELECT id, organization_id, name, description, created_by, created_at, updated_at
		FROM projects
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := []models.Project{}
	for rows.Next() {
		var p models.Project
		if err = rows.Scan(
			&p.ID, &p.OrganizationID, &p.Name, &p.Description, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// Update renames or re-describes a non-deleted project.
func (r *PostgresProjectRepository) Update(ctx context.Context, p *models.Project) (*models.Project, error) {
	const query = `
		UPDATE projects
		SET name = $1, description = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING id, organization_id, name, description, created_by, created_at, updated_at
	`
	var out models.Project
	err := r.pool.QueryRow(ctx, query, p.Name, p.Description, p.ID).Scan(
		&out.ID, &out.OrganizationID, &out.Name, &out.Description, &out.CreatedBy, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SoftDelete marks a project deleted.
func (r *PostgresProjectRepository) SoftDelete(ctx context.Context, id string) error {
	const query = `
		UPDATE projects
		SET deleted_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND deleted_at IS NULL
	`
	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
