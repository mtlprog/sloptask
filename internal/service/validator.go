package service

import (
	"context"
	"fmt"

	"github.com/mtlprog/sloptask/internal/domain"
	"github.com/mtlprog/sloptask/internal/repository"
)

// Validator handles permission and state validation for task operations.
type Validator struct {
	taskRepo *repository.TaskRepository
}

// NewValidator creates a new Validator.
func NewValidator(taskRepo *repository.TaskRepository) *Validator {
	return &Validator{
		taskRepo: taskRepo,
	}
}

// CanClaim validates if an agent can claim a task.
func (v *Validator) CanClaim(task *domain.Task, agent *domain.Agent) error {
	// Must be in NEW status
	if task.Status != domain.TaskStatusNew {
		return fmt.Errorf("%w: task is in %s status", domain.ErrInvalidTransition, task.Status)
	}

	// Must not have assignee
	if task.AssigneeID != nil {
		return domain.ErrTaskAlreadyClaimed
	}

	// Must be public
	if task.Visibility != domain.TaskVisibilityPublic {
		return fmt.Errorf("%w: task is private", domain.ErrPermissionDenied)
	}

	// Must be in same workspace
	if task.WorkspaceID != agent.WorkspaceID {
		return domain.ErrPermissionDenied
	}

	return nil
}

// CanEscalate validates if an agent can escalate a task.
func (v *Validator) CanEscalate(task *domain.Task, agent *domain.Agent) error {
	// Must be in IN_PROGRESS status
	if task.Status != domain.TaskStatusInProgress {
		return fmt.Errorf("%w: task is in %s status", domain.ErrInvalidTransition, task.Status)
	}

	// Cannot escalate own task
	if task.AssigneeID != nil && *task.AssigneeID == agent.ID {
		return fmt.Errorf("%w: cannot escalate own task", domain.ErrPermissionDenied)
	}

	// Must be in same workspace
	if task.WorkspaceID != agent.WorkspaceID {
		return domain.ErrPermissionDenied
	}

	return nil
}

// CanTakeover validates if an agent can takeover a STUCK task.
func (v *Validator) CanTakeover(task *domain.Task, agent *domain.Agent) error {
	// Must be in STUCK status
	if task.Status != domain.TaskStatusStuck {
		return fmt.Errorf("%w: task is in %s status", domain.ErrInvalidTransition, task.Status)
	}

	// Cannot takeover own task
	if task.AssigneeID != nil && *task.AssigneeID == agent.ID {
		return fmt.Errorf("%w: cannot takeover own task", domain.ErrPermissionDenied)
	}

	// Must be in same workspace
	if task.WorkspaceID != agent.WorkspaceID {
		return domain.ErrPermissionDenied
	}

	return nil
}

