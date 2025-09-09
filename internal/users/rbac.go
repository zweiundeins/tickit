package users

import (
	"context"
	"fmt"
	"strings"
)

// Role constants for system-wide roles
const (
	RoleAdmin          = "Admin"
	RoleProjectManager = "ProjectManager"
	RoleDeveloper      = "Developer"
	RoleViewer         = "Viewer"
)

// Project-specific role constants
const (
	ProjectRoleAdmin     = "ProjectAdmin"
	ProjectRoleDeveloper = "Developer"
	ProjectRoleViewer    = "Viewer"
)

// Permission constants
const (
	// System-wide permissions
	PermissionManageUsers    = "manage_users"
	PermissionManageProjects = "manage_projects"
	PermissionViewAllData    = "view_all_data"
	PermissionSystemAdmin    = "system_admin"

	// Project-specific permissions
	PermissionCreateProject      = "create_project"
	PermissionDeleteProject      = "delete_project"
	PermissionManageProjectUsers = "manage_project_users"
	PermissionCreateIssue        = "create_issue"
	PermissionEditIssue          = "edit_issue"
	PermissionDeleteIssue        = "delete_issue"
	PermissionAssignIssue        = "assign_issue"
	PermissionViewProject        = "view_project"
	PermissionViewIssue          = "view_issue"
	PermissionCommentIssue       = "comment_issue"
)

// RoleDefinition defines a role and its permissions
type RoleDefinition struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Permissions  []string `json:"permissions"`
	IsSystemRole bool     `json:"is_system_role"`
}

// SystemRoles defines all available system roles
var SystemRoles = map[string]RoleDefinition{
	RoleAdmin: {
		Name:        RoleAdmin,
		Description: "System administrator with full access to all features and projects",
		Permissions: []string{
			PermissionSystemAdmin,
			PermissionManageUsers,
			PermissionManageProjects,
			PermissionViewAllData,
			PermissionCreateProject,
			PermissionDeleteProject,
			PermissionManageProjectUsers,
			PermissionCreateIssue,
			PermissionEditIssue,
			PermissionDeleteIssue,
			PermissionAssignIssue,
			PermissionViewProject,
			PermissionViewIssue,
			PermissionCommentIssue,
		},
		IsSystemRole: true,
	},
	RoleProjectManager: {
		Name:        RoleProjectManager,
		Description: "Can create projects and manage users within their projects",
		Permissions: []string{
			PermissionCreateProject,
			PermissionManageProjectUsers,
			PermissionCreateIssue,
			PermissionEditIssue,
			PermissionDeleteIssue,
			PermissionAssignIssue,
			PermissionViewProject,
			PermissionViewIssue,
			PermissionCommentIssue,
		},
		IsSystemRole: true,
	},
	RoleDeveloper: {
		Name:        RoleDeveloper,
		Description: "Can manage issues within assigned projects",
		Permissions: []string{
			PermissionCreateIssue,
			PermissionEditIssue,
			PermissionAssignIssue,
			PermissionViewProject,
			PermissionViewIssue,
			PermissionCommentIssue,
		},
		IsSystemRole: true,
	},
	RoleViewer: {
		Name:        RoleViewer,
		Description: "Read-only access to assigned projects",
		Permissions: []string{
			PermissionViewProject,
			PermissionViewIssue,
			PermissionCommentIssue,
		},
		IsSystemRole: true,
	},
}

// ProjectRoles defines all available project-specific roles
var ProjectRoles = map[string]RoleDefinition{
	ProjectRoleAdmin: {
		Name:        ProjectRoleAdmin,
		Description: "Project administrator with full access to the project",
		Permissions: []string{
			PermissionDeleteProject,
			PermissionManageProjectUsers,
			PermissionCreateIssue,
			PermissionEditIssue,
			PermissionDeleteIssue,
			PermissionAssignIssue,
			PermissionViewProject,
			PermissionViewIssue,
			PermissionCommentIssue,
		},
		IsSystemRole: false,
	},
	ProjectRoleDeveloper: {
		Name:        ProjectRoleDeveloper,
		Description: "Can manage issues within the project",
		Permissions: []string{
			PermissionCreateIssue,
			PermissionEditIssue,
			PermissionAssignIssue,
			PermissionViewProject,
			PermissionViewIssue,
			PermissionCommentIssue,
		},
		IsSystemRole: false,
	},
	ProjectRoleViewer: {
		Name:        ProjectRoleViewer,
		Description: "Read-only access to the project",
		Permissions: []string{
			PermissionViewProject,
			PermissionViewIssue,
			PermissionCommentIssue,
		},
		IsSystemRole: false,
	},
}

