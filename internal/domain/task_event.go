package domain

import "time"

// EventType represents the type of task event.
type EventType string

const (
	EventTypeCreated         EventType = "created"
	EventTypeStatusChanged   EventType = "status_changed"
	EventTypeClaimed         EventType = "claimed"
	EventTypeEscalated       EventType = "escalated"
	EventTypeTakenOver       EventType = "taken_over"
	EventTypeCommented       EventType = "commented"
	EventTypeDeadlineExpired EventType = "deadline_expired"
)

// TaskEvent represents an audit log entry for a task action.
type TaskEvent struct {
	ID        string
	TaskID    string
	ActorID   *string // nil for system events
	Type      EventType
	OldStatus *TaskStatus
	NewStatus *TaskStatus
	Comment   string
	CreatedAt time.Time
}

// IsSystemEvent returns true if the event was created by the system.
func (e *TaskEvent) IsSystemEvent() bool {
	return e.ActorID == nil
}
