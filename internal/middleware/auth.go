package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mtlprog/sloptask/internal/domain"
	"github.com/mtlprog/sloptask/internal/repository"
)

type contextKey string

const (
	// ContextKeyAgent is the key for storing agent in request context.
	ContextKeyAgent contextKey = "agent"
)

// AuthMiddleware handles Bearer token authentication.
type AuthMiddleware struct {
	agentRepo *repository.AgentRepository
}

// NewAuthMiddleware creates a new AuthMiddleware.
func NewAuthMiddleware(agentRepo *repository.AgentRepository) *AuthMiddleware {
	return &AuthMiddleware{agentRepo: agentRepo}
}

// Authenticate validates Bearer token and adds agent to request context.
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := parseBearerToken(r.Header.Get("Authorization"))
		if !ok {
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}

		agent, err := m.agentRepo.GetByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, domain.ErrAgentNotFound) {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			slog.Error("failed to fetch agent by token",
				"error", err,
				"remote_addr", r.RemoteAddr,
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if !agent.IsActive {
			http.Error(w, "agent inactive", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ContextKeyAgent, agent)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetAgentFromContext retrieves the authenticated agent from request context.
func GetAgentFromContext(ctx context.Context) (*domain.Agent, error) {
	agent, ok := ctx.Value(ContextKeyAgent).(*domain.Agent)
	if !ok || agent == nil {
		return nil, domain.ErrAgentNotFound
	}
	return agent, nil
}

// parseBearerToken extracts the token from a "Bearer <token>" authorization header.
// Returns the token and true if valid, or empty string and false otherwise.
func parseBearerToken(header string) (string, bool) {
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		// Also handle case-insensitive "bearer" prefix
		token, found = strings.CutPrefix(header, "bearer ")
	}
	if !found || token == "" {
		return "", false
	}
	return token, true
}
