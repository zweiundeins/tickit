package web

import (
	"context"

	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/tracker"
)

// DashboardStatus is a project's counts by status through the server's
// cache, and how many times the cache counted anew.
func DashboardStatus(s *Server, ctx context.Context, q sqlite.Querier, project string, seq uint64) ([]tracker.StatusCount, int64, error) {
	list, err := s.counts.status(ctx, q, project, seq)
	return list, s.counts.reads.Load(), err
}
