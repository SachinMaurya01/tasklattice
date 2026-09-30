package repository

import (
	"context"

	"tasklattice/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CommentRepository persists task comments.
type CommentRepository interface {
	Create(ctx context.Context, c *models.Comment) error
	GetByID(ctx context.Context, id string) (*models.Comment, error)
	ListByTask(ctx context.Context, taskID string) ([]models.Comment, error)
	UpdateBody(ctx context.Context, id, body string) (*models.Comment, error)
	SoftDelete(ctx context.Context, id string) error
}

// PostgresCommentRepository is the PostgreSQL implementation.
type PostgresCommentRepository struct {
	pool *pgxpool.Pool
}

// NewCommentRepository wires a CommentRepository to a pool.
func NewCommentRepository(pool *pgxpool.Pool) *PostgresCommentRepository {
	return &PostgresCommentRepository{pool: pool}
}

const commentColumns = `id, organization_id, task_id, author_id, body, created_at, updated_at`

// Create inserts a comment and fills its generated fields.
func (r *PostgresCommentRepository) Create(ctx context.Context, c *models.Comment) error {
	const query = `
		INSERT INTO comments (organization_id, task_id, author_id, body)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	return r.pool.QueryRow(ctx, query, c.OrganizationID, c.TaskID, c.AuthorID, c.Body).Scan(
		&c.ID, &c.CreatedAt, &c.UpdatedAt,
	)
}

// GetByID fetches one non-deleted comment.
func (r *PostgresCommentRepository) GetByID(ctx context.Context, id string) (*models.Comment, error) {
	const query = `SELECT ` + commentColumns + ` FROM comments WHERE id = $1 AND deleted_at IS NULL`
	var c models.Comment
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.OrganizationID, &c.TaskID, &c.AuthorID, &c.Body, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListByTask returns non-deleted comments of a task, oldest first.
func (r *PostgresCommentRepository) ListByTask(ctx context.Context, taskID string) ([]models.Comment, error) {
	const query = `SELECT ` + commentColumns + ` FROM comments WHERE task_id = $1 AND deleted_at IS NULL ORDER BY created_at`
	rows, err := r.pool.Query(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := []models.Comment{}
	for rows.Next() {
		var c models.Comment
		if err = rows.Scan(
			&c.ID, &c.OrganizationID, &c.TaskID, &c.AuthorID, &c.Body, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// UpdateBody edits a non-deleted comment.
func (r *PostgresCommentRepository) UpdateBody(ctx context.Context, id, body string) (*models.Comment, error) {
	const query = `
		UPDATE comments
		SET body = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING id, organization_id, task_id, author_id, body, created_at, updated_at
	`
	var c models.Comment
	err := r.pool.QueryRow(ctx, query, body, id).Scan(
		&c.ID, &c.OrganizationID, &c.TaskID, &c.AuthorID, &c.Body, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// SoftDelete marks a comment deleted.
func (r *PostgresCommentRepository) SoftDelete(ctx context.Context, id string) error {
	const query = `
		UPDATE comments
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
