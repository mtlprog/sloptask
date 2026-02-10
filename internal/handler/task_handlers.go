package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mtlprog/sloptask/internal/domain"
	"github.com/mtlprog/sloptask/internal/handler/dto"
	"github.com/mtlprog/sloptask/internal/middleware"
	"github.com/mtlprog/sloptask/internal/repository"
	"github.com/mtlprog/sloptask/internal/service"
)

// handleCreateTask creates a new task.
// @Summary Create a new task
// @Description Creates a new task. If assignee_id is provided, task automatically transitions to IN_PROGRESS.
// @Tags tasks
// @Accept json
// @Produce json
// @Param request body dto.CreateTaskRequest true "Task creation request"
// @Success 201 {object} dto.TaskDetail
// @Failure 400 {object} dto.ErrorResponse
// @Failure 422 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /tasks [post]
func (h *Handler) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract authenticated agent
	agent, err := middleware.GetAgentFromContext(ctx)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Authentication required")
		return
	}

	// Parse request body
	var req dto.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	// Validate required fields
	if req.Title == "" || len(req.Title) < 5 || len(req.Title) > 200 {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "title must be between 5 and 200 characters")
		return
	}
	if req.Description == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "description is required")
		return
	}

	// Set defaults
	visibility := domain.TaskVisibilityPublic
	if req.Visibility != "" {
		visibility = domain.TaskVisibility(req.Visibility)
		if visibility != domain.TaskVisibilityPublic && visibility != domain.TaskVisibilityPrivate {
			respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "visibility must be 'public' or 'private'")
			return
		}
	}

	priority := domain.TaskPriorityNormal
	if req.Priority != "" {
		priority = domain.TaskPriority(req.Priority)
		if priority != domain.TaskPriorityLow && priority != domain.TaskPriorityNormal &&
			priority != domain.TaskPriorityHigh && priority != domain.TaskPriorityCritical {
			respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "priority must be 'low', 'normal', 'high', or 'critical'")
			return
		}
	}

	// Create task
	task, err := h.taskService.CreateTask(ctx, service.CreateTaskParams{
		WorkspaceID: agent.WorkspaceID,
		CreatorID:   agent.ID,
		Title:       req.Title,
		Description: req.Description,
		AssigneeID:  req.AssigneeID,
		Visibility:  visibility,
		Priority:    priority,
		BlockedBy:   req.BlockedBy,
	})
	if err != nil {
		status, code, message := dto.MapDomainError(err)
		respondError(w, status, code, message)
		return
	}

	// Return created task
	respondJSON(w, http.StatusCreated, dto.ToTaskDetail(task, false, false))
}

// handleGetTask retrieves task details with events.
// @Summary Get task details
// @Description Get full task details including description and event history
// @Tags tasks
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {object} dto.TaskDetailResponse
// @Failure 404 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /tasks/{id} [get]
func (h *Handler) handleGetTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract authenticated agent
	agent, err := middleware.GetAgentFromContext(ctx)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Authentication required")
		return
	}

	// Extract task ID
	taskID := r.PathValue("id")
	if taskID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "task id is required")
		return
	}

	// Get task
	task, err := h.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		status, code, message := dto.MapDomainError(err)
		respondError(w, status, code, message)
		return
	}

	// Check visibility
	if task.WorkspaceID != agent.WorkspaceID {
		respondError(w, http.StatusForbidden, "INSUFFICIENT_ACCESS", "Task not found")
		return
	}
	if task.Visibility == domain.TaskVisibilityPrivate {
		if task.CreatorID != agent.ID && (task.AssigneeID == nil || *task.AssigneeID != agent.ID) {
			respondError(w, http.StatusForbidden, "INSUFFICIENT_ACCESS", "Task not found")
			return
		}
	}

	// Get events with actor names
	events, err := h.eventRepo.GetByTaskIDWithActors(ctx, taskID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch events")
		return
	}

	// Calculate computed fields
	hasUnresolvedBlockers := false
	if len(task.BlockedBy) > 0 {
		blockers, _ := h.taskRepo.GetBlockedByTasks(ctx, task.BlockedBy)
		for _, blocker := range blockers {
			if blocker.Status != domain.TaskStatusDone {
				hasUnresolvedBlockers = true
				break
			}
		}
	}

	isOverdue := task.StatusDeadlineAt != nil && task.StatusDeadlineAt.Before(time.Now())

	// Build response
	response := dto.TaskDetailResponse{
		Task:   dto.ToTaskDetail(task, hasUnresolvedBlockers, isOverdue),
		Events: make([]dto.TaskEventInfo, len(events)),
	}

	for i, event := range events {
		var oldStatus, newStatus *string
		if event.OldStatus != nil {
			s := string(*event.OldStatus)
			oldStatus = &s
		}
		if event.NewStatus != nil {
			s := string(*event.NewStatus)
			newStatus = &s
		}

		response.Events[i] = dto.TaskEventInfo{
			ID:        event.ID,
			Type:      string(event.Type),
			ActorID:   event.ActorID,
			ActorName: event.ActorName,
			Comment:   event.Comment,
			OldStatus: oldStatus,
			NewStatus: newStatus,
			CreatedAt: event.CreatedAt,
		}
	}

	respondJSON(w, http.StatusOK, response)
}

