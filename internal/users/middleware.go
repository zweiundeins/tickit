package users

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/sharedhttp"
)

// PermissionContext holds permission-related context data
type PermissionContext struct {
	UserID      string
	ProjectID   string
	Permission  string
	RBACService *RBACService
}

// RequireSystemPermission is middleware that requires a specific system permission
func RequireSystemPermission(store sessions.Store, rbacService *RBACService, permission string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// First ensure user is authenticated
			userID, authenticated := sharedhttp.GetAuthenticatedUser(store, r)
			if !authenticated {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Check if user has the required system permission
			hasPermission, err := rbacService.HasSystemPermission(r.Context(), userID, permission)
			if err != nil {
				http.Error(w, "Failed to check permissions", http.StatusInternalServerError)
				return
			}

			if !hasPermission {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}

			// Add permission context to request
			ctx := context.WithValue(r.Context(), "permission_context", &PermissionContext{
				UserID:      userID,
				Permission:  permission,
				RBACService: rbacService,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// RequireProjectPermission is middleware that requires a specific project permission
func RequireProjectPermission(store sessions.Store, rbacService *RBACService, permission string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// First ensure user is authenticated
			userID, authenticated := sharedhttp.GetAuthenticatedUser(store, r)
			if !authenticated {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Extract project ID from URL parameters
			projectID := chi.URLParam(r, "projectId")
			if projectID == "" {
				// Try alternative parameter names
				projectID = chi.URLParam(r, "project_id")
			}
			if projectID == "" {
				http.Error(w, "Project ID is required", http.StatusBadRequest)
				return
			}

			// Check if user has the required project permission
			hasPermission, err := rbacService.HasProjectPermission(r.Context(), userID, projectID, permission)
			if err != nil {
				if strings.Contains(err.Error(), "project membership") {
					http.Error(w, "Access denied: not a member of this project", http.StatusForbidden)
					return
				}
				http.Error(w, "Failed to check permissions", http.StatusInternalServerError)
				return
			}

			if !hasPermission {
				http.Error(w, "Insufficient permissions for this project", http.StatusForbidden)
				return
			}

			// Add permission context to request
			ctx := context.WithValue(r.Context(), "permission_context", &PermissionContext{
				UserID:      userID,
				ProjectID:   projectID,
				Permission:  permission,
				RBACService: rbacService,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// RequireProjectAccess is middleware that requires access to a project (any role)
func RequireProjectAccess(store sessions.Store, rbacService *RBACService) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// First ensure user is authenticated
			userID, authenticated := sharedhttp.GetAuthenticatedUser(store, r)
			if !authenticated {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Extract project ID from URL parameters
			projectID := chi.URLParam(r, "projectId")
			if projectID == "" {
				// Try alternative parameter names
				projectID = chi.URLParam(r, "project_id")
			}
			if projectID == "" {
				http.Error(w, "Project ID is required", http.StatusBadRequest)
				return
			}

			// Check if user can access the project
			canAccess, err := rbacService.CanAccessProject(r.Context(), userID, projectID)
			if err != nil {
				http.Error(w, "Failed to check project access", http.StatusInternalServerError)
				return
			}

			if !canAccess {
				http.Error(w, "Access denied: not a member of this project", http.StatusForbidden)
				return
			}

			// Add permission context to request
			ctx := context.WithValue(r.Context(), "permission_context", &PermissionContext{
				UserID:      userID,
				ProjectID:   projectID,
				RBACService: rbacService,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// RequireSystemRole is middleware that requires a specific system role
func RequireSystemRole(store sessions.Store, rbacService *RBACService, requiredRole string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// First ensure user is authenticated
			userID, authenticated := sharedhttp.GetAuthenticatedUser(store, r)
			if !authenticated {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Get user to check their role
			user, err := rbacService.userStore.GetUser(r.Context(), userID)
			if err != nil {
				http.Error(w, "Failed to get user information", http.StatusInternalServerError)
				return
			}

			// Check if user has the required role or higher
			if !hasRequiredSystemRole(user.Role, requiredRole) {
				http.Error(w, fmt.Sprintf("Access denied: %s role required", requiredRole), http.StatusForbidden)
				return
			}

			// Add permission context to request
			ctx := context.WithValue(r.Context(), "permission_context", &PermissionContext{
				UserID:      userID,
				RBACService: rbacService,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// RequireProjectRole is middleware that requires a specific project role
func RequireProjectRole(store sessions.Store, rbacService *RBACService, requiredRole string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// First ensure user is authenticated
			userID, authenticated := sharedhttp.GetAuthenticatedUser(store, r)
			if !authenticated {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Extract project ID from URL parameters
			projectID := chi.URLParam(r, "projectId")
			if projectID == "" {
				projectID = chi.URLParam(r, "project_id")
			}
			if projectID == "" {
				http.Error(w, "Project ID is required", http.StatusBadRequest)
				return
			}

			// Check if user is admin (admins bypass project role checks)
			user, err := rbacService.userStore.GetUser(r.Context(), userID)
			if err != nil {
				http.Error(w, "Failed to get user information", http.StatusInternalServerError)
				return
			}

			if user.Role == RoleAdmin {
				// Admin users bypass project role requirements
				ctx := context.WithValue(r.Context(), "permission_context", &PermissionContext{
					UserID:      userID,
					ProjectID:   projectID,
					RBACService: rbacService,
				})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Get project membership
			membership, err := rbacService.userStore.GetProjectMembership(r.Context(), userID, projectID)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, "Access denied: not a member of this project", http.StatusForbidden)
					return
				}
				http.Error(w, "Failed to check project membership", http.StatusInternalServerError)
				return
			}

			// Check if user has the required project role or higher
			if !hasRequiredProjectRole(membership.Role, requiredRole) {
				http.Error(w, fmt.Sprintf("Access denied: %s role required in this project", requiredRole), http.StatusForbidden)
				return
			}

			// Add permission context to request
			ctx := context.WithValue(r.Context(), "permission_context", &PermissionContext{
				UserID:      userID,
				ProjectID:   projectID,
				RBACService: rbacService,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// RequireOwnershipOrPermission checks if user owns a resource or has a specific permission
func RequireOwnershipOrPermission(store sessions.Store, rbacService *RBACService, permission string, getOwnerID func(*http.Request) (string, error)) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// First ensure user is authenticated
			userID, authenticated := sharedhttp.GetAuthenticatedUser(store, r)
			if !authenticated {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Get the owner ID of the resource
			ownerID, err := getOwnerID(r)
			if err != nil {
				http.Error(w, "Failed to determine resource ownership", http.StatusInternalServerError)
				return
			}

			// Check if user owns the resource
			if userID == ownerID {
				ctx := context.WithValue(r.Context(), "permission_context", &PermissionContext{
					UserID:      userID,
					RBACService: rbacService,
				})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Check if user has the required permission
			hasPermission, err := rbacService.HasSystemPermission(r.Context(), userID, permission)
			if err != nil {
				http.Error(w, "Failed to check permissions", http.StatusInternalServerError)
				return
			}

			if !hasPermission {
				http.Error(w, "Access denied: insufficient permissions", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), "permission_context", &PermissionContext{
				UserID:      userID,
				Permission:  permission,
				RBACService: rbacService,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// GetPermissionContextFromRequest extracts the permission context from the request
func GetPermissionContextFromRequest(r *http.Request) (*PermissionContext, bool) {
	ctx, ok := r.Context().Value("permission_context").(*PermissionContext)
	return ctx, ok
}

// Helper functions for role hierarchy checking

// hasRequiredSystemRole checks if a user's role meets the minimum required system role
func hasRequiredSystemRole(userRole, requiredRole string) bool {
	roleHierarchy := map[string]int{
		RoleViewer:         1,
		RoleDeveloper:      2,
		RoleProjectManager: 3,
		RoleAdmin:          4,
	}

	userLevel, userExists := roleHierarchy[userRole]
	requiredLevel, requiredExists := roleHierarchy[requiredRole]

	if !userExists || !requiredExists {
		return false
	}

	return userLevel >= requiredLevel
}

// hasRequiredProjectRole checks if a user's project role meets the minimum required project role
func hasRequiredProjectRole(userRole, requiredRole string) bool {
	roleHierarchy := map[string]int{
		ProjectRoleViewer:    1,
		ProjectRoleDeveloper: 2,
		ProjectRoleAdmin:     3,
	}

	userLevel, userExists := roleHierarchy[userRole]
	requiredLevel, requiredExists := roleHierarchy[requiredRole]

	if !userExists || !requiredExists {
		return false
	}

	return userLevel >= requiredLevel
}

// MiddlewareChain allows chaining multiple middleware functions
func MiddlewareChain(middlewares ...func(http.HandlerFunc) http.HandlerFunc) func(http.HandlerFunc) http.HandlerFunc {
	return func(final http.HandlerFunc) http.HandlerFunc {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// Common middleware combinations for convenience

// RequireAdmin is a convenience middleware for admin-only endpoints
func RequireAdmin(store sessions.Store, rbacService *RBACService) func(http.HandlerFunc) http.HandlerFunc {
	return RequireSystemRole(store, rbacService, RoleAdmin)
}

// RequireProjectManager is a convenience middleware for project manager or higher
func RequireProjectManager(store sessions.Store, rbacService *RBACService) func(http.HandlerFunc) http.HandlerFunc {
	return RequireSystemRole(store, rbacService, RoleProjectManager)
}

// RequireProjectAdmin is a convenience middleware for project admin role
func RequireProjectAdmin(store sessions.Store, rbacService *RBACService) func(http.HandlerFunc) http.HandlerFunc {
	return RequireProjectRole(store, rbacService, ProjectRoleAdmin)
}
