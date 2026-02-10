package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtlprog/sloptask/internal/domain"
)

// taskColumns is the shared list of columns for task queries.
var taskColumns = []string{
	"id", "workspace_id", "title", "description", "creator_id", "assignee_id",
	"status", "visibility", "priority", "blocked_by", "status_deadline_at",
	"created_at", "updated_at",
}

// TaskRepository handles database operations for tasks.
type TaskRepository struct {
	pool *pgxpool.Pool
}

// NewTaskRepository creates a new TaskRepository.
func NewTaskRepository(pool *pgxpool.Pool) *TaskRepository {
	return &TaskRepository{pool: pool}
}

// scanTask scans a single row into a Task struct.
func scanTask(row pgx.Row) (*domain.Task, error) {
	var task domain.Task
	err := row.Scan(
		&task.ID,
		&task.WorkspaceID,
		&task.Title,
		&task.Description,
		&task.CreatorID,
		&task.AssigneeID,
		&task.Status,
		&task.Visibility,
		&task.Priority,
		&task.BlockedBy,
		&task.StatusDeadlineAt,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTaskNotFound
		}
		return nil, fmt.Errorf("scan task: %w", err)
	}
	return &task, nil
}

// scanTasks scans multiple rows into a slice of Task structs.
func scanTasks(rows pgx.Rows) ([]*domain.Task, error) {
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return tasks, nil
}

// GetByID retrieves a task by ID.
func (r *TaskRepository) GetByID(ctx context.Context, taskID string) (*domain.Task, error) {
	query, args, err := psql.
		Select(taskColumns...).
		From("tasks").
		Where(sq.Eq{"id": taskID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build GetByID query for task: %w", err)
	}

	return scanTask(r.pool.QueryRow(ctx, query, args...))
}

// GetByIDForUpdate retrieves a task by ID with FOR UPDATE lock (within transaction).
func (r *TaskRepository) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, taskID string) (*domain.Task, error) {
	query, args, err := psql.
		Select(taskColumns...).
		From("tasks").
		Where(sq.Eq{"id": taskID}).
		Suffix("FOR UPDATE").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build GetByIDForUpdate query for task %s: %w", taskID, err)
	}

	return scanTask(tx.QueryRow(ctx, query, args...))
}

// UpdateStatus updates the task status with optimistic locking.
// Returns ErrTaskAlreadyClaimed if the task was modified (oldStatus doesn't match).
func (r *TaskRepository) UpdateStatus(
	ctx context.Context,
	tx pgx.Tx,
	taskID string,
	oldStatus domain.TaskStatus,
	newStatus domain.TaskStatus,
	assigneeID *string,
	statusDeadlineAt *time.Time,
) error {
	query, args, err := psql.
		Update("tasks").
		Set("status", newStatus).
		Set("assignee_id", assigneeID).
		Set("status_deadline_at", statusDeadlineAt).
		Set("updated_at", sq.Expr("NOW()")).
		Where(sq.Eq{
			"id":     taskID,
			"status": oldStatus,
		}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build UpdateStatus query for task %s: %w", taskID, err)
	}

	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update task status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrTaskAlreadyClaimed
	}

	return nil
}

// GetBlockedByTasks retrieves all tasks from the blocked_by array.
func (r *TaskRepository) GetBlockedByTasks(ctx context.Context, blockedBy []string) ([]*domain.Task, error) {
	if len(blockedBy) == 0 {
		return []*domain.Task{}, nil
	}

	query, args, err := psql.
		Select(taskColumns...).
		From("tasks").
		Where(sq.Eq{"id": blockedBy}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build GetBlockedByTasks query: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query blocked tasks: %w", err)
	}

	return scanTasks(rows)
}

// FindExpiredDeadlines finds all tasks with expired deadlines.
func (r *TaskRepository) FindExpiredDeadlines(ctx context.Context) ([]*domain.Task, error) {
	query, args, err := psql.
		Select(taskColumns...).
		From("tasks").
		Where("status_deadline_at < NOW()").
		Where(sq.Eq{"status": []domain.TaskStatus{
			domain.TaskStatusNew,
			domain.TaskStatusInProgress,
			domain.TaskStatusBlocked,
		}}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build FindExpiredDeadlines query: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query expired tasks: %w", err)
	}

	return scanTasks(rows)
}

// Create creates a new task in the database within a transaction.
// Returns the created task with ID, CreatedAt, and UpdatedAt populated.
func (r *TaskRepository) Create(ctx context.Context, tx pgx.Tx, task *domain.Task) (*domain.Task, error) {
	// Set defaults
	if task.Visibility == "" {
		task.Visibility = domain.TaskVisibilityPublic
	}
	if task.Priority == "" {
		task.Priority = domain.TaskPriorityNormal
	}
	if task.BlockedBy == nil {
		task.BlockedBy = []string{}
	}

	query, args, err := psql.
		Insert("tasks").
		Columns(
			"workspace_id", "title", "description", "creator_id", "assignee_id",
			"status", "visibility", "priority", "blocked_by", "status_deadline_at",
		).
		Values(
			task.WorkspaceID,
			task.Title,
			task.Description,
			task.CreatorID,
			task.AssigneeID,
			task.Status,
			task.Visibility,
			task.Priority,
			task.BlockedBy,
			task.StatusDeadlineAt,
		).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build Create query for task: %w", err)
	}

	err = tx.QueryRow(ctx, query, args...).Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	return task, nil
}
