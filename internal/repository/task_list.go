package repository

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/mtlprog/sloptask/internal/domain"
)

// allowedSortFields defines the whitelist of fields that can be used for sorting.
// This prevents SQL injection attacks via user-controlled column names.
var allowedSortFields = map[string]bool{
	"id":          true,
	"status":      true,
	"priority":    true,
	"created_at":  true,
	"updated_at":  true,
	"title":       true,
}

// TaskListFilters holds all supported filters for task listing.
type TaskListFilters struct {
	WorkspaceID            string   // Required: filter by workspace
	AgentID                string   // Required: for filtering private tasks
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

// batchLoadBlockers fetches all blocker tasks in a single query.
// Returns a map of task_id -> list of blocker tasks.
func (r *TaskRepository) batchLoadBlockers(ctx context.Context, tasks []*domain.Task) (map[string][]*domain.Task, error) {
	// Collect all unique blocker IDs
	allBlockerIDs := make([]string, 0)
	seen := make(map[string]bool)

	for _, task := range tasks {
		for _, blockerID := range task.BlockedBy {
			if !seen[blockerID] {
				allBlockerIDs = append(allBlockerIDs, blockerID)
				seen[blockerID] = true
			}
		}
	}

	if len(allBlockerIDs) == 0 {
		return make(map[string][]*domain.Task), nil
	}

	// Single query to fetch ALL blockers at once
	blockers, err := r.GetBlockedByTasks(ctx, allBlockerIDs)
	if err != nil {
		return nil, err
	}

	// Build map: blocker_id -> blocker task
	blockerMap := make(map[string]*domain.Task)
	for _, blocker := range blockers {
		blockerMap[blocker.ID] = blocker
	}

	// Build result: task_id -> list of blocker tasks
	result := make(map[string][]*domain.Task)
	for _, task := range tasks {
		taskBlockers := make([]*domain.Task, 0)
		for _, blockerID := range task.BlockedBy {
			if blocker, ok := blockerMap[blockerID]; ok {
				taskBlockers = append(taskBlockers, blocker)
			}
		}
		result[task.ID] = taskBlockers
	}

	return result, nil
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

	// Apply visibility filter with agent context
	// SECURITY: Prevent private task leaks to unauthorized agents
	if filters.Visibility != nil {
		qb = qb.Where(sq.Eq{"visibility": *filters.Visibility})
	} else {
		// When no visibility specified, filter out private tasks
		// that the agent is not creator or assignee of
		qb = qb.Where(sq.Or{
			sq.Eq{"visibility": "public"},
			sq.And{
				sq.Eq{"visibility": "private"},
				sq.Or{
					sq.Eq{"creator_id": filters.AgentID},
					sq.Eq{"assignee_id": filters.AgentID},
				},
			},
		})
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
			descending := false
			field := sort

			if strings.HasPrefix(sort, "-") {
				descending = true
				field = sort[1:]
			}

			// SECURITY: Validate field against allowlist to prevent SQL injection
			if !allowedSortFields[field] {
				continue // Skip invalid fields silently
			}

			if field == "priority" {
				// Special CASE handling for priority sorting
				if descending {
					qb = qb.OrderBy("CASE priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'normal' THEN 3 WHEN 'low' THEN 4 END DESC")
				} else {
					qb = qb.OrderBy("CASE priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'normal' THEN 3 WHEN 'low' THEN 4 END ASC")
				}
			} else {
				if descending {
					qb = qb.OrderBy(field + " DESC")
				} else {
					qb = qb.OrderBy(field + " ASC")
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
	// Apply visibility filter with agent context (same as main query)
	if filters.Visibility != nil {
		countQb = countQb.Where(sq.Eq{"visibility": *filters.Visibility})
	} else {
		countQb = countQb.Where(sq.Or{
			sq.Eq{"visibility": "public"},
			sq.And{
				sq.Eq{"visibility": "private"},
				sq.Or{
					sq.Eq{"creator_id": filters.AgentID},
					sq.Eq{"assignee_id": filters.AgentID},
				},
			},
		})
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

	// Batch load all blockers at once (fixes N+1 query problem)
	blockersByTask, err := r.batchLoadBlockers(ctx, tasks)
	if err != nil {
		slog.Error("failed to batch load blockers", "error", err)
		// Continue with empty blocker map (fail-safe)
		blockersByTask = make(map[string][]*domain.Task)
	}

	// Compute derived fields for each task
	results := make([]TaskListResult, len(tasks))
	for i, task := range tasks {
		results[i] = TaskListResult{
			Task:                  task,
			HasUnresolvedBlockers: false,
			IsOverdue:             task.StatusDeadlineAt != nil && task.StatusDeadlineAt.Before(time.Now()),
		}

		// Check blockers from batch-loaded map
		if blockers, ok := blockersByTask[task.ID]; ok {
			for _, blocker := range blockers {
				if blocker.Status != domain.TaskStatusDone {
					results[i].HasUnresolvedBlockers = true
					break
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
