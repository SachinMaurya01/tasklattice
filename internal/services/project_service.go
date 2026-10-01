package services

import (
	"context"
	"strings"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/repository"
)

// ProjectService implements project management scoped to organizations.
type ProjectService struct {
	projects repository.ProjectRepository
	orgs     repository.OrganizationRepository
}

// NewProjectService wires the project use cases.
func NewProjectService(projects repository.ProjectRepository, orgs repository.OrganizationRepository) *ProjectService {
	return &ProjectService{projects: projects, orgs: orgs}
}

// resolve loads a project and enforces min role in its org.
func (s *ProjectService) resolve(ctx context.Context, userID, projectID string, min models.Role) (*models.Project, error) {
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

// Resolve loads a project visible to userID (any member role).
func (s *ProjectService) Resolve(ctx context.Context, userID, projectID string) (*models.Project, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()
	return s.resolve(ctx, userID, projectID, models.RoleViewer)
}

// Create creates a project in an org. Admin only.
func (s *ProjectService) Create(ctx context.Context, userID, orgID, name, description string) (*models.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return nil, apperrors.BadRequest("Project name must be 1-255 characters")
	}
	if len(description) > 10000 {
		return nil, apperrors.BadRequest("Project description is too long")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := requireRole(ctx, s.orgs, orgID, userID, models.RoleAdmin); err != nil {
		return nil, err
	}
	by := userID
	p := &models.Project{OrganizationID: orgID, Name: name, Description: description, CreatedBy: &by}
	cs := models.ChangeSet{
		Events: []*models.OutboxEvent{
			models.MustOutboxEvent(
				models.EventProjectCreated, models.EntityProject, "", orgID,
				map[string]any{"name": name},
			),
		},
		Audit: models.NewAuditLog(orgID, userID, models.AuditProjectCreated, models.EntityProject, "",
			nil, map[string]any{"name": name}, nil),
	}
	if err := s.projects.Create(ctx, p, cs); err != nil {
		return nil, apperrors.Internal()
	}
	return p, nil
}

// List returns non-deleted projects of an org for a member.
func (s *ProjectService) List(ctx context.Context, userID, orgID string) ([]models.Project, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := requireRole(ctx, s.orgs, orgID, userID, models.RoleViewer); err != nil {
		return nil, err
	}
	projects, err := s.projects.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, apperrors.Internal()
	}
	return projects, nil
}

// Get returns one project for a member.
func (s *ProjectService) Get(ctx context.Context, userID, projectID string) (*models.Project, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	return s.resolve(ctx, userID, projectID, models.RoleViewer)
}

// Update renames or re-describes a project. Admin only.
func (s *ProjectService) Update(ctx context.Context, userID, projectID, name, description string) (*models.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return nil, apperrors.BadRequest("Project name must be 1-255 characters")
	}
	if len(description) > 10000 {
		return nil, apperrors.BadRequest("Project description is too long")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	p, err := s.resolve(ctx, userID, projectID, models.RoleAdmin)
	if err != nil {
		return nil, err
	}
	p.Name = name
	p.Description = description
	out, err := s.projects.Update(ctx, p)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Project not found")
		}
		return nil, apperrors.Internal()
	}
	return out, nil
}

// Delete soft-deletes a project. Admin only.
func (s *ProjectService) Delete(ctx context.Context, userID, projectID string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := s.resolve(ctx, userID, projectID, models.RoleAdmin); err != nil {
		return err
	}
	if err := s.projects.SoftDelete(ctx, projectID); err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Project not found")
		}
		return apperrors.Internal()
	}
	return nil
}
