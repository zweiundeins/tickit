package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/notifications"
)

// RegisterAPIRoutes registers all API endpoints, grouped by middleware.
func RegisterAPIRoutes(router chi.Router, deps *bootstrap.Deps) {
	// --- Authenticated API routes ---
	router.Group(func(auth chi.Router) {
		auth.Use(deps.UserRoutes.RequireAuthentication)

		// Generic SSE endpoint - subscription context comes from page middleware
		auth.Get("/updates", HandleUpdatesSSE(deps))

		// Generic signals endpoint - updates subscription context
		auth.Post("/signals", HandleSignalsUpdate(deps))
	})

	router.Route("/api", func(apiRouter chi.Router) {
		// --- Notifications API ---
		apiRouter.Route("/notifications", func(notifRouter chi.Router) {
			notifRouter.Post("/", notifications.HandleCreateNotification(deps.NotificationService))
			notifRouter.Delete("/", notifications.HandleDeleteNotification(deps.NotificationService))
			notifRouter.Patch("/read", notifications.HandleMarkNotificationRead(deps.NotificationService))
		})

		// --- Issues API ---
		apiRouter.Route("/issues", func(issueRouter chi.Router) {
			issueRouter.Post("/", issues.HandleCreateIssue(deps.IssueService))
			issueRouter.Patch("/{id}", issues.HandlePatchIssue(deps.IssueService))
			issueRouter.Delete("/{id}", issues.HandleDeleteIssue(deps.IssueService))
			issueRouter.Get("/", issues.HandleListIssues(deps.IssueService))
			issueRouter.Get("/{id}", issues.HandleGetIssueByID(deps.IssueService))
		})

		// --- Projects API (CRUD + membership + members) ---
		apiRouter.Route("/projects", func(projectRouter chi.Router) {
			// CRUD endpoints (main API)
			projectRouter.Post("/", HandleCreateProject(deps.ProjectService))
			projectRouter.Patch("/{id}", HandleUpdateProject(deps.ProjectService))
			projectRouter.Delete("/{id}", HandleDeleteProject(deps.ProjectService))
			projectRouter.Get("/", HandleListProjects(deps.ProjectService))
			projectRouter.Get("/{id}", HandleGetProjectByID(deps.ProjectService))

			// Membership and members endpoints (auth required)
			projectRouter.Group(func(r chi.Router) {
				r.Use(deps.UserRoutes.RequireAuthentication)
				r.Post("/{projectId}/invite", deps.UserRoutes.HandleInviteUser)
				r.Delete("/{projectId}/members/{userId}", deps.UserRoutes.HandleRemoveUserFromProject)
				r.Put("/{projectId}/members/{userId}/role", deps.UserRoutes.HandleUpdateMemberRole)
				r.Get("/{projectId}/members", deps.UserRoutes.HandleGetProjectMembers)
			})
		})

		// --- Auth API (public endpoints) ---
		apiRouter.Route("/auth", func(authRouter chi.Router) {
			authRouter.Post("/forgot-password", deps.UserRoutes.HandleForgotPassword)
			authRouter.Post("/reset-password", deps.UserRoutes.HandleResetPassword)
		})

		// --- Users API (profile, sessions, form-based and token-based) ---
		apiRouter.Route("/users", func(userRouter chi.Router) {
			// Authenticated, CSRF-protected form-based routes
			userRouter.Group(func(r chi.Router) {
				r.Use(deps.UserRoutes.RequireAuthentication)
				r.Put("/profile", deps.UserRoutes.HandleUpdateProfile)
				r.Post("/profile/avatar", deps.UserRoutes.HandleUploadAvatar)
				r.Post("/profile/deactivate", deps.UserRoutes.HandleDeactivateUser)
				// Session management (form-based)
				r.Route("/sessions", func(sessionRouter chi.Router) {
					sessionRouter.Delete("/{sessionId}", deps.UserRoutes.HandleInvalidateSession)
					sessionRouter.Delete("/", deps.UserRoutes.HandleInvalidateAllSessions)
				})
			})

			// Authenticated, token-based API endpoints (no CSRF)
			userRouter.Group(func(r chi.Router) {
				r.Use(deps.UserRoutes.RequireAuthentication)
				r.Get("/profile", deps.UserRoutes.HandleGetProfile)
				r.Get("/sessions", deps.UserRoutes.HandleGetUserSessions)
			})
		})

		// --- Invitation routes (form-based) ---
		apiRouter.Route("/invitations", func(inviteRouter chi.Router) {
			inviteRouter.Use(deps.UserRoutes.RequireAuthentication)
			inviteRouter.Get("/", deps.UserRoutes.HandleGetUserInvitations)
			inviteRouter.Post("/{token}/accept", deps.UserRoutes.HandleAcceptInvitation)
			inviteRouter.Post("/{token}/decline", deps.UserRoutes.HandleDeclineInvitation)
		})

		// --- Administrative routes (form-based) ---
		apiRouter.Route("/admin", func(adminRouter chi.Router) {
			adminRouter.Use(deps.UserRoutes.RequireAuthentication)
			adminRouter.Use(deps.UserRoutes.RequireAdminRole)
			adminRouter.Get("/stats", deps.UserRoutes.HandleGetSystemStats)
			adminRouter.Get("/users", deps.UserRoutes.HandleListUsers)
			adminRouter.Get("/users/{userId}", deps.UserRoutes.HandleGetUser)
			adminRouter.Put("/users/{userId}", deps.UserRoutes.HandleUpdateUser)
			adminRouter.Post("/users/{userId}/deactivate", deps.UserRoutes.HandleDeactivateUser)
			adminRouter.Post("/users/{userId}/reactivate", deps.UserRoutes.HandleReactivateUser)
			adminRouter.Put("/users/{userId}/role", deps.UserRoutes.HandleAssignRole)
			adminRouter.Get("/users/{userId}/activity", deps.UserRoutes.HandleGetUserActivity)
		})

		// --- SSE/Updates API ---
		apiRouter.Get("/updates/status", HandleStatusUpdatesSSE())

		// --- API endpoints, OAuth callbacks, etc. (no CSRF needed) ---
		apiRouter.Group(func(r chi.Router) {
			// Email verification (GET request, no CSRF needed)
			r.Get("/auth/verify-email", deps.UserRoutes.HandleVerifyEmail)

			// OAuth routes (external redirects, CSRF exempt)
			r.Get("/auth/oauth/{provider}", deps.UserRoutes.HandleOAuthLogin)
			r.Get("/auth/oauth/{provider}/callback", deps.UserRoutes.HandleOAuthCallback)

			// WebAuthn routes (JSON API, CSRF exempt)
			r.Post("/auth/webauthn/register/begin", deps.UserRoutes.HandleWebAuthnRegisterBegin)
			r.Post("/auth/webauthn/register/finish", deps.UserRoutes.HandleWebAuthnRegisterFinish)
			r.Post("/auth/webauthn/login/begin", deps.UserRoutes.HandleWebAuthnLoginBegin)
			r.Post("/auth/webauthn/login/finish", deps.UserRoutes.HandleWebAuthnLoginFinish)

			// Magic mail routes (development only, CSRF exempt)
			r.Post("/auth/magic-mail", deps.UserRoutes.HandleSendMagicMail)
			r.Get("/auth/magic-mail/login", deps.UserRoutes.HandleMagicMailLogin)
		})
	})
}
