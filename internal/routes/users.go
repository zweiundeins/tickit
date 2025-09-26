package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/mbolli/tickit/internal/bootstrap"
)

// RegisterUserRoutes registers all user-related HTTP routes
func RegisterUserRoutes(router chi.Router, deps *bootstrap.Deps) {
	// Use the already initialized user routes from dependencies
	// This ensures we use the same session store instance that was configured in bootstrap
	userRoutes := deps.UserRoutes

	// Apply security headers to form-based routes (CSRF now global)
	router.Group(func(r chi.Router) {
		// Authentication routes (public, with CSRF protection for forms)
		r.Post("/auth/register", userRoutes.HandleRegister)
		r.Post("/auth/login", userRoutes.HandleLogin)
		r.Get("/auth/verify-email", userRoutes.HandleVerifyEmail)

		// Password management routes
		r.Post("/auth/forgot-password", userRoutes.HandleForgotPassword)
		r.Post("/auth/reset-password", userRoutes.HandleResetPassword)
		r.Post("/auth/change-password", userRoutes.HandleChangePassword)

		// Magic mail routes (development only)
		r.Post("/auth/magic-mail", userRoutes.HandleSendMagicMail)
	})

	// OAuth routes (CSRF exempt but with security headers)
	router.Group(func(r chi.Router) {

		r.Get("/auth/oauth/{provider}", userRoutes.HandleOAuthLogin)
		r.Get("/auth/oauth/{provider}/callback", userRoutes.HandleOAuthCallback)
	})

	// Magic mail login (CSRF exempt but with security headers)
	router.Get("/auth/magic-mail/login", userRoutes.HandleMagicMailLogin)

	// Logout route (CSRF exempt but with security headers)
	router.Get("/logout", userRoutes.HandleLogout)

	// Debug routes (remove in production)
	router.Get("/auth/clear-cookies", userRoutes.HandleClearCookies)
	router.Get("/auth/test-session", userRoutes.HandleTestSession)

	// WebAuthn routes (CSRF exempt, but with security headers)
	router.Group(func(r chi.Router) {

		r.Post("/auth/webauthn/register/begin", userRoutes.HandleWebAuthnRegisterBegin)
		r.Post("/auth/webauthn/register/finish", userRoutes.HandleWebAuthnRegisterFinish)
		r.Post("/auth/webauthn/login/begin", userRoutes.HandleWebAuthnLoginBegin)
		r.Post("/auth/webauthn/login/finish", userRoutes.HandleWebAuthnLoginFinish)
	})

	// Protected user profile routes
	router.Group(func(r chi.Router) {

		r.Use(userRoutes.RequireAuthentication)

		r.Get("/api/users/profile", userRoutes.HandleGetProfile)
		r.Put("/api/users/profile", userRoutes.HandleUpdateProfile)
		r.Post("/api/users/profile/avatar", userRoutes.HandleUploadAvatar)
	})

	// Protected project membership routes
	router.Group(func(r chi.Router) {

		r.Use(userRoutes.RequireAuthentication)

		r.Get("/api/projects/{projectId}/members", userRoutes.HandleGetProjectMembers)
		r.Post("/api/projects/{projectId}/invite", userRoutes.HandleInviteUser)
		r.Delete("/api/projects/{projectId}/members/{userId}", userRoutes.HandleRemoveUserFromProject)
		r.Put("/api/projects/{projectId}/members/{userId}/role", userRoutes.HandleUpdateMemberRole)
	})

	// Protected invitation routes
	router.Group(func(r chi.Router) {

		r.Use(userRoutes.RequireAuthentication)

		r.Get("/api/invitations", userRoutes.HandleGetUserInvitations)
		r.Post("/api/invitations/{token}/accept", userRoutes.HandleAcceptInvitation)
		r.Post("/api/invitations/{token}/decline", userRoutes.HandleDeclineInvitation)
	})

	// Administrative routes (admin only)
	router.Group(func(r chi.Router) {

		r.Use(userRoutes.RequireAuthentication)
		r.Use(userRoutes.RequireAdminRole)

		r.Get("/api/admin/users", userRoutes.HandleListUsers)
		r.Get("/api/admin/users/{userId}", userRoutes.HandleGetUser)
		r.Put("/api/admin/users/{userId}", userRoutes.HandleUpdateUser)
		r.Post("/api/admin/users/{userId}/deactivate", userRoutes.HandleDeactivateUser)
		r.Post("/api/admin/users/{userId}/reactivate", userRoutes.HandleReactivateUser)
		r.Put("/api/admin/users/{userId}/role", userRoutes.HandleAssignRole)
		r.Get("/api/admin/users/{userId}/activity", userRoutes.HandleGetUserActivity)
		r.Get("/api/admin/stats", userRoutes.HandleGetSystemStats)
	})

	// Session management routes
	router.Group(func(r chi.Router) {

		r.Use(userRoutes.RequireAuthentication)

		r.Get("/api/users/sessions", userRoutes.HandleGetUserSessions)
		r.Delete("/api/users/sessions/{sessionId}", userRoutes.HandleInvalidateSession)
		r.Delete("/api/users/sessions", userRoutes.HandleInvalidateAllSessions)
	})

	// Note: Enhanced security features like rate limiting, audit logging, etc.
	// can be added as middleware to the existing routes above as needed
}
