package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/activity"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/views"
)

// The feeds of the home page, a project and a space (FR-69) are long
// lists in sb-virtual-scroll, of which a frame holds one window of
// feedWindow entries. A window is a view of its own, named by where it
// starts in the feed (views.FeedSpec): "activity-home:<from>",
// "activity-project:<id>.<from>", "activity-space:<id>.<from>", from a
// multiple of views.FeedStep, so tabs scrolled to about the same place
// share its render, and the top window is everyone's. Scrolling asks for
// another window (sb-window), which moves the tab's stream from the old
// window's view to the new one with POST /cmd/feed/window, as the issue
// list moves between lists; the tab's row keeps the window for rescope.
// The window from the head moves when entries arrive: the page keeps the
// reader's place by the entries' ids (static/feed/feed.js).

const (
	homeFeedKind    = "activity-home"
	projectFeedKind = "activity-project"
	spaceFeedKind   = "activity-space"
	// maxFeedEntries is how far down a feed goes; the read of a window
	// reads the entries before it too, at most this many.
	maxFeedEntries = 1000
	feedWindow     = activity.PageSize
	maxFeedFrom    = maxFeedEntries - feedWindow
	feedArea       = "feed"
)

// feedComponents are the Starbase components of a page with a feed.
var feedComponents = []string{"virtual-scroll"}

func (s *Server) routesFeed() {
	s.views.RegisterWithPresenceOf(homeFeedKind, s.activityHomeView, func(string) string { return views.HomePresence })
	s.views.Register(projectFeedKind, s.activityScopeView(access.Project))
	s.views.Register(spaceFeedKind, s.activityScopeView(access.Space))
	s.post("POST /cmd/feed/window", s.feedWindowChange)
}

// feedFrom is the window a page's ?from= names, 0 for none or nonsense.
func feedFrom(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("from"))
	if err != nil {
		return 0
	}
	return clampFrom(n)
}

func clampFrom(n int) int { return min(max(0, n)/views.FeedStep*views.FeedStep, maxFeedFrom) }

// parseFrom reads a window's start from a view's name.
func parseFrom(kind, v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > maxFeedFrom {
		return 0, fmt.Errorf("%w: %s starts at 0 to %d", errBadView, kind, maxFeedFrom)
	}
	return clampFrom(n), nil
}

// feedWindowOf reads the window of a feed from from: up to feedWindow
// entries, and how many the feed has as far as the read went (Known when
// it ended, or at maxFeedEntries).
func (s *Server) feedWindowOf(ctx context.Context, q sqlite.Querier, keep func(access.Scope) bool, kind, scope, path string, from int, now time.Time) (views.ActivityFeed, error) {
	entries, more, err := activity.Feed(ctx, q, keep, from+feedWindow)
	if err != nil {
		return views.ActivityFeed{}, err
	}
	win := entries[min(from, len(entries)):]
	id := views.ActivityID("home")
	if scope != "" {
		id = views.ActivityID(strings.TrimPrefix(kind, "activity-") + "-" + scope)
	}
	f, err := s.feed(ctx, q, id, win, now)
	f.Kind, f.Scope, f.From, f.Window, f.MaxFrom, f.Path = kind, scope, from, feedWindow, maxFeedFrom, path
	switch {
	case !more:
		f.Total, f.Known = len(entries), true
	case from+feedWindow >= maxFeedEntries:
		f.Total, f.Known = maxFeedEntries, true
	default:
		f.Total = from + len(win) + feedWindow
	}
	return f, err
}

// homeFeed is the home page's feed from from.
func (s *Server) homeFeed(ctx context.Context, q sqlite.Querier, readable []access.Scope, from int, now time.Time) (views.ActivityFeed, error) {
	return s.feedWindowOf(ctx, q, activity.In(readable), homeFeedKind, "", "/", from, now)
}

// ScopeActivity is the feed of a project or a space (FR-69.1) from from,
// for its page: the page checks that the person may read the scope,
// renders views.Activity(feed) and names the view FeedSpec(feed.Kind,
// feed.Scope, feed.From) in Page.Views.
func (s *Server) ScopeActivity(ctx context.Context, q sqlite.Querier, scope access.Scope, from int, now time.Time) (views.ActivityFeed, error) {
	info, err := access.Get(ctx, q, scope)
	if err != nil {
		return views.ActivityFeed{}, err
	}
	kind, path := projectFeedKind, "/projects/"+info.Key+"/activity"
	if scope.Kind == access.Space {
		kind, path = spaceFeedKind, "/spaces/"+info.Key
	}
	return s.feedWindowOf(ctx, q, activity.In([]access.Scope{scope}), kind, scope.ID, path, from, now)
}

