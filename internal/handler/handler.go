package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/mtlprog/sloptask/docs" // Import generated docs
	"github.com/mtlprog/sloptask/internal/handler/dto"
	"github.com/mtlprog/sloptask/internal/middleware"
	"github.com/mtlprog/sloptask/internal/repository"
	"github.com/mtlprog/sloptask/internal/service"
	"github.com/mtlprog/sloptask/internal/static"
	httpSwagger "github.com/swaggo/http-swagger"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	pool           *pgxpool.Pool
	taskService    *service.TaskService
	taskRepo       *repository.TaskRepository
	eventRepo      *repository.TaskEventRepository
	agentRepo      *repository.AgentRepository
	workspaceRepo  *repository.WorkspaceRepository
	authMiddleware *middleware.AuthMiddleware
}

// New creates a new Handler instance with all dependencies.
func New(pool *pgxpool.Pool) *Handler {
	// Create repositories
	taskRepo := repository.NewTaskRepository(pool)
	eventRepo := repository.NewTaskEventRepository(pool)
	agentRepo := repository.NewAgentRepository(pool)
	workspaceRepo := repository.NewWorkspaceRepository(pool)

	// Create services
	taskService := service.NewTaskService(pool, taskRepo, eventRepo, agentRepo, workspaceRepo)

	// Create middleware
	authMiddleware := middleware.NewAuthMiddleware(agentRepo)

	return &Handler{
		pool:           pool,
		taskService:    taskService,
		taskRepo:       taskRepo,
		eventRepo:      eventRepo,
		agentRepo:      agentRepo,
		workspaceRepo:  workspaceRepo,
		authMiddleware: authMiddleware,
	}
}

// RegisterRoutes registers all HTTP routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /healthz", h.handleHealthz)

	// Static files for AI agents
	mux.HandleFunc("GET /skill.md", h.handleSkillMd)

	// Swagger UI
	mux.HandleFunc("GET /swagger/", httpSwagger.Handler())

	// API v1 routes with authentication
	mux.Handle("GET /api/v1/tasks", h.authMiddleware.Authenticate(http.HandlerFunc(h.handleListTasks)))
	mux.Handle("POST /api/v1/tasks", h.authMiddleware.Authenticate(http.HandlerFunc(h.handleCreateTask)))
	mux.Handle("GET /api/v1/tasks/{id}", h.authMiddleware.Authenticate(http.HandlerFunc(h.handleGetTask)))
	mux.Handle("PATCH /api/v1/tasks/{id}/status", h.authMiddleware.Authenticate(http.HandlerFunc(h.handleTransitionStatus)))
	mux.Handle("POST /api/v1/tasks/{id}/claim", h.authMiddleware.Authenticate(http.HandlerFunc(h.handleClaimTask)))
	mux.Handle("POST /api/v1/tasks/{id}/escalate", h.authMiddleware.Authenticate(http.HandlerFunc(h.handleEscalateTask)))
	mux.Handle("POST /api/v1/tasks/{id}/takeover", h.authMiddleware.Authenticate(http.HandlerFunc(h.handleTakeoverTask)))
	mux.Handle("POST /api/v1/tasks/{id}/comments", h.authMiddleware.Authenticate(http.HandlerFunc(h.handleCommentTask)))
	mux.Handle("GET /api/v1/stats", h.authMiddleware.Authenticate(http.HandlerFunc(h.handleGetStats)))
}

// handleHealthz returns 200 OK if the database is reachable.
func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := h.pool.Ping(ctx); err != nil {
		slog.Error("database health check failed", "error", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleSkillMd serves the embedded skill.md file for AI agents.
func (h *Handler) handleSkillMd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(static.SkillMd))
}

// Ping checks if the database is reachable (used for testing).
func (h *Handler) Ping(ctx context.Context) error {
	return h.pool.Ping(ctx)
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// respondError writes a standard error response.
func respondError(w http.ResponseWriter, status int, code, message string) {
	respondJSON(w, status, dto.NewErrorResponse(code, message))
}

// extractTaskID extracts and validates task ID from path parameter.
// Returns (taskID, true) if valid, ("", false) if invalid (error already sent to client).
func extractTaskID(w http.ResponseWriter, r *http.Request) (string, bool) {
	taskID := r.PathValue("id")
	if taskID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "task id is required")
		return "", false
	}

	if _, err := uuid.Parse(taskID); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "task_id must be a valid UUID")
		return "", false
	}

	return taskID, true
}
