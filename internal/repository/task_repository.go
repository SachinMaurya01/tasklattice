package repository

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"tasklattice/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TaskRepository persists tasks scoped to organizations and projects.
type TaskRepository interface {
	Create(ctx context.Context, t *models.Task) error
	GetByID(ctx context.Context, id string) (*models.Task, error)
	GetByIDIncludingDeleted(ctx context.Context, id string) (*models.Task, error)
	List(ctx context.Context, orgID, projectID string, q models.TaskQuery) (models.TaskPage, error)
	// UpdateWithVersion applies edits only when the version matches,
	// bumping it atomically. pgx.ErrNoRows means missing or stale.
	UpdateWithVersion(ctx context.Context, t *models.Task, expectedVersion int) (*models.Task, error)
	SoftDelete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) (*models.Task, error)
}

// PostgresTaskRepository is the PostgreSQL implementation.
type PostgresTaskRepository struct {
	pool *pgxpool.Pool
}

// NewTaskRepository wires a TaskRepository to a pool.
func NewTaskRepository(pool *pgxpool.Pool) *PostgresTaskRepository {
	return &PostgresTaskRepository{pool: pool}
}

const taskColumns = `id, organization_id, project_id, title, description, status,
	priority, creator_id, assignee_id, due_date, version, created_at, updated_at`

// Create inserts a task and fills its generated fields.
func (r *PostgresTaskRepository) Create(ctx context.Context, t *models.Task) error {
	const query = `
		INSERT INTO tasks (organization_id, project_id, title, description, status, priority, creator_id, assignee_id, due_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, version, created_at, updated_at
	`
	return r.pool.QueryRow(ctx, query,
		t.OrganizationID, t.ProjectID, t.Title, t.Description, t.Status,
		t.Priority, t.CreatorID, t.AssigneeID, t.DueDate,
	).Scan(&t.ID, &t.Version, &t.CreatedAt, &t.UpdatedAt)
}

// GetByID fetches one non-deleted task.
func (r *PostgresTaskRepository) GetByID(ctx context.Context, id string) (*models.Task, error) {
	return r.getByID(ctx, id, true)
}

// GetByIDIncludingDeleted fetches a task regardless of soft deletion.
func (r *PostgresTaskRepository) GetByIDIncludingDeleted(ctx context.Context, id string) (*models.Task, error) {
	return r.getByID(ctx, id, false)
}

