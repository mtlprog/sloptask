# SlopTask API Implementation TODO

## Phase 1: Discovery ✅
- [x] Create TODO list
- [x] Review requirements from docs/04-API.md
- [x] Review business logic from docs/SLOPTASK.md

## Phase 2: Codebase Exploration ✅
- [x] Launch code-explorer agents to understand existing patterns
- [x] Read key files identified by agents
- [x] Document current architecture and patterns

### Key Findings:
- Layered architecture: Handler → Service → Repository → Domain
- Auth middleware with Bearer token (internal/middleware/auth.go)
- Service layer has all operations ready (Claim, Escalate, Takeover, TransitionStatus)
- Domain errors for proper error handling
- Squirrel query builder for SQL
- testify suite for integration tests
- Handler currently only has pool, needs TaskService instance

## Phase 3: Clarifying Questions ✅
- [x] Identify underspecified aspects
- [x] Present questions to user
- [x] Wait for answers

### Decisions Made:
1. Statistics: простой подход через created_at (lead time), без поиска первого IN_PROGRESS
2. tasks_taken_over: через task_events с type='taken_over'
3. Filters: всегда SQL запросы (включая has_unresolved_blockers)
4. Events: JOIN с agents, показывать actor_id + actor_name
5. POST /tasks с assignee: **автоматический переход в IN_PROGRESS** (не NEW!)
6. Swagger: использовать `swaggo/http-swagger` пакет
7. Validation errors: простой формат с message
8. Manual testing: markdown файл с curl командами

## Phase 4: Architecture Design ✅
- [x] Design single straightforward approach based on existing patterns
- [x] Present detailed implementation plan
- [x] Get user approval

### Architecture Decisions:
- Handler struct expanded with service dependencies
- DTO layer with snake_case JSON tags
- Unified error response mapper
- SQL-based filters for all GET /tasks queries
- Auto-transition to IN_PROGRESS when assignee set on creation
- swaggo/swag for Swagger generation
- Integration tests with httptest
- Manual testing plan with curl commands

## Phase 5: Implementation ✅
- [x] Setup: DTO layer, error handling, dependencies
- [x] Handler struct with all services and repos
- [x] Helper functions (respondJSON, respondError)
- [x] Implement API endpoints:
  - [x] GET /api/v1/tasks - List tasks with filters
  - [x] POST /api/v1/tasks - Create task
  - [x] GET /api/v1/tasks/:id - Get task details
  - [x] PATCH /api/v1/tasks/:id/status - Change status
  - [x] POST /api/v1/tasks/:id/claim - Claim task
  - [x] POST /api/v1/tasks/:id/escalate - Escalate task
  - [x] POST /api/v1/tasks/:id/takeover - Takeover task
  - [x] POST /api/v1/tasks/:id/comments - Add comment
  - [x] GET /api/v1/stats - Get statistics (basic MVP version)
- [x] Add CreateTask method to service layer
- [x] Add List method in repository with all filters
- [x] Add GetAgentStats and GetWorkspaceStats in repository
- [x] Server starts and health check works
- [x] Add Swagger/OpenAPI documentation (generated, available at /swagger/)
- [ ] Write integration tests for all endpoints (can be done later)
- [x] Authentication middleware properly integrated

## Phase 6: Quality Review
- [ ] Launch code-reviewer agents (optional - can be done later)
- [ ] Review and address findings
- [ ] Run all tests (integration tests not implemented yet)

## Phase 7: Manual Testing Plan ✅
- [x] Create comprehensive manual testing plan from SLOPTASK.md scenarios
- [x] Plan covers all 9 scenarios from docs/SLOPTASK.md
- [x] Includes curl commands for all endpoints
- [x] Error handling scenarios included
- [x] Swagger UI instructions included
- [ ] Run application locally and execute all scenarios
- [ ] Document results

## Phase 8: MVP Ready
- [ ] Execute manual testing plan
- [ ] Verify all features work
- [ ] Final summary
