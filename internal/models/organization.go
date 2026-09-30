package models

import "time"

// Role is an organization membership role.
type Role string

// Supported organization roles, weakest to strongest.
const (
	RoleViewer Role = "VIEWER"
	RoleMember Role = "MEMBER"
	RoleAdmin  Role = "ADMIN"
	RoleOwner  Role = "OWNER"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	switch r {
	case RoleViewer, RoleMember, RoleAdmin, RoleOwner:
		return true
	}
	return false
}

// Rank orders roles for comparison: viewer < member < admin < owner.
func (r Role) Rank() int {
	switch r {
	case RoleOwner:
		return 4
	case RoleAdmin:
		return 3
	case RoleMember:
		return 2
	case RoleViewer:
		return 1
	}
	return 0
}

// AtLeast reports whether r meets the required role.
func (r Role) AtLeast(required Role) bool { return r.Rank() >= required.Rank() }

// Organization is the multi-tenant boundary.
type Organization struct {
	ID          string     `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	CreatedBy   *string    `json:"created_by" db:"created_by"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// OrganizationMember links a user to an organization with a role.
type OrganizationMember struct {
	OrganizationID string    `json:"organization_id" db:"organization_id"`
	UserID         string    `json:"user_id" db:"user_id"`
	Role           Role      `json:"role" db:"role"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}
