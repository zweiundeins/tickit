package routes

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/delaneyj/toolbelt/embeddednats"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/projects"
	"github.com/mbolli/tickit/internal/ui/pages"
	natsserver "github.com/nats-io/nats-server/v2/server"
)

func SetupRoutes(ctx context.Context, router chi.Router) (err error) {
	natsPort, err := getFreeNatsPort()
	if err != nil {
		return fmt.Errorf("error obtaining NATS port: %w", err)
	}

	ns, err := embeddednats.New(ctx, embeddednats.WithNATSServerOptions(&natsserver.Options{
		JetStream: true,
		NoSigs:    true,
		Port:      natsPort,
		StoreDir:  "data/nats",
	}))

	if err != nil {
		return fmt.Errorf("error creating embedded nats server: %w", err)
	}

	ns.WaitForServer()
	slog.Info(fmt.Sprintf("starting NATS on port :%d", natsPort))

	sessionStore := sessions.NewCookieStore([]byte("session-secret"))
	sessionStore.MaxAge(int(24 * time.Hour / time.Second))

	cqrsDeps, err := bootstrap.WireCQRS(ctx, ns)
	if err != nil {
		return fmt.Errorf("error wiring CQRS: %w", err)
	}

	// register routes
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		pages.Index(r).Render(r.Context(), w)
	})

	router.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		projectsList, _ := cqrsDeps.ProjectService.ListProjects(r.Context())
		pages.DashboardPage(r, projectsList).Render(r.Context(), w)
	})
	router.Get("/create-ticket", func(w http.ResponseWriter, r *http.Request) {
		projectsList, _ := cqrsDeps.ProjectService.ListProjects(r.Context())
		pages.CreateTicketPage(r, projectsList).Render(r.Context(), w)
	})
	router.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
		projectsList, _ := cqrsDeps.ProjectService.ListProjects(r.Context())
		pages.AdminPanelPage(r, projectsList).Render(r.Context(), w)
	})
	router.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		pages.LoginPage(r).Render(r.Context(), w)
	})
	router.Get("/register", func(w http.ResponseWriter, r *http.Request) {
		pages.RegisterPage(r).Render(r.Context(), w)
	})
	// Placeholder logout route (should clear session in future)
	router.Get("/logout", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
	// Ticket detail route (dynamic)
	router.Get("/ticket/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		issue, err := cqrsDeps.IssueService.GetIssueByID(r.Context(), id)
		if err != nil || issue == nil {
			http.NotFound(w, r)
			return
		}
		projectsList, _ := cqrsDeps.ProjectService.ListProjects(r.Context())
		pages.TicketDetailPage(issue, projectsList, r).Render(r.Context(), w)
	})

	router.Route("/api", func(apiRouter chi.Router) {
		apiRouter.Route("/notifications", func(notifRouter chi.Router) {
			notifRouter.Post("/", notifications.HandleCreateNotification(cqrsDeps.SessionStore, cqrsDeps.NotificationService))
			notifRouter.Delete("/", notifications.HandleDeleteNotification(cqrsDeps.SessionStore, cqrsDeps.NotificationService))
			notifRouter.Patch("/read", notifications.HandleMarkNotificationRead(cqrsDeps.SessionStore, cqrsDeps.NotificationService))
		})
		apiRouter.Route("/issues", func(issueRouter chi.Router) {
			issueRouter.Post("/", issues.HandleCreateIssue(cqrsDeps.SessionStore, cqrsDeps.IssueService))
			issueRouter.Patch("/{id}", issues.HandlePatchIssue(cqrsDeps.SessionStore, cqrsDeps.IssueService))
			issueRouter.Delete("/{id}", issues.HandleDeleteIssue(cqrsDeps.SessionStore, cqrsDeps.IssueService))
			issueRouter.Get("/", issues.HandleListIssues(cqrsDeps.SessionStore, cqrsDeps.IssueService))
			issueRouter.Get("/{id}", issues.HandleGetIssueByID(cqrsDeps.SessionStore, cqrsDeps.IssueService))
		})
		apiRouter.Route("/projects", func(projectRouter chi.Router) {
			projectRouter.Post("/", projects.HandleCreateProject(cqrsDeps.SessionStore, cqrsDeps.ProjectService))
			projectRouter.Patch("/{id}", projects.HandleUpdateProject(cqrsDeps.SessionStore, cqrsDeps.ProjectService))
			projectRouter.Delete("/{id}", projects.HandleDeleteProject(cqrsDeps.SessionStore, cqrsDeps.ProjectService))
			projectRouter.Get("/", projects.HandleListProjects(cqrsDeps.SessionStore, cqrsDeps.ProjectService))
			projectRouter.Get("/{id}", projects.HandleGetProjectByID(cqrsDeps.SessionStore, cqrsDeps.ProjectService))
		})
		// New SSE route for issues table only
		apiRouter.Get("/updates/issues", HandleIssuesUpdatesSSE(cqrsDeps.SessionStore, cqrsDeps.NatsConn, cqrsDeps.IssueService, cqrsDeps.ProjectService))
		// New SSE route for notifications only
		apiRouter.Get("/updates/notifications", HandleNotificationsUpdatesSSE(cqrsDeps.SessionStore, cqrsDeps.NatsConn, cqrsDeps.NotificationService))
		// New SSE route for API status/keepalive
		apiRouter.Get("/updates/status", HandleStatusUpdatesSSE())
		// Register the new SSE route for single issue updates
		apiRouter.Get("/updates/issue/{id}", HandleIssueUpdatesSSE(cqrsDeps.SessionStore, cqrsDeps.NatsConn, cqrsDeps.IssueService, cqrsDeps.ProjectService))
	})

	// Upload routes
	router.Post("/upload", HandleUpload())
	router.Handle("/uploads/*", HandleUploads())

	return nil
}

func getFreeNatsPort() (int, error) {
	if p, ok := os.LookupEnv("NATS_PORT"); ok {
		natsPort, err := strconv.Atoi(p)
		if err != nil {
			return 0, fmt.Errorf("error parsing NATS_PORT: %w", err)
		}
		if isPortFree(natsPort) {
			return natsPort, nil
		}
	}
	return toolbelt.FreePort()
}

func isPortFree(port int) bool {
	address := fmt.Sprintf(":%d", port)

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}

	if err := ln.Close(); err != nil {
		return false
	}

	return true
}
