# State Machine Implementation Guide

## Overview

The task state machine has been fully implemented in the service layer. This document describes the architecture and how to use it.

## Architecture

### Layers

```
internal/
├── domain/              # Domain types and errors
│   ├── task.go         # Task, TaskStatus, TaskVisibility, TaskPriority
│   ├── agent.go        # Agent
│   ├── workspace.go    # Workspace
│   ├── task_event.go   # TaskEvent, EventType
│   └── errors.go       # Domain-specific errors
│
├── repository/         # Data access layer (SQL with squirrel)
│   ├── task.go         # Task CRUD operations
│   ├── task_event.go   # TaskEvent creation
│   ├── agent.go        # Agent queries
│   └── workspace.go    # Workspace queries
│
├── service/            # Business logic and state machine
│   ├── task_service.go # Main service coordinating operations
│   ├── validator.go    # Permission and state validation
│   └── deadline.go     # Deadline calculation
│
└── middleware/         # HTTP middleware
    └── auth.go         # Bearer token authentication
```

## State Machine Operations

### 1. Claim Task (NEW → IN_PROGRESS)

Agent proactively takes a free task:

```go
event, err := taskService.ClaimTask(
    ctx,
    taskID,
    agentID,
    "Taking this task - I have the required access",
)
```

**Validations:**
- Task must be in NEW status
- Task must not have assignee
- Task must be public
- All blocked_by tasks must be DONE
- Agent must be active and in same workspace

**Side effects:**
- Sets assignee_id to agent
- Changes status to IN_PROGRESS
- Calculates and sets status_deadline_at
- Creates TaskEvent (type: claimed)

### 2. Escalate Task (IN_PROGRESS → BLOCKED)

Agent blocks someone else's stuck task:

```go
event, err := taskService.EscalateTask(
    ctx,
    taskID,
    agentID,
    "Task has been stuck for 6 hours, needs attention",
)
```

**Validations:**
- Task must be in IN_PROGRESS status
- Agent cannot escalate own task
- Agent must be in same workspace

**Side effects:**
- Changes status to BLOCKED
- Keeps assignee_id (doesn't change)
- Recalculates status_deadline_at
- Creates TaskEvent (type: escalated)

### 3. Takeover Task (STUCK → IN_PROGRESS)

Agent takes over an abandoned STUCK task:

```go
event, err := taskService.TakeoverTask(
    ctx,
    taskID,
    agentID,
    "Taking over - previous agent didn't complete",
)
```

**Validations:**
- Task must be in STUCK status
- Agent cannot takeover own task
- All blocked_by tasks must be DONE
- Agent must be in same workspace

**Side effects:**
- Sets assignee_id to new agent
- Changes status to IN_PROGRESS
- Calculates and sets status_deadline_at
- Creates TaskEvent (type: taken_over)

### 4. Regular Status Transition

For standard status changes:

```go
event, err := taskService.TransitionStatus(
    ctx,
    taskID,
    agentID,
    domain.TaskStatusDone,
    "Task completed successfully",
)
```

**Validations:**
- Transition must be allowed by state machine rules
- Agent must have permission (creator/assignee/any based on transition)
- For IN_PROGRESS: checks blocked_by and cyclic dependencies

**Side effects:**
- Changes task status
- Clears assignee_id if transitioning to NEW
- Recalculates status_deadline_at
- Creates TaskEvent (type: status_changed)

### 5. Process Expired Deadlines

Background job to move expired tasks to STUCK:

```go
count, err := taskService.ProcessExpiredDeadlines(ctx)
```

**Process:**
- Finds all tasks with status_deadline_at < now()
- For each task:
  - Changes status to STUCK
  - Clears status_deadline_at (STUCK has no deadline)
  - Creates system TaskEvent (type: deadline_expired, actor_id: nil)

## CLI Integration

### Run Deadline Checker

```bash
./bin/sloptask check-deadlines --database-url="postgres://..."
```

This command:
1. Connects to database
2. Runs migrations
3. Initializes repositories and service
4. Calls ProcessExpiredDeadlines()
5. Logs number of tasks updated

## Testing

### Running Integration Tests

```bash
# Start database
docker-compose up -d db

# Run tests
go test ./internal/service -v

# Or use make
make test
```

Tests use real PostgreSQL database and testify suite:
- SetupSuite: Connect to DB, run migrations
- SetupTest: Clean tables, create test fixtures (workspace, agents)
- Test methods: Test each operation and validation
- TearDownSuite: Close DB connection

### Test Coverage

Current tests cover:
- ✅ ClaimTask success and failure cases
- ✅ EscalateTask success and failure cases
- ✅ TakeoverTask success and failure cases
- ✅ TransitionStatus valid and invalid transitions
- ✅ ProcessExpiredDeadlines
- ✅ Unresolved blockers validation
- ✅ Permission checks

## Error Handling

All operations return domain-specific errors:

```go
var (
    ErrTaskNotFound       = errors.New("task not found")
    ErrTaskAlreadyClaimed = errors.New("task already claimed")
    ErrInvalidTransition  = errors.New("invalid status transition")
    ErrUnresolvedBlockers = errors.New("task has unresolved blockers")
    ErrCyclicDependency   = errors.New("cyclic dependency detected")
    ErrPermissionDenied   = errors.New("permission denied")
    ErrAgentInactive      = errors.New("agent is inactive")
    // ... more in domain/errors.go
)
```

Check errors with `errors.Is()`:

```go
if errors.Is(err, domain.ErrTaskAlreadyClaimed) {
    // Handle race condition
}
```

## Optimistic Locking

The implementation uses optimistic locking for race conditions:

```sql
UPDATE tasks
SET status = $1, assignee_id = $2, ...
WHERE id = $3 AND status = $4  -- Check old status
```

If `RowsAffected() == 0`, the task was modified by another agent → returns `ErrTaskAlreadyClaimed`.

## Transaction Guarantees

Each operation runs in a single transaction:
1. Begin transaction
2. Read task
3. Validate
4. Update task
5. Create TaskEvent
6. Commit

If any step fails, the entire operation rolls back.

## Middleware

### Authentication Middleware

```go
authMiddleware := middleware.NewAuthMiddleware(agentRepo)

// Wrap handlers
mux.Handle("/api/v1/tasks", authMiddleware.Authenticate(handler))

// Get agent from context
agent, err := middleware.GetAgentFromContext(r.Context())
```

Validates Bearer token and adds agent to request context.

## Next Steps

To use this state machine in HTTP handlers:

1. Create handler methods (e.g., `handleClaimTask`)
2. Extract authenticated agent from context
3. Call TaskService methods
4. Return appropriate HTTP status codes and JSON responses
5. Map domain errors to HTTP status codes:
   - ErrTaskNotFound → 404
   - ErrTaskAlreadyClaimed → 409
   - ErrPermissionDenied → 403
   - ErrUnresolvedBlockers → 409
   - etc.

Example:

```go
func (h *Handler) handleClaimTask(w http.ResponseWriter, r *http.Request) {
    agent, _ := middleware.GetAgentFromContext(r.Context())
    taskID := r.PathValue("id")

    var req struct {
        Comment string `json:"comment"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    event, err := h.taskService.ClaimTask(r.Context(), taskID, agent.ID, req.Comment)
    if err != nil {
        if errors.Is(err, domain.ErrTaskAlreadyClaimed) {
            http.Error(w, "task already claimed", http.StatusConflict)
            return
        }
        // ... handle other errors
    }

    json.NewEncoder(w).Encode(event)
}
```
