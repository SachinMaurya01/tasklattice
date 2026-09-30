package services

import (
	"context"
	stderrors "errors"
	"strings"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// OrganizationService implements multi-tenancy: orgs, membership, RBAC.
type OrganizationService struct {
	orgs repository.OrganizationRepository
}

// NewOrganizationService wires the organization use cases.
func NewOrganizationService(orgs repository.OrganizationRepository) *OrganizationService {
	return &OrganizationService{orgs: orgs}
}

// requireRole loads the org (404 when missing/deleted) then enforces that
// userID holds at least min. Never trusts client-supplied roles.
func requireRole(ctx context.Context, orgs repository.OrganizationRepository, orgID, userID string, min models.Role) (*models.OrganizationMember, error) {
	if !models.IsUUID(orgID) {
		return nil, apperrors.BadRequest("Invalid organization ID")
	}
	if _, err := orgs.GetByID(ctx, orgID); err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Organization not found")
		}
		return nil, apperrors.Internal()
	}
	m, err := orgs.GetMembership(ctx, orgID, userID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.Forbidden("Organization access denied")
		}
		return nil, apperrors.Internal()
	}
	if !m.Role.AtLeast(min) {
		return nil, apperrors.Forbidden("Insufficient role")
	}
	return m, nil
}

func isNoRows(err error) bool {
	return stderrors.Is(err, pgx.ErrNoRows)
}

func isDuplicate(err error) bool {
	var pgErr *pgconn.PgError
	return stderrors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Create creates an org with the caller as OWNER, atomically.
func (s *OrganizationService) Create(ctx context.Context, userID, name, description string) (*models.Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return nil, apperrors.BadRequest("Organization name must be 1-255 characters")
	}
	if len(description) > 10000 {
		return nil, apperrors.BadRequest("Organization description is too long")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	org := &models.Organization{Name: name, Description: description}
	if err := s.orgs.CreateWithOwner(ctx, org, userID); err != nil {
		return nil, apperrors.Internal()
	}
	return org, nil
}

// List returns orgs where the user is a member.
func (s *OrganizationService) List(ctx context.Context, userID string) ([]models.Organization, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	orgs, err := s.orgs.ListForUser(ctx, userID)
	if err != nil {
		return nil, apperrors.Internal()
	}
	return orgs, nil
}

// Get returns one org for a member.
func (s *OrganizationService) Get(ctx context.Context, userID, orgID string) (*models.Organization, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := requireRole(ctx, s.orgs, orgID, userID, models.RoleViewer); err != nil {
		return nil, err
	}
	org, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Organization not found")
		}
		return nil, apperrors.Internal()
	}
	return org, nil
}

// Update renames or re-describes an org. Admin only.
func (s *OrganizationService) Update(ctx context.Context, userID, orgID, name, description string) (*models.Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return nil, apperrors.BadRequest("Organization name must be 1-255 characters")
	}
	if len(description) > 10000 {
		return nil, apperrors.BadRequest("Organization description is too long")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := requireRole(ctx, s.orgs, orgID, userID, models.RoleAdmin); err != nil {
		return nil, err
	}
	org, err := s.orgs.Update(ctx, &models.Organization{ID: orgID, Name: name, Description: description})
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Organization not found")
		}
		return nil, apperrors.Internal()
	}
	return org, nil
}

// Delete soft-deletes an org. Owner only.
func (s *OrganizationService) Delete(ctx context.Context, userID, orgID string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := requireRole(ctx, s.orgs, orgID, userID, models.RoleOwner); err != nil {
		return err
	}
	if err := s.orgs.SoftDelete(ctx, orgID); err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Organization not found")
		}
		return apperrors.Internal()
	}
	return nil
}

