package service

import (
	"context"
	"fmt"

	"github.com/mtlprog/sloptask/internal/domain"
	"github.com/mtlprog/sloptask/internal/repository"
)

const maxDependencyDepth = 100

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
		return fmt.Errorf("%w: task %s is in %s status, expected NEW", domain.ErrInvalidTransition, task.ID, task.Status)
	}

	// Must not have assignee
	if task.AssigneeID != nil {
		return fmt.Errorf("%w: task %s already assigned to %s", domain.ErrTaskAlreadyClaimed, task.ID, *task.AssigneeID)
	}

	// Must be public
	if task.Visibility != domain.TaskVisibilityPublic {
		return fmt.Errorf("%w: task %s is private, agent %s cannot claim", domain.ErrPermissionDenied, task.ID, agent.ID)
	}

	// Must be in same workspace
	if task.WorkspaceID != agent.WorkspaceID {
		return fmt.Errorf("%w: task %s in workspace %s, agent %s in workspace %s", domain.ErrPermissionDenied, task.ID, task.WorkspaceID, agent.ID, agent.WorkspaceID)
	}

	return nil
}

// CanEscalate validates if an agent can escalate a task.
func (v *Validator) CanEscalate(task *domain.Task, agent *domain.Agent) error {
	// Must be in IN_PROGRESS status
	if task.Status != domain.TaskStatusInProgress {
		return fmt.Errorf("%w: task %s is in %s status, expected IN_PROGRESS", domain.ErrInvalidTransition, task.ID, task.Status)
	}

	// Cannot escalate own task
	if task.AssigneeID != nil && *task.AssigneeID == agent.ID {
		return fmt.Errorf("%w: agent %s cannot escalate own task %s", domain.ErrPermissionDenied, agent.ID, task.ID)
	}

	// Must be in same workspace
	if task.WorkspaceID != agent.WorkspaceID {
		return fmt.Errorf("%w: task %s in workspace %s, agent %s in workspace %s", domain.ErrPermissionDenied, task.ID, task.WorkspaceID, agent.ID, agent.WorkspaceID)
	}

	return nil
}