func (r *PostgresTaskRepository) getByID(ctx context.Context, id string, hideDeleted bool) (*models.Task, error) {
	query := `SELECT ` + taskColumns + `, deleted_at FROM tasks WHERE id = $1`
	if hideDeleted {
		query += ` AND deleted_at IS NULL`
	}
	var t models.Task
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.OrganizationID, &t.ProjectID, &t.Title, &t.Description,
		&t.Status, &t.Priority, &t.CreatorID, &t.AssigneeID, &t.DueDate,
		&t.Version, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

var taskSortColumns = map[string]string{
	models.TaskSortCreated: "t.created_at",
	models.TaskSortUpdated: "t.updated_at",
	models.TaskSortDue:     "t.due_date",
}

// List returns one keyset-paginated slice of non-deleted tasks.
func (r *PostgresTaskRepository) List(ctx context.Context, orgID, projectID string, q models.TaskQuery) (models.TaskPage, error) {
	col := taskSortColumns[q.Sort]
	if col == "" {
		col = taskSortColumns[models.TaskSortCreated]
	}
	dir := "DESC"
	if strings.ToUpper(q.Order) == "ASC" {
		dir = "ASC"
	}
	cmp := "<"
	if dir == "ASC" {
		cmp = ">"
	}

	var sb strings.Builder
	args := []any{projectID, orgID}
	sb.WriteString(`SELECT ` + taskColumns + ` FROM tasks t WHERE t.project_id = $1 AND t.organization_id = $2 AND t.deleted_at IS NULL`)

	if q.Status != "" {
		args = append(args, q.Status)
		fmt.Fprintf(&sb, ` AND t.status = $%d`, len(args))
	}
	if q.Priority != "" {
		args = append(args, q.Priority)
		fmt.Fprintf(&sb, ` AND t.priority = $%d`, len(args))
	}
	if q.AssigneeID != "" {
		args = append(args, q.AssigneeID)
		fmt.Fprintf(&sb, ` AND t.assignee_id = $%d::uuid`, len(args))
	}
	if q.CreatorID != "" {
		args = append(args, q.CreatorID)
		fmt.Fprintf(&sb, ` AND t.creator_id = $%d::uuid`, len(args))
	}
	if q.DueBefore != nil {
		args = append(args, *q.DueBefore)
		fmt.Fprintf(&sb, ` AND t.due_date <= $%d`, len(args))
	}
	if q.DueAfter != nil {
		args = append(args, *q.DueAfter)
		fmt.Fprintf(&sb, ` AND t.due_date >= $%d`, len(args))
	}
	if q.CreatedBefore != nil {
		args = append(args, *q.CreatedBefore)
		fmt.Fprintf(&sb, ` AND t.created_at <= $%d`, len(args))
	}
	if q.CreatedAfter != nil {
		args = append(args, *q.CreatedAfter)
		fmt.Fprintf(&sb, ` AND t.created_at >= $%d`, len(args))
	}
	if q.LabelID != "" {
		args = append(args, q.LabelID)
		fmt.Fprintf(&sb, ` AND EXISTS (SELECT 1 FROM task_labels tl WHERE tl.task_id = t.id AND tl.label_id = $%d::uuid)`, len(args))
	}
	if q.Search != "" {
		args = append(args, q.Search)
		fmt.Fprintf(&sb, ` AND to_tsvector('english', t.title || ' ' || COALESCE(t.description, '')) @@ plainto_tsquery('english', $%d)`, len(args))
	}
	if q.Cursor != "" {
		cur, err := decodeTaskCursor(q.Cursor)
		if err != nil {
			return models.TaskPage{}, err
		}
		if cur.sort != q.Sort || cur.order != strings.ToLower(q.Order) {
			return models.TaskPage{}, fmt.Errorf("cursor does not match sort/order")
		}
		if q.Sort == models.TaskSortDue {
			if cur.value == "" {
				args = append(args, cur.id)
				fmt.Fprintf(&sb, ` AND t.due_date IS NULL AND t.id %s $%d::uuid`, cmp, len(args))
			} else {
				args = append(args, cur.value, cur.id)
				fmt.Fprintf(&sb, ` AND ((t.due_date, t.id) %s ($%d::timestamptz, $%d::uuid) OR t.due_date IS NULL)`,
					cmp, len(args)-1, len(args))
			}
		} else {
			args = append(args, cur.value, cur.id)
			fmt.Fprintf(&sb, ` AND (%s, t.id) %s ($%d::timestamptz, $%d::uuid)`, col, cmp, len(args)-1, len(args))
		}
	}

	limit := q.Limit
	if limit <= 0 {
		limit = models.DefaultTaskLimit
	}
	if limit > models.MaxTaskLimit {
		limit = models.MaxTaskLimit
	}
	fmt.Fprintf(&sb, ` ORDER BY %s %s NULLS LAST, t.id %s LIMIT %d`, col, dir, dir, limit+1)

	rows, err := r.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return models.TaskPage{}, err
	}
	defer rows.Close()

	tasks := []models.Task{}
	for rows.Next() {
		var t models.Task
		if err = rows.Scan(
			&t.ID, &t.OrganizationID, &t.ProjectID, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.CreatorID, &t.AssigneeID, &t.DueDate,
			&t.Version, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return models.TaskPage{}, err
		}
		tasks = append(tasks, t)
	}
	if err = rows.Err(); err != nil {
		return models.TaskPage{}, err
	}

	page := models.TaskPage{Data: tasks}
	if len(tasks) > limit {
		page.Data = tasks[:limit]
		page.HasMore = true
		last := page.Data[len(page.Data)-1]
		page.Pagination.NextCursor = encodeTaskCursor(q.Sort, strings.ToLower(q.Order), cursorValue(q.Sort, last), last.ID)
	}
	if page.Data == nil {
		page.Data = []models.Task{}
	}
	return page, nil
}

