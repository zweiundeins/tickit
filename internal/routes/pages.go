package routes

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"
	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/realtime"
	"github.com/mbolli/tickit/internal/sharedhttp"
	"github.com/mbolli/tickit/internal/users"
	"github.com/mbolli/tickit/web/ui/components"
	"github.com/mbolli/tickit/web/ui/pages"
)

// RegisterPageRoutes registers public and authenticated (non-admin, non-API) page routes.
func RegisterPageRoutes(router chi.Router, deps *bootstrap.Deps) {
	// --- Public routes (no auth required) ---
	// CSRF middleware is applied globally, so no need to apply it here again
	router.Group(func(r chi.Router) {

		r.Get("/auth/login", func(w http.ResponseWriter, r *http.Request) {
			// Check for flash messages
			flashMessage, err := sharedhttp.GetFlashMessage(deps.SessionStore, w, r)
			if err != nil {
				// Log error but continue - flash message is not critical
				// In a real application, you'd use your logger here
			}

			// Debug: Log CSRF token being generated for the login page
			csrfToken := nosurf.Token(r)
			log.Printf("DEBUG: Login GET request - generating CSRF token: %s", csrfToken)

			pages.LoginPage(deps.SessionStore, r, "", flashMessage).Render(r.Context(), w)
		})
		r.Get("/register", func(w http.ResponseWriter, r *http.Request) {
			pages.RegisterPage(deps.SessionStore, r).Render(r.Context(), w)
		})
		r.Get("/auth/forgot-password", func(w http.ResponseWriter, r *http.Request) {
			pages.ForgotPasswordPage(deps.SessionStore, r).Render(r.Context(), w)
		})
		r.Get("/auth/reset-password", func(w http.ResponseWriter, r *http.Request) {
			token := r.URL.Query().Get("token")
			if token == "" {
				http.Error(w, "Reset token is required", http.StatusBadRequest)
				return
			}
			pages.ResetPasswordPage(deps.SessionStore, r, token).Render(r.Context(), w)
		})
	})

	// Issue key redirect route - must be before other routes to avoid conflicts
	router.Get("/{issueKey:[A-Z]{2,6}-[0-9]+}", func(w http.ResponseWriter, r *http.Request) {
		issueKey := chi.URLParam(r, "issueKey")
		// Parse the key: SLUG-NUMBER
		var slug string
		var number int
		sep := strings.LastIndex(issueKey, "-")
		if sep > 0 {
			slug = strings.ToLower(issueKey[:sep])
			numStr := issueKey[sep+1:]
			n, err := strconv.Atoi(numStr)
			if err == nil {
				number = n
			}
		}
		if slug == "" || number == 0 {
			http.NotFound(w, r)
			return
		}
		project, err := deps.ProjectService.GetProjectBySlug(r.Context(), slug)
		if err != nil || project == nil {
			http.NotFound(w, r)
			return
		}
		// Now look up the issue by project ID and number
		issuesList, err := deps.IssueService.ListIssues(r.Context(), issues.ListIssuesQuery{ProjectID: &project.ID})
		if err != nil {
			http.NotFound(w, r)
			return
		}
		var found *issues.Issue
		for _, issue := range issuesList {
			if issue.Number == number {
				found = issue
				break
			}
		}
		if found == nil {
			http.NotFound(w, r)
			return
		}
		// Redirect to the ticket detail page
		http.Redirect(w, r, "/ticket/"+found.ID, http.StatusSeeOther)
	})

	// --- Authenticated routes (RequireAuthentication) ---
	router.Group(func(auth chi.Router) {
		auth.Use(deps.UserRoutes.RequireAuthentication)

		auth.Get("/", func(w http.ResponseWriter, r *http.Request) {
			// pages.Index(r).Render(r.Context(), w)
			http.Redirect(w, r, "/dashboard", 302)
		})

		// Dashboard routes
		auth.Group(func(dashboard chi.Router) {
			dashboard.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
				// Set event patterns for dashboard page
				realtime.SetEventPatterns(r, []string{"issues.*", "projects.*", "notifications.*"})

				projectsList, _ := deps.ProjectService.ListProjects(r.Context())
				pages.DashboardPage(deps.SessionStore, r, projectsList, nil).Render(r.Context(), w)
			})
			dashboard.Get("/dashboard/{issueId}", func(w http.ResponseWriter, r *http.Request) {
				issueID := chi.URLParam(r, "issueId")
				// Set event patterns for issue detail page
				realtime.SetEventPatterns(r, []string{fmt.Sprintf("issues.%s.*", issueID), "notifications.*"})

				projectsList, _ := deps.ProjectService.ListProjects(r.Context())
				issue, _ := deps.IssueService.GetIssueByID(r.Context(), issueID)

				// Get user_id directly from context (set by authentication middleware)
				if userID, ok := r.Context().Value("user_id").(string); ok && userID != "" {
					if user, err := deps.UserService.GetUserByID(r.Context(), userID); err == nil && user != nil {
						notificationTitle := fmt.Sprintf("Issue Viewed: %s", issue.Title)
						var issueKey string
						if project, err := deps.ProjectService.GetProjectByID(r.Context(), issue.ProjectID); err == nil && project != nil {
							issueKey = issue.GetKey(project.Slug)
						} else {
							issueKey = issue.ID
						}
						notificationBody := fmt.Sprintf("%s %s viewed issue %s", user.FirstName, user.LastName, issueKey)

						logger.Info().Str("title", notificationTitle).Str("body", notificationBody).Msg("Creating issue view notification")

						// Send notification (this will be sent to all connected clients via SSE)
						if err := deps.NotificationService.CreateNotification(
							r.Context(),
							notificationTitle,
							notificationBody,
						); err != nil {
							logger.Error().Err(err).Msg("Failed to create issue view notification")
						} else {
							logger.Info().Msg("Successfully created issue view notification")
						}
					} else {
						logger.Error().Err(err).Msg("Failed to get user for notification")
					}
				} else {
					logger.Error().Msg("No user_id found in context for notification")
				}

				pages.DashboardPage(deps.SessionStore, r, projectsList, issue).Render(r.Context(), w)
			})
		})

		auth.Get("/create-ticket", func(w http.ResponseWriter, r *http.Request) {
			projectsList, _ := deps.ProjectService.ListProjects(r.Context())
			pages.CreateTicketPage(deps.SessionStore, r, projectsList).Render(r.Context(), w)
		})
		auth.Get("/ticket/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			issue, err := deps.IssueService.GetIssueByID(r.Context(), id)
			if err != nil || issue == nil {
				http.NotFound(w, r)
				return
			}

			// Create notification when user views an issue
			sessionID := r.Context().Value("session_id").(string)
			if userSession, err := deps.UserService.GetUserSession(r.Context(), sessionID); err == nil && userSession != nil {
				if user, err := deps.UserService.GetUserByID(r.Context(), userSession.UserID); err == nil && user != nil {
					notificationTitle := fmt.Sprintf("Issue Viewed: %s", issue.Title)
					var issueKey string
					if project, err := deps.ProjectService.GetProjectByID(r.Context(), issue.ProjectID); err == nil && project != nil {
						issueKey = issue.GetKey(project.Slug)
					} else {
						issueKey = issue.ID
					}
					notificationBody := fmt.Sprintf("%s %s viewed issue %s", user.FirstName, user.LastName, issueKey)

					// Send notification (this will be sent to all connected clients via SSE)
					_ = deps.NotificationService.CreateNotification(
						r.Context(),
						notificationTitle,
						notificationBody,
					)
				}
			}

			projectsList, _ := deps.ProjectService.ListProjects(r.Context())
			pages.TicketDetailPage(issue, projectsList, deps.SessionStore, r).Render(r.Context(), w)
		})

		// --- Admin-only page (RequireAdminRole) ---
		auth.Group(func(admin chi.Router) {
			admin.Use(deps.UserRoutes.RequireAdminRole)
			admin.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
				projectsList, _ := deps.ProjectService.ListProjects(r.Context())

				// Fetch users for admin panel
				usersList, err := deps.UserService.ListUsers(r.Context(), 0, 0, "", "")
				if err != nil {
					log.Printf("Error fetching users for admin panel: %v", err)
					usersList = []*users.User{} // fallback to empty list
				}

				// Convert users to AdminUser structs to avoid import cycles
				adminUsers := make([]components.AdminUser, len(usersList))
				for i, user := range usersList {
					adminUsers[i] = components.AdminUser{
						ID:     user.ID,
						Name:   user.FirstName + " " + user.LastName,
						Role:   user.Role,
						Email:  user.Email,
						Status: user.Status,
					}
				}

				pages.AdminPanelPage(deps.SessionStore, r, projectsList, adminUsers).Render(r.Context(), w)
			})
		})
	})
}
