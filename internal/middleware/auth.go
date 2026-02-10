package middleware

import (
	"context"
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
	return &AuthMiddleware{
		agentRepo: agentRepo,
	}
}

// Authenticate validates Bearer token and adds agent to request context.
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		// Parse Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}

		token := parts[1]
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		// Find agent by token
		agent, err := m.agentRepo.GetByToken(r.Context(), token)
		if err != nil {
			if err == domain.ErrAgentNotFound {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Check if agent is active
		if !agent.IsActive {
			http.Error(w, "agent inactive", http.StatusUnauthorized)
			return
		}

		// Add agent to context
		ctx := context.WithValue(r.Context(), ContextKeyAgent, agent)

		// Call next handler
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
