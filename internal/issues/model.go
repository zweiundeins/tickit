package issues

import "time"

// Common Jira issue states as constants
const (
	IssueStatusToDo       = "To Do"
	IssueStatusInProgress = "In Progress"
	IssueStatusDone       = "Done"
	IssueStatusBacklog    = "Backlog"
	IssueStatusSelected   = "Selected for Development"
	IssueStatusReview     = "In Review"
	IssueStatusBlocked    = "Blocked"
)

// Exported slice of all allowed issue statuses
var IssueStatuses = []string{
	IssueStatusToDo,
	IssueStatusInProgress,
	IssueStatusDone,
	IssueStatusBacklog,
	IssueStatusSelected,
	IssueStatusReview,
	IssueStatusBlocked,
}

// Issue represents a minimal Jira-like issue
// Extendable for more fields (e.g. labels, comments, etc.)
type Issue struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // see IssueStatus* constants
	Assignee    string    `json:"assignee"`
	Reporter    string    `json:"reporter"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
