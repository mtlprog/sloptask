package handler

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	pool *pgxpool.Pool
}

// New creates a new Handler instance.
func New(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// RegisterRoutes registers all HTTP routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.handleHealthz)
}

// handleHealthz returns 200 OK if the database is reachable.
func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := h.pool.Ping(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Ping checks if the database is reachable (used for testing).
func (h *Handler) Ping(ctx context.Context) error {
	return h.pool.Ping(ctx)
}
