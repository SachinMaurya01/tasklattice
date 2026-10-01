package services

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"time"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

// orgCacheTTL is the short-lived cache for organization reads.
const orgCacheTTL = 60 * time.Second

// OrganizationService implements multi-tenancy: orgs, membership, RBAC.
type OrganizationService struct {
	orgs  repository.OrganizationRepository
	redis *redis.Client
}

// NewOrganizationService wires the organization use cases. A nil redis
// client disables caching.
func NewOrganizationService(orgs repository.OrganizationRepository, redisClient *redis.Client) *OrganizationService {
	return &OrganizationService{orgs: orgs, redis: redisClient}
}

func orgCacheKey(orgID string) string { return "org:" + orgID }

// cachedOrg returns the cached org, or nil on any miss/failure.
func (s *OrganizationService) cachedOrg(ctx context.Context, orgID string) *models.Organization {
	if s.redis == nil {
		return nil
	}
	raw, err := s.redis.Get(ctx, orgCacheKey(orgID)).Bytes()
	if err != nil {
		return nil
	}
	var org models.Organization
	if err := json.Unmarshal(raw, &org); err != nil {
		return nil
	}
	return &org
}

func (s *OrganizationService) putCachedOrg(ctx context.Context, org *models.Organization) {
	if s.redis == nil || org == nil {
		return
	}
	raw, err := json.Marshal(org)
	if err != nil {
		return
	}
	_ = s.redis.Set(ctx, orgCacheKey(org.ID), raw, orgCacheTTL).Err()
}

func (s *OrganizationService) dropCachedOrg(ctx context.Context, orgID string) {
	if s.redis == nil {
		return
	}
	_ = s.redis.Del(ctx, orgCacheKey(orgID)).Err()
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
	var cs models.ChangeSet
	cs.AppendEvent(models.MustOutboxEvent(
		models.EventOrgCreated, models.EntityOrganization, "", "",
		map[string]any{"name": name},
	))
	// OrganizationID fills in after insert; adoptCreatedID targets the event.
	cs.Audit = models.NewAuditLog("", userID, models.AuditOrgCreated, models.EntityOrganization, "",
		nil, map[string]any{"name": name}, nil)
	if err := s.orgs.CreateWithOwner(ctx, org, userID, cs); err != nil {
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
	// Membership is always rechecked; only the org row is cached.
	if cached := s.cachedOrg(ctx, orgID); cached != nil {
		return cached, nil
	}
	org, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Organization not found")
		}
		return nil, apperrors.Internal()
	}
	s.putCachedOrg(ctx, org)
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
	s.dropCachedOrg(ctx, orgID)
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
	s.dropCachedOrg(ctx, orgID)
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
	m, err := s.orgs.AddMember(ctx, orgID, targetID, role, models.ChangeSet{
		Events: []*models.OutboxEvent{
			models.MustOutboxEvent(
				models.EventMemberAdded, models.EntityOrganization, orgID, orgID,
				map[string]any{"user_id": targetID, "role": string(role)},
			),
		},
		Audit: models.NewAuditLog(orgID, actorID, models.AuditMemberAdded, models.EntityMember, targetID,
			nil, map[string]any{"role": string(role)}, nil),
	})
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
	m, err := s.orgs.UpdateMemberRole(ctx, orgID, targetID, role, models.ChangeSet{
		Events: []*models.OutboxEvent{
			models.MustOutboxEvent(
				models.EventMemberRole, models.EntityOrganization, orgID, orgID,
				map[string]any{"user_id": targetID, "old_role": string(current.Role), "new_role": string(role)},
			),
		},
		Audit: models.NewAuditLog(orgID, actorID, models.AuditMemberRole, models.EntityMember, targetID,
			map[string]any{"role": string(current.Role)}, map[string]any{"role": string(role)}, nil),
	})
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
	if err := s.orgs.RemoveMember(ctx, orgID, targetID, models.ChangeSet{
		Audit: models.NewAuditLog(orgID, actorID, models.AuditMemberRemoved, models.EntityMember, targetID,
			map[string]any{"role": string(current.Role)}, nil, nil),
	}); err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Member not found")
		}
		return apperrors.Internal()
	}
	return nil
}
