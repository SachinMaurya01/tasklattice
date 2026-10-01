package services

import (
	"context"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/repository"
)

// Audit page bounds.
const (
	DefaultAuditLimit = 50
	MaxAuditLimit     = 100
)

// AuditService serves immutable audit records to organization admins.
type AuditService struct {
	audits repository.AuditRepository
	orgs   repository.OrganizationRepository
}

// NewAuditService wires the audit use cases.
func NewAuditService(audits repository.AuditRepository, orgs repository.OrganizationRepository) *AuditService {
	return &AuditService{audits: audits, orgs: orgs}
}

// List returns audit rows newest first. Admin only.
func (s *AuditService) List(ctx context.Context, userID, orgID string, limit, offset int) ([]models.AuditLog, error) {
	if limit <= 0 {
		limit = DefaultAuditLimit
	}
	if limit > MaxAuditLimit {
		limit = MaxAuditLimit
	}
	if offset < 0 {
		offset = 0
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	if _, err := requireRole(ctx, s.orgs, orgID, userID, models.RoleAdmin); err != nil {
		return nil, err
	}
	logs, err := s.audits.ListByOrg(ctx, orgID, limit, offset)
	if err != nil {
		return nil, apperrors.Internal()
	}
	return logs, nil
}
