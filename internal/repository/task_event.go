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

// TaskEventRepository handles database operations for task events.
type TaskEventRepository struct {
	pool *pgxpool.Pool
}

// NewTaskEventRepository creates a new TaskEventRepository.
func NewTaskEventRepository(pool *pgxpool.Pool) *TaskEventRepository {
	return &TaskEventRepository{pool: pool}
}

// Create creates a new task event.
func (r *TaskEventRepository) Create(
	ctx context.Context,
	tx pgx.Tx,
	event *domain.TaskEvent,
) error {
	query, args, err := psql.
		Insert("task_events").
		Columns("task_id", "actor_id", "type", "old_status", "new_status", "comment").
		Values(event.TaskID, event.ActorID, event.Type, event.OldStatus, event.NewStatus, event.Comment).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build Create query for task event: %w", err)
	}

	err = tx.QueryRow(ctx, query, args...).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return fmt.Errorf("create task event: %w", err)
	}

	return nil
}

// GetByTaskID retrieves all events for a task.
func (r *TaskEventRepository) GetByTaskID(ctx context.Context, taskID string) ([]*domain.TaskEvent, error) {
	query, args, err := psql.
		Select("id", "task_id", "actor_id", "type", "old_status", "new_status", "comment", "created_at").
		From("task_events").
		Where(sq.Eq{"task_id": taskID}).
		OrderBy("created_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build GetByTaskID query for task %s: %w", taskID, err)
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

// TaskEventWithActor extends TaskEvent with actor name.
type TaskEventWithActor struct {
	ID        string
	TaskID    string
	ActorID   *string
	ActorName *string // NULL for system events
	Type      domain.EventType
	OldStatus *domain.TaskStatus
	NewStatus *domain.TaskStatus
	Comment   string
	CreatedAt time.Time
}

// GetByTaskIDWithActors retrieves all events for a task with actor names.
func (r *TaskEventRepository) GetByTaskIDWithActors(ctx context.Context, taskID string) ([]TaskEventWithActor, error) {
	query := `
		SELECT
			te.id, te.task_id, te.actor_id, a.name as actor_name,
			te.type, te.old_status, te.new_status, te.comment, te.created_at
		FROM task_events te
		LEFT JOIN agents a ON te.actor_id = a.id
		WHERE te.task_id = $1
		ORDER BY te.created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("query task events with actors: %w", err)
	}
	defer rows.Close()

	var events []TaskEventWithActor
	for rows.Next() {
		var event TaskEventWithActor
		err := rows.Scan(
			&event.ID,
			&event.TaskID,
			&event.ActorID,
			&event.ActorName,
			&event.Type,
			&event.OldStatus,
			&event.NewStatus,
			&event.Comment,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task event with actor: %w", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return events, nil
}
