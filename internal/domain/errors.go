package domain

import "errors"

// Domain-specific errors for business logic validation.
var (
	// Task errors
	ErrTaskNotFound       = errors.New("task not found")
	ErrTaskAlreadyClaimed = errors.New("task already claimed")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrUnresolvedBlockers = errors.New("task has unresolved blockers")
	ErrCyclicDependency   = errors.New("cyclic dependency detected")

	// Permission errors
	ErrPermissionDenied = errors.New("permission denied")
	ErrNotTaskOwner     = errors.New("not task owner")
	ErrNotTaskCreator   = errors.New("not task creator")

	// Agent errors
	ErrAgentNotFound = errors.New("agent not found")
	ErrAgentInactive = errors.New("agent is inactive")
	ErrInvalidToken  = errors.New("invalid authentication token")

	// Workspace errors
	ErrWorkspaceNotFound = errors.New("workspace not found")

	// Validation errors
	ErrInvalidStatus     = errors.New("invalid task status")
	ErrInvalidVisibility = errors.New("invalid task visibility")
	ErrInvalidPriority   = errors.New("invalid task priority")
	ErrEmptyComment      = errors.New("comment is required")
)