// CanTransitionStatus validates if an agent can transition task to a new status.
func (v *Validator) CanTransitionStatus(
	task *domain.Task,
	agent *domain.Agent,
	newStatus domain.TaskStatus,
) error {
	// Must be in same workspace
	if task.WorkspaceID != agent.WorkspaceID {
		return domain.ErrPermissionDenied
	}

	currentStatus := task.Status

	// Check if transition is allowed based on state machine rules
	switch currentStatus {
	case domain.TaskStatusNew:
		switch newStatus {
		case domain.TaskStatusInProgress:
			// Must be assignee or claim operation (handled separately)
			if task.AssigneeID != nil && *task.AssigneeID != agent.ID {
				return domain.ErrPermissionDenied
			}
		case domain.TaskStatusCancelled:
			// Only creator can cancel
			if task.CreatorID != agent.ID {
				return domain.ErrNotTaskCreator
			}
		default:
			return fmt.Errorf("%w: NEW -> %s", domain.ErrInvalidTransition, newStatus)
		}

	case domain.TaskStatusInProgress:
		switch newStatus {
		case domain.TaskStatusDone:
			// Only assignee can complete
			if !task.IsOwnedBy(agent.ID) {
				return domain.ErrNotTaskOwner
			}
		case domain.TaskStatusBlocked:
			// Assignee can block own task
			if task.IsOwnedBy(agent.ID) {
				return nil
			}
			// Others can escalate (handled by CanEscalate)
			return fmt.Errorf("%w: use escalate operation", domain.ErrPermissionDenied)
		case domain.TaskStatusNew:
			// Only assignee can release task
			if !task.IsOwnedBy(agent.ID) {
				return domain.ErrNotTaskOwner
			}
		case domain.TaskStatusCancelled:
			// Creator or assignee can cancel
			if !task.IsCreatedBy(agent.ID) && !task.IsOwnedBy(agent.ID) {
				return fmt.Errorf("%w: only creator or assignee can cancel", domain.ErrPermissionDenied)
			}
		default:
			return fmt.Errorf("%w: IN_PROGRESS -> %s", domain.ErrInvalidTransition, newStatus)
		}

	case domain.TaskStatusBlocked:
		switch newStatus {
		case domain.TaskStatusInProgress:
			// Only assignee can unblock
			if !task.IsOwnedBy(agent.ID) {
				return domain.ErrNotTaskOwner
			}
		case domain.TaskStatusNew:
			// Creator or assignee can release
			if !task.IsCreatedBy(agent.ID) && !task.IsOwnedBy(agent.ID) {
				return fmt.Errorf("%w: only creator or assignee can release", domain.ErrPermissionDenied)
			}
		case domain.TaskStatusCancelled:
			// Only creator can cancel
			if !task.IsCreatedBy(agent.ID) {
				return domain.ErrNotTaskCreator
			}
		default:
			return fmt.Errorf("%w: BLOCKED -> %s", domain.ErrInvalidTransition, newStatus)
		}

	case domain.TaskStatusStuck:
		switch newStatus {
		case domain.TaskStatusInProgress:
			// Takeover operation (handled by CanTakeover)
			// Or assignee resuming own task
			return nil
		case domain.TaskStatusNew:
			// Creator or system can release
			if !task.IsCreatedBy(agent.ID) {
				return fmt.Errorf("%w: only creator can release STUCK task", domain.ErrPermissionDenied)
			}
		case domain.TaskStatusCancelled:
			// Only creator can cancel
			if !task.IsCreatedBy(agent.ID) {
				return domain.ErrNotTaskCreator
			}
		default:
			return fmt.Errorf("%w: STUCK -> %s", domain.ErrInvalidTransition, newStatus)
		}

	case domain.TaskStatusDone, domain.TaskStatusCancelled:
		// Terminal statuses - no transitions allowed
		return fmt.Errorf("%w: cannot transition from terminal status %s", domain.ErrInvalidTransition, currentStatus)

	default:
		return fmt.Errorf("%w: unknown status %s", domain.ErrInvalidStatus, currentStatus)
	}

	return nil
}

// CheckBlockedByResolved checks if all blocker tasks are in DONE status.
func (v *Validator) CheckBlockedByResolved(ctx context.Context, blockedBy []string) error {
	if len(blockedBy) == 0 {
		return nil
	}

	tasks, err := v.taskRepo.GetBlockedByTasks(ctx, blockedBy)
	if err != nil {
		return fmt.Errorf("get blocker tasks: %w", err)
	}

	// Check if we found all blocker tasks
	if len(tasks) != len(blockedBy) {
		return fmt.Errorf("%w: some blocker tasks not found", domain.ErrUnresolvedBlockers)
	}

	// Check if all are in DONE status
	for _, task := range tasks {
		if task.Status != domain.TaskStatusDone {
			return fmt.Errorf("%w: task %s is in %s status", domain.ErrUnresolvedBlockers, task.ID, task.Status)
		}
	}

	return nil
}

// CheckCyclicDependency performs DFS to detect cycles in task dependencies.
// This is called when transitioning to IN_PROGRESS status.
func (v *Validator) CheckCyclicDependency(
	ctx context.Context,
	taskID string,
	visited map[string]bool,
	recStack map[string]bool,
) error {
	// Mark current task as visited and in recursion stack
	visited[taskID] = true
	recStack[taskID] = true

	// Get the task to check its blocked_by
	task, err := v.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	// Check all tasks that this task blocks (reverse of blocked_by)
	// For simplicity, we check the blocked_by array
	for _, blockedTaskID := range task.BlockedBy {
		if !visited[blockedTaskID] {
			if err := v.CheckCyclicDependency(ctx, blockedTaskID, visited, recStack); err != nil {
				return err
			}
		} else if recStack[blockedTaskID] {
			// Found a cycle
			return fmt.Errorf("%w: task %s creates a cycle", domain.ErrCyclicDependency, blockedTaskID)
		}
	}

	// Remove from recursion stack
	recStack[taskID] = false
	return nil
}
