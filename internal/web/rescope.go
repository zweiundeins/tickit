package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sse"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// A view checks that its watcher may read it when the stream asks for it
// (ViewFactory), and its key holds the scopes it shows. A change of a
// person's role or membership applies to their open streams at once
// (NFR-01.2): rescope makes every view each stream watches again, as the
// person now may, swaps those whose key changed (a list now without the
// project's issues) and takes off those the person may no longer read.
// A page that lost one is told to load again ($_revoked), which then
// answers 404 or 403 as it would to anyone without the right; its
// regions never keep showing what the person may no longer read.

// A page that shows the scope itself (its board, its backlog, its
// settings, one of its issues) loads again too: what the person may do
// there (the controls of a Viewer or of an Admin) comes with the page.

// rescope applies a change of the person's rights in scope (a project's
// id) to their open streams. Routes that change a membership call it
// after the commit.
func (s *Server) rescope(ctx context.Context, person, scope string) {
	streams := s.open.of(person)
	if len(streams) == 0 {
		return
	}
	watching := map[string][]string{} // stream ID to the keys it watches now
	for key := range s.hub.Stats().RendersByKey {
		for _, id := range s.hub.Watchers(key) {
			if _, ok := streams[id]; ok {
				watching[id] = append(watching[id], key)
			}
		}
	}
	for id, st := range streams {
		want, revoked, err := s.wanted(ctx, st)
		if err != nil {
			s.log.Warn("rescope", "err", err)
			continue
		}
		revoked = revoked || s.shows(ctx, watching[id], scope)
		keep := map[string]bool{
			s.personView(st.actor).Key: true, s.demoView().Key: true, connectedView.Key: true,
			tabs.PrefsView(st.sid, st.tab).Key: true, tabs.View(st.sid, st.tab).Key: true, revokedKey(id): true,
		}
		have := map[string]bool{}
		for _, key := range watching[id] {
			have[key] = true
			if _, ok := want[key]; !ok && !keep[key] {
				s.hub.Unwatch(id, key)
			}
		}
		for key, v := range want {
			if !have[key] {
				s.hub.Watch(id, v)
			}
		}
		if revoked {
			s.hub.Watch(id, revokedView(id))
		}
	}
}

// wanted are the views the stream's page asks for, by key, as its person
// may now read them, and whether one is no longer theirs to read: the
// views its URL named, the list the tab shows now and the views its row
// added (side panels, the editor).
func (s *Server) wanted(ctx context.Context, st *openStream) (map[string]live.View, bool, error) {
	want := map[string]live.View{}
	revoked := false
	err := s.db.View(ctx, func(tx *sql.Tx) error {
		row, err := tabs.Get(ctx, tx, st.sid, st.tab)
		if err != nil {
			return err
		}
		list, err := tabs.Load[listState](row, listArea)
		if err != nil {
			return err
		}
		feed, err := tabs.Load[feedState](row, feedArea)
		if err != nil {
			return err
		}
		var specs []string
		for _, spec := range st.specs {
			if strings.HasPrefix(spec, "list:") && list.Query != "" {
				spec, _ = listSpec(list.Query) // the list it moved to (/cmd/list/view)
			}
			if sameFeed(spec, feed.Spec) {
				spec = feed.Spec // the window it moved to (/cmd/feed/window)
			}
			specs = append(specs, spec)
		}
		for _, w := range row.Watched {
			specs = append(specs, w.View)
		}
		for _, spec := range specs {
			vs, err := s.resolve(ctx, tx, st.actor, spec)
			var denial interface{ Denied() bool }
			switch {
			case errors.Is(err, access.ErrNotFound), errors.As(err, &denial) && denial.Denied():
				revoked = true
				continue
			case err != nil:
				s.log.Debug("rescope: a view is gone", "view", spec, "err", err)
				continue
			}
			for _, v := range vs {
				want[v.Key] = v
			}
		}
		return nil
	})
	return want, revoked, err
}

// shows reports whether one of the view keys shows the project: its
// board, board head, backlog, settings or feed, or one of its issues.
func (s *Server) shows(ctx context.Context, keys []string, project string) bool {
	for _, key := range keys {
		kind, rest, _ := strings.Cut(key, ":")
		switch kind {
		case "board", "boardhead", "backlog", "settings":
			if strings.HasPrefix(rest, project) {
				return true
			}
		case "activity":
			if strings.HasPrefix(rest, "project:"+project+":") {
				return true
			}
		case "issue", "issue-typing":
			id, _, _ := strings.Cut(rest, ":")
			var in int
			if s.db.R.QueryRowContext(ctx, `SELECT 1 FROM issues WHERE id = ? AND project_id = ?`, id, project).Scan(&in) == nil {
				return true
			}
		}
	}
	return false
}

// revokedKey is the key of the view that tells the stream's page to load
// again; it holds the stream's ID, so it is the stream's alone.
func revokedKey(id string) string { return "revoked:" + id }

// revokedView sets $_revoked, on which the page loads again.
func revokedView(id string) live.View {
	frame, _ := json.Marshal(map[string]bool{"_revoked": true})
	return live.View{
		Key: revokedKey(id),
		Render: func(context.Context, *sql.Tx) (any, error) {
			return sse.PatchSignals(frame, false), nil
		},
	}
}
