package services

import (
	"context"
	"strings"
	"time"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/repository"
)

// TaskService implements task management with filtering, cursor paging,
// PostgreSQL search, and optimistic concurrency.
type TaskService struct {
	tasks    repository.TaskRepository
	projects repository.ProjectRepository
	orgs     repository.OrganizationRepository
	labels   repository.LabelRepository
}

// NewTaskService wires the task use cases.
func NewTaskService(tasks repository.TaskRepository, projects repository.ProjectRepository, orgs repository.OrganizationRepository, labels repository.LabelRepository) *TaskService {
	return &TaskService{tasks: tasks, projects: projects, orgs: orgs, labels: labels}
}

// resolveProject loads a project and enforces min role in its org.
func (s *TaskService) resolveProject(ctx context.Context, userID, projectID string, min models.Role) (*models.Project, error) {
	if !models.IsUUID(projectID) {
		return nil, apperrors.BadRequest("Invalid project ID")
	}
	p, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Project not found")
		}
		return nil, apperrors.Internal()
	}
	if _, err := requireRole(ctx, s.orgs, p.OrganizationID, userID, min); err != nil {
		return nil, err
	}
	return p, nil
}

// resolveTask loads a non-deleted task and enforces min role in its org.
func (s *TaskService) resolveTask(ctx context.Context, userID, taskID string, min models.Role) (*models.Task, error) {
	if !models.IsUUID(taskID) {
		return nil, apperrors.BadRequest("Invalid task ID")
	}
	t, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Task not found")
		}
		return nil, apperrors.Internal()
	}
	if _, err := requireRole(ctx, s.orgs, t.OrganizationID, userID, min); err != nil {
		return nil, err
	}
	return t, nil
}

func checkAssignee(ctx context.Context, orgs repository.OrganizationRepository, orgID string, assigneeID *string) error {
	if assigneeID == nil || *assigneeID == "" {
		return nil
	}
	if !models.IsUUID(*assigneeID) {
		return apperrors.BadRequest("Invalid assignee ID")
	}
	if _, err := orgs.GetMembership(ctx, orgID, *assigneeID); err != nil {
		if isNoRows(err) {
			return apperrors.BadRequest("Assignee must be an organization member")
		}
		return apperrors.Internal()
	}
	return nil
}

// TaskCreate carries validated task creation input.
type TaskCreate struct {
	Title       string
	Description string
	Status      string
	Priority    string
	AssigneeID  *string
	DueDate     *time.Time
}

