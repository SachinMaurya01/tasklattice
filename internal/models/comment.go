package models

import "time"

// Comment belongs to one task.
type Comment struct {
	ID             string     `json:"id" db:"id"`
	OrganizationID string     `json:"organization_id" db:"organization_id"`
	TaskID         string     `json:"task_id" db:"task_id"`
	AuthorID       *string    `json:"author_id" db:"author_id"`
	Body           string     `json:"body" db:"body"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}
