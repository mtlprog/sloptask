package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
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

// getActiveAgent fetches an agent by ID and verifies it is active.
func (s *TaskService) getActiveAgent(ctx context.Context, agentID string) (*domain.Agent, error) {
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if !agent.IsActive {
		return nil, domain.ErrAgentInactive
	}
	return agent, nil
}

// createEventAndCommit persists a task event within the transaction, then commits.
func (s *TaskService) createEventAndCommit(ctx context.Context, tx pgx.Tx, event *domain.TaskEvent) error {
	if err := s.eventRepo.Create(ctx, tx, event); err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	task, err := s.taskRepo.GetByIDForUpdate(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}

	agent, err := s.getActiveAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if err := s.validator.CanClaim(task, agent); err != nil {
		return nil, err
	}

	if err := s.validator.CheckBlockedByResolved(ctx, task.BlockedBy); err != nil {
		return nil, err
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	newDeadline := CalculateDeadline(workspace, domain.TaskStatusInProgress)

	err = s.taskRepo.UpdateStatus(ctx, tx, taskID,
		domain.TaskStatusNew, domain.TaskStatusInProgress,
		&agentID, newDeadline,
	)
	if err != nil {
		return nil, err
	}

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

	if err := s.createEventAndCommit(ctx, tx, event); err != nil {
		return nil, err
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	task, err := s.taskRepo.GetByIDForUpdate(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}

	agent, err := s.getActiveAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if err := s.validator.CanEscalate(task, agent); err != nil {
		return nil, err
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	newDeadline := CalculateDeadline(workspace, domain.TaskStatusBlocked)

	err = s.taskRepo.UpdateStatus(ctx, tx, taskID,
		domain.TaskStatusInProgress, domain.TaskStatusBlocked,
		task.AssigneeID, newDeadline,
	)
	if err != nil {
		return nil, err
	}

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

	if err := s.createEventAndCommit(ctx, tx, event); err != nil {
		return nil, err
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	task, err := s.taskRepo.GetByIDForUpdate(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}

	agent, err := s.getActiveAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if err := s.validator.CanTakeover(task, agent); err != nil {
		return nil, err
	}

	if err := s.validator.CheckBlockedByResolved(ctx, task.BlockedBy); err != nil {
		return nil, err
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	newDeadline := CalculateDeadline(workspace, domain.TaskStatusInProgress)

	err = s.taskRepo.UpdateStatus(ctx, tx, taskID,
		domain.TaskStatusStuck, domain.TaskStatusInProgress,
		&agentID, newDeadline,
	)
	if err != nil {
		return nil, err
	}

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

	if err := s.createEventAndCommit(ctx, tx, event); err != nil {
		return nil, err
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	task, err := s.taskRepo.GetByIDForUpdate(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}

	oldStatus := task.Status

	agent, err := s.getActiveAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if err := s.validator.CanTransitionStatus(task, agent, newStatus); err != nil {
		return nil, err
	}

	// When transitioning to IN_PROGRESS, verify blockers are resolved and no cycles exist
	if newStatus == domain.TaskStatusInProgress {
		if err := s.validator.CheckBlockedByResolved(ctx, task.BlockedBy); err != nil {
			return nil, err
		}
		if err := s.validator.CheckCyclicDependency(ctx, taskID, make(map[string]bool), make(map[string]bool)); err != nil {
			return nil, err
		}
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	newDeadline := CalculateDeadline(workspace, newStatus)

	// Transitioning to NEW returns the task to the pool by clearing the assignee
	newAssignee := task.AssigneeID
	if ShouldClearAssignee(newStatus) {
		newAssignee = nil
	}

	err = s.taskRepo.UpdateStatus(ctx, tx, taskID,
		oldStatus, newStatus,
		newAssignee, newDeadline,
	)
	if err != nil {
		return nil, err
	}

	event := &domain.TaskEvent{
		TaskID:    taskID,
		ActorID:   &agentID,
		Type:      domain.EventTypeStatusChanged,
		OldStatus: &oldStatus,
		NewStatus: &newStatus,
		Comment:   comment,
	}

	if err := s.createEventAndCommit(ctx, tx, event); err != nil {
		return nil, err
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
// Returns the number of tasks successfully updated, and an error if any tasks failed.
func (s *TaskService) ProcessExpiredDeadlines(ctx context.Context) (int, error) {
	tasks, err := s.taskRepo.FindExpiredDeadlines(ctx)
	if err != nil {
		return 0, fmt.Errorf("find expired tasks: %w", err)
	}

	if len(tasks) == 0 {
		slog.Info("no expired deadlines found")
		return 0, nil
	}

	count := 0
	var errs []error // Accumulate errors
	for _, task := range tasks {
		if err := s.processExpiredTask(ctx, task); err != nil {
			slog.Error("failed to process expired task",
				"task_id", task.ID,
				"error", err,
			)
			errs = append(errs, fmt.Errorf("task %s: %w", task.ID, err))
			continue
		}
		count++
	}

	failedCount := len(tasks) - count
	slog.Info("processed expired deadlines",
		"total", len(tasks),
		"successful", count,
		"failed", failedCount,
	)

	// Return error if there were failures
	if len(errs) > 0 {
		return count, fmt.Errorf("processed %d/%d tasks, %d failures: %v",
			count, len(tasks), failedCount, errs)
	}

	return count, nil
}

// processExpiredTask transitions a single task to STUCK status.
func (s *TaskService) processExpiredTask(ctx context.Context, task *domain.Task) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	oldStatus := task.Status

	err = s.taskRepo.UpdateStatus(ctx, tx, task.ID,
		oldStatus, domain.TaskStatusStuck,
		task.AssigneeID, nil,
	)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	var durationMinutes int
	if task.StatusDeadlineAt != nil {
		durationMinutes = int(task.StatusDeadlineAt.Sub(task.UpdatedAt).Minutes())
	}

	newStatus := domain.TaskStatusStuck
	event := &domain.TaskEvent{
		TaskID:    task.ID,
		ActorID:   nil, // system event
		Type:      domain.EventTypeDeadlineExpired,
		OldStatus: &oldStatus,
		NewStatus: &newStatus,
		Comment:   fmt.Sprintf("Status deadline expired. Was in %s for %d minutes.", oldStatus, durationMinutes),
	}

	if err := s.createEventAndCommit(ctx, tx, event); err != nil {
		return err
	}

	slog.Info("task deadline expired",
		"task_id", task.ID,
		"old_status", oldStatus,
		"duration_minutes", durationMinutes,
	)

	return nil
}

// CreateTaskParams holds parameters for creating a new task.
type CreateTaskParams struct {
	WorkspaceID string
	CreatorID   string
	Title       string
	Description string
	AssigneeID  *string
	Visibility  domain.TaskVisibility
	Priority    domain.TaskPriority
	BlockedBy   []string
}

// CreateTask creates a new task with the given parameters.
// If AssigneeID is provided, the task is created in IN_PROGRESS status automatically.
// Otherwise, it's created in NEW status.
func (s *TaskService) CreateTask(ctx context.Context, params CreateTaskParams) (*domain.Task, error) {
	// Validate required fields
	if params.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if len(params.Title) < 5 || len(params.Title) > 200 {
		return nil, fmt.Errorf("title must be between 5 and 200 characters")
	}
	if params.Description == "" {
		return nil, fmt.Errorf("description is required")
	}

	// Validate creator exists and is active
	creator, err := s.getActiveAgent(ctx, params.CreatorID)
	if err != nil {
		return nil, fmt.Errorf("validate creator: %w", err)
	}

	// If assignee is provided, validate they exist, are active, and in same workspace
	if params.AssigneeID != nil {
		assignee, err := s.getActiveAgent(ctx, *params.AssigneeID)
		if err != nil {
			return nil, fmt.Errorf("validate assignee: %w", err)
		}
		if assignee.WorkspaceID != creator.WorkspaceID {
			return nil, fmt.Errorf("%w: assignee must be in same workspace", domain.ErrPermissionDenied)
		}
	}

	// Check cyclic dependencies if blockedBy is provided
	if len(params.BlockedBy) > 0 {
		if err := s.validator.CheckCyclicDependency(ctx, "", make(map[string]bool), make(map[string]bool)); err != nil {
			return nil, err
		}
	}

	// Determine initial status: IN_PROGRESS if assignee provided, otherwise NEW
	initialStatus := domain.TaskStatusNew
	if params.AssigneeID != nil {
		initialStatus = domain.TaskStatusInProgress
	}

	// Get workspace for deadline calculation
	workspace, err := s.workspaceRepo.GetByID(ctx, params.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	// Calculate deadline
	deadline := CalculateDeadline(workspace, initialStatus)

	// Begin transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	// Create task in repository
	task, err := s.taskRepo.Create(ctx, tx, &domain.Task{
		WorkspaceID:      params.WorkspaceID,
		Title:            params.Title,
		Description:      params.Description,
		CreatorID:        params.CreatorID,
		AssigneeID:       params.AssigneeID,
		Status:           initialStatus,
		Visibility:       params.Visibility,
		Priority:         params.Priority,
		BlockedBy:        params.BlockedBy,
		StatusDeadlineAt: deadline,
	})
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	// Create "created" event
	event := &domain.TaskEvent{
		TaskID:    task.ID,
		ActorID:   &params.CreatorID,
		Type:      domain.EventTypeCreated,
		OldStatus: nil,
		NewStatus: &initialStatus,
		Comment:   "Task created",
	}

	if err := s.createEventAndCommit(ctx, tx, event); err != nil {
		return nil, err
	}

	slog.Info("task created",
		"task_id", task.ID,
		"creator_id", params.CreatorID,
		"status", initialStatus,
		"assignee_id", params.AssigneeID,
	)

	return task, nil
}

// CommentTask adds a comment to a task without changing status.
func (s *TaskService) CommentTask(ctx context.Context, taskID, agentID, comment string) (*domain.TaskEvent, error) {
	if comment == "" {
		return nil, domain.ErrEmptyComment
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			slog.Error("failed to rollback transaction", "error", err, "task_id", taskID)
		}
	}()

	// Get task with lock
	task, err := s.taskRepo.GetByIDForUpdate(ctx, tx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	// Get agent
	agent, err := s.getActiveAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}

	// Check permissions (workspace and visibility)
	if task.WorkspaceID != agent.WorkspaceID {
		return nil, domain.ErrPermissionDenied
	}

	if task.Visibility == domain.TaskVisibilityPrivate {
		if task.CreatorID != agent.ID && (task.AssigneeID == nil || *task.AssigneeID != agent.ID) {
			return nil, domain.ErrPermissionDenied
		}
	}

	// Create comment event
	event := &domain.TaskEvent{
		TaskID:  taskID,
		ActorID: &agentID,
		Type:    domain.EventTypeCommented,
		Comment: comment,
	}

	if err := s.createEventAndCommit(ctx, tx, event); err != nil {
		return nil, err
	}

	slog.Info("comment added",
		"task_id", taskID,
		"agent_id", agentID,
		"event_id", event.ID,
	)

	return event, nil
}
