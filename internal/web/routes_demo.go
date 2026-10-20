package web

import (
	"context"
	"database/sql"
	"time"

	"github.com/a-h/templ"
	"github.com/zweiundeins/lith/live"

	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/views"
)

// routesDemo registers the demo's own: the reset's notices and the
// first-visit hint (DM-03, DM-10, DM-11). "Reset now" is the admin's
// (routes_admin.go), moderation has routes_moderation.go.
func (s *Server) routesDemo() {}

// demoView is the demo notice of every page (DM-10.1), which counts down
// to the nightly reset, and the reset's warning ten minutes before
// (DM-03.2) and while it runs (DM-03.4). It is the same for everyone, and
// renders again every minute and when the demo's topic is touched (the
// schedule touches it as the warning starts, the reset as it starts and
// ends): an unchanged frame goes nowhere.
func (s *Server) demoView() live.View {
	return live.View{
		Key:    "demo",
		Topics: []string{demo.Topic},
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			st := s.demo.Status(time.Now())
			return patch(ctx, templ.Join(views.DemoNotice(st.Remaining, false), views.DemoAlert(demoAlert(st))))
		},
	}
}

// demoAlert is what the warning says at a status, "" for nothing.
func demoAlert(st demo.Status) string {
	switch {
	case st.Resetting:
		return demo.MsgResetting + ". This page shows the fresh demo in a moment."
	case st.Soon:
		return demo.MsgWarning
	}
	return ""
}