// CanTakeover validates if an agent can takeover a STUCK task.
func (v *Validator) CanTakeover(task *domain.Task, agent *domain.Agent) error {
	// Must be in STUCK status
	if task.Status != domain.TaskStatusStuck {
		return fmt.Errorf("%w: task %s is in %s status, expected STUCK", domain.ErrInvalidTransition, task.ID, task.Status)
	}

	// Cannot takeover own task
	if task.AssigneeID != nil && *task.AssigneeID == agent.ID {
		return fmt.Errorf("%w: agent %s cannot takeover own task %s", domain.ErrPermissionDenied, agent.ID, task.ID)
	}

	// Must be in same workspace
	if task.WorkspaceID != agent.WorkspaceID {
		return fmt.Errorf("%w: task %s in workspace %s, agent %s in workspace %s", domain.ErrPermissionDenied, task.ID, task.WorkspaceID, agent.ID, agent.WorkspaceID)
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
		return fmt.Errorf("%w: task %s in workspace %s, agent %s in workspace %s", domain.ErrPermissionDenied, task.ID, task.WorkspaceID, agent.ID, agent.WorkspaceID)
	}

	currentStatus := task.Status

	// Check if transition is allowed based on state machine rules
	switch currentStatus {
	case domain.TaskStatusNew:
		switch newStatus {
		case domain.TaskStatusInProgress:
			// Must be assignee or claim operation (handled separately)
			if task.AssigneeID != nil && *task.AssigneeID != agent.ID {
				return fmt.Errorf("%w: agent %s cannot transition task %s, not assignee", domain.ErrPermissionDenied, agent.ID, task.ID)
			}
		case domain.TaskStatusCancelled:
			// Only creator can cancel
			if task.CreatorID != agent.ID {
				return fmt.Errorf("%w: agent %s is not creator of task %s", domain.ErrNotTaskCreator, agent.ID, task.ID)
			}
		default:
			return fmt.Errorf("%w: task %s cannot transition NEW -> %s", domain.ErrInvalidTransition, task.ID, newStatus)
		}

	case domain.TaskStatusInProgress:
		switch newStatus {
		case domain.TaskStatusDone:
			// Only assignee can complete
			if !task.IsOwnedBy(agent.ID) {
				return fmt.Errorf("%w: agent %s is not owner of task %s", domain.ErrNotTaskOwner, agent.ID, task.ID)
			}
		case domain.TaskStatusBlocked:
			// Assignee can block own task
			if task.IsOwnedBy(agent.ID) {
				return nil
			}
			// Others can escalate (handled by CanEscalate)
			return fmt.Errorf("%w: agent %s must use escalate operation for task %s", domain.ErrPermissionDenied, agent.ID, task.ID)
		case domain.TaskStatusNew:
			// Only assignee can release task
			if !task.IsOwnedBy(agent.ID) {
				return fmt.Errorf("%w: agent %s is not owner of task %s", domain.ErrNotTaskOwner, agent.ID, task.ID)
			}
		case domain.TaskStatusCancelled:
			// Creator or assignee can cancel
			if !task.IsCreatedBy(agent.ID) && !task.IsOwnedBy(agent.ID) {
				return fmt.Errorf("%w: agent %s is neither creator nor assignee of task %s", domain.ErrPermissionDenied, agent.ID, task.ID)
			}
		default:
			return fmt.Errorf("%w: task %s cannot transition IN_PROGRESS -> %s", domain.ErrInvalidTransition, task.ID, newStatus)
		}

	case domain.TaskStatusBlocked:
		switch newStatus {
		case domain.TaskStatusInProgress:
			// Only assignee can unblock
			if !task.IsOwnedBy(agent.ID) {
				return fmt.Errorf("%w: agent %s is not owner of task %s", domain.ErrNotTaskOwner, agent.ID, task.ID)
			}
		case domain.TaskStatusNew:
			// Creator or assignee can release
			if !task.IsCreatedBy(agent.ID) && !task.IsOwnedBy(agent.ID) {
				return fmt.Errorf("%w: agent %s is neither creator nor assignee of task %s", domain.ErrPermissionDenied, agent.ID, task.ID)
			}
		case domain.TaskStatusCancelled:
			// Only creator can cancel
			if !task.IsCreatedBy(agent.ID) {
				return fmt.Errorf("%w: agent %s is not creator of task %s", domain.ErrNotTaskCreator, agent.ID, task.ID)
			}
		default:
			return fmt.Errorf("%w: task %s cannot transition BLOCKED -> %s", domain.ErrInvalidTransition, task.ID, newStatus)
		}

	case domain.TaskStatusStuck:
		switch newStatus {
		case domain.TaskStatusInProgress:
			// Only owner can resume their STUCK task
			// Others must use TakeoverTask
			if !task.IsOwnedBy(agent.ID) {
				return fmt.Errorf("%w: use takeover operation for STUCK tasks", domain.ErrPermissionDenied)
			}
			return nil
		case domain.TaskStatusNew:
			// Creator or system can release
			if !task.IsCreatedBy(agent.ID) {
				return fmt.Errorf("%w: agent %s is not creator of task %s, only creator can release STUCK task", domain.ErrPermissionDenied, agent.ID, task.ID)
			}
		case domain.TaskStatusCancelled:
			// Only creator can cancel
			if !task.IsCreatedBy(agent.ID) {
				return fmt.Errorf("%w: agent %s is not creator of task %s", domain.ErrNotTaskCreator, agent.ID, task.ID)
			}
		default:
			return fmt.Errorf("%w: task %s cannot transition STUCK -> %s", domain.ErrInvalidTransition, task.ID, newStatus)
		}

	case domain.TaskStatusDone, domain.TaskStatusCancelled:
		// Terminal statuses - no transitions allowed
		return fmt.Errorf("%w: task %s cannot transition from terminal status %s", domain.ErrInvalidTransition, task.ID, currentStatus)

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
	return v.checkCyclicDependencyWithDepth(ctx, taskID, visited, recStack, 0)
}

// checkCyclicDependencyWithDepth performs DFS with depth tracking to prevent DoS.
func (v *Validator) checkCyclicDependencyWithDepth(
	ctx context.Context,
	taskID string,
	visited map[string]bool,
	recStack map[string]bool,
	depth int,
) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return fmt.Errorf("dependency check cancelled: %w", ctx.Err())
	default:
	}

	// Check depth limit to prevent DoS
	if depth > maxDependencyDepth {
		return fmt.Errorf("%w: dependency chain exceeds maximum depth of %d for task %s", domain.ErrCyclicDependency, maxDependencyDepth, taskID)
	}

	// Mark current task as visited and in recursion stack
	visited[taskID] = true
	recStack[taskID] = true

	// Get the task to check its blocked_by
	task, err := v.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task %s: %w", taskID, err)
	}

	// Check all tasks that this task blocks (reverse of blocked_by)
	// For simplicity, we check the blocked_by array
	for _, blockedTaskID := range task.BlockedBy {
		if !visited[blockedTaskID] {
			if err := v.checkCyclicDependencyWithDepth(ctx, blockedTaskID, visited, recStack, depth+1); err != nil {
				return err
			}
		} else if recStack[blockedTaskID] {
			// Found a cycle
			return fmt.Errorf("%w: task %s -> %s creates a cycle", domain.ErrCyclicDependency, taskID, blockedTaskID)
		}
	}

	// Remove from recursion stack
	recStack[taskID] = false
	return nil
}
