package repository

import (
	"context"

	"tasklattice/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LabelRepository persists project labels and task assignments.
type LabelRepository interface {
	Create(ctx context.Context, l *models.Label) error
	GetByID(ctx context.Context, id string) (*models.Label, error)
	ListByProject(ctx context.Context, projectID string) ([]models.Label, error)
	ListByTask(ctx context.Context, taskID string) ([]models.Label, error)
	UpdateName(ctx context.Context, id, name string) (*models.Label, error)
	Delete(ctx context.Context, id string) error
	AttachToTask(ctx context.Context, taskID, labelID string) error
	DetachFromTask(ctx context.Context, taskID, labelID string) error
}

// PostgresLabelRepository is the PostgreSQL implementation.
type PostgresLabelRepository struct {
	pool *pgxpool.Pool
}

// NewLabelRepository wires a LabelRepository to a pool.
func NewLabelRepository(pool *pgxpool.Pool) *PostgresLabelRepository {
	return &PostgresLabelRepository{pool: pool}
}

const labelColumns = `id, organization_id, project_id, name, created_at`

func scanLabel(row pgx.Row) (*models.Label, error) {
	var l models.Label
	err := row.Scan(&l.ID, &l.OrganizationID, &l.ProjectID, &l.Name, &l.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// Create inserts a label and fills its generated fields.
func (r *PostgresLabelRepository) Create(ctx context.Context, l *models.Label) error {
	const query = `
		INSERT INTO labels (organization_id, project_id, name)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`
	return r.pool.QueryRow(ctx, query, l.OrganizationID, l.ProjectID, l.Name).Scan(&l.ID, &l.CreatedAt)
}

// GetByID fetches one label.
func (r *PostgresLabelRepository) GetByID(ctx context.Context, id string) (*models.Label, error) {
	const query = `SELECT ` + labelColumns + ` FROM labels WHERE id = $1`
	return scanLabel(r.pool.QueryRow(ctx, query, id))
}

// ListByProject returns all labels of a project, ordered by name.
func (r *PostgresLabelRepository) ListByProject(ctx context.Context, projectID string) ([]models.Label, error) {
	const query = `SELECT ` + labelColumns + ` FROM labels WHERE project_id = $1 ORDER BY name`
	rows, err := r.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := []models.Label{}
	for rows.Next() {
		var l models.Label
		if err = rows.Scan(&l.ID, &l.OrganizationID, &l.ProjectID, &l.Name, &l.CreatedAt); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

// ListByTask returns all labels assigned to a task, ordered by name.
func (r *PostgresLabelRepository) ListByTask(ctx context.Context, taskID string) ([]models.Label, error) {
	const query = `
		SELECT l.id, l.organization_id, l.project_id, l.name, l.created_at
		FROM labels l
		JOIN task_labels tl ON tl.label_id = l.id
		WHERE tl.task_id = $1
		ORDER BY l.name
	`
	rows, err := r.pool.Query(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := []models.Label{}
	for rows.Next() {
		var l models.Label
		if err = rows.Scan(&l.ID, &l.OrganizationID, &l.ProjectID, &l.Name, &l.CreatedAt); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

// UpdateName renames a label.
func (r *PostgresLabelRepository) UpdateName(ctx context.Context, id, name string) (*models.Label, error) {
	const query = `
		UPDATE labels SET name = $1 WHERE id = $2
		RETURNING id, organization_id, project_id, name, created_at
	`
	return scanLabel(r.pool.QueryRow(ctx, query, name, id))
}

// Delete hard-deletes a label; assignments cascade.
func (r *PostgresLabelRepository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM labels WHERE id = $1`
	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// AttachToTask assigns a label to a task. Idempotent.
func (r *PostgresLabelRepository) AttachToTask(ctx context.Context, taskID, labelID string) error {
	const query = `
		INSERT INTO task_labels (task_id, label_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := r.pool.Exec(ctx, query, taskID, labelID)
	return err
}

// DetachFromTask removes a label from a task. Idempotent.
func (r *PostgresLabelRepository) DetachFromTask(ctx context.Context, taskID, labelID string) error {
	const query = `DELETE FROM task_labels WHERE task_id = $1 AND label_id = $2`
	_, err := r.pool.Exec(ctx, query, taskID, labelID)
	return err
}
