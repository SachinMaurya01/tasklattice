package repository

import (
	"context"

	"tasklattice/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrganizationRepository persists organizations and their memberships.
type OrganizationRepository interface {
	CreateWithOwner(ctx context.Context, org *models.Organization, ownerID string) error
	ListForUser(ctx context.Context, userID string) ([]models.Organization, error)
	GetByID(ctx context.Context, id string) (*models.Organization, error)
	Update(ctx context.Context, org *models.Organization) (*models.Organization, error)
	SoftDelete(ctx context.Context, id string) error
	GetMembership(ctx context.Context, orgID, userID string) (*models.OrganizationMember, error)
	ListMembers(ctx context.Context, orgID string) ([]models.OrganizationMember, error)
	AddMember(ctx context.Context, orgID, userID string, role models.Role) (*models.OrganizationMember, error)
	UpdateMemberRole(ctx context.Context, orgID, userID string, role models.Role) (*models.OrganizationMember, error)
	RemoveMember(ctx context.Context, orgID, userID string) error
	CountOwners(ctx context.Context, orgID string) (int, error)
	UserExists(ctx context.Context, userID string) (bool, error)
}

// PostgresOrganizationRepository is the PostgreSQL implementation.
type PostgresOrganizationRepository struct {
	pool *pgxpool.Pool
}

// NewOrganizationRepository wires an OrganizationRepository to a pool.
func NewOrganizationRepository(pool *pgxpool.Pool) *PostgresOrganizationRepository {
	return &PostgresOrganizationRepository{pool: pool}
}

// CreateWithOwner inserts the org and its owner membership atomically.
func (r *PostgresOrganizationRepository) CreateWithOwner(ctx context.Context, org *models.Organization, ownerID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}

	const insertOrg = `
		INSERT INTO organizations (name, description, created_by)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`
	if err = tx.QueryRow(ctx, insertOrg, org.Name, org.Description, ownerID).Scan(
		&org.ID, &org.CreatedAt, &org.UpdatedAt,
	); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	org.CreatedBy = &ownerID

	const insertMember = `
		INSERT INTO organization_members (organization_id, user_id, role)
		VALUES ($1, $2, 'OWNER')
	`
	if _, err = tx.Exec(ctx, insertMember, org.ID, ownerID); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}

// ListForUser returns non-deleted orgs where the user is a member.
func (r *PostgresOrganizationRepository) ListForUser(ctx context.Context, userID string) ([]models.Organization, error) {
	const query = `
		SELECT o.id, o.name, o.description, o.created_by, o.created_at, o.updated_at
		FROM organizations o
		JOIN organization_members m ON m.organization_id = o.id
		WHERE m.user_id = $1 AND o.deleted_at IS NULL
		ORDER BY o.created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orgs := []models.Organization{}
	for rows.Next() {
		var o models.Organization
		if err = rows.Scan(&o.ID, &o.Name, &o.Description, &o.CreatedBy, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

// GetByID fetches one non-deleted organization.
func (r *PostgresOrganizationRepository) GetByID(ctx context.Context, id string) (*models.Organization, error) {
	const query = `
		SELECT id, name, description, created_by, created_at, updated_at
		FROM organizations
		WHERE id = $1 AND deleted_at IS NULL
	`
	var o models.Organization
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&o.ID, &o.Name, &o.Description, &o.CreatedBy, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// Update renames or re-describes a non-deleted organization.
func (r *PostgresOrganizationRepository) Update(ctx context.Context, org *models.Organization) (*models.Organization, error) {
	const query = `
		UPDATE organizations
		SET name = $1, description = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING id, name, description, created_by, created_at, updated_at
	`
	var out models.Organization
	err := r.pool.QueryRow(ctx, query, org.Name, org.Description, org.ID).Scan(
		&out.ID, &out.Name, &out.Description, &out.CreatedBy, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SoftDelete marks an organization deleted.
func (r *PostgresOrganizationRepository) SoftDelete(ctx context.Context, id string) error {
	const query = `
		UPDATE organizations
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

// GetMembership fetches a user's membership in an org.
func (r *PostgresOrganizationRepository) GetMembership(ctx context.Context, orgID, userID string) (*models.OrganizationMember, error) {
	const query = `
		SELECT organization_id, user_id, role, created_at, updated_at
		FROM organization_members
		WHERE organization_id = $1 AND user_id = $2
	`
	var m models.OrganizationMember
	err := r.pool.QueryRow(ctx, query, orgID, userID).Scan(
		&m.OrganizationID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMembers returns all memberships of an org.
func (r *PostgresOrganizationRepository) ListMembers(ctx context.Context, orgID string) ([]models.OrganizationMember, error) {
	const query = `
		SELECT organization_id, user_id, role, created_at, updated_at
		FROM organization_members
		WHERE organization_id = $1
		ORDER BY created_at
	`
	rows, err := r.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []models.OrganizationMember{}
	for rows.Next() {
		var m models.OrganizationMember
		if err = rows.Scan(&m.OrganizationID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// AddMember inserts a membership.
func (r *PostgresOrganizationRepository) AddMember(ctx context.Context, orgID, userID string, role models.Role) (*models.OrganizationMember, error) {
	const query = `
		INSERT INTO organization_members (organization_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING organization_id, user_id, role, created_at, updated_at
	`
	var m models.OrganizationMember
	err := r.pool.QueryRow(ctx, query, orgID, userID, string(role)).Scan(
		&m.OrganizationID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateMemberRole changes a member's role.
func (r *PostgresOrganizationRepository) UpdateMemberRole(ctx context.Context, orgID, userID string, role models.Role) (*models.OrganizationMember, error) {
	const query = `
		UPDATE organization_members
		SET role = $1, updated_at = CURRENT_TIMESTAMP
		WHERE organization_id = $2 AND user_id = $3
		RETURNING organization_id, user_id, role, created_at, updated_at
	`
	var m models.OrganizationMember
	err := r.pool.QueryRow(ctx, query, string(role), orgID, userID).Scan(
		&m.OrganizationID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// RemoveMember deletes a membership.
func (r *PostgresOrganizationRepository) RemoveMember(ctx context.Context, orgID, userID string) error {
	const query = `
		DELETE FROM organization_members
		WHERE organization_id = $1 AND user_id = $2
	`
	tag, err := r.pool.Exec(ctx, query, orgID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// CountOwners counts OWNER memberships in an org.
func (r *PostgresOrganizationRepository) CountOwners(ctx context.Context, orgID string) (int, error) {
	const query = `
		SELECT COUNT(*) FROM organization_members
		WHERE organization_id = $1 AND role = 'OWNER'
	`
	var n int
	if err := r.pool.QueryRow(ctx, query, orgID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// UserExists reports whether a user id exists.
func (r *PostgresOrganizationRepository) UserExists(ctx context.Context, userID string) (bool, error) {
	const query = `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`
	var exists bool
	if err := r.pool.QueryRow(ctx, query, userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
