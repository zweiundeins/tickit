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
	if err := setupCQRSRoutes(router, cqrsDeps.SessionStore, cqrsDeps.NotificationService, cqrsDeps.IssueService, cqrsDeps); err != nil {
		return fmt.Errorf("error setting up CQRS routes: %w", err)
	}

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

func upsertSessionID(store sessions.Store, r *http.Request, w http.ResponseWriter) (string, error) {
	sess, err := store.Get(r, "connections")
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	id, ok := sess.Values["id"].(string)

	if !ok {
		id = toolbelt.NextEncodedID()
		sess.Values["id"] = id
		if err := sess.Save(r, w); err != nil {
			return "", fmt.Errorf("failed to save session: %w", err)
		}
	}

	return id, nil
}
