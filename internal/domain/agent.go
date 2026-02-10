package domain

import "time"

// Agent represents an AI agent registered in the system.
type Agent struct {
	ID          string
	WorkspaceID string
	Name        string
	Token       string
	IsActive    bool
	CreatedAt   time.Time
}
