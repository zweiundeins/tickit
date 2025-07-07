package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/cqrs/application"
	"github.com/mbolli/tickit/internal/ui/pages"
)

// setupCQRSRoutes sets up routes using CQRS architecture
func setupCQRSRoutes(
	router chi.Router,
	store sessions.Store,
	notificationService *application.NotificationService,
	issueService *application.IssueService,
	cqrsDeps *bootstrap.CQRSDeps,
) error {
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		if err := pages.Index().Render(r.Context(), w); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})

	router.Route("/api", func(apiRouter chi.Router) {
		apiRouter.Route("/notifications", func(notifRouter chi.Router) {
			notifRouter.Post("/", handleCreateNotification(store, notificationService))
			notifRouter.Delete("/", handleDeleteNotification(store, notificationService))
			notifRouter.Patch("/read", handleMarkNotificationRead(store, notificationService))
		})
		apiRouter.Route("/issues", func(issueRouter chi.Router) {
			issueRouter.Post("/", handleCreateIssue(store, issueService))
			issueRouter.Patch("/{id}", handleUpdateIssue(store, issueService))
			issueRouter.Delete("/{id}", handleDeleteIssue(store, issueService))
			issueRouter.Get("/", handleListIssues(store, issueService))
			issueRouter.Get("/{id}", handleGetIssueByID(store, issueService))
		})
		apiRouter.Get("/updates", handleUpdatesSSE(store, cqrsDeps.NatsConn, notificationService, issueService))
	})

	return nil
}