// ListMembers returns org members. Any member may view.
func (s *OrganizationService) ListMembers(ctx context.Context, userID, orgID string) ([]models.OrganizationMember, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := requireRole(ctx, s.orgs, orgID, userID, models.RoleViewer); err != nil {
		return nil, err
	}
	members, err := s.orgs.ListMembers(ctx, orgID)
	if err != nil {
		return nil, apperrors.Internal()
	}
	return members, nil
}

// AddMember adds a user to the org. Admin only; granting admin/owner
// requires ownership. The target user must exist.
func (s *OrganizationService) AddMember(ctx context.Context, actorID, orgID, targetID string, role models.Role) (*models.OrganizationMember, error) {
	if !models.IsUUID(targetID) {
		return nil, apperrors.BadRequest("Invalid user ID")
	}
	if !role.Valid() {
		return nil, apperrors.BadRequest("Invalid role")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	actor, err := requireRole(ctx, s.orgs, orgID, actorID, models.RoleAdmin)
	if err != nil {
		return nil, err
	}
	if (role == models.RoleAdmin || role == models.RoleOwner) && actor.Role != models.RoleOwner {
		return nil, apperrors.Forbidden("Only owners may grant admin or owner")
	}
	exists, err := s.orgs.UserExists(ctx, targetID)
	if err != nil {
		return nil, apperrors.Internal()
	}
	if !exists {
		return nil, apperrors.NotFound("User not found")
	}
	m, err := s.orgs.AddMember(ctx, orgID, targetID, role)
	if err != nil {
		if isDuplicate(err) {
			return nil, apperrors.Conflict("User is already a member")
		}
		return nil, apperrors.Internal()
	}
	return m, nil
}

// UpdateMemberRole changes a member's role, guarding the final owner.
func (s *OrganizationService) UpdateMemberRole(ctx context.Context, actorID, orgID, targetID string, role models.Role) (*models.OrganizationMember, error) {
	if !models.IsUUID(targetID) {
		return nil, apperrors.BadRequest("Invalid user ID")
	}
	if !role.Valid() {
		return nil, apperrors.BadRequest("Invalid role")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	actor, err := requireRole(ctx, s.orgs, orgID, actorID, models.RoleAdmin)
	if err != nil {
		return nil, err
	}
	if (role == models.RoleAdmin || role == models.RoleOwner) && actor.Role != models.RoleOwner {
		return nil, apperrors.Forbidden("Only owners may grant admin or owner")
	}
	current, err := s.orgs.GetMembership(ctx, orgID, targetID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Member not found")
		}
		return nil, apperrors.Internal()
	}
	if current.Role == models.RoleOwner && role != models.RoleOwner {
		n, err := s.orgs.CountOwners(ctx, orgID)
		if err != nil {
			return nil, apperrors.Internal()
		}
		if n <= 1 {
			return nil, apperrors.Conflict("Cannot demote the final owner")
		}
	}
	m, err := s.orgs.UpdateMemberRole(ctx, orgID, targetID, role)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Member not found")
		}
		return nil, apperrors.Internal()
	}
	return m, nil
}

// RemoveMember removes a member, guarding the final owner. Admin only.
func (s *OrganizationService) RemoveMember(ctx context.Context, actorID, orgID, targetID string) error {
	if !models.IsUUID(targetID) {
		return apperrors.BadRequest("Invalid user ID")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := requireRole(ctx, s.orgs, orgID, actorID, models.RoleAdmin); err != nil {
		return err
	}
	current, err := s.orgs.GetMembership(ctx, orgID, targetID)
	if err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Member not found")
		}
		return apperrors.Internal()
	}
	if current.Role == models.RoleOwner {
		n, err := s.orgs.CountOwners(ctx, orgID)
		if err != nil {
			return apperrors.Internal()
		}
		if n <= 1 {
			return apperrors.Conflict("Cannot remove the final owner")
		}
	}
	if err := s.orgs.RemoveMember(ctx, orgID, targetID); err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Member not found")
		}
		return apperrors.Internal()
	}
	return nil
}
