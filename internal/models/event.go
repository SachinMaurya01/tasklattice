package models

import (
	"encoding/json"
	"time"
)

// Domain event types published through the transactional outbox.
const (
	EventOrgCreated      = "organization.created"
	EventMemberAdded     = "member.added"
	EventMemberRole      = "member.role_changed"
	EventProjectCreated  = "project.created"
	EventTaskCreated     = "task.created"
	EventTaskUpdated     = "task.updated"
	EventTaskAssigned    = "task.assigned"
	EventTaskCompleted   = "task.completed"
	EventTaskDeleted     = "task.deleted"
	EventCommentCreated  = "comment.created"
)

// Audit actions recorded for important mutations.
const (
	AuditOrgCreated     = "organization.created"
	AuditMemberAdded    = "member.added"
	AuditMemberRole     = "member.role_changed"
	AuditMemberRemoved  = "member.removed"
	AuditProjectCreated = "project.created"
	AuditTaskCreated    = "task.created"
	AuditTaskUpdated    = "task.updated"
	AuditTaskAssigned   = "task.assigned"
	AuditTaskCompleted  = "task.completed"
	AuditTaskDeleted    = "task.deleted"
	AuditTaskRestored   = "task.restored"
	AuditCommentCreated = "comment.created"
)

// Entity types used by audit rows and outbox aggregates.
const (
	EntityOrganization = "organization"
	EntityMember       = "member"
	EntityProject      = "project"
	EntityTask         = "task"
	EntityComment      = "comment"
)

// OutboxEvent is one reliable async event. Payload is JSON text.
type OutboxEvent struct {
	ID             string    `json:"id" db:"id"`
	EventType      string    `json:"event_type" db:"event_type"`
	AggregateType  string    `json:"aggregate_type" db:"aggregate_type"`
	AggregateID    string    `json:"aggregate_id" db:"aggregate_id"`
	OrganizationID string    `json:"organization_id" db:"organization_id"`
	Payload        string    `json:"payload" db:"payload"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	Attempts       int       `json:"attempts" db:"attempts"`
}

// NewOutboxEvent builds an event; payload marshals to JSON text.
func NewOutboxEvent(eventType, aggregateType, aggregateID, orgID string, payload map[string]any) (*OutboxEvent, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &OutboxEvent{
		EventType:      eventType,
		AggregateType:  aggregateType,
		AggregateID:    aggregateID,
		OrganizationID: orgID,
		Payload:        string(raw),
	}, nil
}

// MustOutboxEvent is NewOutboxEvent for static payloads; nil on failure.
// Services fall back to omitting the event rather than failing the write.
func MustOutboxEvent(eventType, aggregateType, aggregateID, orgID string, payload map[string]any) *OutboxEvent {
	e, err := NewOutboxEvent(eventType, aggregateType, aggregateID, orgID, payload)
	if err != nil {
		return nil
	}
	return e
}

// AuditLog is one immutable audit record. Values hold JSON text.
type AuditLog struct {
	ID             string    `json:"id" db:"id"`
	OrganizationID string    `json:"organization_id" db:"organization_id"`
	ActorID        *string   `json:"actor_id" db:"actor_id"`
	Action         string    `json:"action" db:"action"`
	EntityType     string    `json:"entity_type" db:"entity_type"`
	EntityID       *string   `json:"entity_id" db:"entity_id"`
	OldValue       *string   `json:"old_value" db:"old_value"`
	NewValue       *string   `json:"new_value" db:"new_value"`
	Metadata       *string   `json:"metadata" db:"metadata"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// toJSON marshals v to JSON text, or nil when v is nil.
func toJSON(v any) *string {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	s := string(raw)
	return &s
}

// NewAuditLog builds an audit row; values marshal to JSON text.
func NewAuditLog(orgID, actorID, action, entityType, entityID string, oldValue, newValue, metadata any) *AuditLog {
	a := &AuditLog{OrganizationID: orgID, Action: action, EntityType: entityType}
	if actorID != "" {
		a.ActorID = &actorID
	}
	if entityID != "" {
		a.EntityID = &entityID
	}
	a.OldValue = toJSON(oldValue)
	a.NewValue = toJSON(newValue)
	a.Metadata = toJSON(metadata)
	return a
}

// ChangeSet bundles the side effects committed atomically with a mutation.
// Convention: an event with an empty AggregateID, or an audit with a nil
// EntityID, adopts the ID of the entity created by that same mutation.
type ChangeSet struct {
	Events []*OutboxEvent
	Audit  *AuditLog
}

// AppendEvent adds a non-nil event to the set.
func (c *ChangeSet) AppendEvent(e *OutboxEvent) {
	if e != nil {
		c.Events = append(c.Events, e)
	}
}
