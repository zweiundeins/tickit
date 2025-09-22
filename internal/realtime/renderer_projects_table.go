package realtime

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/web/ui/components"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/mbolli/tickit/internal/projects"
	"github.com/mbolli/tickit/internal/users"
)

// ProjectsTableRenderer renders projects table updates
type ProjectsTableRenderer struct {
	projectService *projects.ProjectService
	userService    *users.UserService
}

// NewProjectsTableRenderer creates a new projects table renderer
func NewProjectsTableRenderer(
	projectService *projects.ProjectService,
	userService *users.UserService,
) *ProjectsTableRenderer {
	return &ProjectsTableRenderer{
		projectService: projectService,
		userService:    userService,
	}
}

// RenderUpdate renders an updated projects table
func (r *ProjectsTableRenderer) RenderUpdate(ctx context.Context, data interface{}) (datastar.TemplComponent, map[string]any, error) {
	//groupData, ok := data.(*GroupUpdateData)
	//if !ok {
	//	return nil, fmt.Errorf("invalid data type for projects table renderer")
	//}

	// Extract pagination from group
	//page := 1
	//pageSize := 10
	//if p, ok := groupData.Filter["page"].(float64); ok {
	//	page = int(p)
	//}
	//if ps, ok := groupData.Filter["page_size"].(float64); ok {
	//	pageSize = int(ps)
	//}

	//// Build filter criteria
	//filter := &projects.Filter{}
	//if filterData, ok := groupData.Filter["filter"].(map[string]interface{}); ok {
	//	if status, ok := filterData["status"].(string); ok && status != "" {
	//		filter.Status = &status
	//	}
	//	if ownerID, ok := filterData["owner_id"].(string); ok && ownerID != "" {
	//		filter.OwnerID = &ownerID
	//	}
	//}

	// Fetch paginated projects
	result, err := r.projectService.ListProjects(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch projects: %w", err)
	}

	//// Get related users for rendering
	//userIDs := make([]string, 0)
	//for _, project := range result.Projects {
	//	if project.OwnerID != "" {
	//		userIDs = append(userIDs, project.OwnerID)
	//	}
	//}
	//
	//usersMap := make(map[string]*users.User)
	//if len(userIDs) > 0 {
	//	usersList, err := r.userService.GetUsersByID(ctx, userIDs)
	//	if err == nil {
	//		for _, user := range usersList {
	//			usersMap[user.ID] = user
	//		}
	//	}
	//}

	// Prepare signals for the table
	signals := map[string]any{
		"entityType": "projects",
	}

	// Render the projects table component
	return components.ProjectTable(result), signals, nil
}

// GetEntityType returns the entity type this renderer handles
func (r *ProjectsTableRenderer) GetEntityType() string {
	return "projects"
}
