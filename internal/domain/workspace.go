package domain

import "time"

// Workspace represents an isolated environment for a group of agents.
type Workspace struct {
	ID              string
	Name            string
	Slug            string
	StatusDeadlines map[string]int // status -> minutes
	CreatedAt       time.Time
}

// GetDeadlineMinutes returns the deadline in minutes for a given status.
// Returns 0 if the status has no deadline configured.
func (w *Workspace) GetDeadlineMinutes(status TaskStatus) int {
	if minutes, ok := w.StatusDeadlines[string(status)]; ok {
		return minutes
	}
	return 0
}
