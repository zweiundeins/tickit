package web

import (
	"context"
	"database/sql"
	"time"

	"github.com/zweiundeins/lith/live"

	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/views"
)

// routesDemo registers the demo's own: the reset's notices and the
// first-visit hint (DM-03, DM-10, DM-11).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesDemo() {}

// demoView is the demo notice of every page (DM-10.1), which counts down
// to the nightly reset; the warning of DM-03.2 belongs here too. It is the
// same for everyone, and renders again every minute: an unchanged frame
// goes nowhere.
func (s *Server) demoView() live.View {
	return live.View{
		Key:    "demo",
		Topics: []string{demo.Topic},
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			return patch(ctx, views.DemoNotice(demo.Notice(time.Now())))
		},
	}
}
