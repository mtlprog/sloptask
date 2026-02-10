package handler

import (
	"net/http"
	"time"

	"github.com/mtlprog/sloptask/internal/handler/dto"
	"github.com/mtlprog/sloptask/internal/middleware"
	"github.com/mtlprog/sloptask/internal/repository"
)

// handleGetStats returns workspace and agent statistics.
// @Summary Get statistics
// @Description Get workspace and agent statistics for a given period
// @Tags stats
// @Produce json
// @Param period query string false "Period: day, week (default), month, all"
// @Param agent_id query string false "Filter by specific agent UUID"
// @Success 200 {object} dto.StatsResponse
// @Security BearerAuth
// @Router /stats [get]
func (h *Handler) handleGetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agent, err := middleware.GetAgentFromContext(ctx)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Authentication required")
		return
	}

	// Parse period parameter
	query := r.URL.Query()
	period := query.Get("period")
	if period == "" {
		period = "week"
	}

	// Calculate period boundaries
	now := time.Now()
	var periodStart time.Time
	switch period {
	case "day":
		periodStart = now.AddDate(0, 0, -1)
	case "week":
		periodStart = now.AddDate(0, 0, -7)
	case "month":
		periodStart = now.AddDate(0, -1, 0)
	case "all":
		periodStart = time.Time{} // Beginning of time
	default:
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period, must be: day, week, month, all")
		return
	}

	// Parse agent_id filter
	var agentIDFilter *string
	if agentID := query.Get("agent_id"); agentID != "" {
		agentIDFilter = &agentID
	}

	// Get agent stats
	agentStats, err := h.taskRepo.GetAgentStats(ctx, repository.StatsFilters{
		WorkspaceID: agent.WorkspaceID,
		PeriodStart: periodStart,
		PeriodEnd:   now,
		AgentID:     agentIDFilter,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch agent stats")
		return
	}

	// Get workspace stats
	workspaceStats, err := h.taskRepo.GetWorkspaceStats(ctx, repository.StatsFilters{
		WorkspaceID: agent.WorkspaceID,
		PeriodStart: periodStart,
		PeriodEnd:   now,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch workspace stats")
		return
	}

	// Convert to response format
	agents := make([]dto.AgentStats, len(agentStats))
	for i, stat := range agentStats {
		agents[i] = dto.AgentStats{
			AgentID:         stat.AgentID,
			AgentName:       stat.AgentName,
			TasksCompleted:  stat.TasksCompleted,
			TasksCancelled:  stat.TasksCancelled,
			TasksStuckCount: stat.TasksStuckCount,
			TasksInProgress: stat.TasksInProgress,
			// Other fields defaulted to 0 (not implemented in MVP)
			AvgLeadTimeMinutes:      0,
			AvgCycleTimeMinutes:     0,
			TasksTakenOverFromAgent: 0,
			TasksTakenOverByAgent:   0,
			EscalationsInitiated:    0,
			EscalationsReceived:     0,
		}
	}

	// Calculate completion rate
	totalTasks := 0
	for _, count := range workspaceStats.TasksByStatus {
		totalTasks += count
	}
	completionRate := 0.0
	if totalTasks > 0 {
		doneCount := workspaceStats.TasksByStatus["DONE"]
		completionRate = float64(doneCount) / float64(totalTasks) * 100
	}

	respondJSON(w, http.StatusOK, dto.StatsResponse{
		Period:      period,
		PeriodStart: periodStart,
		PeriodEnd:   now,
		Agents:      agents,
		Workspace: dto.WorkspaceStats{
			TotalTasksCreated:     workspaceStats.TotalTasksCreated,
			TasksByStatus:         workspaceStats.TasksByStatus,
			AvgLeadTimeMinutes:    0, // Not implemented in MVP
			AvgCycleTimeMinutes:   0, // Not implemented in MVP
			OverdueCount:          workspaceStats.OverdueCount,
			StuckCount:            workspaceStats.StuckCount,
			CompletionRatePercent: completionRate,
		},
	})
}
