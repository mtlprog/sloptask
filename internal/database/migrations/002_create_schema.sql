-- +goose Up
-- Enable UUID generation extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Workspaces: isolated environments for groups of agents
CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE,
    status_deadlines JSONB NOT NULL DEFAULT '{"NEW": 120, "IN_PROGRESS": 1440, "BLOCKED": 2880}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE workspaces IS 'Isolated workspaces for groups of agents';

-- Agents: AI agents registered in the system
CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    token TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, name)
);

COMMENT ON TABLE agents IS 'AI agents registered in the system';

-- Partial index for active agent authentication (workspace_id covered by UNIQUE constraint)
CREATE INDEX idx_agents_token ON agents(token) WHERE is_active = true;

-- Tasks: units of work for agents
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL CHECK (char_length(title) >= 5 AND char_length(title) <= 200),
    description TEXT NOT NULL CHECK (char_length(description) > 0),
    creator_id UUID NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    assignee_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'NEW'
        CHECK (status IN ('NEW', 'IN_PROGRESS', 'BLOCKED', 'STUCK', 'DONE', 'CANCELLED')),
    visibility VARCHAR(10) NOT NULL DEFAULT 'public'
        CHECK (visibility IN ('public', 'private')),
    priority VARCHAR(10) NOT NULL DEFAULT 'normal'
        CHECK (priority IN ('low', 'normal', 'high', 'critical')),
    blocked_by UUID[] NOT NULL DEFAULT '{}',
    status_deadline_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE tasks IS 'Tasks for agents to work on';
COMMENT ON COLUMN tasks.blocked_by IS 'Array of task IDs that block this task (validated in application: same workspace, no cycles)';
COMMENT ON COLUMN tasks.status_deadline_at IS 'When current status expires (NULL for terminal statuses)';

-- Indexes for common queries (workspace_id covered by composite indexes below)
CREATE INDEX idx_tasks_creator_id ON tasks(creator_id);
CREATE INDEX idx_tasks_assignee_id ON tasks(assignee_id);
CREATE INDEX idx_tasks_status ON tasks(status);

-- Composite index for main agent queries (workspace + status + assignee)
CREATE INDEX idx_tasks_workspace_status_assignee ON tasks(workspace_id, status, assignee_id);

-- Index for visibility filtering
CREATE INDEX idx_tasks_workspace_visibility ON tasks(workspace_id, visibility);

-- Partial index for overdue tasks (deadline checker)
CREATE INDEX idx_tasks_overdue ON tasks(status_deadline_at)
    WHERE status_deadline_at IS NOT NULL AND status IN ('NEW', 'IN_PROGRESS', 'BLOCKED');

-- Task events: complete audit log
CREATE TABLE task_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    actor_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    type VARCHAR(20) NOT NULL
        CHECK (type IN ('created', 'status_changed', 'claimed', 'escalated', 'taken_over', 'commented', 'deadline_expired')),
    old_status VARCHAR(20)
        CHECK (old_status IN ('NEW', 'IN_PROGRESS', 'BLOCKED', 'STUCK', 'DONE', 'CANCELLED')),
    new_status VARCHAR(20)
        CHECK (new_status IN ('NEW', 'IN_PROGRESS', 'BLOCKED', 'STUCK', 'DONE', 'CANCELLED')),
    comment TEXT,
    CONSTRAINT comment_required_for_status_change
        CHECK (type != 'status_changed' OR comment IS NOT NULL),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE task_events IS 'Audit log of all task actions';

CREATE INDEX idx_task_events_task_id ON task_events(task_id);
CREATE INDEX idx_task_events_actor_id ON task_events(actor_id);
CREATE INDEX idx_task_events_created_at ON task_events(created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS task_events CASCADE;
DROP TABLE IF EXISTS tasks CASCADE;
DROP TABLE IF EXISTS agents CASCADE;
DROP TABLE IF EXISTS workspaces CASCADE;
DROP EXTENSION IF EXISTS "uuid-ossp";
