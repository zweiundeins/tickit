package realtime

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/web/ui/components"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/projects"
	"github.com/mbolli/tickit/internal/users"
)

// IssuesTableRenderer renders issues table updates for dashboard views
type IssuesTableRenderer struct {
	issueService   *issues.IssueService
	projectService *projects.ProjectService
	userService    *users.UserService
}

// NewIssuesTableRenderer creates a new issues table renderer
func NewIssuesTableRenderer(
	issueService *issues.IssueService,
	projectService *projects.ProjectService,
	userService *users.UserService,
) *IssuesTableRenderer {
	return &IssuesTableRenderer{
		issueService:   issueService,
		projectService: projectService,
		userService:    userService,
	}
}

// UnifiedIssuesSignals - strongly typed signals for the unified endpoint
type UnifiedIssuesSignals struct {
	EntityType string                  `json:"entityType"`
	EntityID   *string                 `json:"entityId,omitempty"`
	Filter     issues.ListIssuesFilter `json:"filter"`
	OrderBy    string                  `json:"order_by,omitempty"`
	OrderDir   string                  `json:"order_dir,omitempty"`
}

// IssuesUpdatesSignals matches the existing pattern (legacy)
type IssuesUpdatesSignals struct {
	Filter   issues.ListIssuesFilter `json:"filter"`
	OrderBy  string                  `json:"order_by"`
	OrderDir string                  `json:"order_dir"`
}

// RenderUpdate renders an updated issues table based on group filter and pagination
func (r *IssuesTableRenderer) RenderUpdate(ctx context.Context, signals any) (datastar.TemplComponent, map[string]any, error) {
	var query issues.ListIssuesQuery

	logger.Debug().Type("signals", signals).Interface("signals", signals).Msg("IssuesTableRenderer: RenderUpdate called")

	// Handle different input types
	switch data := signals.(type) {
	case nil:
	case map[string]any:
		// No signals provided - set default query (e.g. all open issues)
		filter := issues.ListIssuesFilter{
			OrderBy:  "created_at",
			OrderDir: "desc",
			// Note: pagination (Limit, Offset) should be handled elsewhere in the system
		}
		query = filter.ToQuery()
		logger.Warn().Msg("IssuesTableRenderer: RenderUpdate called with nil signals - using default query for open issues")
	case *UnifiedIssuesSignals:
		// From the new unified /updates endpoint - strongly typed
		if data.EntityType == "issues" {
			query = data.Filter.ToQuery()
		}
	case *IssuesUpdatesSignals:
		// From legacy specific endpoints like /api/updates/issues
		query = data.Filter.ToQuery()
		logger.Error().Msg("IssuesTableRenderer: RenderUpdate called with legacy IssuesUpdatesSignals - please migrate to UnifiedIssuesSignals")
	case *GroupUpdateData:
		// Fallback for generic map-based data (temporary compatibility)
		if data.EntityType == "issues" {
			// Convert filter map to issues query
			filter := issues.ListIssuesFilter{}

			// Extract filter data from the map
			if projectID, ok := data.Filter["project_id"].(string); ok && projectID != "" {
				filter.ProjectID = projectID
			}
			if status, ok := data.Filter["status"].(string); ok && status != "" {
				filter.Status = status
			}
			if priority, ok := data.Filter["priority"].(string); ok && priority != "" {
				filter.Priority = priority
			}
			if assignee, ok := data.Filter["assignee_id"].(string); ok && assignee != "" {
				filter.Assignee = assignee
			}
			if containsText, ok := data.Filter["contains_text"].(string); ok && containsText != "" {
				filter.ContainsText = containsText
			}

			query = filter.ToQuery()
			// Note: pagination is handled elsewhere in the system, not in the query itself
		}
		logger.Error().Msg("IssuesTableRenderer: RenderUpdate called with generic GroupUpdateData - please migrate to UnifiedIssuesSignals")
	default:
		return nil, nil, fmt.Errorf("unsupported signals type: %T", signals)
	}
	logger.Debug().Interface("query", query).Msg("IssuesTableRenderer: RenderUpdate called with query")
	issuesList, err := r.issueService.ListIssues(ctx, query)
	if err == nil {
		// Collect unique project IDs
		projectIDs := make(map[string]struct{})
		userIDs := make(map[string]struct{})
		for _, issue := range issuesList {
			projectIDs[issue.ProjectID] = struct{}{}
			if issue.Reporter != "" {
				userIDs[issue.Reporter] = struct{}{}
			}
			if issue.Assignee != "" {
				userIDs[issue.Assignee] = struct{}{}
			}
		}
		// Fetch only needed projects
		projectMap := make(map[string]*projects.Project)
		for id := range projectIDs {
			if p, err := r.projectService.GetProjectByID(ctx, id); err == nil && p != nil {
				projectMap[id] = p
			}
		}
		// Fetch only needed users
		userMap := make(map[string]*components.UserInfo)
		for id := range userIDs {
			if u, err := r.userService.GetUserByID(ctx, id); err == nil && u != nil {
				userMap[id] = &components.UserInfo{
					ID:       u.ID,
					Username: u.Username,
				}
			}
		}

		// Prepare signals for the table
		signals := map[string]any{}

		return components.IssuesTable(issuesList, projectMap, userMap), signals, nil
	}
	return nil, nil, err
}

// GetEntityType returns the entity type this renderer handles
func (r *IssuesTableRenderer) GetEntityType() string {
	return "issues"
}