// handleTransitionStatus changes task status.
// @Summary Transition task status
// @Description Change task status with comment
// @Tags tasks
// @Accept json
// @Produce json
// @Param id path string true "Task ID"
// @Param request body dto.TransitionStatusRequest true "Status transition request"
// @Success 200 {object} dto.TaskEventResponse
// @Failure 409 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /tasks/{id}/status [patch]
func (h *Handler) handleTransitionStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agent, err := middleware.GetAgentFromContext(ctx)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Authentication required")
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "task id is required")
		return
	}

	var req dto.TransitionStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Status == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "status is required")
		return
	}
	if req.Comment == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "comment is required")
		return
	}

	newStatus := domain.TaskStatus(req.Status)
	if !newStatus.IsValid() {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid status")
		return
	}

	event, err := h.taskService.TransitionStatus(ctx, taskID, agent.ID, newStatus, req.Comment)
	if err != nil {
		status, code, message := dto.MapDomainError(err)
		respondError(w, status, code, message)
		return
	}

	respondJSON(w, http.StatusOK, dto.ToTaskEventResponse(event))
}

// handleClaimTask claims an unassigned NEW task.
// @Summary Claim a task
// @Description Agent claims an unassigned NEW task
// @Tags tasks
// @Accept json
// @Produce json
// @Param id path string true "Task ID"
// @Param request body dto.ClaimTaskRequest true "Claim request"
// @Success 200 {object} dto.TaskEventResponse
// @Failure 409 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /tasks/{id}/claim [post]
func (h *Handler) handleClaimTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agent, err := middleware.GetAgentFromContext(ctx)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Authentication required")
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "task id is required")
		return
	}

	var req dto.ClaimTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Comment == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "comment is required")
		return
	}

	event, err := h.taskService.ClaimTask(ctx, taskID, agent.ID, req.Comment)
	if err != nil {
		status, code, message := dto.MapDomainError(err)
		respondError(w, status, code, message)
		return
	}

	respondJSON(w, http.StatusOK, dto.ToTaskEventResponse(event))
}

// handleEscalateTask escalates a stuck IN_PROGRESS task.
// @Summary Escalate a task
// @Description Agent escalates another agent's IN_PROGRESS task
// @Tags tasks
// @Accept json
// @Produce json
// @Param id path string true "Task ID"
// @Param request body dto.EscalateTaskRequest true "Escalate request"
// @Success 200 {object} dto.TaskEventResponse
// @Failure 409 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /tasks/{id}/escalate [post]
func (h *Handler) handleEscalateTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agent, err := middleware.GetAgentFromContext(ctx)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Authentication required")
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "task id is required")
		return
	}

	var req dto.EscalateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Comment == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "comment is required")
		return
	}

	event, err := h.taskService.EscalateTask(ctx, taskID, agent.ID, req.Comment)
	if err != nil {
		status, code, message := dto.MapDomainError(err)
		respondError(w, status, code, message)
		return
	}

	respondJSON(w, http.StatusOK, dto.ToTaskEventResponse(event))
}

// handleTakeoverTask takes over a STUCK task.
// @Summary Takeover a STUCK task
// @Description Agent takes over an abandoned STUCK task
// @Tags tasks
// @Accept json
// @Produce json
// @Param id path string true "Task ID"
// @Param request body dto.TakeoverTaskRequest true "Takeover request"
// @Success 200 {object} dto.TaskEventResponse
// @Failure 409 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /tasks/{id}/takeover [post]
func (h *Handler) handleTakeoverTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agent, err := middleware.GetAgentFromContext(ctx)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Authentication required")
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "task id is required")
		return
	}

	var req dto.TakeoverTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Comment == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "comment is required")
		return
	}

	event, err := h.taskService.TakeoverTask(ctx, taskID, agent.ID, req.Comment)
	if err != nil {
		status, code, message := dto.MapDomainError(err)
		respondError(w, status, code, message)
		return
	}

	respondJSON(w, http.StatusOK, dto.ToTaskEventResponse(event))
}

