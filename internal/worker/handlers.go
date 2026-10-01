package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// RegisterHandlers wires domain events to workers. Task lifecycle events
// fan out to notifications and the due-date reminder check.
func RegisterHandlers(c *Consumer) {
	c.On("organization.created", NotifyHandler)
	c.On("member.added", NotifyHandler)
	c.On("member.role_changed", NotifyHandler)
	c.On("project.created", NotifyHandler)
	c.On("task.created", Chain(NotifyHandler, ReminderHandler))
	c.On("task.updated", Chain(NotifyHandler, ReminderHandler))
	c.On("task.assigned", NotifyHandler)
	c.On("task.completed", NotifyHandler)
	c.On("task.deleted", NotifyHandler)
	c.On("comment.created", NotifyHandler)
}

// Chain runs handlers in order, stopping at the first error.
func Chain(handlers ...Handler) Handler {
	return func(ctx context.Context, env Envelope) error {
		for _, h := range handlers {
			if err := h(ctx, env); err != nil {
				return err
			}
		}
		return nil
	}
}

// NotifyHandler delivers notifications. They are logged; real channels
// (email/push) plug in here without touching dispatch or dedupe.
func NotifyHandler(_ context.Context, env Envelope) error {
	var payload map[string]any
	if len(env.Payload) > 0 {
		_ = json.Unmarshal(env.Payload, &payload)
	}
	log.Printf("notify type=%s org=%s aggregate=%s:%s payload=%v",
		env.EventType, env.OrganizationID, env.AggregateType, env.AggregateID, payload)
	return nil
}

// reminderWindow flags tasks due within the next day.
const reminderWindow = 24 * time.Hour

// ReminderHandler logs due-soon reminders for dated, unfinished tasks.
func ReminderHandler(_ context.Context, env Envelope) error {
	var payload struct {
		DueDate *time.Time `json:"due_date"`
		Status  string     `json:"status"`
		Title   string     `json:"title"`
	}
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return nil
		}
	}
	if payload.DueDate == nil {
		return nil
	}
	if payload.Status == "DONE" || payload.Status == "CANCELLED" {
		return nil
	}
	until := time.Until(*payload.DueDate)
	if until <= 0 || until > reminderWindow {
		return nil
	}
	log.Printf("reminder task=%s org=%s due=%s title=%q",
		env.AggregateID, env.OrganizationID, payload.DueDate.Format(time.RFC3339), payload.Title)
	return nil
}
