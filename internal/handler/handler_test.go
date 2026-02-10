package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"

	"github.com/mtlprog/sloptask/internal/database"
	"github.com/mtlprog/sloptask/internal/handler"
	"github.com/mtlprog/sloptask/internal/handler/dto"
	"github.com/mtlprog/sloptask/internal/middleware"
	"github.com/mtlprog/sloptask/internal/repository"
)

type HandlerTestSuite struct {
	suite.Suite
	pool    *pgxpool.Pool
	handler *handler.Handler

	// Test fixtures
	workspaceID string
	agent1ID    string
	agent1Token string
	agent2ID    string
	agent2Token string
}

func (s *HandlerTestSuite) SetupSuite() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://sloptask:sloptask@localhost:5432/sloptask?sslmode=disable"
	}

	ctx := context.Background()
	db, err := database.New(ctx, databaseURL)
	s.Require().NoError(err)
	s.pool = db.Pool()

	err = database.RunMigrations(ctx, s.pool)
	s.Require().NoError(err)

	s.handler = handler.New(s.pool)
}

func (s *HandlerTestSuite) SetupTest() {
	ctx := context.Background()

	// TRUNCATE all tables
	_, err := s.pool.Exec(ctx, "TRUNCATE workspaces, agents, tasks, task_events CASCADE")
	s.Require().NoError(err)

	// Create workspace
	_, err = s.pool.Exec(ctx, `
		INSERT INTO workspaces (id, name, slug, status_deadlines)
		VALUES ('00000000-0000-0000-0000-000000000001', 'Test Workspace', 'test',
				'{"NEW": 120, "IN_PROGRESS": 1440}'::jsonb)
	`)
	s.Require().NoError(err)
	s.workspaceID = "00000000-0000-0000-0000-000000000001"

	// Create agents
	_, err = s.pool.Exec(ctx, `
		INSERT INTO agents (id, workspace_id, name, token, is_active)
		VALUES
			('00000000-0000-0000-0000-000000000011', $1, 'agent-1', 'token-1', true),
			('00000000-0000-0000-0000-000000000012', $1, 'agent-2', 'token-2', true)
	`, s.workspaceID)
	s.Require().NoError(err)

	s.agent1ID = "00000000-0000-0000-0000-000000000011"
	s.agent1Token = "token-1"
	s.agent2ID = "00000000-0000-0000-0000-000000000012"
	s.agent2Token = "token-2"
}

func (s *HandlerTestSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}

// Helper to make authenticated request
func (s *HandlerTestSuite) makeRequest(method, path, token string, body interface{}) *httptest.ResponseRecorder {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyBytes, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(bodyBytes)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	mux := http.NewServeMux()

	// Register auth middleware and routes
	agentRepo := repository.NewAgentRepository(s.pool)
	authMiddleware := middleware.NewAuthMiddleware(agentRepo)
	s.handler.RegisterRoutes(mux)

	// Wrap with auth middleware
	authMiddleware.Authenticate(mux).ServeHTTP(w, req)

	return w
}

// Test 1: Unauthenticated request returns 401
func (s *HandlerTestSuite) TestCreateTask_Unauthorized() {
	reqBody := dto.CreateTaskRequest{
		Title:       "Test Task",
		Description: "Test description",
	}

	w := s.makeRequest("POST", "/api/v1/tasks", "", reqBody)

	s.Equal(http.StatusUnauthorized, w.Code)
}

// Test 2: Private task visibility check
func (s *HandlerTestSuite) TestGetTask_PrivateTaskUnauthorized() {
	ctx := context.Background()

	// Create private task by agent1
	var taskID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, visibility, status)
		VALUES ($1, 'Private Task', 'Secret', $2, 'private', 'NEW')
		RETURNING id
	`, s.workspaceID, s.agent1ID).Scan(&taskID)
	s.Require().NoError(err)

	// Agent2 tries to access
	w := s.makeRequest("GET", "/api/v1/tasks/"+taskID, s.agent2Token, nil)

	s.Equal(http.StatusForbidden, w.Code)
}

// Test 3: Private task NOT in list for unauthorized agent
func (s *HandlerTestSuite) TestListTasks_PrivateTaskFiltered() {
	ctx := context.Background()

	// Agent1 creates private task
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, visibility, status)
		VALUES ($1, 'Private Task', 'Secret', $2, 'private', 'NEW')
	`, s.workspaceID, s.agent1ID)
	s.Require().NoError(err)

	// Agent1 creates public task
	_, err = s.pool.Exec(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, visibility, status)
		VALUES ($1, 'Public Task', 'Public', $2, 'public', 'NEW')
	`, s.workspaceID, s.agent1ID)
	s.Require().NoError(err)

	// Agent2 lists tasks
	w := s.makeRequest("GET", "/api/v1/tasks", s.agent2Token, nil)

	s.Equal(http.StatusOK, w.Code)

	var respBody dto.TasksListResponse
	err = json.NewDecoder(w.Body).Decode(&respBody)
	s.Require().NoError(err)

	// Should only see public task, not private
	s.Equal(1, respBody.Total)
	s.Equal("Public Task", respBody.Tasks[0].Title)
}

// Test 4: Validation error returns 422
func (s *HandlerTestSuite) TestCreateTask_ValidationError() {
	reqBody := dto.CreateTaskRequest{
		Title:       "Bad",  // Too short (< 5 chars)
		Description: "Test",
	}

	w := s.makeRequest("POST", "/api/v1/tasks", s.agent1Token, reqBody)

	s.Equal(http.StatusUnprocessableEntity, w.Code)

	var errResp dto.ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	s.Require().NoError(err)
	s.Equal("VALIDATION_ERROR", errResp.Error.Code)
}

// Test 5: Concurrent claims (race condition)
func (s *HandlerTestSuite) TestClaimTask_Concurrent() {
	ctx := context.Background()

	// Create task
	var taskID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, status)
		VALUES ($1, 'Test Task', 'Test', $2, 'NEW')
		RETURNING id
	`, s.workspaceID, s.agent1ID).Scan(&taskID)
	s.Require().NoError(err)

	// Two agents try to claim simultaneously
	var wg sync.WaitGroup
	results := make(chan int, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		token := s.agent1Token
		if i == 1 {
			token = s.agent2Token
		}

		go func(agentToken string) {
			defer wg.Done()

			reqBody := dto.ClaimTaskRequest{Comment: "Claiming"}
			w := s.makeRequest("POST", "/api/v1/tasks/"+taskID+"/claim", agentToken, reqBody)
			results <- w.Code
		}(token)
	}

	wg.Wait()
	close(results)

	// Exactly one should succeed (200), other should fail (409)
	codes := []int{}
	for code := range results {
		codes = append(codes, code)
	}

	s.True((codes[0] == http.StatusOK && codes[1] == http.StatusConflict) ||
		(codes[0] == http.StatusConflict && codes[1] == http.StatusOK))
}