// RBACService provides role-based access control functionality
type RBACService struct {
	userStore UserStore
}

// NewRBACService creates a new RBAC service
func NewRBACService(userStore UserStore) *RBACService {
	return &RBACService{
		userStore: userStore,
	}
}

// ValidateSystemRole validates if a role is a valid system role
func (s *RBACService) ValidateSystemRole(role string) error {
	if _, exists := SystemRoles[role]; !exists {
		return fmt.Errorf("invalid system role: %s", role)
	}
	return nil
}

// ValidateProjectRole validates if a role is a valid project role
func (s *RBACService) ValidateProjectRole(role string) error {
	if _, exists := ProjectRoles[role]; !exists {
		return fmt.Errorf("invalid project role: %s", role)
	}
	return nil
}

// GetSystemRolePermissions returns the permissions for a system role
func (s *RBACService) GetSystemRolePermissions(role string) ([]string, error) {
	roleDefinition, exists := SystemRoles[role]
	if !exists {
		return nil, fmt.Errorf("invalid system role: %s", role)
	}
	return roleDefinition.Permissions, nil
}

// GetProjectRolePermissions returns the permissions for a project role
func (s *RBACService) GetProjectRolePermissions(role string) ([]string, error) {
	roleDefinition, exists := ProjectRoles[role]
	if !exists {
		return nil, fmt.Errorf("invalid project role: %s", role)
	}
	return roleDefinition.Permissions, nil
}

// GetUserSystemPermissions returns all system permissions for a user
func (s *RBACService) GetUserSystemPermissions(ctx context.Context, userID string) ([]string, error) {
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	permissions, err := s.GetSystemRolePermissions(user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to get role permissions: %w", err)
	}

	return permissions, nil
}

// GetUserProjectPermissions returns all project permissions for a user in a specific project
func (s *RBACService) GetUserProjectPermissions(ctx context.Context, userID, projectID string) ([]string, error) {
	// First check if user has system-level permissions that override project permissions
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Admin users have all permissions
	if user.Role == RoleAdmin {
		systemPermissions, _ := s.GetSystemRolePermissions(user.Role)
		return systemPermissions, nil
	}

	// Get project membership
	membership, err := s.userStore.GetProjectMembership(ctx, userID, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project membership: %w", err)
	}

	// Get project role permissions
	projectPermissions, err := s.GetProjectRolePermissions(membership.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to get project role permissions: %w", err)
	}

	// Combine with system role permissions if applicable
	systemPermissions, err := s.GetSystemRolePermissions(user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to get system role permissions: %w", err)
	}

	// Merge permissions (remove duplicates)
	permissionSet := make(map[string]bool)
	for _, perm := range systemPermissions {
		permissionSet[perm] = true
	}
	for _, perm := range projectPermissions {
		permissionSet[perm] = true
	}

	var allPermissions []string
	for perm := range permissionSet {
		allPermissions = append(allPermissions, perm)
	}

	return allPermissions, nil
}

// HasSystemPermission checks if a user has a specific system permission
func (s *RBACService) HasSystemPermission(ctx context.Context, userID, permission string) (bool, error) {
	permissions, err := s.GetUserSystemPermissions(ctx, userID)
	if err != nil {
		return false, err
	}

	for _, perm := range permissions {
		if perm == permission {
			return true, nil
		}
	}

	return false, nil
}

