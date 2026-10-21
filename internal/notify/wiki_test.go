package notify_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"

	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// FR-114: a comment on a page notifies its watchers but the commenter;
// FR-112: a version that mentions someone tells them.
func TestPageNotifications(t *testing.T) {
	e := newEnv(t)
	e.store = events.Must(events.New(append(append(tracker.Parts(), wiki.Parts()...), notify.Parts()...)...))
	tracker.Bind(e.store)
	wiki.Bind(e.store)
	t.Cleanup(func() { wiki.Bind(nil) })
	e.runner = e.Run(t, e.store)
	space := e.result(wiki.CreateSpace{By: e.by(sunny), Key: "ENG", Name: "Engineering", Open: true})
	page := e.result(wiki.CreatePage{By: e.by(sunny), Space: space["id"], Parent: space["home"], Title: "Deploy"})["id"]
	e.must(wiki.WatchPage{By: e.by(misty), Page: page, Watch: true})
	e.result(wiki.AddComment{By: e.by(rainy), Page: page, Body: tracker.PlainDoc("Looks good")})
	time.Sleep(50 * time.Millisecond)
	e.settle()
	for _, p := range []struct {
		who  string
		got  []string
		want bool
	}{{"sunny, the creator", e.of(sunny), true}, {"misty, a watcher", e.of(misty), true}, {"rainy, the commenter", e.of(rainy), false}} {
		if has := strings.Contains(strings.Join(p.got, "|"), "commented on Deploy"); has != p.want {
			t.Errorf("%s: %v", p.who, p.got)
		}
	}
	mention, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{
		map[string]any{"type": "text", "text": "Ask "}, map[string]any{"type": "mention", "attrs": map[string]any{"personId": rainy.ID, "label": "Rainy"}}}}}})
	e.must(wiki.SetDocument{By: e.by(sunny), Page: page, Doc: mention, Message: "Ask Rainy"})
	time.Sleep(50 * time.Millisecond)
	e.settle()
	if got := strings.Join(e.of(rainy), "|"); !strings.Contains(got, "mentioned you") {
		t.Errorf("rainy mentioned in a version: %s", got)
	}
}

// FR-91, FR-114: an inline comment is a page comment: it notifies the
// page's watchers but the commenter.
func TestInlineCommentNotifies(t *testing.T) {
	e := newEnv(t)
	e.store = events.Must(events.New(append(append(tracker.Parts(), wiki.Parts()...), notify.Parts()...)...))
	tracker.Bind(e.store)
	wiki.Bind(e.store)
	t.Cleanup(func() { wiki.Bind(nil) })
	e.runner = e.Run(t, e.store)
	space := e.result(wiki.CreateSpace{By: e.by(sunny), Key: "ENG", Name: "Engineering", Open: true})
	page := e.result(wiki.CreatePage{By: e.by(sunny), Space: space["id"], Parent: space["home"], Title: "Deploy", Doc: tracker.PlainDoc("Ship it on Friday")})["id"]
	e.must(wiki.WatchPage{By: e.by(misty), Page: page, Watch: true})
	var h wiki.Head
	e.View(t, func(tx *sql.Tx) (err error) { h, err = wiki.HeadOf(context.Background(), tx, page); return err })
	e.result(wiki.AddInlineComment{By: e.by(rainy), Page: page, Epoch: h.Epoch, Base: h.Version, From: 12, To: 18, Text: "Friday", Body: tracker.PlainDoc("Not on a Friday")})
	time.Sleep(50 * time.Millisecond)
	e.settle()
	for _, p := range []struct {
		who  string
		got  []string
		want bool
	}{{"sunny, the creator", e.of(sunny), true}, {"misty, a watcher", e.of(misty), true}, {"rainy, the commenter", e.of(rainy), false}} {
		if has := strings.Contains(strings.Join(p.got, "|"), "commented on Deploy"); has != p.want {
			t.Errorf("%s: %v", p.who, p.got)
		}
	}
}