// Test 6: SQL injection in sort parameter (should be blocked)
func (s *HandlerTestSuite) TestListTasks_SQLInjectionBlocked() {
	ctx := context.Background()

	// Create a task
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, status)
		VALUES ($1, 'Test Task', 'Test', $2, 'NEW')
	`, s.workspaceID, s.agent1ID)
	s.Require().NoError(err)

	// Try SQL injection via sort parameter
	w := s.makeRequest("GET", "/api/v1/tasks?sort=created_at;DROP+TABLE+tasks;--", s.agent1Token, nil)

	// Should succeed (injection blocked)
	s.Equal(http.StatusOK, w.Code)

	// Verify tasks table still exists
	var count int
	err = s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM tasks").Scan(&count)
	s.NoError(err)
	s.Equal(1, count)
}

// Test 7: Blocker error handling
func (s *HandlerTestSuite) TestGetTask_BlockerErrorHandling() {
	ctx := context.Background()

	// Create task with non-existent blocker ID (should handle gracefully)
	nonExistentBlockerID := "99999999-9999-9999-9999-999999999999"
	var taskID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, status, blocked_by)
		VALUES ($1, 'Test Task', 'Test', $2, 'NEW', ARRAY[$3]::uuid[])
		RETURNING id
	`, s.workspaceID, s.agent1ID, nonExistentBlockerID).Scan(&taskID)
	s.Require().NoError(err)

	// Get task should still work (blocker won't be found but should not crash)
	w := s.makeRequest("GET", "/api/v1/tasks/"+taskID, s.agent1Token, nil)

	s.Equal(http.StatusOK, w.Code)

	var respBody dto.TaskDetailResponse
	err = json.NewDecoder(w.Body).Decode(&respBody)
	s.Require().NoError(err)

	// Blocker doesn't exist, so no blockers found = false
	// (The error handling logs but doesn't fail the request)
	s.False(respBody.Task.HasUnresolvedBlockers)
}

// Test 8: Agent can see private task they created
func (s *HandlerTestSuite) TestListTasks_PrivateTaskVisibleToCreator() {
	ctx := context.Background()

	// Agent1 creates private task
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, visibility, status)
		VALUES ($1, 'Private Task', 'Secret', $2, 'private', 'NEW')
	`, s.workspaceID, s.agent1ID)
	s.Require().NoError(err)

	// Agent1 lists tasks
	w := s.makeRequest("GET", "/api/v1/tasks", s.agent1Token, nil)

	s.Equal(http.StatusOK, w.Code)

	var respBody dto.TasksListResponse
	err = json.NewDecoder(w.Body).Decode(&respBody)
	s.Require().NoError(err)

	// Should see their private task
	s.Equal(1, respBody.Total)
	s.Equal("Private Task", respBody.Tasks[0].Title)
}

// Test 9: Agent can see private task they're assigned to
func (s *HandlerTestSuite) TestListTasks_PrivateTaskVisibleToAssignee() {
	ctx := context.Background()

	// Agent1 creates private task assigned to Agent2
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tasks (workspace_id, title, description, creator_id, assignee_id, visibility, status)
		VALUES ($1, 'Private Task', 'Secret', $2, $3, 'private', 'IN_PROGRESS')
	`, s.workspaceID, s.agent1ID, s.agent2ID)
	s.Require().NoError(err)

	// Agent2 lists tasks (should see it because they're assignee)
	w := s.makeRequest("GET", "/api/v1/tasks", s.agent2Token, nil)

	s.Equal(http.StatusOK, w.Code)

	var respBody dto.TasksListResponse
	err = json.NewDecoder(w.Body).Decode(&respBody)
	s.Require().NoError(err)

	// Should see the private task
	s.Equal(1, respBody.Total)
	s.Equal("Private Task", respBody.Tasks[0].Title)
}
