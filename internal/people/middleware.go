package people

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/sqlite"
)

// Middleware resolves the actor of every request from its session
// (Resolve) and puts it in the request's context for Actor. It goes inside
// edge.Sessions, which gives the request its session id. It only reads.
func Middleware(db *sqlite.DB, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			p, err := Resolve(ctx, db.R, edge.SessionID(ctx), time.Now())
			if err != nil {
				if ctx.Err() == nil {
					log.Error("resolve the actor", "err", err)
				}
				http.Error(w, "The server is busy. Try again in a moment.", http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithActor(ctx, p)))
		})
	}
}