// activityHomeView is a window of the home page's feed, shared by everyone
// who reads the same projects and spaces (FR-69.2): its key holds both.
// arg is where the window starts.
func (s *Server) activityHomeView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	from, err := parseFrom(homeFeedKind, arg)
	if err != nil {
		return live.View{}, err
	}
	readable, err := activity.Readable(ctx, q, actor)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "activity:home:" + activity.Key(readable) + ":" + strconv.Itoa(from),
		Topics: feedTopics,
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			f, err := s.homeFeed(ctx, querier(ctx, tx), readable, from, time.Now())
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.Activity(f))
		},
	}, nil
}

// activityScopeView is a window of the feed of one project or space, for
// its page: "activity-project:<id>.<from>", "activity-space:<id>.<from>"
// ("<id>" alone from 0), shared by everyone who may read it.
func (s *Server) activityScopeView(kind access.Kind) ViewFactory {
	return func(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
		id, at, dotted := strings.Cut(arg, ".")
		from := 0
		if dotted {
			var err error
			if from, err = parseFrom(string(kind), at); err != nil {
				return live.View{}, err
			}
		}
		scope := access.Scope{Kind: kind, ID: id}
		ok, err := access.Can(ctx, q, actor, access.Read, scope)
		switch {
		case err != nil:
			return live.View{}, err
		case !ok:
			return live.View{}, fmt.Errorf("%s %s: %w", kind, id, access.ErrNotFound)
		}
		return live.View{
			Key:    "activity:" + string(kind) + ":" + id + ":" + strconv.Itoa(from),
			Topics: feedTopics,
			Every:  time.Minute,
			Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
				f, err := s.ScopeActivity(ctx, querier(ctx, tx), scope, from, time.Now())
				if err != nil {
					return nil, err
				}
				return patch(ctx, views.Activity(f))
			},
		}, nil
	}
}

// feedState is the feed area's state of a tab: the window its stream
// watches.
type feedState struct {
	Spec string `json:"spec"`
	Key  string `json:"key"`
}

// feedOf is the kind and scope of a feed's view name: "activity-home",
// ""; "activity-project", the project's id; ok false for another view.
func feedOf(spec string) (kind, scope string, ok bool) {
	kind, arg, _ := strings.Cut(spec, ":")
	switch kind {
	case homeFeedKind:
		return kind, "", true
	case projectFeedKind, spaceFeedKind:
		scope, _, _ = strings.Cut(arg, ".")
		return kind, scope, true
	}
	return "", "", false
}

func sameFeed(a, b string) bool {
	ka, sa, oka := feedOf(a)
	kb, sb, okb := feedOf(b)
	return oka && okb && ka == kb && sa == sb
}

// feedWindowChange moves the tab's open stream to another window of a
// feed: POST /cmd/feed/window {tab, views, feed}, views the stream's
// views as the page has them, feed the new window's name. It unwatches
// the window the stream watched (the one in views, or the one the tab's
// row says it moved to) after the commit and in its order, so of two
// quick changes the later window is the one the stream keeps.
func (s *Server) feedWindowChange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tab   string `json:"tab"`
		Views string `json:"views"`
		Feed  string `json:"feed"`
	}
	if !s.allow(w, r, classView) {
		return
	}
	ctx := r.Context()
	actor, sid := people.Actor(ctx), edge.SessionID(ctx)
	if err := sse.ReadSignalsLimit(w, r, &req, 8<<10); err != nil || !validID(req.Tab) || actor.IsZero() {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if specs, err := parseViews(req.Feed); err != nil || len(specs) != 1 {
		http.Error(w, "bad view", http.StatusBadRequest)
		return
	}
	if _, _, ok := feedOf(req.Feed); !ok {
		http.Error(w, "bad view", http.StatusBadRequest)
		return
	}
	if mustChange(w, actor) || !s.connected(w, r, actor, req.Tab) {
		return
	}
	var view live.View
	var prev string // the key of the window the page's stream named
	err := s.db.View(ctx, func(tx *sql.Tx) error {
		vs, err := s.resolve(ctx, tx, actor, req.Feed)
		if err != nil {
			return err
		}
		view = vs[0]
		old, _ := parseViews(req.Views)
		for _, spec := range old {
			if spec != req.Feed && sameFeed(spec, req.Feed) {
				if vs, err := s.resolve(ctx, tx, actor, spec); err == nil {
					prev = vs[0].Key
				}
			}
		}
		return nil
	})
	if err != nil {
		s.failView(w, r, err)
		return
	}
	id, now := streamID(actor, req.Tab), time.Now()
	err = s.w.Do(ctx, writer.Func(func(tx *writer.Tx) error {
		var old string
		err := tabs.Update(tx, sid, req.Tab, feedArea, now, func(st *feedState) error {
			if sameFeed(st.Spec, req.Feed) {
				old = st.Key
			}
			st.Spec, st.Key = req.Feed, view.Key
			return nil
		})
		if err != nil {
			return err
		}
		tx.AfterCommit(func() {
			for _, k := range []string{old, prev} {
				if k != "" && k != view.Key {
					s.hub.Unwatch(id, k)
				}
			}
			s.hub.Watch(id, view)
		})
		return nil
	}))
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
