package web

import (
	"context"
	"database/sql"
	"time"

	"github.com/a-h/templ"
	"github.com/zweiundeins/lith/live"

	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/views"
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

// quiet makes v render nothing while the demo resets: the wipe empties
// the tables and the seed fills them over several commits, and a frame
// read in between would show a half-seeded board. The reset renders every
// view again once it ends (Demo.Reset's MarkAll). The demo's, the tab's and
// the prefs' views stay out of it: they say that the demo resets.
func (s *Server) quiet(v live.View) live.View {
	render := v.Render
	v.Render = func(ctx context.Context, tx *sql.Tx) (any, error) {
		q := s.demo.Quiet()
		if q%2 == 1 {
			return nil, nil
		}
		frame, err := render(ctx, tx)
		if err != nil || s.demo.Quiet() != q {
			return nil, err
		}
		return frame, nil
	}
	return v
}