// Create creates a task in a project. Member only.
func (s *TaskService) Create(ctx context.Context, userID, projectID string, in TaskCreate) (*models.Task, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 500 {
		return nil, apperrors.BadRequest("Task title must be 1-500 characters")
	}
	if len(in.Description) > 20000 {
		return nil, apperrors.BadRequest("Task description is too long")
	}
	status := in.Status
	if status == "" {
		status = models.TaskTodo
	}
	if !models.ValidTaskStatus(status) {
		return nil, apperrors.BadRequest("Invalid task status")
	}
	priority := in.Priority
	if priority == "" {
		priority = models.PriorityMedium
	}
	if !models.ValidTaskPriority(priority) {
		return nil, apperrors.BadRequest("Invalid task priority")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	p, err := s.resolveProject(ctx, userID, projectID, models.RoleMember)
	if err != nil {
		return nil, err
	}
	if err := checkAssignee(ctx, s.orgs, p.OrganizationID, in.AssigneeID); err != nil {
		return nil, err
	}
	t := &models.Task{
		OrganizationID: p.OrganizationID,
		ProjectID:      p.ID,
		Title:          title,
		Description:    in.Description,
		Status:         status,
		Priority:       priority,
		CreatorID:      &userID,
		AssigneeID:     in.AssigneeID,
		DueDate:        in.DueDate,
	}
	if err := s.tasks.Create(ctx, t); err != nil {
		return nil, apperrors.Internal()
	}
	return t, nil
}

// Get returns one task with its labels for a member.
func (s *TaskService) Get(ctx context.Context, userID, taskID string) (*models.Task, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	t, err := s.resolveTask(ctx, userID, taskID, models.RoleViewer)
	if err != nil {
		return nil, err
	}
	labels, err := s.labels.ListByTask(ctx, t.ID)
	if err != nil {
		return nil, apperrors.Internal()
	}
	t.Labels = labels
	return t, nil
}

// List returns one cursor-paginated, filtered task slice for a member.
func (s *TaskService) List(ctx context.Context, userID, projectID string, q models.TaskQuery) (models.TaskPage, error) {
	if q.Status != "" && !models.ValidTaskStatus(q.Status) {
		return models.TaskPage{}, apperrors.BadRequest("Invalid status filter")
	}
	if q.Priority != "" && !models.ValidTaskPriority(q.Priority) {
		return models.TaskPage{}, apperrors.BadRequest("Invalid priority filter")
	}
	for _, id := range []string{q.AssigneeID, q.CreatorID, q.LabelID} {
		if id != "" && !models.IsUUID(id) {
			return models.TaskPage{}, apperrors.BadRequest("Invalid filter ID")
		}
	}
	if q.Sort == "" {
		q.Sort = models.TaskSortCreated
	}
	switch q.Sort {
	case models.TaskSortCreated, models.TaskSortUpdated, models.TaskSortDue:
	default:
		return models.TaskPage{}, apperrors.BadRequest("Invalid sort field")
	}
	if q.Order == "" {
		q.Order = "desc"
	}
	if q.Order != "asc" && q.Order != "desc" {
		return models.TaskPage{}, apperrors.BadRequest("Invalid sort order")
	}
	if q.Limit <= 0 {
		q.Limit = models.DefaultTaskLimit
	}
	if q.Limit > models.MaxTaskLimit {
		q.Limit = models.MaxTaskLimit
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	p, err := s.resolveProject(ctx, userID, projectID, models.RoleViewer)
	if err != nil {
		return models.TaskPage{}, err
	}
	if q.LabelID != "" {
		l, err := s.labels.GetByID(ctx, q.LabelID)
		if err != nil {
			if isNoRows(err) {
				return models.TaskPage{}, apperrors.NotFound("Label not found")
			}
			return models.TaskPage{}, apperrors.Internal()
		}
		if l.ProjectID != p.ID {
			return models.TaskPage{}, apperrors.NotFound("Label not found")
		}
	}
	page, err := s.tasks.List(ctx, p.OrganizationID, p.ID, q)
	if err != nil {
		if strings.HasPrefix(err.Error(), "invalid cursor") || strings.HasPrefix(err.Error(), "cursor does not match") {
			return models.TaskPage{}, apperrors.BadRequest("Invalid pagination cursor")
		}
		return models.TaskPage{}, apperrors.Internal()
	}
	return page, nil
}

// TaskPatch carries validated task update input. Version is required.
type TaskPatch struct {
	Title       *string
	Description *string
	Status      *string
	Priority    *string
	AssigneeID  *string
	ClearAssign bool
	DueDate     *time.Time
	ClearDue    bool
	Version     int
}

// Update applies edits only when Version matches, else 409. Member only.
func (s *TaskService) Update(ctx context.Context, userID, taskID string, in TaskPatch) (*models.Task, error) {
	if in.Version < 1 {
		return nil, apperrors.BadRequest("Task version is required")
	}
	if in.Title != nil {
		trimmed := strings.TrimSpace(*in.Title)
		if trimmed == "" || len(trimmed) > 500 {
			return nil, apperrors.BadRequest("Task title must be 1-500 characters")
		}
		in.Title = &trimmed
	}
	if in.Description != nil && len(*in.Description) > 20000 {
		return nil, apperrors.BadRequest("Task description is too long")
	}
	if in.Status != nil && !models.ValidTaskStatus(*in.Status) {
		return nil, apperrors.BadRequest("Invalid task status")
	}
	if in.Priority != nil && !models.ValidTaskPriority(*in.Priority) {
		return nil, apperrors.BadRequest("Invalid task priority")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	t, err := s.resolveTask(ctx, userID, taskID, models.RoleMember)
	if err != nil {
		return nil, err
	}
	if in.Title != nil {
		t.Title = *in.Title
	}
	if in.Description != nil {
		t.Description = *in.Description
	}
	if in.Status != nil {
		t.Status = *in.Status
	}
	if in.Priority != nil {
		t.Priority = *in.Priority
	}
	if in.ClearAssign {
		t.AssigneeID = nil
	} else if in.AssigneeID != nil {
		t.AssigneeID = in.AssigneeID
	}
	if err := checkAssignee(ctx, s.orgs, t.OrganizationID, t.AssigneeID); err != nil {
		return nil, err
	}
	if in.ClearDue {
		t.DueDate = nil
	} else if in.DueDate != nil {
		t.DueDate = in.DueDate
	}
	out, err := s.tasks.UpdateWithVersion(ctx, t, in.Version)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.VersionConflict()
		}
		return nil, apperrors.Internal()
	}
	return out, nil
}

// Delete soft-deletes a task. Member only.
func (s *TaskService) Delete(ctx context.Context, userID, taskID string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := s.resolveTask(ctx, userID, taskID, models.RoleMember); err != nil {
		return err
	}
	if err := s.tasks.SoftDelete(ctx, taskID); err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Task not found")
		}
		return apperrors.Internal()
	}
	return nil
}

// Restore clears soft deletion. Member only; active tasks conflict.
func (s *TaskService) Restore(ctx context.Context, userID, taskID string) (*models.Task, error) {
	if !models.IsUUID(taskID) {
		return nil, apperrors.BadRequest("Invalid task ID")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	t, err := s.tasks.GetByIDIncludingDeleted(ctx, taskID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Task not found")
		}
		return nil, apperrors.Internal()
	}
	if _, err := requireRole(ctx, s.orgs, t.OrganizationID, userID, models.RoleMember); err != nil {
		return nil, err
	}
	if t.DeletedAt == nil {
		return nil, apperrors.Conflict("Task is already active")
	}
	out, err := s.tasks.Restore(ctx, taskID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Task not found")
		}
		return nil, apperrors.Internal()
	}
	return out, nil
}