// HasProjectPermission checks if a user has a specific permission in a project
func (s *RBACService) HasProjectPermission(ctx context.Context, userID, projectID, permission string) (bool, error) {
	permissions, err := s.GetUserProjectPermissions(ctx, userID, projectID)
	if err != nil {
		return false, err
	}

	for _, perm := range permissions {
		if perm == permission {
			return true, nil
		}
	}

	return false, nil
}

// CanAccessProject checks if a user can access a project
func (s *RBACService) CanAccessProject(ctx context.Context, userID, projectID string) (bool, error) {
	// Admin users can access all projects
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}

	if user.Role == RoleAdmin {
		return true, nil
	}

	// Check project membership
	hasAccess, err := s.userStore.CheckUserProjectAccess(ctx, userID, projectID)
	if err != nil {
		return false, fmt.Errorf("failed to check project access: %w", err)
	}

	return hasAccess, nil
}

// CanManageUser checks if a user can manage another user
func (s *RBACService) CanManageUser(ctx context.Context, managerUserID, targetUserID string) (bool, error) {
	// Check if manager has system admin permission
	hasPermission, err := s.HasSystemPermission(ctx, managerUserID, PermissionManageUsers)
	if err != nil {
		return false, err
	}

	if hasPermission {
		return true, nil
	}

	// Project managers can manage users within their projects
	manager, err := s.userStore.GetUser(ctx, managerUserID)
	if err != nil {
		return false, fmt.Errorf("failed to get manager user: %w", err)
	}

	if manager.Role != RoleProjectManager {
		return false, nil
	}

	// Get projects where manager is a project admin or has management permissions
	managerProjects, err := s.userStore.GetUserProjects(ctx, managerUserID)
	if err != nil {
		return false, fmt.Errorf("failed to get manager projects: %w", err)
	}

	// Get target user's projects
	targetProjects, err := s.userStore.GetUserProjects(ctx, targetUserID)
	if err != nil {
		return false, fmt.Errorf("failed to get target user projects: %w", err)
	}

	// Check if there's any overlap where manager has admin role
	for _, managerProject := range managerProjects {
		if managerProject.Role == ProjectRoleAdmin {
			for _, targetProject := range targetProjects {
				if managerProject.ProjectID == targetProject.ProjectID {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// GetAvailableSystemRoles returns all available system roles
func (s *RBACService) GetAvailableSystemRoles() []RoleDefinition {
	var roles []RoleDefinition
	for _, role := range SystemRoles {
		roles = append(roles, role)
	}
	return roles
}

// GetAvailableProjectRoles returns all available project roles
func (s *RBACService) GetAvailableProjectRoles() []RoleDefinition {
	var roles []RoleDefinition
	for _, role := range ProjectRoles {
		roles = append(roles, role)
	}
	return roles
}

// IsValidRoleTransition checks if a role transition is valid
func (s *RBACService) IsValidRoleTransition(fromRole, toRole string) bool {
	// Admin can change to any role
	if fromRole == RoleAdmin {
		return true
	}

	// Project managers can only be demoted to developer or viewer
	if fromRole == RoleProjectManager {
		return toRole == RoleDeveloper || toRole == RoleViewer
	}

	// Developers can be promoted to project manager or demoted to viewer
	if fromRole == RoleDeveloper {
		return toRole == RoleProjectManager || toRole == RoleViewer
	}

	// Viewers can be promoted to any role
	if fromRole == RoleViewer {
		return true
	}

	return false
}

// NormalizeRoleName normalizes role names to handle case variations
func NormalizeRoleName(role string) string {
	role = strings.TrimSpace(role)

	// Handle common variations
	switch strings.ToLower(role) {
	case "admin", "administrator", "system_admin":
		return RoleAdmin
	case "project_manager", "projectmanager", "pm", "manager":
		return RoleProjectManager
	case "developer", "dev", "engineer":
		return RoleDeveloper
	case "viewer", "read_only", "readonly", "guest":
		return RoleViewer
	case "project_admin", "projectadmin":
		return ProjectRoleAdmin
	case "project_developer", "projectdeveloper":
		return ProjectRoleDeveloper
	case "project_viewer", "projectviewer":
		return ProjectRoleViewer
	default:
		return role
	}
}
