package services

import (
	"context"
	"strings"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/repository"
)

// LabelService implements project labels and task assignments.
type LabelService struct {
	labels   repository.LabelRepository
	tasks    repository.TaskRepository
	projects repository.ProjectRepository
	orgs     repository.OrganizationRepository
}

// NewLabelService wires the label use cases.
func NewLabelService(labels repository.LabelRepository, tasks repository.TaskRepository, projects repository.ProjectRepository, orgs repository.OrganizationRepository) *LabelService {
	return &LabelService{labels: labels, tasks: tasks, projects: projects, orgs: orgs}
}

// resolveProject loads a project and enforces min role in its org.
func (s *LabelService) resolveProject(ctx context.Context, userID, projectID string, min models.Role) (*models.Project, error) {
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
func (s *LabelService) resolveTask(ctx context.Context, userID, taskID string, min models.Role) (*models.Task, error) {
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

// Create creates a label in a project. Member only.
func (s *LabelService) Create(ctx context.Context, userID, projectID, name string) (*models.Label, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return nil, apperrors.BadRequest("Label name must be 1-100 characters")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	p, err := s.resolveProject(ctx, userID, projectID, models.RoleMember)
	if err != nil {
		return nil, err
	}
	l := &models.Label{OrganizationID: p.OrganizationID, ProjectID: p.ID, Name: name}
	if err := s.labels.Create(ctx, l); err != nil {
		if isDuplicate(err) {
			return nil, apperrors.Conflict("Label already exists in this project")
		}
		return nil, apperrors.Internal()
	}
	return l, nil
}

// List returns all labels of a project for a member.
func (s *LabelService) List(ctx context.Context, userID, projectID string) ([]models.Label, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	p, err := s.resolveProject(ctx, userID, projectID, models.RoleViewer)
	if err != nil {
		return nil, err
	}
	labels, err := s.labels.ListByProject(ctx, p.ID)
	if err != nil {
		return nil, apperrors.Internal()
	}
	return labels, nil
}

// Rename renames a label. Admin only.
func (s *LabelService) Rename(ctx context.Context, userID, labelID, name string) (*models.Label, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return nil, apperrors.BadRequest("Label name must be 1-100 characters")
	}
	if !models.IsUUID(labelID) {
		return nil, apperrors.BadRequest("Invalid label ID")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	l, err := s.labels.GetByID(ctx, labelID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Label not found")
		}
		return nil, apperrors.Internal()
	}
	if _, err := requireRole(ctx, s.orgs, l.OrganizationID, userID, models.RoleAdmin); err != nil {
		return nil, err
	}
	out, err := s.labels.UpdateName(ctx, l.ID, name)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Label not found")
		}
		if isDuplicate(err) {
			return nil, apperrors.Conflict("Label already exists in this project")
		}
		return nil, apperrors.Internal()
	}
	return out, nil
}

// Delete removes a label. Admin only.
func (s *LabelService) Delete(ctx context.Context, userID, labelID string) error {
	if !models.IsUUID(labelID) {
		return apperrors.BadRequest("Invalid label ID")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	l, err := s.labels.GetByID(ctx, labelID)
	if err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Label not found")
		}
		return apperrors.Internal()
	}
	if _, err := requireRole(ctx, s.orgs, l.OrganizationID, userID, models.RoleAdmin); err != nil {
		return err
	}
	if err := s.labels.Delete(ctx, l.ID); err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Label not found")
		}
		return apperrors.Internal()
	}
	return nil
}

// Attach assigns a project label to a task. Member only.
func (s *LabelService) Attach(ctx context.Context, userID, taskID, labelID string) error {
	if !models.IsUUID(labelID) {
		return apperrors.BadRequest("Invalid label ID")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	t, err := s.resolveTask(ctx, userID, taskID, models.RoleMember)
	if err != nil {
		return err
	}
	l, err := s.labels.GetByID(ctx, labelID)
	if err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Label not found")
		}
		return apperrors.Internal()
	}
	if l.ProjectID != t.ProjectID {
		return apperrors.BadRequest("Label does not belong to the task's project")
	}
	if err := s.labels.AttachToTask(ctx, t.ID, l.ID); err != nil {
		return apperrors.Internal()
	}
	return nil
}

// Detach removes a label from a task. Member only, idempotent.
func (s *LabelService) Detach(ctx context.Context, userID, taskID, labelID string) error {
	if !models.IsUUID(labelID) {
		return apperrors.BadRequest("Invalid label ID")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	t, err := s.resolveTask(ctx, userID, taskID, models.RoleMember)
	if err != nil {
		return err
	}
	if err := s.labels.DetachFromTask(ctx, t.ID, labelID); err != nil {
		return apperrors.Internal()
	}
	return nil
}
