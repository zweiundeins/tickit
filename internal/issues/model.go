package issues

import (
	"strconv"
	"strings"
	"time"
)

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

// Common Jira issue priorities as constants
const (
	IssuePriorityLow      = "Low"
	IssuePriorityMedium   = "Medium"
	IssuePriorityHigh     = "High"
	IssuePriorityCritical = "Critical"
)

// Exported slice of all allowed issue priorities
var IssuePriorities = []string{
	IssuePriorityLow,
	IssuePriorityMedium,
	IssuePriorityHigh,
	IssuePriorityCritical,
}

// Issue represents a minimal Jira-like issue
// Extendable for more fields (e.g. labels, comments, etc.)
type Issue struct {
	ID          string    `json:"id"`
	Number      int       `json:"number"` // Per-project incrementing number
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`   // see IssueStatus* constants
	Priority    string    `json:"priority"` // see IssuePriority* constants
	Assignee    string    `json:"assignee"`
	Reporter    string    `json:"reporter"`
	ProjectID   string    `json:"project_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GetKey returns the human-readable key for the issue (e.g., PROJ-24242)
func (i *Issue) GetKey(projectSlug string) string {
	return strings.ToUpper(projectSlug) + "-" + strconv.Itoa(i.Number)
}

// PatchIssueInput is used for type-safe partial updates to an Issue
// Only non-nil fields will be updated
// ID is required
type PatchIssueInput struct {
	ID          string
	Title       *string
	Description *string
	Status      *string
	Priority    *string
	Assignee    *string
	Reporter    *string
	ProjectID   *string
}
