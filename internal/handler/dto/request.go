package dto

// CreateTaskRequest represents the request body for POST /tasks.
type CreateTaskRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	AssigneeID  *string  `json:"assignee_id,omitempty"`
	Visibility  string   `json:"visibility,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
}

// TransitionStatusRequest represents the request body for PATCH /tasks/:id/status.
type TransitionStatusRequest struct {
	Status  string `json:"status"`
	Comment string `json:"comment"`
}

// ClaimTaskRequest represents the request body for POST /tasks/:id/claim.
type ClaimTaskRequest struct {
	Comment string `json:"comment"`
}

// EscalateTaskRequest represents the request body for POST /tasks/:id/escalate.
type EscalateTaskRequest struct {
	Comment string `json:"comment"`
}

// TakeoverTaskRequest represents the request body for POST /tasks/:id/takeover.
type TakeoverTaskRequest struct {
	Comment string `json:"comment"`
}

// CommentTaskRequest represents the request body for POST /tasks/:id/comments.
type CommentTaskRequest struct {
	Comment string `json:"comment"`
}

// ListTasksFilters represents query parameters for GET /tasks.
type ListTasksFilters struct {
	Status                []string // Multiple statuses: ?status=NEW,STUCK
	AssigneeID            *string  // ?assignee=<uuid> or ?assignee=me
	Unassigned            bool     // ?unassigned=true
	Visibility            *string  // ?visibility=public
	Priority              []string // ?priority=high,critical
	Overdue               bool     // ?overdue=true
	HasUnresolvedBlockers bool     // ?has_unresolved_blockers=true
	Sort                  []string // ?sort=-priority,created_at
	Limit                 int      // ?limit=50
	Offset                int      // ?offset=0
}

// StatsFilters represents query parameters for GET /stats.
type StatsFilters struct {
	Period  string  // day, week, month, all
	AgentID *string // Filter by specific agent
}