// handleCommentTask adds a comment to a task.
// @Summary Add comment to task
// @Description Add a comment without changing task status
// @Tags tasks
// @Accept json
// @Produce json
// @Param id path string true "Task ID"
// @Param request body dto.CommentTaskRequest true "Comment request"
// @Success 201 {object} dto.TaskEventResponse
// @Failure 403 {object} dto.ErrorResponse
// @Security BearerAuth
// @Router /tasks/{id}/comments [post]
func (h *Handler) handleCommentTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agent, err := middleware.GetAgentFromContext(ctx)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Authentication required")
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "task id is required")
		return
	}

	var req dto.CommentTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Comment == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "comment is required")
		return
	}

	// Get task to check visibility
	task, err := h.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		status, code, message := dto.MapDomainError(err)
		respondError(w, status, code, message)
		return
	}

	// Check workspace and visibility permissions
	if task.WorkspaceID != agent.WorkspaceID {
		respondError(w, http.StatusForbidden, "INSUFFICIENT_ACCESS", "Task not found")
		return
	}
	if task.Visibility == domain.TaskVisibilityPrivate {
		if task.CreatorID != agent.ID && (task.AssigneeID == nil || *task.AssigneeID != agent.ID) {
			respondError(w, http.StatusForbidden, "INSUFFICIENT_ACCESS", "Task not found")
			return
		}
	}

	// Create comment event
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create comment")
		return
	}
	defer tx.Rollback(ctx)

	event := &domain.TaskEvent{
		TaskID:    taskID,
		ActorID:   &agent.ID,
		Type:      domain.EventTypeCommented,
		OldStatus: nil,
		NewStatus: nil,
		Comment:   req.Comment,
	}

	if err := h.eventRepo.Create(ctx, tx, event); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create comment")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create comment")
		return
	}

	respondJSON(w, http.StatusCreated, dto.ToTaskEventResponse(event))
}

// handleListTasks returns a list of tasks with filters.
// @Summary List tasks
// @Description Get a list of tasks with optional filters
// @Tags tasks
// @Produce json
// @Param status query string false "Comma-separated statuses: NEW,STUCK"
// @Param assignee query string false "Filter by assignee: 'me' or agent UUID"
// @Param unassigned query bool false "Show only unassigned tasks"
// @Param visibility query string false "Filter by visibility: public or private"
// @Param priority query string false "Comma-separated priorities: high,critical"
// @Param overdue query bool false "Show only overdue tasks"
// @Param has_unresolved_blockers query bool false "Show only tasks with unresolved blockers"
// @Param sort query string false "Sort fields: -priority,created_at"
// @Param limit query int false "Page size (1-200, default 50)"
// @Param offset query int false "Page offset (default 0)"
// @Success 200 {object} dto.TasksListResponse
// @Security BearerAuth
// @Router /tasks [get]
func (h *Handler) handleListTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agent, err := middleware.GetAgentFromContext(ctx)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Authentication required")
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	// Parse statuses (comma-separated)
	var statuses []string
	if statusParam := query.Get("status"); statusParam != "" {
		statuses = splitAndTrim(statusParam, ",")
	}

	// Parse assignee
	var assigneeID *string
	unassigned := false
	if assigneeParam := query.Get("assignee"); assigneeParam != "" {
		if assigneeParam == "me" {
			assigneeID = &agent.ID
		} else {
			assigneeID = &assigneeParam
		}
	}
	if query.Get("unassigned") == "true" {
		unassigned = true
	}

	// Parse visibility
	var visibility *string
	if visibilityParam := query.Get("visibility"); visibilityParam != "" {
		visibility = &visibilityParam
	}

	// Parse priorities (comma-separated)
	var priorities []string
	if priorityParam := query.Get("priority"); priorityParam != "" {
		priorities = splitAndTrim(priorityParam, ",")
	}

	// Parse boolean filters
	overdue := query.Get("overdue") == "true"
	hasUnresolvedBlockers := query.Get("has_unresolved_blockers") == "true"

	// Parse sort (comma-separated)
	var sort []string
	if sortParam := query.Get("sort"); sortParam != "" {
		sort = splitAndTrim(sortParam, ",")
	}

	// Parse pagination
	limit := 50
	if limitParam := query.Get("limit"); limitParam != "" {
		if n, err := strconv.Atoi(limitParam); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	offset := 0
	if offsetParam := query.Get("offset"); offsetParam != "" {
		if n, err := strconv.Atoi(offsetParam); err == nil && n >= 0 {
			offset = n
		}
	}

	// Call repository
	results, total, err := h.taskRepo.List(ctx, repository.TaskListFilters{
		WorkspaceID:           agent.WorkspaceID,
		Statuses:              statuses,
		AssigneeID:            assigneeID,
		Unassigned:            unassigned,
		Visibility:            visibility,
		Priorities:            priorities,
		Overdue:               overdue,
		HasUnresolvedBlockers: hasUnresolvedBlockers,
		Sort:                  sort,
		Limit:                 limit,
		Offset:                offset,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list tasks")
		return
	}

	// Convert to response format
	tasks := make([]dto.TaskListResponse, len(results))
	for i, result := range results {
		tasks[i] = dto.ToTaskListResponse(result.Task, result.HasUnresolvedBlockers, result.IsOverdue)
	}

	respondJSON(w, http.StatusOK, dto.TasksListResponse{
		Tasks:  tasks,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// splitAndTrim splits a string by delimiter and trims whitespace.
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
