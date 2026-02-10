package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/mtlprog/sloptask/internal/domain"
)

// StatsFilters holds filters for statistics queries.
type StatsFilters struct {
	WorkspaceID string
	PeriodStart time.Time
	PeriodEnd   time.Time
	AgentID     *string // Optional: filter by specific agent
}

// AgentStatsResult holds statistics for a single agent.
type AgentStatsResult struct {
	AgentID          string
	AgentName        string
	TasksCompleted   int
	TasksCancelled   int
	TasksStuckCount  int
	TasksInProgress  int
}

// WorkspaceStatsResult holds overall workspace statistics.
type WorkspaceStatsResult struct {
	TotalTasksCreated int
	TasksByStatus     map[string]int
	OverdueCount      int
	StuckCount        int
}

// GetAgentStats retrieves statistics for agents in a workspace.
func (r *TaskRepository) GetAgentStats(ctx context.Context, filters StatsFilters) ([]AgentStatsResult, error) {
	query := `
		SELECT
			a.id,
			a.name,
			COUNT(CASE WHEN t.status = 'DONE' AND t.updated_at >= $2 AND t.updated_at <= $3 THEN 1 END) as tasks_completed,
			COUNT(CASE WHEN t.status = 'CANCELLED' AND t.updated_at >= $2 AND t.updated_at <= $3 THEN 1 END) as tasks_cancelled,
			COUNT(CASE WHEN t.status = 'STUCK' THEN 1 END) as tasks_stuck_count,
			COUNT(CASE WHEN t.status = 'IN_PROGRESS' THEN 1 END) as tasks_in_progress
		FROM agents a
		LEFT JOIN tasks t ON t.assignee_id = a.id AND t.workspace_id = $1
		WHERE a.workspace_id = $1 AND a.is_active = true
	`

	args := []interface{}{filters.WorkspaceID, filters.PeriodStart, filters.PeriodEnd}

	// Filter by specific agent if provided
	if filters.AgentID != nil {
		query += " AND a.id = $4"
		args = append(args, *filters.AgentID)
	}

	query += " GROUP BY a.id, a.name ORDER BY a.name"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query agent stats: %w", err)
	}
	defer rows.Close()

	var results []AgentStatsResult
	for rows.Next() {
		var result AgentStatsResult
		err := rows.Scan(
			&result.AgentID,
			&result.AgentName,
			&result.TasksCompleted,
			&result.TasksCancelled,
			&result.TasksStuckCount,
			&result.TasksInProgress,
		)
		if err != nil {
			return nil, fmt.Errorf("scan agent stats: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent stats rows: %w", err)
	}

	return results, nil
}

// GetWorkspaceStats retrieves overall workspace statistics.
func (r *TaskRepository) GetWorkspaceStats(ctx context.Context, filters StatsFilters) (*WorkspaceStatsResult, error) {
	// Get total tasks created in period
	var totalCreated int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE workspace_id = $1 AND created_at >= $2 AND created_at <= $3
	`, filters.WorkspaceID, filters.PeriodStart, filters.PeriodEnd).Scan(&totalCreated)
	if err != nil {
		return nil, fmt.Errorf("count total tasks: %w", err)
	}

	// Get tasks by status (current state, not historical)
	tasksByStatus := make(map[string]int)
	rows, err := r.pool.Query(ctx, `
		SELECT status, COUNT(*)
		FROM tasks
		WHERE workspace_id = $1
		GROUP BY status
	`, filters.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("query tasks by status: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan status count: %w", err)
		}
		tasksByStatus[status] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate status rows: %w", err)
	}

	// Get overdue count
	var overdueCount int
	err = r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE workspace_id = $1
		  AND status IN ($2, $3, $4)
		  AND status_deadline_at < NOW()
	`, filters.WorkspaceID,
		domain.TaskStatusNew,
		domain.TaskStatusInProgress,
		domain.TaskStatusBlocked,
	).Scan(&overdueCount)
	if err != nil {
		return nil, fmt.Errorf("count overdue tasks: %w", err)
	}

	// Stuck count is already in tasksByStatus
	stuckCount := tasksByStatus[string(domain.TaskStatusStuck)]

	return &WorkspaceStatsResult{
		TotalTasksCreated: totalCreated,
		TasksByStatus:     tasksByStatus,
		OverdueCount:      overdueCount,
		StuckCount:        stuckCount,
	}, nil
}
