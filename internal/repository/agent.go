package repository

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtlprog/sloptask/internal/domain"
)

// AgentRepository handles database operations for agents.
type AgentRepository struct {
	pool *pgxpool.Pool
	psql sq.StatementBuilderType
}

// NewAgentRepository creates a new AgentRepository.
func NewAgentRepository(pool *pgxpool.Pool) *AgentRepository {
	return &AgentRepository{
		pool: pool,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

// GetByToken finds an agent by authentication token.
func (r *AgentRepository) GetByToken(ctx context.Context, token string) (*domain.Agent, error) {
	query, args, err := r.psql.
		Select("id", "workspace_id", "name", "token", "is_active", "created_at").
		From("agents").
		Where(sq.Eq{"token": token}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var agent domain.Agent
	err = r.pool.QueryRow(ctx, query, args...).Scan(
		&agent.ID,
		&agent.WorkspaceID,
		&agent.Name,
		&agent.Token,
		&agent.IsActive,
		&agent.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrAgentNotFound
		}
		return nil, fmt.Errorf("query agent: %w", err)
	}

	return &agent, nil
}

// GetByID retrieves an agent by ID.
func (r *AgentRepository) GetByID(ctx context.Context, agentID string) (*domain.Agent, error) {
	query, args, err := r.psql.
		Select("id", "workspace_id", "name", "token", "is_active", "created_at").
		From("agents").
		Where(sq.Eq{"id": agentID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var agent domain.Agent
	err = r.pool.QueryRow(ctx, query, args...).Scan(
		&agent.ID,
		&agent.WorkspaceID,
		&agent.Name,
		&agent.Token,
		&agent.IsActive,
		&agent.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrAgentNotFound
		}
		return nil, fmt.Errorf("query agent: %w", err)
	}

	return &agent, nil
}
