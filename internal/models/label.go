package models

import "time"

// Label belongs to one project; tasks carry many via task_labels.
type Label struct {
	ID             string    `json:"id" db:"id"`
	OrganizationID string    `json:"organization_id" db:"organization_id"`
	ProjectID      string    `json:"project_id" db:"project_id"`
	Name           string    `json:"name" db:"name"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}
