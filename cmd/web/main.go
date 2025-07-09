package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/andybalholm/brotli"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mbolli/tickit/internal/routes"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/sync/errgroup"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: func() slog.Level {
			logLvlStr := os.Getenv("LOG_LEVEL")
			switch logLvlStr {
			case "DEBUG":
				return slog.LevelDebug
			case "WARN":
				return slog.LevelWarn
			case "ERROR":
				return slog.LevelError
			default:
				return slog.LevelInfo
			}
		}(),
	}))
	slog.SetDefault(logger)

	defer slog.Info("stopping server")

	if err := run(context.Background()); err != nil && err != http.ErrServerClosed {
		slog.Error("error running server", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	sctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	getPort := func() string {
		if p, ok := os.LookupEnv("PORT"); ok {
			return p
		}
		return "8080"
	}

	slog.Info("starting server on https://0.0.0.0:" + getPort())

	eg, egctx := errgroup.WithContext(sctx)
	eg.Go(func() error {
		router := chi.NewMux()
		compressor := middleware.NewCompressor(5)
		compressor.SetEncoder("br", func(w io.Writer, level int) io.Writer {
			return brotli.NewWriterV2(w, level)
		})

		cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
		if err != nil {
			return fmt.Errorf("failed to load TLS certificate or key: %w", err)
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h3", "h2"},
		}

		httpSrv := &http.Server{
			Addr:      "0.0.0.0:" + getPort(),
			Handler:   router,
			TLSConfig: tlsConfig,
			ErrorLog:  slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
		}

		http3Srv := &http3.Server{
			Addr:      "0.0.0.0:" + getPort(),
			Handler:   router,
			TLSConfig: http3.ConfigureTLSConfig(tlsConfig),
			Logger:    slog.Default(),
		}

		// Set up middlewares
		router.Use(
			compressor.Handler,
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.ProtoMajor < 3 {
						http3Srv.SetQUICHeaders(w.Header())
					}
					next.ServeHTTP(w, r)
				})
			},
			middleware.Logger,
			middleware.Recoverer,
		)

		// Serve static files
		router.Handle("/static/*", static())

		// Serve registered routes
		if err := routes.SetupRoutes(egctx, router); err != nil {
			return fmt.Errorf("error setting up routes: %w", err)
		}

		eg.Go(func() error {
			// HTTP/1.1 and HTTP/2
			if err := httpSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("http server error: %w", err)
			}
			return nil
		})

		eg.Go(func() error {
			// HTTP/3
			if err := http3Srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("http3 server error: %w", err)
			}
			return nil
		})

		go func() {
			<-egctx.Done()
			_ = httpSrv.Shutdown(context.Background())
			_ = http3Srv.Close()
		}()

		return eg.Wait()
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}
