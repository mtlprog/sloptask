package dto

import (
	"time"

	"github.com/mtlprog/sloptask/internal/domain"
)

// TaskListResponse represents a task in the list view (without description and events).
type TaskListResponse struct {
	ID                    string     `json:"id"`
	Title                 string     `json:"title"`
	Status                string     `json:"status"`
	Priority              string     `json:"priority"`
	Visibility            string     `json:"visibility"`
	CreatorID             string     `json:"creator_id"`
	AssigneeID            *string    `json:"assignee_id"`
	BlockedBy             []string   `json:"blocked_by"`
	HasUnresolvedBlockers bool       `json:"has_unresolved_blockers"`
	IsOverdue             bool       `json:"is_overdue"`
	StatusDeadlineAt      *time.Time `json:"status_deadline_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// TasksListResponse represents the response for GET /tasks.
type TasksListResponse struct {
	Tasks  []TaskListResponse `json:"tasks"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

// TaskDetailResponse represents full task details with events.
type TaskDetailResponse struct {
	Task   TaskDetail      `json:"task"`
	Events []TaskEventInfo `json:"events"`
}

// TaskDetail represents the full task object.
type TaskDetail struct {
	ID                    string     `json:"id"`
	Title                 string     `json:"title"`
	Description           string     `json:"description"`
	Status                string     `json:"status"`
	Priority              string     `json:"priority"`
	Visibility            string     `json:"visibility"`
	CreatorID             string     `json:"creator_id"`
	AssigneeID            *string    `json:"assignee_id"`
	BlockedBy             []string   `json:"blocked_by"`
	HasUnresolvedBlockers bool       `json:"has_unresolved_blockers"`
	IsOverdue             bool       `json:"is_overdue"`
	StatusDeadlineAt      *time.Time `json:"status_deadline_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// TaskEventInfo represents a task event with actor information.
type TaskEventInfo struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	ActorID   *string `json:"actor_id"`
	ActorName *string `json:"actor_name"`
	Comment   string  `json:"comment"`
	OldStatus *string `json:"old_status"`
	NewStatus *string `json:"new_status"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskEventResponse represents a single event response (for claim, escalate, etc).
type TaskEventResponse struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Type      string    `json:"type"`
	ActorID   *string   `json:"actor_id"`
	OldStatus *string   `json:"old_status"`
	NewStatus *string   `json:"new_status"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

// StatsResponse represents workspace statistics.
type StatsResponse struct {
	Period      string           `json:"period"`
	PeriodStart time.Time        `json:"period_start"`
	PeriodEnd   time.Time        `json:"period_end"`
	Agents      []AgentStats     `json:"agents"`
	Workspace   WorkspaceStats   `json:"workspace"`
}

// AgentStats represents statistics for a single agent.
type AgentStats struct {
	AgentID                  string  `json:"agent_id"`
	AgentName                string  `json:"agent_name"`
	TasksCompleted           int     `json:"tasks_completed"`
	TasksCancelled           int     `json:"tasks_cancelled"`
	TasksStuckCount          int     `json:"tasks_stuck_count"`
	TasksInProgress          int     `json:"tasks_in_progress"`
	AvgLeadTimeMinutes       float64 `json:"avg_lead_time_minutes"`
	AvgCycleTimeMinutes      float64 `json:"avg_cycle_time_minutes"`
	TasksTakenOverFromAgent  int     `json:"tasks_taken_over_from_agent"`
	TasksTakenOverByAgent    int     `json:"tasks_taken_over_by_agent"`
	EscalationsInitiated     int     `json:"escalations_initiated"`
	EscalationsReceived      int     `json:"escalations_received"`
}

// WorkspaceStats represents overall workspace statistics.
type WorkspaceStats struct {
	TotalTasksCreated      int                `json:"total_tasks_created"`
	TasksByStatus          map[string]int     `json:"tasks_by_status"`
	AvgLeadTimeMinutes     float64            `json:"avg_lead_time_minutes"`
	AvgCycleTimeMinutes    float64            `json:"avg_cycle_time_minutes"`
	OverdueCount           int                `json:"overdue_count"`
	StuckCount             int                `json:"stuck_count"`
	CompletionRatePercent  float64            `json:"completion_rate_percent"`
}

// ToTaskListResponse converts domain.Task to TaskListResponse.
func ToTaskListResponse(task *domain.Task, hasUnresolvedBlockers, isOverdue bool) TaskListResponse {
	return TaskListResponse{
		ID:                    task.ID,
		Title:                 task.Title,
		Status:                string(task.Status),
		Priority:              string(task.Priority),
		Visibility:            string(task.Visibility),
		CreatorID:             task.CreatorID,
		AssigneeID:            task.AssigneeID,
		BlockedBy:             task.BlockedBy,
		HasUnresolvedBlockers: hasUnresolvedBlockers,
		IsOverdue:             isOverdue,
		StatusDeadlineAt:      task.StatusDeadlineAt,
		CreatedAt:             task.CreatedAt,
		UpdatedAt:             task.UpdatedAt,
	}
}

// ToTaskDetail converts domain.Task to TaskDetail.
func ToTaskDetail(task *domain.Task, hasUnresolvedBlockers, isOverdue bool) TaskDetail {
	return TaskDetail{
		ID:                    task.ID,
		Title:                 task.Title,
		Description:           task.Description,
		Status:                string(task.Status),
		Priority:              string(task.Priority),
		Visibility:            string(task.Visibility),
		CreatorID:             task.CreatorID,
		AssigneeID:            task.AssigneeID,
		BlockedBy:             task.BlockedBy,
		HasUnresolvedBlockers: hasUnresolvedBlockers,
		IsOverdue:             isOverdue,
		StatusDeadlineAt:      task.StatusDeadlineAt,
		CreatedAt:             task.CreatedAt,
		UpdatedAt:             task.UpdatedAt,
	}
}

// ToTaskEventResponse converts domain.TaskEvent to TaskEventResponse.
func ToTaskEventResponse(event *domain.TaskEvent) TaskEventResponse {
	var oldStatus, newStatus *string
	if event.OldStatus != nil {
		s := string(*event.OldStatus)
		oldStatus = &s
	}
	if event.NewStatus != nil {
		s := string(*event.NewStatus)
		newStatus = &s
	}

	return TaskEventResponse{
		ID:        event.ID,
		TaskID:    event.TaskID,
		Type:      string(event.Type),
		ActorID:   event.ActorID,
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Comment:   event.Comment,
		CreatedAt: event.CreatedAt,
	}
}
