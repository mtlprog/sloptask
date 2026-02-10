package service_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtlprog/sloptask/internal/database"
	"github.com/mtlprog/sloptask/internal/domain"
	"github.com/mtlprog/sloptask/internal/repository"
	"github.com/mtlprog/sloptask/internal/service"
	"github.com/stretchr/testify/suite"
)

// TaskServiceTestSuite is the test suite for TaskService.
type TaskServiceTestSuite struct {
	suite.Suite
	pool          *pgxpool.Pool
	taskService   *service.TaskService
	taskRepo      *repository.TaskRepository
	eventRepo     *repository.TaskEventRepository
	agentRepo     *repository.AgentRepository
	workspaceRepo *repository.WorkspaceRepository

	// Test fixtures
	workspaceID string
	agent1ID    string
	agent2ID    string
}

// SetupSuite runs once before all tests.
func (s *TaskServiceTestSuite) SetupSuite() {
	// Get database URL from environment or use default
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://sloptask:sloptask@localhost:5432/sloptask?sslmode=disable"
	}

	ctx := context.Background()

	// Connect to database
	db, err := database.New(ctx, databaseURL)
	s.Require().NoError(err, "failed to connect to database")

	s.pool = db.Pool()

	// Run migrations
	err = database.RunMigrations(ctx, s.pool)
	s.Require().NoError(err, "failed to run migrations")

	// Create repositories
	s.taskRepo = repository.NewTaskRepository(s.pool)
	s.eventRepo = repository.NewTaskEventRepository(s.pool)
	s.agentRepo = repository.NewAgentRepository(s.pool)
	s.workspaceRepo = repository.NewWorkspaceRepository(s.pool)

	// Create service
	s.taskService = service.NewTaskService(
		s.pool,
		s.taskRepo,
		s.eventRepo,
		s.agentRepo,
		s.workspaceRepo,
	)
}

