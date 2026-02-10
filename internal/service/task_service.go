package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtlprog/sloptask/internal/domain"
	"github.com/mtlprog/sloptask/internal/repository"
)

// TaskService coordinates task operations and state transitions.
type TaskService struct {
	pool          *pgxpool.Pool
	taskRepo      *repository.TaskRepository
	eventRepo     *repository.TaskEventRepository
	agentRepo     *repository.AgentRepository
	workspaceRepo *repository.WorkspaceRepository
	validator     *Validator
}

// NewTaskService creates a new TaskService.
func NewTaskService(
	pool *pgxpool.Pool,
	taskRepo *repository.TaskRepository,
	eventRepo *repository.TaskEventRepository,
	agentRepo *repository.AgentRepository,
	workspaceRepo *repository.WorkspaceRepository,
) *TaskService {
	return &TaskService{
		pool:          pool,
		taskRepo:      taskRepo,
		eventRepo:     eventRepo,
		agentRepo:     agentRepo,
		workspaceRepo: workspaceRepo,
		validator:     NewValidator(taskRepo),
	}
}

// ClaimTask implements the claim operation: agent takes a free NEW task.
func (s *TaskService) ClaimTask(
	ctx context.Context,
	taskID string,
	agentID string,
	comment string,
) (*domain.TaskEvent, error) {
	if comment == "" {
		return nil, domain.ErrEmptyComment
	}

	// Begin transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get task (without FOR UPDATE - optimistic locking)
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// Get agent
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if !agent.IsActive {
		return nil, domain.ErrAgentInactive
	}

	// Validate claim
	if err := s.validator.CanClaim(task, agent); err != nil {
		return nil, err
	}

	// Check blocked_by resolved
	if err := s.validator.CheckBlockedByResolved(ctx, task.BlockedBy); err != nil {
		return nil, err
	}

	// Get workspace for deadline calculation
	workspace, err := s.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	// Calculate new deadline
	newDeadline := CalculateDeadline(workspace, domain.TaskStatusInProgress)

	// Update task status with optimistic locking
	err = s.taskRepo.UpdateStatus(
		ctx,
		tx,
		taskID,
		domain.TaskStatusNew, // oldStatus
		domain.TaskStatusInProgress,
		&agentID, // assignee
		newDeadline,
	)
	if err != nil {
		return nil, err
	}

	// Create task event
	oldStatus := domain.TaskStatusNew
	newStatus := domain.TaskStatusInProgress
	event := &domain.TaskEvent{
		TaskID:    taskID,
		ActorID:   &agentID,
		Type:      domain.EventTypeClaimed,
		OldStatus: &oldStatus,
		NewStatus: &newStatus,
		Comment:   comment,
	}

	if err := s.eventRepo.Create(ctx, tx, event); err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	slog.Info("task claimed",
		"task_id", taskID,
		"agent_id", agentID,
		"event_id", event.ID,
	)

	return event, nil
}

// EscalateTask implements the escalate operation: agent blocks someone else's task.
func (s *TaskService) EscalateTask(
	ctx context.Context,
	taskID string,
	agentID string,
	comment string,
) (*domain.TaskEvent, error) {
	if comment == "" {
		return nil, domain.ErrEmptyComment
	}

	// Begin transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get task
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// Get agent
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if !agent.IsActive {
		return nil, domain.ErrAgentInactive
	}

	// Validate escalation
	if err := s.validator.CanEscalate(task, agent); err != nil {
		return nil, err
	}

	// Get workspace for deadline calculation
	workspace, err := s.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	// Calculate new deadline
	newDeadline := CalculateDeadline(workspace, domain.TaskStatusBlocked)

	// Update task status
	err = s.taskRepo.UpdateStatus(
		ctx,
		tx,
		taskID,
		domain.TaskStatusInProgress, // oldStatus
		domain.TaskStatusBlocked,
		task.AssigneeID, // keep assignee
		newDeadline,
	)
	if err != nil {
		return nil, err
	}

	// Create task event
	oldStatus := domain.TaskStatusInProgress
	newStatus := domain.TaskStatusBlocked
	event := &domain.TaskEvent{
		TaskID:    taskID,
		ActorID:   &agentID,
		Type:      domain.EventTypeEscalated,
		OldStatus: &oldStatus,
		NewStatus: &newStatus,
		Comment:   comment,
	}

	if err := s.eventRepo.Create(ctx, tx, event); err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	slog.Info("task escalated",
		"task_id", taskID,
		"agent_id", agentID,
		"event_id", event.ID,
	)

	return event, nil
}

// TakeoverTask implements the takeover operation: agent takes a STUCK task.
func (s *TaskService) TakeoverTask(
	ctx context.Context,
	taskID string,
	agentID string,
	comment string,
) (*domain.TaskEvent, error) {
	if comment == "" {
		return nil, domain.ErrEmptyComment
	}

	// Begin transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get task
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// Get agent
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if !agent.IsActive {
		return nil, domain.ErrAgentInactive
	}

	// Validate takeover
	if err := s.validator.CanTakeover(task, agent); err != nil {
		return nil, err
	}

	// Check blocked_by resolved
	if err := s.validator.CheckBlockedByResolved(ctx, task.BlockedBy); err != nil {
		return nil, err
	}

	// Get workspace for deadline calculation
	workspace, err := s.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	// Calculate new deadline
	newDeadline := CalculateDeadline(workspace, domain.TaskStatusInProgress)

	// Update task status with new assignee
	err = s.taskRepo.UpdateStatus(
		ctx,
		tx,
		taskID,
		domain.TaskStatusStuck, // oldStatus
		domain.TaskStatusInProgress,
		&agentID, // new assignee
		newDeadline,
	)
	if err != nil {
		return nil, err
	}

	// Create task event
	oldStatus := domain.TaskStatusStuck
	newStatus := domain.TaskStatusInProgress
	event := &domain.TaskEvent{
		TaskID:    taskID,
		ActorID:   &agentID,
		Type:      domain.EventTypeTakenOver,
		OldStatus: &oldStatus,
		NewStatus: &newStatus,
		Comment:   comment,
	}

	if err := s.eventRepo.Create(ctx, tx, event); err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	slog.Info("task taken over",
		"task_id", taskID,
		"agent_id", agentID,
		"event_id", event.ID,
	)

	return event, nil
}

