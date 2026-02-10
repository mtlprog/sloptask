package service

import (
	"time"

	"github.com/mtlprog/sloptask/internal/domain"
)

// CalculateDeadline calculates the deadline for a task based on workspace configuration.
// Returns nil for statuses without deadlines (DONE, CANCELLED, STUCK).
func CalculateDeadline(workspace *domain.Workspace, status domain.TaskStatus) *time.Time {
	if !status.HasDeadline() {
		return nil
	}

	minutes := workspace.GetDeadlineMinutes(status)
	if minutes == 0 {
		// No deadline configured for this status
		return nil
	}

	deadline := time.Now().Add(time.Duration(minutes) * time.Minute)
	return &deadline
}

// ShouldClearAssignee returns true if the transition requires clearing the assignee.
// Transitions to NEW status always clear the assignee (returning task to pool).
func ShouldClearAssignee(newStatus domain.TaskStatus) bool {
	return newStatus == domain.TaskStatusNew
}
