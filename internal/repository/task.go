package repository

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtlprog/sloptask/internal/domain"
)

// TaskRepository handles database operations for tasks.
type TaskRepository struct {
	pool *pgxpool.Pool
	psql sq.StatementBuilderType
}

// NewTaskRepository creates a new TaskRepository.
func NewTaskRepository(pool *pgxpool.Pool) *TaskRepository {
	return &TaskRepository{
		pool: pool,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

// GetByID retrieves a task by ID.
func (r *TaskRepository) GetByID(ctx context.Context, taskID string) (*domain.Task, error) {
	query, args, err := r.psql.
		Select("id", "workspace_id", "title", "description", "creator_id", "assignee_id",
			"status", "visibility", "priority", "blocked_by", "status_deadline_at",
			"created_at", "updated_at").
		From("tasks").
		Where(sq.Eq{"id": taskID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var task domain.Task
	err = r.pool.QueryRow(ctx, query, args...).Scan(
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
		if err == pgx.ErrNoRows {
			return nil, domain.ErrTaskNotFound
		}
		return nil, fmt.Errorf("query task: %w", err)
	}

	return &task, nil
}

// GetByIDForUpdate retrieves a task by ID with FOR UPDATE lock (within transaction).
func (r *TaskRepository) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, taskID string) (*domain.Task, error) {
	query, args, err := r.psql.
		Select("id", "workspace_id", "title", "description", "creator_id", "assignee_id",
			"status", "visibility", "priority", "blocked_by", "status_deadline_at",
			"created_at", "updated_at").
		From("tasks").
		Where(sq.Eq{"id": taskID}).
		Suffix("FOR UPDATE").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var task domain.Task
	err = tx.QueryRow(ctx, query, args...).Scan(
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
		if err == pgx.ErrNoRows {
			return nil, domain.ErrTaskNotFound
		}
		return nil, fmt.Errorf("query task: %w", err)
	}

	return &task, nil
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
	query, args, err := r.psql.
		Update("tasks").
		Set("status", newStatus).
		Set("assignee_id", assigneeID).
		Set("status_deadline_at", statusDeadlineAt).
		Set("updated_at", sq.Expr("NOW()")).
		Where(sq.Eq{
			"id":     taskID,
			"status": oldStatus, // Optimistic locking
		}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
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

	query, args, err := r.psql.
		Select("id", "workspace_id", "title", "description", "creator_id", "assignee_id",
			"status", "visibility", "priority", "blocked_by", "status_deadline_at",
			"created_at", "updated_at").
		From("tasks").
		Where(sq.Eq{"id": blockedBy}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query blocked tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		var task domain.Task
		err := rows.Scan(
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
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, &task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return tasks, nil
}

// FindExpiredDeadlines finds all tasks with expired deadlines.
func (r *TaskRepository) FindExpiredDeadlines(ctx context.Context) ([]*domain.Task, error) {
	query, args, err := r.psql.
		Select("id", "workspace_id", "title", "description", "creator_id", "assignee_id",
			"status", "visibility", "priority", "blocked_by", "status_deadline_at",
			"created_at", "updated_at").
		From("tasks").
		Where("status_deadline_at < NOW()").
		Where(sq.Eq{"status": []domain.TaskStatus{
			domain.TaskStatusNew,
			domain.TaskStatusInProgress,
			domain.TaskStatusBlocked,
		}}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query expired tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		var task domain.Task
		err := rows.Scan(
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
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, &task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return tasks, nil
}
