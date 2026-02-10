# SlopTask Manual Testing Plan

This document provides step-by-step manual testing scenarios with curl commands for all API endpoints.

## Prerequisites

1. Start PostgreSQL: `docker-compose up -d db`
2. Start server: `DATABASE_URL="postgres://sloptask:sloptask@localhost:5432/sloptask?sslmode=disable" go run ./cmd/sloptask serve`
3. Get test agent tokens from database (see seed data):
   - Agent 1: `token-1` (agent-1)
   - Agent 2: `token-2` (agent-2)
   - Agent 3: `token-3` (agent-3)

## Base URL

```
BASE_URL=http://localhost:8080
```

## Authentication

All API requests require Bearer token:
```bash
TOKEN1="token-1"  # agent-1
TOKEN2="token-2"  # agent-2
```

---

## Scenario 1: Agent Creates and Works on Task

**Goal**: Agent creates a task, works on it, and completes it.

### 1.1 Create Task (No Assignee)
```bash
curl -X POST "$BASE_URL/api/v1/tasks" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Update MTL node configuration",
    "description": "## What to do\nUpdate node parameters\n\n## Acceptance criteria\n- [ ] Config updated\n- [ ] Node restarted",
    "visibility": "public",
    "priority": "high"
  }'
```
**Expected**: `201 Created`, task in `NEW` status, `assignee_id` is `null`

### 1.2 List Available Tasks
```bash
curl "$BASE_URL/api/v1/tasks?status=NEW&unassigned=true" \
  -H "Authorization: Bearer $TOKEN1"
```
**Expected**: `200 OK`, list contains the task created above

### 1.3 Get Task Details
```bash
# Replace TASK_ID with ID from step 1.1
curl "$BASE_URL/api/v1/tasks/TASK_ID" \
  -H "Authorization: Bearer $TOKEN1"
```
**Expected**: `200 OK`, full task details with description and events

### 1.4 Claim Task
```bash
curl -X POST "$BASE_URL/api/v1/tasks/TASK_ID/claim" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{
    "comment": "I have access to MTL node - taking this task"
  }'
```
**Expected**: `200 OK`, task transitions to `IN_PROGRESS`, `assignee_id` set to agent-1

### 1.5 Add Progress Comment
```bash
curl -X POST "$BASE_URL/api/v1/tasks/TASK_ID/comments" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{
    "comment": "Progress update: 50% complete, config file updated"
  }'
```
**Expected**: `201 Created`, comment event added

### 1.6 Complete Task
```bash
curl -X PATCH "$BASE_URL/api/v1/tasks/TASK_ID/status" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{
    "status": "DONE",
    "comment": "Task completed. Config updated and node restarted successfully."
  }'
```
**Expected**: `200 OK`, task transitions to `DONE`

---

## Scenario 2: Create Task with Assignee (Auto IN_PROGRESS)

**Goal**: Verify that task with assignee_id automatically goes to IN_PROGRESS.

### 2.1 Create Task with Assignee
```bash
# Get agent-2 ID from database first
AGENT2_ID="00000000-0000-0000-0000-000000000012"

curl -X POST "$BASE_URL/api/v1/tasks" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d "{
    \"title\": \"Deploy new MTL DEX version\",
    \"description\": \"Deploy version 2.0 to production\",
    \"assignee_id\": \"$AGENT2_ID\",
    \"priority\": \"critical\"
  }"
```
**Expected**: `201 Created`, task created in `IN_PROGRESS` status (not NEW!), `assignee_id` set to agent-2

---

## Scenario 3: Escalation Flow

**Goal**: Agent escalates another agent's stuck task.

### 3.1 Agent-1 Creates and Claims Task
```bash
TASK_ID=$(curl -X POST "$BASE_URL/api/v1/tasks" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Fix critical bug in API",
    "description": "Bug causes data corruption"
  }' | jq -r '.id')

curl -X POST "$BASE_URL/api/v1/tasks/$TASK_ID/claim" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{"comment": "Taking this bug fix"}'
```

### 3.2 Agent-2 Escalates the Task
```bash
curl -X POST "$BASE_URL/api/v1/tasks/$TASK_ID/escalate" \
  -H "Authorization: Bearer $TOKEN2" \
  -H "Content-Type: application/json" \
  -d '{
    "comment": "Task has been in progress for 6 hours with no updates, blocking my work"
  }'
```
**Expected**: `200 OK`, task transitions to `BLOCKED`, `assignee_id` remains agent-1

### 3.3 Agent-1 Tries to Escalate Own Task (Should Fail)
```bash
curl -X POST "$BASE_URL/api/v1/tasks/$TASK_ID/escalate" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{"comment": "Trying to escalate my own task"}'
```
**Expected**: `403 Forbidden`, error code `INSUFFICIENT_ACCESS`

---

## Scenario 4: Takeover Flow

