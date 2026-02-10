package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtlprog/sloptask/internal/domain"
)

// WorkspaceRepository handles database operations for workspaces.
type WorkspaceRepository struct {
	pool *pgxpool.Pool
}

// NewWorkspaceRepository creates a new WorkspaceRepository.
func NewWorkspaceRepository(pool *pgxpool.Pool) *WorkspaceRepository {
	return &WorkspaceRepository{pool: pool}
}

// GetByID retrieves a workspace by ID.
func (r *WorkspaceRepository) GetByID(ctx context.Context, workspaceID string) (*domain.Workspace, error) {
	query, args, err := psql.
		Select("id", "name", "slug", "status_deadlines", "created_at").
		From("workspaces").
		Where(sq.Eq{"id": workspaceID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build GetByID query for workspace %s: %w", workspaceID, err)
	}

	var workspace domain.Workspace
	var statusDeadlinesJSON []byte

	err = r.pool.QueryRow(ctx, query, args...).Scan(
		&workspace.ID,
		&workspace.Name,
		&workspace.Slug,
		&statusDeadlinesJSON,
		&workspace.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrWorkspaceNotFound
		}
		return nil, fmt.Errorf("query workspace: %w", err)
	}

	if err := json.Unmarshal(statusDeadlinesJSON, &workspace.StatusDeadlines); err != nil {
		return nil, fmt.Errorf("parse status_deadlines: %w", err)
	}

	return &workspace, nil
}