// TransitionStatus implements regular status transitions.
func (s *TaskService) TransitionStatus(
	ctx context.Context,
	taskID string,
	agentID string,
	newStatus domain.TaskStatus,
	comment string,
) (*domain.TaskEvent, error) {
	if comment == "" {
		return nil, domain.ErrEmptyComment
	}

	if !newStatus.IsValid() {
		return nil, domain.ErrInvalidStatus
	}

	// Begin transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get task
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	oldStatus := task.Status

	// Get agent
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if !agent.IsActive {
		return nil, domain.ErrAgentInactive
	}

	// Validate transition
	if err := s.validator.CanTransitionStatus(task, agent, newStatus); err != nil {
		return nil, err
	}

	// If transitioning to IN_PROGRESS, check blocked_by and cycles
	if newStatus == domain.TaskStatusInProgress {
		if err := s.validator.CheckBlockedByResolved(ctx, task.BlockedBy); err != nil {
			return nil, err
		}

		// Check cyclic dependencies
		visited := make(map[string]bool)
		recStack := make(map[string]bool)
		if err := s.validator.CheckCyclicDependency(ctx, taskID, visited, recStack); err != nil {
			return nil, err
		}
	}

	// Get workspace for deadline calculation
	workspace, err := s.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	// Calculate new deadline
	newDeadline := CalculateDeadline(workspace, newStatus)

	// Determine new assignee
	var newAssignee *string
	if ShouldClearAssignee(newStatus) {
		newAssignee = nil
	} else {
		newAssignee = task.AssigneeID
	}

	// Update task status
	err = s.taskRepo.UpdateStatus(
		ctx,
		tx,
		taskID,
		oldStatus,
		newStatus,
		newAssignee,
		newDeadline,
	)
	if err != nil {
		return nil, err
	}

	// Create task event
	event := &domain.TaskEvent{
		TaskID:    taskID,
		ActorID:   &agentID,
		Type:      domain.EventTypeStatusChanged,
		OldStatus: &oldStatus,
		NewStatus: &newStatus,
		Comment:   comment,
	}

	if err := s.eventRepo.Create(ctx, tx, event); err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	slog.Info("task status changed",
		"task_id", taskID,
		"agent_id", agentID,
		"old_status", oldStatus,
		"new_status", newStatus,
		"event_id", event.ID,
	)

	return event, nil
}

// ProcessExpiredDeadlines finds and processes all tasks with expired deadlines.
// Returns the number of tasks updated.
func (s *TaskService) ProcessExpiredDeadlines(ctx context.Context) (int, error) {
	// Find all expired tasks
	tasks, err := s.taskRepo.FindExpiredDeadlines(ctx)
	if err != nil {
		return 0, fmt.Errorf("find expired tasks: %w", err)
	}

	if len(tasks) == 0 {
		slog.Info("no expired deadlines found")
		return 0, nil
	}

	count := 0
	for _, task := range tasks {
		if err := s.processExpiredTask(ctx, task); err != nil {
			slog.Error("failed to process expired task",
				"task_id", task.ID,
				"error", err,
			)
			// Continue processing other tasks
			continue
		}
		count++
	}

	slog.Info("processed expired deadlines",
		"total", len(tasks),
		"successful", count,
		"failed", len(tasks)-count,
	)

	return count, nil
}

// processExpiredTask transitions a single task to STUCK status.
func (s *TaskService) processExpiredTask(ctx context.Context, task *domain.Task) error {
	// Begin transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	oldStatus := task.Status

	// Update to STUCK (no deadline, keep assignee)
	err = s.taskRepo.UpdateStatus(
		ctx,
		tx,
		task.ID,
		oldStatus,
		domain.TaskStatusStuck,
		task.AssigneeID, // keep assignee
		nil,             // no deadline for STUCK
	)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	// Calculate duration
	var durationMinutes int
	if task.StatusDeadlineAt != nil {
		durationMinutes = int(task.StatusDeadlineAt.Sub(task.UpdatedAt).Minutes())
	}

	// Create system event
	newStatus := domain.TaskStatusStuck
	comment := fmt.Sprintf("Status deadline expired. Was in %s for %d minutes.", oldStatus, durationMinutes)
	event := &domain.TaskEvent{
		TaskID:    task.ID,
		ActorID:   nil, // system event
		Type:      domain.EventTypeDeadlineExpired,
		OldStatus: &oldStatus,
		NewStatus: &newStatus,
		Comment:   comment,
	}

	if err := s.eventRepo.Create(ctx, tx, event); err != nil {
		return fmt.Errorf("create event: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	slog.Info("task deadline expired",
		"task_id", task.ID,
		"old_status", oldStatus,
		"duration_minutes", durationMinutes,
	)

	return nil
}