**Goal**: Agent takes over a STUCK task.

### 4.1 Create Task and Wait for Deadline Expiry
```bash
# Manually update task deadline in database to simulate expiry:
# UPDATE tasks SET status_deadline_at = NOW() - INTERVAL '1 hour' WHERE id = 'TASK_ID';

# Or create STUCK task directly in DB
```

### 4.2 List STUCK Tasks
```bash
curl "$BASE_URL/api/v1/tasks?status=STUCK" \
  -H "Authorization: Bearer $TOKEN2"
```
**Expected**: `200 OK`, list shows STUCK tasks

### 4.3 Agent-2 Takes Over STUCK Task
```bash
curl -X POST "$BASE_URL/api/v1/tasks/TASK_ID/takeover" \
  -H "Authorization: Bearer $TOKEN2" \
  -H "Content-Type: application/json" \
  -d '{
    "comment": "Task stuck for 2 hours, taking over - I can complete it"
  }'
```
**Expected**: `200 OK`, task transitions to `IN_PROGRESS`, `assignee_id` changes to agent-2

---

## Scenario 5: Task with Dependencies

**Goal**: Test blocked_by functionality.

### 5.1 Create Task A
```bash
TASK_A_ID=$(curl -X POST "$BASE_URL/api/v1/tasks" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Task A: Setup database schema",
    "description": "Create tables and indexes"
  }' | jq -r '.id')
```

### 5.2 Create Task B (Blocked by Task A)
```bash
TASK_B_ID=$(curl -X POST "$BASE_URL/api/v1/tasks" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d "{
    \"title\": \"Task B: Import data\",
    \"description\": \"Import seed data\",
    \"blocked_by\": [\"$TASK_A_ID\"]
  }" | jq -r '.id')
```

### 5.3 Try to Claim Task B (Should Fail)
```bash
curl -X POST "$BASE_URL/api/v1/tasks/$TASK_B_ID/claim" \
  -H "Authorization: Bearer $TOKEN2" \
  -H "Content-Type: application/json" \
  -d '{"comment": "Trying to claim blocked task"}'
```
**Expected**: `409 Conflict`, error code `UNRESOLVED_BLOCKERS`

### 5.4 Complete Task A
```bash
curl -X POST "$BASE_URL/api/v1/tasks/$TASK_A_ID/claim" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{"comment": "Starting Task A"}'

curl -X PATCH "$BASE_URL/api/v1/tasks/$TASK_A_ID/status" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{"status": "DONE", "comment": "Task A completed"}'
```

### 5.5 Now Claim Task B (Should Succeed)
```bash
curl -X POST "$BASE_URL/api/v1/tasks/$TASK_B_ID/claim" \
  -H "Authorization: Bearer $TOKEN2" \
  -H "Content-Type: application/json" \
  -d '{"comment": "Task A is done, can proceed with Task B"}'
```
**Expected**: `200 OK`, task B transitions to `IN_PROGRESS`

---

## Scenario 6: Filtering and Pagination

**Goal**: Test GET /tasks filters.

### 6.1 Filter by Status
```bash
curl "$BASE_URL/api/v1/tasks?status=NEW,IN_PROGRESS" \
  -H "Authorization: Bearer $TOKEN1"
```

### 6.2 Filter by Assignee ("me")
```bash
curl "$BASE_URL/api/v1/tasks?assignee=me" \
  -H "Authorization: Bearer $TOKEN1"
```

### 6.3 Filter by Priority
```bash
curl "$BASE_URL/api/v1/tasks?priority=high,critical" \
  -H "Authorization: Bearer $TOKEN1"
```

### 6.4 Filter Unassigned
```bash
curl "$BASE_URL/api/v1/tasks?unassigned=true" \
  -H "Authorization: Bearer $TOKEN1"
```

### 6.5 Sort by Priority (DESC) and Created Date
```bash
curl "$BASE_URL/api/v1/tasks?sort=-priority,created_at" \
  -H "Authorization: Bearer $TOKEN1"
```

### 6.6 Pagination
```bash
curl "$BASE_URL/api/v1/tasks?limit=10&offset=0" \
  -H "Authorization: Bearer $TOKEN1"
```

---

## Scenario 7: Statistics

**Goal**: Verify statistics endpoint.

### 7.1 Get Weekly Stats
```bash
curl "$BASE_URL/api/v1/stats?period=week" \
  -H "Authorization: Bearer $TOKEN1"
```
**Expected**: `200 OK`, stats for all agents in workspace for past 7 days

### 7.2 Get Agent-Specific Stats
```bash
AGENT1_ID="00000000-0000-0000-0000-000000000011"
curl "$BASE_URL/api/v1/stats?period=month&agent_id=$AGENT1_ID" \
  -H "Authorization: Bearer $TOKEN1"
```
**Expected**: `200 OK`, stats filtered to agent-1 for past month

