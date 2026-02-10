package repository

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtlprog/sloptask/internal/domain"
)

// TaskEventRepository handles database operations for task events.
type TaskEventRepository struct {
	pool *pgxpool.Pool
	psql sq.StatementBuilderType
}

// NewTaskEventRepository creates a new TaskEventRepository.
func NewTaskEventRepository(pool *pgxpool.Pool) *TaskEventRepository {
	return &TaskEventRepository{
		pool: pool,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

// Create creates a new task event.
func (r *TaskEventRepository) Create(
	ctx context.Context,
	tx pgx.Tx,
	event *domain.TaskEvent,
) error {
	query, args, err := r.psql.
		Insert("task_events").
		Columns("task_id", "actor_id", "type", "old_status", "new_status", "comment").
		Values(event.TaskID, event.ActorID, event.Type, event.OldStatus, event.NewStatus, event.Comment).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	err = tx.QueryRow(ctx, query, args...).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return fmt.Errorf("create task event: %w", err)
	}

	return nil
}

// GetByTaskID retrieves all events for a task.
func (r *TaskEventRepository) GetByTaskID(ctx context.Context, taskID string) ([]*domain.TaskEvent, error) {
	query, args, err := r.psql.
		Select("id", "task_id", "actor_id", "type", "old_status", "new_status", "comment", "created_at").
		From("task_events").
		Where(sq.Eq{"task_id": taskID}).
		OrderBy("created_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query task events: %w", err)
	}
	defer rows.Close()

	var events []*domain.TaskEvent
	for rows.Next() {
		var event domain.TaskEvent
		err := rows.Scan(
			&event.ID,
			&event.TaskID,
			&event.ActorID,
			&event.Type,
			&event.OldStatus,
			&event.NewStatus,
			&event.Comment,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task event: %w", err)
		}
		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return events, nil
}