// SetupTest runs before each test.
func (s *TaskServiceTestSuite) SetupTest() {
	ctx := context.Background()

	// Clean up all data
	_, err := s.pool.Exec(ctx, "TRUNCATE workspaces, agents, tasks, task_events CASCADE")
	s.Require().NoError(err, "failed to truncate tables")

	// Create test workspace (same as seed data)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO workspaces (id, name, slug, status_deadlines)
		VALUES ('00000000-0000-0000-0000-000000000001', 'Test Workspace', 'test',
				'{"NEW": 120, "IN_PROGRESS": 1440, "BLOCKED": 2880}'::jsonb)
	`)
	s.Require().NoError(err, "failed to create workspace")
	s.workspaceID = "00000000-0000-0000-0000-000000000001"

	// Create test agents
	_, err = s.pool.Exec(ctx, `
		INSERT INTO agents (id, workspace_id, name, token, is_active)
		VALUES
			('00000000-0000-0000-0000-000000000011', $1, 'agent-1', 'token-1', true),
			('00000000-0000-0000-0000-000000000012', $1, 'agent-2', 'token-2', true)
	`, s.workspaceID)
	s.Require().NoError(err, "failed to create agents")
	s.agent1ID = "00000000-0000-0000-0000-000000000011"
	s.agent2ID = "00000000-0000-0000-0000-000000000012"
}

// TearDownSuite runs once after all tests.
func (s *TaskServiceTestSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// TestClaimTask_Success tests successful claim operation.
func (s *TaskServiceTestSuite) TestClaimTask_Success() {
	ctx := context.Background()

	// Create a NEW task without assignee
	taskID := s.createTask(ctx, domain.TaskStatusNew, nil, nil)

	// Claim the task
	event, err := s.taskService.ClaimTask(ctx, taskID, s.agent1ID, "Taking this task")
	s.Require().NoError(err)
	s.NotNil(event)
	s.Equal(domain.EventTypeClaimed, event.Type)

	// Verify task status changed
	task, err := s.taskRepo.GetByID(ctx, taskID)
	s.Require().NoError(err)
	s.Equal(domain.TaskStatusInProgress, task.Status)
	s.NotNil(task.AssigneeID)
	s.Equal(s.agent1ID, *task.AssigneeID)
	s.NotNil(task.StatusDeadlineAt)
}

// TestClaimTask_AlreadyClaimed tests claiming an already claimed task.
func (s *TaskServiceTestSuite) TestClaimTask_AlreadyClaimed() {
	ctx := context.Background()

	// Create a task with assignee
	taskID := s.createTask(ctx, domain.TaskStatusNew, &s.agent1ID, nil)

	// Try to claim - should fail
	_, err := s.taskService.ClaimTask(ctx, taskID, s.agent2ID, "Trying to claim")
	s.Error(err)
	s.ErrorIs(err, domain.ErrTaskAlreadyClaimed)
}

// TestClaimTask_WithUnresolvedBlockers tests claiming task with blockers.
func (s *TaskServiceTestSuite) TestClaimTask_WithUnresolvedBlockers() {
	ctx := context.Background()

	// Create blocker task in IN_PROGRESS
	blockerID := s.createTask(ctx, domain.TaskStatusInProgress, &s.agent1ID, nil)

	// Create task blocked by the blocker
	blockedBy := []string{blockerID}
	taskID := s.createTask(ctx, domain.TaskStatusNew, nil, blockedBy)

	// Try to claim - should fail
	_, err := s.taskService.ClaimTask(ctx, taskID, s.agent2ID, "Trying to claim")
	s.Error(err)
	s.ErrorIs(err, domain.ErrUnresolvedBlockers)
}

// TestEscalateTask_Success tests successful escalation.
func (s *TaskServiceTestSuite) TestEscalateTask_Success() {
	ctx := context.Background()

	// Create IN_PROGRESS task owned by agent1
	taskID := s.createTask(ctx, domain.TaskStatusInProgress, &s.agent1ID, nil)

	// Agent2 escalates the task
	event, err := s.taskService.EscalateTask(ctx, taskID, s.agent2ID, "Task is stuck")
	s.Require().NoError(err)
	s.NotNil(event)
	s.Equal(domain.EventTypeEscalated, event.Type)

	// Verify task status changed to BLOCKED
	task, err := s.taskRepo.GetByID(ctx, taskID)
	s.Require().NoError(err)
	s.Equal(domain.TaskStatusBlocked, task.Status)
	// Assignee should remain
	s.NotNil(task.AssigneeID)
	s.Equal(s.agent1ID, *task.AssigneeID)
}

// TestEscalateTask_CannotEscalateOwnTask tests that agent cannot escalate own task.
func (s *TaskServiceTestSuite) TestEscalateTask_CannotEscalateOwnTask() {
	ctx := context.Background()

	// Create IN_PROGRESS task owned by agent1
	taskID := s.createTask(ctx, domain.TaskStatusInProgress, &s.agent1ID, nil)

	// Agent1 tries to escalate own task - should fail
	_, err := s.taskService.EscalateTask(ctx, taskID, s.agent1ID, "Trying to escalate own task")
	s.Error(err)
	s.ErrorIs(err, domain.ErrPermissionDenied)
}

// TestTakeoverTask_Success tests successful takeover of STUCK task.
func (s *TaskServiceTestSuite) TestTakeoverTask_Success() {
	ctx := context.Background()

	// Create STUCK task
	taskID := s.createTask(ctx, domain.TaskStatusStuck, &s.agent1ID, nil)

	// Agent2 takes over
	event, err := s.taskService.TakeoverTask(ctx, taskID, s.agent2ID, "Taking over")
	s.Require().NoError(err)
	s.NotNil(event)
	s.Equal(domain.EventTypeTakenOver, event.Type)

	// Verify task status changed and assignee updated
	task, err := s.taskRepo.GetByID(ctx, taskID)
	s.Require().NoError(err)
	s.Equal(domain.TaskStatusInProgress, task.Status)
	s.NotNil(task.AssigneeID)
	s.Equal(s.agent2ID, *task.AssigneeID)
}

// TestTransitionStatus_NewToDone_ShouldFail tests invalid transition.
func (s *TaskServiceTestSuite) TestTransitionStatus_NewToDone_ShouldFail() {
	ctx := context.Background()

	// Create NEW task
	taskID := s.createTask(ctx, domain.TaskStatusNew, nil, nil)

	// Try to transition directly to DONE - should fail
	_, err := s.taskService.TransitionStatus(ctx, taskID, s.agent1ID, domain.TaskStatusDone, "Invalid transition")
	s.Error(err)
	s.ErrorIs(err, domain.ErrInvalidTransition)
}

// TestTransitionStatus_InProgressToDone_Success tests valid transition.
func (s *TaskServiceTestSuite) TestTransitionStatus_InProgressToDone_Success() {
	ctx := context.Background()

	// Create IN_PROGRESS task owned by agent1
	taskID := s.createTask(ctx, domain.TaskStatusInProgress, &s.agent1ID, nil)

	// Agent1 completes the task
	event, err := s.taskService.TransitionStatus(ctx, taskID, s.agent1ID, domain.TaskStatusDone, "Task completed")
	s.Require().NoError(err)
	s.NotNil(event)
	s.Equal(domain.EventTypeStatusChanged, event.Type)

	// Verify task status changed
	task, err := s.taskRepo.GetByID(ctx, taskID)
	s.Require().NoError(err)
	s.Equal(domain.TaskStatusDone, task.Status)
	s.Nil(task.StatusDeadlineAt) // Terminal status has no deadline
}

// TestProcessExpiredDeadlines tests deadline checker.
func (s *TaskServiceTestSuite) TestProcessExpiredDeadlines() {
	ctx := context.Background()

	// Create task with expired deadline (set in the past)
	taskID := s.createTaskWithExpiredDeadline(ctx)

	// Run deadline checker
	count, err := s.taskService.ProcessExpiredDeadlines(ctx)
	s.Require().NoError(err)
	s.Equal(1, count)

	// Verify task moved to STUCK
	task, err := s.taskRepo.GetByID(ctx, taskID)
	s.Require().NoError(err)
	s.Equal(domain.TaskStatusStuck, task.Status)
	s.Nil(task.StatusDeadlineAt)

	// Verify system event created
	events, err := s.eventRepo.GetByTaskID(ctx, taskID)
	s.Require().NoError(err)
	s.Len(events, 2) // created + deadline_expired
	s.Equal(domain.EventTypeDeadlineExpired, events[1].Type)
	s.Nil(events[1].ActorID) // System event
}

// Helper: createTask creates a test task.
func (s *TaskServiceTestSuite) createTask(
	ctx context.Context,
	status domain.TaskStatus,
	assigneeID *string,
	blockedBy []string,
) string {
	if blockedBy == nil {
		blockedBy = []string{}
	}

	var taskID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, assignee_id, status, blocked_by)
		VALUES ($1, 'Test Task', 'Test Description', $2, $3, $4, $5)
		RETURNING id
	`, s.workspaceID, s.agent1ID, assigneeID, status, blockedBy).Scan(&taskID)
	s.Require().NoError(err, "failed to create task")

	// Create "created" event
	_, err = s.pool.Exec(ctx, `
		INSERT INTO task_events (task_id, actor_id, type, new_status, comment)
		VALUES ($1, $2, 'created', $3, 'Task created')
	`, taskID, s.agent1ID, status)
	s.Require().NoError(err, "failed to create event")

	return taskID
}

// Helper: createTaskWithExpiredDeadline creates task with past deadline.
func (s *TaskServiceTestSuite) createTaskWithExpiredDeadline(ctx context.Context) string {
	expiredTime := time.Now().Add(-1 * time.Hour)

	var taskID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, status, status_deadline_at)
		VALUES ($1, 'Expired Task', 'Test Description', $2, 'IN_PROGRESS', $3)
		RETURNING id
	`, s.workspaceID, s.agent1ID, expiredTime).Scan(&taskID)
	s.Require().NoError(err, "failed to create task with expired deadline")

	// Create "created" event
	_, err = s.pool.Exec(ctx, `
		INSERT INTO task_events (task_id, actor_id, type, new_status, comment)
		VALUES ($1, $2, 'created', 'IN_PROGRESS', 'Task created')
	`, taskID, s.agent1ID)
	s.Require().NoError(err, "failed to create event")

	return taskID
}

// TestTaskServiceTestSuite runs the test suite.
func TestTaskServiceTestSuite(t *testing.T) {
	suite.Run(t, new(TaskServiceTestSuite))
}
