package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/nats"
	"github.com/mbolli/tickit/internal/realtime"
	"github.com/mbolli/tickit/internal/session"
	"github.com/mbolli/tickit/internal/users"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mbolli/tickit/config"
	"github.com/mbolli/tickit/internal/routes"
	"golang.org/x/sync/errgroup"
)

func main() {

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Setup zerolog logger (use dev mode when not in production)
	isDev := config.Global.Environment == config.Dev
	logger.Setup(isDev)

	if err := run(ctx); err != nil && err != http.ErrServerClosed {
		logger.Error().Err(err).Msg("error running server")
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	logger.Info().Str("https_port", config.Global.HttpsPort).Msg("starting HTTPS server")
	logger.Info().Str("http_port", config.Global.HttpPort).Msg("starting HTTP redirect server")

	eg, egctx := errgroup.WithContext(ctx)

	/* compressor := middleware.NewCompressor(5)
	compressor.SetEncoder("br", func(w io.Writer, level int) io.Writer {
		return brotli.NewWriterV2(w, level)
	}) */

	// Create user routes using centralized session store
	sessionStore, err := session.GetGlobalSessionStore()
	if err != nil {
		return fmt.Errorf("error getting global session store: %w", err)
	}

	// Static Router ohne Middleware
	router := http.NewServeMux()
	router.Handle("/static/", static())

	appRouter := chi.NewMux()

	// Set up global middlewares
	appRouter.Use(
		// CSRF protection must come BEFORE DatastarSignalsMiddleware to read form data first
		users.NewCSRFMiddleware(nil, sessionStore),
		// DatastarSignalsMiddleware processes JSON signals
		realtime.DatastarSignalsMiddleware(),
		// compression (including brotli)
		// compressor.Handler,
		middleware.NoCache,
		middleware.GetHead,
		middleware.Logger,
		middleware.Recoverer,
		realtime.SetSessionMiddleware(sessionStore),
		realtime.ConnectionIDMiddleware(),
	)

	router.Handle("/", appRouter)

	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	if err != nil {
		return fmt.Errorf("failed to load TLS certificate or key: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3", "h2"},
	}

	/* redirectSrv := &http.Server{
		Addr: "0.0.0.0:" + config.Global.HttpPort,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always redirect HTTP to HTTPS
			host := r.Host
			// Remove port from host if present
			if colonIndex := len(host) - 1; colonIndex > 0 {
				for i := len(host) - 1; i >= 0; i-- {
					if host[i] == ':' {
						host = host[:i]
						break
					}
				}
			}
			httpsURL := "https://" + host + ":" + config.Global.HttpsPort + r.URL.String()
			http.Redirect(w, r, httpsURL, http.StatusMovedPermanently)
		}),
		ErrorLog: logger.NewStdLibLogger(),
	} */

	http2Srv := &http.Server{
		Addr:      "0.0.0.0:" + config.Global.HttpsPort,
		Handler:   router,
		TLSConfig: tlsConfig,
		ErrorLog:  logger.NewStdLibLogger(),
	}

	/* http3Srv := &http3.Server{
		Addr:      "0.0.0.0:" + config.Global.HttpsPort,
		Handler:   router,
		TLSConfig: http3.ConfigureTLSConfig(tlsConfig),
		Logger:    logger.GetLogger(),
	} */

	/* appRouter.Use(
		// QUIC headers for HTTP/3
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ProtoMajor < 3 {
					http3Srv.SetQUICHeaders(w.Header())
				}
				next.ServeHTTP(w, r)
			})
		},
	) */

	// Start embedded NATS server
	ns, err := nats.SetupNATS(ctx)
	if err != nil {
		return err
	}

	deps, err := bootstrap.WireDeps(ctx, ns, sessionStore)
	if err != nil {
		return fmt.Errorf("error wiring CQRS: %w", err)
	}

	// Start the global event processor
	if err := deps.EventProcessor.Start(); err != nil {
		return fmt.Errorf("error starting event processor: %w", err)
	}

	// Create a default notification for all users when server starts
	go func() {
		time.Sleep(2 * time.Second) // Wait a bit for the system to fully initialize
		err := deps.NotificationService.CreateGeneralNotification(
			ctx,
			"Server Started",
			"TickIt server has started successfully!",
		)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create startup notification")
		} else {
			logger.Info().Msg("Created startup notification")
		}
	}()

	// Ensure event processor is stopped on shutdown
	eg.Go(func() error {
		<-ctx.Done()
		return deps.EventProcessor.Stop()
	})

	// Serve registered routes
	if err := routes.SetupRoutes(appRouter, deps); err != nil {
		return fmt.Errorf("error setting up routes: %w", err)
	}

	/* eg.Go(func() error {
		// HTTP/1.1 without TLS
		if err := redirectSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http server error: %w", err)
		}
		return nil
	})
	*/
	eg.Go(func() error {
		// HTTP/1.1 and HTTP/2
		if err := http2Srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http server error: %w", err)
		}
		return nil
	})

	/* eg.Go(func() error {
		// HTTP/3
		if err := http3Srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http3 server error: %w", err)
		}
		return nil
	}) */

	eg.Go(func() error {
		<-egctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		logger.Debug().Msg("shutting down server...")
		//_ = redirectSrv.Shutdown(shutdownCtx)
		_ = http2Srv.Shutdown(shutdownCtx)
		//_ = http3Srv.Close()
		return nil
	})

	return eg.Wait()
}
