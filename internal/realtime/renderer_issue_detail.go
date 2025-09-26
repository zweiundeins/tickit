package realtime

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/web/ui/components"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/projects"
	"github.com/mbolli/tickit/internal/users"
)

// IssueDetailRenderer renders issue detail updates for individual issue views
type IssueDetailRenderer struct {
	issueService   *issues.IssueService
	projectService *projects.ProjectService
	userService    *users.UserService
}

// NewIssueDetailRenderer creates a new issue detail renderer
func NewIssueDetailRenderer(
	issueService *issues.IssueService,
	projectService *projects.ProjectService,
	userService *users.UserService,
) *IssueDetailRenderer {
	return &IssueDetailRenderer{
		issueService:   issueService,
		projectService: projectService,
		userService:    userService,
	}
}

// RenderUpdate renders an updated issue detail view
func (r *IssueDetailRenderer) RenderUpdate(ctx context.Context, data interface{}) (datastar.TemplComponent, map[string]any, error) {
	groupData, ok := data.(*GroupUpdateData)
	if !ok {
		return nil, nil, fmt.Errorf("invalid data type for issue detail renderer")
	}

	if groupData.EntityID == nil {
		return nil, nil, fmt.Errorf("entity ID required for issue detail renderer")
	}

	// Fetch the specific issue
	issue, err := r.issueService.GetIssueByID(ctx, *groupData.EntityID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch issue: %w", err)
	}

	if issue == nil {
		logger.Error().Str("entity_id", *groupData.EntityID).Msg("IssueDetailRenderer: Issue is nil")
		return nil, nil, fmt.Errorf("issue not found with ID: %s", *groupData.EntityID)
	}

	logger.Debug().Str("issue_id", issue.ID).Str("issue_title", issue.Title).Msg("IssueDetailRenderer: Issue fetched successfully")

	// Fetch related project
	var projectsList []*projects.Project
	projectsList, _ = r.projectService.ListProjects(ctx) // todo remove
	if projectsList == nil {
		projectsList = []*projects.Project{}
	}

	// Fetch related users
	//var assignee, reporter *users.User
	//if issue.Assignee != "" {
	//	assignee, _ = r.userService.GetUserByID(ctx, issue.Assignee)
	//}
	//if issue.Reporter != "" {
	//	reporter, _ = r.userService.GetUserByID(ctx, issue.Reporter)
	//}

	// Prepare signals
	signals := map[string]any{
		"issue":      map[string]any{"id": issue.ID},
		"entityType": "issues",
		"entityId":   issue.ID,
	}

	// Render the issue detail component
	return components.Issue(issue, projectsList), signals, nil
}

// GetEntityType returns the entity type this renderer handles
func (r *IssueDetailRenderer) GetEntityType() string {
	return "issues"
}
