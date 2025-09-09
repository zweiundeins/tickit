package routes

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"

	"github.com/delaneyj/toolbelt"
	"github.com/delaneyj/toolbelt/embeddednats"
	"github.com/go-chi/chi/v5"
	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/users"
	natsserver "github.com/nats-io/nats-server/v2/server"
)

var setupRoutesCallCount int

func SetupRoutes(ctx context.Context, router chi.Router) (err error) {
	setupRoutesCallCount++
	slog.Info("SetupRoutes called", "call_count", setupRoutesCallCount)
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

	// Initialize centralized session store
	sessionStore, err := users.InitializeGlobalSessionStore()
	if err != nil {
		return fmt.Errorf("error initializing session store: %w", err)
	}

	slog.Info("Session store configured using centralized configuration")

	cqrsDeps, err := bootstrap.WireCQRS(ctx, ns)
	if err != nil {
		return fmt.Errorf("error wiring CQRS: %w", err)
	}

	// Register route groups from new files
	RegisterPageRoutes(router, cqrsDeps)
	RegisterAPIRoutes(router, cqrsDeps)
	RegisterUploadRoutes(router, cqrsDeps)
	RegisterUserRoutes(router, cqrsDeps, sessionStore)

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
