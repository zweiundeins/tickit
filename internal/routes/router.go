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

	slog.Info("using CQRS implementation")
	cqrsDeps, err := bootstrap.WireCQRS(ctx, ns)
	if err != nil {
		return fmt.Errorf("error wiring CQRS: %w", err)
	}

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		if err := pages.Index().Render(r.Context(), w); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})

	router.Route("/api", func(apiRouter chi.Router) {
		apiRouter.Route("/notifications", func(notifRouter chi.Router) {
			notifRouter.Post("/", notifications.HandleCreateNotification(cqrsDeps.SessionStore, cqrsDeps.NotificationService))
			notifRouter.Delete("/", notifications.HandleDeleteNotification(cqrsDeps.SessionStore, cqrsDeps.NotificationService))
			notifRouter.Patch("/read", notifications.HandleMarkNotificationRead(cqrsDeps.SessionStore, cqrsDeps.NotificationService))
		})
		apiRouter.Route("/issues", func(issueRouter chi.Router) {
			issueRouter.Post("/", issues.HandleCreateIssue(cqrsDeps.SessionStore, cqrsDeps.IssueService))
			issueRouter.Patch("/{id}", issues.HandleUpdateIssue(cqrsDeps.SessionStore, cqrsDeps.IssueService))
			issueRouter.Delete("/{id}", issues.HandleDeleteIssue(cqrsDeps.SessionStore, cqrsDeps.IssueService))
			issueRouter.Get("/", issues.HandleListIssues(cqrsDeps.SessionStore, cqrsDeps.IssueService))
			issueRouter.Get("/{id}", issues.HandleGetIssueByID(cqrsDeps.SessionStore, cqrsDeps.IssueService))
		})
		apiRouter.Get("/updates", handleUpdatesSSE(cqrsDeps.SessionStore, cqrsDeps.NatsConn, cqrsDeps.NotificationService, cqrsDeps.IssueService))
	})

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
