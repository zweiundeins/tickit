package web

import (
	"context"

	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/tracker"
)

// HomeProjects is the home page's projects through the server's cache.
func HomeProjects(s *Server, ctx context.Context, q sqlite.Querier, readable []string, seq uint64) ([]tracker.Project, error) {
	return s.projects.get(ctx, q, readable, seq)
}
