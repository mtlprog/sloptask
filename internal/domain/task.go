package domain

import "time"

// TaskStatus represents the status of a task in the state machine.
type TaskStatus string

const (
	TaskStatusNew        TaskStatus = "NEW"
	TaskStatusInProgress TaskStatus = "IN_PROGRESS"
	TaskStatusBlocked    TaskStatus = "BLOCKED"
	TaskStatusStuck      TaskStatus = "STUCK"
	TaskStatusDone       TaskStatus = "DONE"
	TaskStatusCancelled  TaskStatus = "CANCELLED"
)

// IsTerminal returns true if the status is terminal (no transitions allowed).
func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusDone || s == TaskStatusCancelled
}

// HasDeadline returns true if the status has an associated deadline.
func (s TaskStatus) HasDeadline() bool {
	return s == TaskStatusNew || s == TaskStatusInProgress || s == TaskStatusBlocked
}

// IsValid checks if the status is one of the allowed values.
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusNew, TaskStatusInProgress, TaskStatusBlocked,
		TaskStatusStuck, TaskStatusDone, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// TaskVisibility represents whether a task is public or private.
type TaskVisibility string

const (
	TaskVisibilityPublic  TaskVisibility = "public"
	TaskVisibilityPrivate TaskVisibility = "private"
)

// TaskPriority represents the priority level of a task.
type TaskPriority string

const (
	TaskPriorityLow      TaskPriority = "low"
	TaskPriorityNormal   TaskPriority = "normal"
	TaskPriorityHigh     TaskPriority = "high"
	TaskPriorityCritical TaskPriority = "critical"
)

// Task represents a unit of work for agents.
type Task struct {
	ID               string
	WorkspaceID      string
	Title            string
	Description      string
	CreatorID        string
	AssigneeID       *string
	Status           TaskStatus
	Visibility       TaskVisibility
	Priority         TaskPriority
	BlockedBy        []string
	StatusDeadlineAt *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// IsClaimable checks if the task can be claimed by an agent.
func (t *Task) IsClaimable() bool {
	return t.Status == TaskStatusNew &&
		t.AssigneeID == nil &&
		t.Visibility == TaskVisibilityPublic
}

// IsOwnedBy checks if the task is assigned to the given agent.
func (t *Task) IsOwnedBy(agentID string) bool {
	return t.AssigneeID != nil && *t.AssigneeID == agentID
}

// IsCreatedBy checks if the task was created by the given agent.
func (t *Task) IsCreatedBy(agentID string) bool {
	return t.CreatorID == agentID
}
