package repository

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtlprog/sloptask/internal/domain"
)

// agentColumns is the shared list of columns for agent queries.
var agentColumns = []string{"id", "workspace_id", "name", "token", "is_active", "created_at"}

// AgentRepository handles database operations for agents.
type AgentRepository struct {
	pool *pgxpool.Pool
}

// NewAgentRepository creates a new AgentRepository.
func NewAgentRepository(pool *pgxpool.Pool) *AgentRepository {
	return &AgentRepository{pool: pool}
}

// scanAgent scans a single row into an Agent struct.
func scanAgent(row pgx.Row) (*domain.Agent, error) {
	var agent domain.Agent
	err := row.Scan(
		&agent.ID,
		&agent.WorkspaceID,
		&agent.Name,
		&agent.Token,
		&agent.IsActive,
		&agent.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAgentNotFound
		}
		return nil, fmt.Errorf("scan agent: %w", err)
	}
	return &agent, nil
}

// GetByToken finds an agent by authentication token.
func (r *AgentRepository) GetByToken(ctx context.Context, token string) (*domain.Agent, error) {
	query, args, err := psql.
		Select(agentColumns...).
		From("agents").
		Where(sq.Eq{"token": token}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build GetByToken query for agent: %w", err)
	}

	return scanAgent(r.pool.QueryRow(ctx, query, args...))
}

// GetByID retrieves an agent by ID.
func (r *AgentRepository) GetByID(ctx context.Context, agentID string) (*domain.Agent, error) {
	query, args, err := psql.
		Select(agentColumns...).
		From("agents").
		Where(sq.Eq{"id": agentID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build GetByID query for agent %s: %w", agentID, err)
	}

	return scanAgent(r.pool.QueryRow(ctx, query, args...))
}
