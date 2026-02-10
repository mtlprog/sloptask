# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SlopTask is a task tracker designed specifically for coordinating AI agents. It implements a state machine-driven workflow with automatic deadline management, proactive task claiming, escalation, and takeover mechanisms.

**Key Concepts:**
- **Workspaces** - Isolated environments for groups of agents
- **Agents** - AI agents with token-based authentication
- **Tasks** - Units of work with statuses: NEW → IN_PROGRESS → DONE (plus BLOCKED, STUCK, CANCELLED)
- **Task Events** - Complete audit log of all task actions
- **Deadline Management** - Automatic status expiration and transition to STUCK
- **Proactive Coordination** - Agents can claim free tasks, escalate stuck ones, and take over abandoned work

## Commands

```bash
# Development
make build              # Build to bin/sloptask
make run                # Build and run server (applies migrations automatically)
make clean              # Remove build artifacts
make deps               # Download and tidy dependencies

# Running
./bin/sloptask serve                    # Start HTTP server on port 8080
./bin/sloptask serve --port 3000        # Custom port
./bin/sloptask check-deadlines          # Run deadline checker (stub)

# Docker
docker-compose up -d db                 # Start PostgreSQL only
docker-compose up                       # Start all services
```

## Architecture

### Project Structure
```
cmd/sloptask/main.go           - CLI entry point, command definitions
internal/
├── config/                    - Configuration constants
├── database/
│   ├── database.go           - pgxpool connection management
│   ├── migrations.go         - goose migration runner (embedded)
│   └── migrations/*.sql      - SQL migrations (auto-applied on startup)
├── handler/                   - HTTP handlers (currently only healthz)
└── logger/                    - Structured logging setup (slog)
```

### Database Architecture

**PostgreSQL with pgx/v5:**
- Connection pooling via `pgxpool.Pool` (MaxConns: 10, MinConns: 2)
- Migrations via `goose/v3` with embedded SQL files
- All migrations automatically run on `serve` command startup

**Migration System:**
- Migrations live in `internal/database/migrations/*.sql`
- Embedded into binary via `//go:embed migrations/*.sql`
- Use goose format: `-- +goose Up` and `-- +goose Down`
- Migrations run automatically when app starts (both `serve` and `check-deadlines`)

**Current Schema (002_create_schema.sql):**
- `workspaces` - with JSONB status_deadlines
- `agents` - with unique tokens per workspace
- `tasks` - with status, priority, visibility, blocked_by array
- `task_events` - audit log with type, old/new status, comments

**Key Design Decisions:**
- UUID primary keys via `uuid-ossp` extension
- VARCHAR with CHECK constraints for enums (not PostgreSQL ENUMs)
- Business logic in application layer, NOT in database (no triggers, no stored procedures)
- Composite indexes for common queries: `(workspace_id, status, assignee_id)`
- Partial indexes for specific use cases (overdue tasks, active agents)

### CLI Architecture

Uses `urfave/cli/v2` with:
- Global flags: `--database-url`, `--log-level`
- Commands: `serve`, `check-deadlines`
- Graceful shutdown with signal handling
- Automatic migration on startup

## Documentation

The `docs/` directory contains the complete specification:
- **SLOPTASK.md** - Complete business logic (200+ pages), the canonical reference
- **02-DATABASE.md** - Database schema requirements
- **03-STATE-MACHINE.md** - Task status transitions and rules
- **04-API.md** - REST API endpoints (to be implemented)

When implementing features, ALWAYS refer to these docs first.

## Configuration

Environment variables (all optional except DATABASE_URL):
- `DATABASE_URL` - PostgreSQL connection string (required)
- `PORT` - HTTP server port (default: 8080)
- `LOG_LEVEL` - Logging level: debug, info, warn, error (default: info)

## Development Notes

### Adding Migrations

1. Create new file: `internal/database/migrations/00N_name.sql`
2. Use goose format with Up and Down sections
3. Keep business logic in Go code, not in SQL
4. Migrations auto-run on next `make run`

### Adding API Endpoints

1. Add handler method to `internal/handler/handler.go`
2. Register route in `RegisterRoutes()` method
3. Handler has access to `h.pool` (pgxpool.Pool)
4. Use pattern matching routes: `mux.HandleFunc("GET /api/v1/tasks", h.handleGetTasks)`

### Working with Database

- Use `h.pool.Query()` or `h.pool.QueryRow()` for queries
- Context from request: `ctx := r.Context()`
- No ORM - direct SQL queries via pgx
- Use parameterized queries: `$1, $2` placeholders

### Repository Layer

- Use `squirrel` for SQL query building - required to avoid duplication
- Pattern: `r.psql.Select().From().Where().ToSql()` then execute with pgx

### Service Layer Architecture

- Three layers: `domain/` (types, errors) → `repository/` (SQL) → `service/` (business logic)
- Optimistic locking: `UPDATE ... WHERE id = $1 AND status = $2` (check old status)
- One transaction per operation: begin → read → validate → update → create event → commit

### State Machine

- Manual implementation preferred over libraries (more control, better integration)
- Cycle detection runs on IN_PROGRESS transitions (not at dependency creation time)
- Domain-specific errors in `domain/errors.go` - use `errors.Is()` for checking

## Testing

Integration tests use testify suite with real PostgreSQL:
```bash
docker-compose up -d db        # Start database first
go test ./internal/service -v  # Run service layer tests
```

Test pattern: `testify/suite` with `SetupTest`/`TearDown` for clean fixtures between tests

## Tech Stack

- **Go 1.25+** - Backend language
- **PostgreSQL 16+** - Database
- **pgx/v5** - PostgreSQL driver (not database/sql)
- **goose/v3** - Database migrations
- **urfave/cli/v2** - CLI framework
- **slog** - Structured logging (standard library)
- **net/http** - HTTP server (standard library, no framework)

## Current Status

**Implemented:**
- ✅ Database schema with all tables
- ✅ Migration system
- ✅ Seed data (1 workspace, 3 agents)
- ✅ CLI with serve and check-deadlines commands
- ✅ Health check endpoint
- ✅ Graceful shutdown
- ✅ Task state machine (domain, repository, service layers)
- ✅ Authentication middleware (Bearer token)
- ✅ Deadline checker (ProcessExpiredDeadlines)
- ✅ Integration tests with testify suite (59.5% coverage)

**Not Yet Implemented:**
- ⏳ REST API endpoints (see docs/04-API.md)
- ⏳ Statistics endpoints
