package routes

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/realtime"
	"github.com/mbolli/tickit/internal/users"
	"github.com/mbolli/tickit/web/ui/pages"
)

// RegisterPageRoutes registers public and authenticated (non-admin, non-API) page routes.
func RegisterPageRoutes(router chi.Router, deps *bootstrap.Deps) {
	// Configure CSRF protection for form pages
	csrfMiddleware := users.NewCSRFMiddleware(nil)

	// --- Public routes (no auth required) ---
	// Apply CSRF middleware to pages that contain forms, so CSRF cookies get set
	router.Group(func(r chi.Router) {
		r.Use(csrfMiddleware) // This sets the CSRF cookie on GET requests

		r.Get("/auth/login", func(w http.ResponseWriter, r *http.Request) {
			pages.LoginPage(deps.SessionStore, r, "").Render(r.Context(), w)
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

		// Dashboard routes with realtime subscription middleware
		auth.Group(func(dashboard chi.Router) {
			dashboard.Use(realtime.IssuesTableSubscription(realtime.NewIssuesTableRenderer(deps.IssueService, deps.ProjectService, deps.UserService)))
			dashboard.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
				projectsList, _ := deps.ProjectService.ListProjects(r.Context())
				pages.DashboardPage(deps.SessionStore, r, projectsList, nil).Render(r.Context(), w)
			})
			dashboard.With(realtime.IssueDetailSubscription(realtime.NewIssueDetailRenderer(deps.IssueService, deps.ProjectService, deps.UserService))).Get("/dashboard/{issueId}", func(w http.ResponseWriter, r *http.Request) {
				projectsList, _ := deps.ProjectService.ListProjects(r.Context())
				issue, _ := deps.IssueService.GetIssueByID(r.Context(), chi.URLParam(r, "issueId"))
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
			projectsList, _ := deps.ProjectService.ListProjects(r.Context())
			pages.TicketDetailPage(issue, projectsList, deps.SessionStore, r).Render(r.Context(), w)
		})

		// --- Admin-only page (RequireAdminRole) ---
		auth.Group(func(admin chi.Router) {
			admin.Use(deps.UserRoutes.RequireAdminRole)
			admin.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
				projectsList, _ := deps.ProjectService.ListProjects(r.Context())
				pages.AdminPanelPage(deps.SessionStore, r, projectsList).Render(r.Context(), w)
			})
		})
	})
}