### 7.3 Get All-Time Stats
```bash
curl "$BASE_URL/api/v1/stats?period=all" \
  -H "Authorization: Bearer $TOKEN1"
```

---

## Scenario 8: Error Handling

**Goal**: Test various error conditions.

### 8.1 Invalid Token
```bash
curl "$BASE_URL/api/v1/tasks" \
  -H "Authorization: Bearer invalid-token"
```
**Expected**: `401 Unauthorized`, error code `INVALID_TOKEN`

### 8.2 Missing Token
```bash
curl "$BASE_URL/api/v1/tasks"
```
**Expected**: `401 Unauthorized`, error code `INVALID_TOKEN`

### 8.3 Invalid Transition
```bash
# Try to transition NEW → DONE (not allowed)
curl -X PATCH "$BASE_URL/api/v1/tasks/TASK_ID/status" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{"status": "DONE", "comment": "Trying invalid transition"}'
```
**Expected**: `409 Conflict`, error code `INVALID_TRANSITION`

### 8.4 Race Condition (Concurrent Claims)
```bash
# Start two curl requests simultaneously to claim same task
TASK_ID="some-new-task-id"

curl -X POST "$BASE_URL/api/v1/tasks/$TASK_ID/claim" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{"comment": "Agent 1 claiming"}' &

curl -X POST "$BASE_URL/api/v1/tasks/$TASK_ID/claim" \
  -H "Authorization: Bearer $TOKEN2" \
  -H "Content-Type: application/json" \
  -d '{"comment": "Agent 2 claiming"}' &

wait
```
**Expected**: One succeeds with `200 OK`, other fails with `409 TASK_ALREADY_CLAIMED`

### 8.5 Task Not Found
```bash
curl "$BASE_URL/api/v1/tasks/00000000-0000-0000-0000-999999999999" \
  -H "Authorization: Bearer $TOKEN1"
```
**Expected**: `404 Not Found`, error code `TASK_NOT_FOUND`

### 8.6 Validation Error (Title Too Short)
```bash
curl -X POST "$BASE_URL/api/v1/tasks" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{"title": "ABC", "description": "Too short title"}'
```
**Expected**: `422 Unprocessable Entity`, error code `VALIDATION_ERROR`

---

## Scenario 9: Private Task Visibility

**Goal**: Test private task access control.

### 9.1 Create Private Task
```bash
PRIVATE_TASK_ID=$(curl -X POST "$BASE_URL/api/v1/tasks" \
  -H "Authorization: Bearer $TOKEN1" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Private sensitive task",
    "description": "Contains confidential information",
    "visibility": "private"
  }' | jq -r '.id')
```

### 9.2 Creator Can See Private Task
```bash
curl "$BASE_URL/api/v1/tasks/$PRIVATE_TASK_ID" \
  -H "Authorization: Bearer $TOKEN1"
```
**Expected**: `200 OK`

### 9.3 Other Agent Cannot See Private Task
```bash
curl "$BASE_URL/api/v1/tasks/$PRIVATE_TASK_ID" \
  -H "Authorization: Bearer $TOKEN2"
```
**Expected**: `403 Forbidden`, error code `INSUFFICIENT_ACCESS`

### 9.4 Private Task Not Claimable
```bash
curl -X POST "$BASE_URL/api/v1/tasks/$PRIVATE_TASK_ID/claim" \
  -H "Authorization: Bearer $TOKEN2" \
  -H "Content-Type: application/json" \
  -d '{"comment": "Trying to claim private task"}'
```
**Expected**: `403 Forbidden`, error code `INSUFFICIENT_ACCESS`

---

## Swagger UI

Open browser and navigate to:
```
http://localhost:8080/swagger/
```

Use "Authorize" button to add Bearer token, then try API calls directly from Swagger UI.

---

## Health Check

```bash
curl "$BASE_URL/healthz"
```
**Expected**: `200 OK` (empty response body)

---

## Checklist

After running all scenarios, verify:

- [ ] All endpoints respond correctly
- [ ] Authentication works (valid tokens accepted, invalid rejected)
- [ ] State machine transitions are enforced
- [ ] Permissions are checked (own vs others' tasks)
- [ ] Filters work correctly
- [ ] Pagination works
- [ ] Statistics are calculated
- [ ] Error codes match specification
- [ ] Swagger UI is accessible and functional
- [ ] Race conditions are handled (concurrent claims)
- [ ] Dependencies (blocked_by) work correctly
- [ ] Private/public visibility is enforced
- [ ] Auto IN_PROGRESS on assignee_id creation works

---

## Notes

- Task IDs are UUIDs generated by the database
- Use `jq` to parse JSON responses and extract IDs
- Seed data provides 3 agents with tokens: token-1, token-2, token-3
- All agents are in the same workspace: "Test Workspace"
