package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/mtlprog/sloptask/internal/domain"
)

// TaskListFilters holds all supported filters for task listing.
type TaskListFilters struct {
	WorkspaceID            string   // Required: filter by workspace
	Statuses               []string // Optional: filter by status
	AssigneeID             *string  // Optional: filter by assignee
	Unassigned             bool     // Optional: show only unassigned
	Visibility             *string  // Optional: filter by visibility
	Priorities             []string // Optional: filter by priority
	Overdue                bool     // Optional: show only overdue
	HasUnresolvedBlockers  bool     // Optional: show only with unresolved blockers
	Sort                   []string // Optional: sort fields (with - prefix for DESC)
	Limit                  int      // Required: page size
	Offset                 int      // Required: page offset
}

// TaskListResult holds a task with computed fields.
type TaskListResult struct {
	Task                  *domain.Task
	HasUnresolvedBlockers bool
	IsOverdue             bool
}

// List retrieves tasks with filters and pagination.
func (r *TaskRepository) List(ctx context.Context, filters TaskListFilters) ([]TaskListResult, int, error) {
	// Build base query
	qb := psql.Select(taskColumns...).From("tasks").
		Where(sq.Eq{"workspace_id": filters.WorkspaceID})

	// Apply status filter
	if len(filters.Statuses) > 0 {
		qb = qb.Where(sq.Eq{"status": filters.Statuses})
	}

	// Apply assignee filter
	if filters.Unassigned {
		qb = qb.Where(sq.Eq{"assignee_id": nil})
	} else if filters.AssigneeID != nil {
		qb = qb.Where(sq.Eq{"assignee_id": *filters.AssigneeID})
	}

	// Apply visibility filter
	if filters.Visibility != nil {
		qb = qb.Where(sq.Eq{"visibility": *filters.Visibility})
	}

	// Apply priority filter
	if len(filters.Priorities) > 0 {
		qb = qb.Where(sq.Eq{"priority": filters.Priorities})
	}

	// Apply overdue filter
	if filters.Overdue {
		qb = qb.Where("status_deadline_at < NOW()")
	}

	// Apply sorting (default: -priority,created_at)
	if len(filters.Sort) == 0 {
		qb = qb.OrderBy("CASE priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'normal' THEN 3 WHEN 'low' THEN 4 END ASC")
		qb = qb.OrderBy("created_at ASC")
	} else {
		for _, sort := range filters.Sort {
			if strings.HasPrefix(sort, "-") {
				field := sort[1:]
				// Special handling for priority DESC
				if field == "priority" {
					qb = qb.OrderBy("CASE priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'normal' THEN 3 WHEN 'low' THEN 4 END DESC")
				} else {
					qb = qb.OrderBy(field + " DESC")
				}
			} else {
				// Special handling for priority ASC
				if sort == "priority" {
					qb = qb.OrderBy("CASE priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'normal' THEN 3 WHEN 'low' THEN 4 END ASC")
				} else {
					qb = qb.OrderBy(sort + " ASC")
				}
			}
		}
	}

	// Apply pagination
	qb = qb.Limit(uint64(filters.Limit)).Offset(uint64(filters.Offset))

	// Execute query
	query, args, err := qb.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build List query: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query tasks: %w", err)
	}

	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, 0, err
	}

	// Get total count (without pagination)
	countQb := psql.Select("COUNT(*)").From("tasks").
		Where(sq.Eq{"workspace_id": filters.WorkspaceID})

	// Apply same filters for count
	if len(filters.Statuses) > 0 {
		countQb = countQb.Where(sq.Eq{"status": filters.Statuses})
	}
	if filters.Unassigned {
		countQb = countQb.Where(sq.Eq{"assignee_id": nil})
	} else if filters.AssigneeID != nil {
		countQb = countQb.Where(sq.Eq{"assignee_id": *filters.AssigneeID})
	}
	if filters.Visibility != nil {
		countQb = countQb.Where(sq.Eq{"visibility": *filters.Visibility})
	}
	if len(filters.Priorities) > 0 {
		countQb = countQb.Where(sq.Eq{"priority": filters.Priorities})
	}
	if filters.Overdue {
		countQb = countQb.Where("status_deadline_at < NOW()")
	}

	countQuery, countArgs, err := countQb.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build count query: %w", err)
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	// Compute derived fields for each task
	results := make([]TaskListResult, len(tasks))
	for i, task := range tasks {
		results[i] = TaskListResult{
			Task:                  task,
			HasUnresolvedBlockers: false,
			IsOverdue:             task.StatusDeadlineAt != nil && task.StatusDeadlineAt.Before(time.Now()),
		}

		// Check unresolved blockers
		if len(task.BlockedBy) > 0 {
			blockers, err := r.GetBlockedByTasks(ctx, task.BlockedBy)
			if err == nil {
				for _, blocker := range blockers {
					if blocker.Status != domain.TaskStatusDone {
						results[i].HasUnresolvedBlockers = true
						break
					}
				}
			}
		}
	}

	// Apply has_unresolved_blockers filter if requested
	if filters.HasUnresolvedBlockers {
		filtered := make([]TaskListResult, 0)
		for _, result := range results {
			if result.HasUnresolvedBlockers {
				filtered = append(filtered, result)
			}
		}
		results = filtered
		total = len(results)
	}

	return results, total, nil
}