func cursorValue(sort string, t models.Task) string {
	switch sort {
	case models.TaskSortUpdated:
		return t.UpdatedAt.UTC().Format(time.RFC3339Nano)
	case models.TaskSortDue:
		if t.DueDate == nil {
			return ""
		}
		return t.DueDate.UTC().Format(time.RFC3339Nano)
	default:
		return t.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
}

// taskCursor is the decoded opaque pagination cursor.
type taskCursor struct {
	sort  string
	order string
	value string
	id    string
}

func encodeTaskCursor(sort, order, value, id string) string {
	raw := sort + "|" + order + "|" + value + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeTaskCursor(cursor string) (taskCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return taskCursor{}, fmt.Errorf("invalid cursor")
	}
	parts := strings.SplitN(string(raw), "|", 4)
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[3] == "" {
		return taskCursor{}, fmt.Errorf("invalid cursor")
	}
	if _, ok := taskSortColumns[parts[0]]; !ok {
		return taskCursor{}, fmt.Errorf("invalid cursor")
	}
	if parts[1] != "asc" && parts[1] != "desc" {
		return taskCursor{}, fmt.Errorf("invalid cursor")
	}
	if parts[2] != "" {
		if _, err := time.Parse(time.RFC3339Nano, parts[2]); err != nil {
			return taskCursor{}, fmt.Errorf("invalid cursor")
		}
	}
	if !models.IsUUID(parts[3]) {
		return taskCursor{}, fmt.Errorf("invalid cursor")
	}
	return taskCursor{sort: parts[0], order: parts[1], value: parts[2], id: parts[3]}, nil
}

// UpdateWithVersion applies edits only on a version match, bumping it.
func (r *PostgresTaskRepository) UpdateWithVersion(ctx context.Context, t *models.Task, expectedVersion int) (*models.Task, error) {
	const query = `
		UPDATE tasks
		SET title = $1, description = $2, status = $3, priority = $4,
		    assignee_id = $5, due_date = $6, version = version + 1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $7 AND version = $8 AND deleted_at IS NULL
		RETURNING id, organization_id, project_id, title, description, status,
			priority, creator_id, assignee_id, due_date, version, created_at, updated_at
	`
	var out models.Task
	err := r.pool.QueryRow(ctx, query,
		t.Title, t.Description, t.Status, t.Priority, t.AssigneeID, t.DueDate,
		t.ID, expectedVersion,
	).Scan(
		&out.ID, &out.OrganizationID, &out.ProjectID, &out.Title, &out.Description,
		&out.Status, &out.Priority, &out.CreatorID, &out.AssigneeID, &out.DueDate,
		&out.Version, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SoftDelete marks a task deleted.
func (r *PostgresTaskRepository) SoftDelete(ctx context.Context, id string) error {
	const query = `
		UPDATE tasks
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

// Restore clears soft deletion, bumping the version.
func (r *PostgresTaskRepository) Restore(ctx context.Context, id string) (*models.Task, error) {
	const query = `
		UPDATE tasks
		SET deleted_at = NULL, version = version + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND deleted_at IS NOT NULL
		RETURNING id, organization_id, project_id, title, description, status,
			priority, creator_id, assignee_id, due_date, version, created_at, updated_at
	`
	var out models.Task
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&out.ID, &out.OrganizationID, &out.ProjectID, &out.Title, &out.Description,
		&out.Status, &out.Priority, &out.CreatorID, &out.AssigneeID, &out.DueDate,
		&out.Version, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
