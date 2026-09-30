package models

import "time"

// Task statuses.
const (
	TaskTodo       = "TODO"
	TaskInProgress = "IN_PROGRESS"
	TaskBlocked    = "BLOCKED"
	TaskDone       = "DONE"
	TaskCancelled  = "CANCELLED"
)

// Task priorities.
const (
	PriorityLow    = "LOW"
	PriorityMedium = "MEDIUM"
	PriorityHigh   = "HIGH"
	PriorityUrgent = "URGENT"
)

// ValidTaskStatus reports whether s is a known task status.
func ValidTaskStatus(s string) bool {
	switch s {
	case TaskTodo, TaskInProgress, TaskBlocked, TaskDone, TaskCancelled:
		return true
	}
	return false
}

// ValidTaskPriority reports whether p is a known task priority.
func ValidTaskPriority(p string) bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

// Task is the primary domain resource. Version guards concurrent updates.
type Task struct {
	ID             string     `json:"id" db:"id"`
	OrganizationID string     `json:"organization_id" db:"organization_id"`
	ProjectID      string     `json:"project_id" db:"project_id"`
	Title          string     `json:"title" db:"title"`
	Description    string     `json:"description" db:"description"`
	Status         string     `json:"status" db:"status"`
	Priority       string     `json:"priority" db:"priority"`
	CreatorID      *string    `json:"creator_id" db:"creator_id"`
	AssigneeID     *string    `json:"assignee_id" db:"assignee_id"`
	DueDate        *time.Time `json:"due_date" db:"due_date"`
	Version        int        `json:"version" db:"version"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	Labels         []Label    `json:"labels,omitempty"`
}

// Task list defaults and limits.
const (
	DefaultTaskLimit = 20
	MaxTaskLimit     = 100
)

// Task sort columns accepted from query params.
const (
	TaskSortCreated = "created_at"
	TaskSortUpdated = "updated_at"
	TaskSortDue     = "due_date"
)

// TaskQuery carries validated list filters, sort, and cursor pagination.
type TaskQuery struct {
	Status        string
	Priority      string
	AssigneeID    string
	CreatorID     string
	DueBefore     *time.Time
	DueAfter      *time.Time
	CreatedBefore *time.Time
	CreatedAfter  *time.Time
	LabelID       string
	Search        string
	Sort          string
	Order         string
	Limit         int
	Cursor        string
}

// Pagination carries the opaque cursor for the next page.
type Pagination struct {
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}

// TaskPage is one cursor-paginated slice of tasks.
type TaskPage struct {
	Data       []Task     `json:"data"`
	Pagination Pagination `json:"pagination"`
}
