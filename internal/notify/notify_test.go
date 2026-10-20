package notify_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/dbtest"
	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

var (
	t0    = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	sunny = people.GuestOf("session-sunny")
	rainy = people.GuestOf("session-rainy")
	misty = people.GuestOf("session-misty")
)

// env is a database with the tracker's and notify's parts, an events
// runner and three guests.
type env struct {
	*dbtest.Env
	t      *testing.T
	store  *events.Store
	runner *events.Runner
	ops    int
	now    time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{Env: dbtest.Open(t), t: t, now: t0}
	e.store = events.Must(events.New(append(tracker.Parts(), notify.Parts()...)...))
	tracker.Bind(e.store)
	notify.Bind(e.W)
	t.Cleanup(func() { tracker.Bind(nil); notify.Bind(nil) })
	e.Do(t, people.SeedPeople{At: t0})
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		for _, g := range []people.Person{sunny, rainy, misty} {
			if err := people.Ensure(tx, g, t0); err != nil {
				return err
			}
		}
		return nil
	}))
	e.runner = e.Run(t, e.store)
	return e
}

// by is p acting a second after the last action, or after a pause.
func (e *env) by(p people.Person) tabs.By {
	e.ops++
	e.now = e.now.Add(time.Second)
	return tabs.By{Actor: p, Op: fmt.Sprint("op", e.ops), At: e.now}
}

func (e *env) must(cmd writer.Command) {
	e.t.Helper()
	if err := e.W.Do(context.Background(), cmd); err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
	e.settle()
}

func (e *env) result(cmd writer.ResultCommand) map[string]string {
	e.t.Helper()
	b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return cmd.ApplyResult(tx) })
	if err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		e.t.Fatal(err)
	}
	e.settle()
	return m
}

// settle waits until the reactions have handled every event.
func (e *env) settle() {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for calm := 0; calm < 3; {
		calm++
		for _, p := range e.runner.Stats().Projectors {
			if p.CatchUp && (p.Lag > 0 || p.Status != "live") {
				calm = 0
			}
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("reactions behind after 5s: %+v", e.runner.Stats())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := e.W.Flush(context.Background()); err != nil {
		e.t.Fatal(err)
	}
}

// project is an open project of sunny's, in which rainy and misty acted
// (so they can be assigned), and its first issue, sunny's.
func (e *env) project() (project, issue string) {
	e.t.Helper()
	project = e.result(tracker.CreateProject{By: e.by(sunny), Key: "WEB", Name: "Website", Open: true})["id"]
	issue = e.result(tracker.CreateIssue{By: e.by(sunny), Project: project, Type: tracker.Task, Summary: "Fix the login"})["id"]
	for _, p := range []people.Person{rainy, misty} {
		e.result(tracker.CreateIssue{By: e.by(p), Project: project, Type: tracker.Task, Summary: "Something of " + p.Name})
	}
	return project, issue
}

// of lists a person's notifications as sentences, newest first.
func (e *env) of(p people.Person) []string {
	e.t.Helper()
	var out []string
	e.View(e.t, func(tx *sql.Tx) error {
		list, err := notify.List(context.Background(), tx, p.ID, 100)
		for _, n := range list {
			s := n.Sentence("X")
			if n.Text != "" {
				s += ": " + n.Text
			}
			out = append(out, s)
		}
		return err
	})
	return out
}

func (e *env) count() int {
	var n int
	e.View(e.t, func(tx *sql.Tx) error { return tx.QueryRow(`SELECT count(*) FROM notifications`).Scan(&n) })
	return n
}

func want(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n\t%s\nwant\n\t%s", strings.Join(got, "\n\t"), strings.Join(want, "\n\t"))
	}
}

func doc(inline ...any) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": inline}}})
	return b
}

func text(s string) any { return map[string]any{"type": "text", "text": s} }

func mention(p people.Person) any {
	return map[string]any{"type": "mention", "attrs": map[string]string{"personId": p.ID, "label": p.Name}}
}

// mentionDoc is a paragraph that mentions p between two sentences.
func mentionDoc(p people.Person) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{
		map[string]any{"type": "text", "text": "First things first. Could "},
		map[string]any{"type": "mention", "attrs": map[string]string{"personId": p.ID, "label": p.Name}},
		map[string]any{"type": "text", "text": " look at this? Thanks."},
	}}}})
	return b
}

// FR-111: assigning someone else notifies them; assigning oneself, and
// unassigning, notify nobody.
func TestAssignment(t *testing.T) {
	e := newEnv(t)
	_, issue := e.project()
	e.must(tracker.Assign{By: e.by(sunny), Issue: issue, Seen: 1 << 40, Assignee: rainy.ID})
	want(t, e.of(rainy), "X assigned WEB-1 to you")
	e.must(tracker.Assign{By: e.by(sunny), Issue: issue, Seen: 1 << 40, Assignee: sunny.ID})
	want(t, e.of(sunny))
	e.must(tracker.Assign{By: e.by(sunny), Issue: issue, Seen: 1 << 40, Assignee: ""})
	if n := e.count(); n != 1 {
		t.Errorf("%d notifications, want 1", n)
	}
	// An issue created with an assignee notifies them, not its reporter.
	project := e.result(tracker.CreateProject{By: e.by(rainy), Key: "OPS", Name: "Ops", Open: true})["id"]
	e.result(tracker.CreateIssue{By: e.by(misty), Project: project, Type: tracker.Task, Summary: "Own", Assignee: misty.ID})
	e.result(tracker.CreateIssue{By: e.by(rainy), Project: project, Type: tracker.Task, Summary: "For Misty", Assignee: misty.ID})
	want(t, e.of(misty), "X assigned OPS-2 to you")
	n := e.of(rainy)
	if len(n) != 1 {
		t.Errorf("rainy: %q", n)
	}
	// The sentence names the actor as the requirement says.
	e.View(t, func(tx *sql.Tx) error {
		list, err := notify.List(context.Background(), tx, rainy.ID, 1)
		if err == nil && list[0].Sentence("Sunny Otter") != "Sunny Otter assigned WEB-1 to you" {
			t.Errorf("sentence: %q", list[0].Sentence("Sunny Otter"))
		}
		return err
	})
}

// FR-112, FR-67.3: a mention in a description or a comment notifies once,
// with the sentence around it; an edit that keeps it notifies nobody, one
// that adds a new person notifies them; the author mentioning themselves
// is told nothing.
func TestMentions(t *testing.T) {
	e := newEnv(t)
	project, _ := e.project()
	issue := e.result(tracker.CreateIssue{By: e.by(sunny), Project: project, Type: tracker.Task, Summary: "With a mention", Description: mentionDoc(rainy)})["id"]
	sentence := "Could @" + rainy.Name + " look at this?"
	want(t, e.of(rainy), "X mentioned you in WEB-4: "+sentence)
	// The same mention again, and more text: nothing new.
	e.must(tracker.SetDescription{By: e.by(sunny), Issue: issue, Description: mentionDoc(rainy)[:0]})
	e.must(tracker.SetDescription{By: e.by(sunny), Issue: issue, Description: mentionDoc(rainy)})
	want(t, e.of(rainy), "X mentioned you in WEB-4: "+sentence)
	// A comment that mentions misty and sunny, by sunny.
	body := doc(text("Hi "+sunny.Name+", could "), mention(misty), text(" look at this? Thanks."))
	e.result(tracker.AddComment{By: e.by(sunny), Issue: issue, Body: body})
	want(t, e.of(misty), "X mentioned you in WEB-4: Hi "+sunny.Name+", could @"+misty.Name+" look at this?")
	want(t, e.of(sunny))
	// Editing the comment to add rainy tells rainy once; editing again, nobody.
	var comment string
	e.View(t, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT id FROM comments WHERE subject_id = ?`, issue).Scan(&comment)
	})
	both := doc(text("Hi "+sunny.Name+", could "), mention(misty), text(" and "), mention(rainy), text(" look?"))
	e.must(tracker.EditComment{By: e.by(sunny), Comment: comment, Body: both})
	e.must(tracker.EditComment{By: e.by(sunny), Comment: comment, Body: body})
	e.must(tracker.EditComment{By: e.by(sunny), Comment: comment, Body: both})
	if got := e.of(rainy); len(got) != 3 || !strings.HasPrefix(got[0], "X mentioned you in WEB-4: Hi") { // the description made rainy a watcher: the comment, once
		t.Errorf("rainy: %q", got)
	}
	if got := e.of(misty); len(got) != 1 {
		t.Errorf("misty: %q", got)
	}
	// FR-15: a mention typed into the co-edited description tells the
	// person once, when its version is cut; later versions keep it and
	// tell nobody.
	other := e.result(tracker.CreateIssue{By: e.by(sunny), Project: project, Type: tracker.Task, Summary: "Co-edited", Description: doc(text("Notes. "))})["id"]
	commit := func(ref, steps string) {
		t.Helper()
		var h tracker.DescHead
		e.View(t, func(tx *sql.Tx) (err error) {
			h, err = tracker.DescriptionHead(context.Background(), tx, other)
			return err
		})
		e.must(&tracker.CommitDescription{By: e.by(rainy), Session: "s-rainy", Tab: "t-rainy", Issue: other, Epoch: h.Epoch, Base: h.Version,
			Ref: ref, Client: "0123456789ab", Steps: json.RawMessage(steps)})
	}
	m, _ := json.Marshal(mention(misty))
	commit("r1", `[{"stepType":"replace","from":8,"to":8,"slice":{"content":[`+string(m)+`,{"type":"text","text":" please look."}]}}]`)
	e.must(tracker.CutDescriptions{At: e.now})
	want(t, e.of(misty)[:1], "X mentioned you in WEB-5: @"+misty.Name+" please look.")
	commit("r2", `[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"More. "}]}}]`)
	e.must(tracker.CutDescriptions{At: e.now})
	if got := e.of(misty); len(got) != 2 {
		t.Errorf("misty after a second version: %q", got)
	}
}

// FR-113: a comment on, and a status change of, a watched issue notify its
// watchers but the actor; FR-113.2: several of them by one person within
// a minute are one notification, a later one another.
func TestWatchers(t *testing.T) {
	e := newEnv(t)
	_, issue := e.project() // sunny reports it, and watches
	for _, p := range []people.Person{rainy, misty} {
		e.must(tracker.Watch{By: e.by(p), Issue: issue, Watch: true})
	}
	e.must(tracker.SetStatus{By: e.by(rainy), Issue: issue, Seen: 1 << 40, Status: tracker.StatusInProgress})
	want(t, e.of(sunny), "X moved WEB-1 to In Progress")
	want(t, e.of(misty), "X moved WEB-1 to In Progress")
	want(t, e.of(rainy))
	// Within the minute: a comment and another move merge into it.
	e.result(tracker.AddComment{By: e.by(rainy), Issue: issue, Body: tracker.PlainDoc("Started on it.")})
	e.must(tracker.Move{By: e.by(rainy), Issue: issue, Status: tracker.StatusInReview, Seen: 1 << 40})
	want(t, e.of(sunny), "X moved WEB-1 to In Review and commented: Started on it.")
	// A comment by someone else is a notification of its own, and so is
	// rainy's next change after the minute.
	e.result(tracker.AddComment{By: e.by(misty), Issue: issue, Body: tracker.PlainDoc("Looks good.")})
	e.now = e.now.Add(2 * time.Minute)
	e.result(tracker.AddComment{By: e.by(rainy), Issue: issue, Body: tracker.PlainDoc("Done now.")})
	e.result(tracker.AddComment{By: e.by(rainy), Issue: issue, Body: tracker.PlainDoc("Really.")})
	want(t, e.of(sunny), "X left 2 comments on WEB-1: Really.", "X commented on WEB-1: Looks good.", "X moved WEB-1 to In Review and commented: Started on it.")
	want(t, e.of(misty), "X left 2 comments on WEB-1: Really.", "X moved WEB-1 to In Review and commented: Started on it.")
	// A read notification takes no more changes: the next one is new.
	e.View(t, func(tx *sql.Tx) error { return nil })
	e.must(notify.ReadAll{By: e.by(misty)})
	e.result(tracker.AddComment{By: e.by(rainy), Issue: issue, Body: tracker.PlainDoc("One more.")})
	if got := e.of(misty); len(got) != 3 || got[0] != "X commented on WEB-1: One more." {
		t.Errorf("misty: %q", got)
	}
	// A move within a column and other fields tell nobody.
	before := e.count()
	e.must(tracker.SetPriority{By: e.by(rainy), Issue: issue, Seen: 1 << 40, Priority: tracker.High})
	if e.count() != before {
		t.Error("a priority change notified")
	}
}

// FR-116, lith's catch-up: the reactions skip the history a new runner
// finds, and a projector that goes back to the start makes nothing twice
// (writer.Once by position, and the table's keys).
func TestReplayAndRestart(t *testing.T) {
	e := newEnv(t)
	_, issue := e.project()
	e.must(tracker.Assign{By: e.by(sunny), Issue: issue, Seen: 1 << 40, Assignee: rainy.ID})
	e.must(tracker.Watch{By: e.by(misty), Issue: issue, Watch: true})
	e.result(tracker.AddComment{By: e.by(sunny), Issue: issue, Body: tracker.PlainDoc("Hello.")})
	before := e.count()
	if before != 3 { // rainy assigned, and as a watcher of the comment; misty of the comment
		t.Fatalf("%d notifications, want 3", before)
	}
	e.replay()
	if n := e.count(); n != before {
		t.Errorf("a replay made %d notifications of %d", n, before)
	}
	// The table's keys alone hold too, once the op ids are gone.
	e.must(writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`DELETE FROM lith_ops`)
		return err
	}))
	e.replay()
	if n := e.count(); n != before {
		t.Errorf("a replay without op ids made %d notifications of %d", n, before)
	}
	// A new runner, as after a restart or a deploy, makes nothing (FR-116.2).
	e.restart()
	if n := e.count(); n != before {
		t.Errorf("a restart made %d notifications of %d", n, before)
	}
}

// replay runs the reactions again from the first event, with a runner of
// their own, as one would after a failure.
func (e *env) replay() {
	e.t.Helper()
	e.must(writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE lith_projections SET position = 0 WHERE name LIKE 'notify_%'`)
		return err
	}))
	e.restart()
}

// restart runs a second runner until the reactions have caught up.
func (e *env) restart() {
	e.t.Helper()
	var head int64
	e.View(e.t, func(tx *sql.Tx) error { return tx.QueryRow(`SELECT max(position) FROM lith_events`).Scan(&head) })
	second := events.NewRunner(e.store, e.DB, e.W, events.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); second.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		caught := true
		for _, p := range second.Stats().Projectors {
			caught = caught && (!p.CatchUp || p.Lag == 0 && p.Status == "live")
		}
		if caught {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	e.settle()
}

// FR-115: one's own notifications are marked read and dismissed one by one
// or all at once; someone else's are refused for want of the right;
// opening a subject marks its notifications read.
func TestReadAndDismiss(t *testing.T) {
	e := newEnv(t)
	_, issue := e.project()
	e.must(tracker.Assign{By: e.by(sunny), Issue: issue, Seen: 1 << 40, Assignee: rainy.ID})
	e.must(tracker.Watch{By: e.by(rainy), Issue: issue, Watch: true})
	e.now = e.now.Add(2 * time.Minute)
	e.result(tracker.AddComment{By: e.by(sunny), Issue: issue, Body: tracker.PlainDoc("Hello.")})
	panel := func() notify.Panel {
		var p notify.Panel
		e.View(t, func(tx *sql.Tx) error {
			var err error
			p, err = notify.PanelOf(context.Background(), tx, rainy.ID)
			return err
		})
		return p
	}
	p := panel()
	if p.Unread != 2 || len(p.Items) != 2 {
		t.Fatalf("panel: %+v", p)
	}
	first, second := p.Items[1].ID, p.Items[0].ID
	err := e.W.Do(context.Background(), notify.Read{By: e.by(misty), ID: first})
	var denial *access.Denial
	if !errors.As(err, &denial) {
		t.Errorf("someone else's: %v", err)
	}
	if err := e.W.Do(context.Background(), notify.Read{By: e.by(rainy), ID: 1 << 40}); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("an unknown one: %v", err)
	}
	e.Touched()
	e.must(notify.Read{By: e.by(rainy), ID: first})
	if got := e.Touched(); !contains(got, people.PersonTopic(rainy.ID)) {
		t.Errorf("touched %v", got)
	}
	if p := panel(); p.Unread != 1 || len(p.Items) != 2 {
		t.Errorf("after read: %+v", p)
	}
	e.must(notify.Seen{By: e.by(rainy), Subject: issue})
	if p := panel(); p.Unread != 0 {
		t.Errorf("after seen: %+v", p)
	}
	e.must(notify.Dismiss{By: e.by(rainy), ID: second})
	if p := panel(); len(p.Items) != 1 || p.Items[0].ID != first {
		t.Errorf("after dismiss: %+v", p)
	}
	e.must(notify.DismissAll{By: e.by(rainy)})
	if p := panel(); len(p.Items) != 0 || p.More != 0 {
		t.Errorf("after dismiss all: %+v", p)
	}
	// Open answers where to go.
	e.result(tracker.AddComment{By: e.by(sunny), Issue: issue, Body: tracker.PlainDoc("Again.")})
	id := panel().Items[0].ID
	if got := e.result(notify.Open{By: e.by(rainy), ID: id}); got["href"] != "/WEB-1" {
		t.Errorf("open: %v", got)
	}
	if p := panel(); p.Unread != 0 {
		t.Errorf("after open: %+v", p)
	}
}

// FR-117: the sweep deletes dismissed notifications and read ones older
// than 30 days, and keeps a person's newest 500.
func TestSweep(t *testing.T) {
	e := newEnv(t)
	now := t0.Add(60 * 24 * time.Hour)
	e.must(writer.Func(func(tx *writer.Tx) error {
		for i := range notify.Keep + 20 {
			at := now.Add(-time.Duration(notify.Keep+20-i) * time.Minute).UnixMilli()
			var read, dismissed any
			switch i {
			case 0:
				read = now.Add(-31 * 24 * time.Hour).UnixMilli() // old and read: goes
			case 1:
				read = now.Add(-29 * 24 * time.Hour).UnixMilli() // read lately: stays, unless beyond 500
			case notify.Keep + 19:
				dismissed = now.UnixMilli() // the newest, dismissed: goes
			}
			_, err := tx.Exec(`INSERT INTO notifications (recipient, kind, subject_kind, subject_id, subject_label, href, actor, source, last_source, created_at, updated_at, read_at, dismissed_at)
				VALUES (?, 'assigned', 'issue', 'x', 'WEB-1', '/WEB-1', ?, ?, ?, ?, ?, ?, ?)`, rainy.ID, sunny.ID, i+1, i+1, at, at, read, dismissed)
			if err != nil {
				return err
			}
		}
		_, err := tx.Exec(`INSERT INTO notifications (recipient, kind, subject_kind, subject_id, subject_label, href, actor, source, last_source, created_at, updated_at)
			VALUES (?, 'assigned', 'issue', 'x', 'WEB-1', '/WEB-1', ?, 1, 1, 0, 0)`, misty.ID, sunny.ID)
		return err
	}))
	e.must(notify.Sweep{At: now})
	var n, newest, oldest int
	e.View(t, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*), max(source), min(source) FROM notifications WHERE recipient = ?`, rainy.ID).Scan(&n, &newest, &oldest)
	})
	if n != notify.Keep || newest != notify.Keep+19 || oldest != 20 {
		t.Errorf("rainy keeps %d, sources %d to %d", n, oldest, newest)
	}
	var mine int
	e.View(t, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM notifications WHERE recipient = ?`, misty.ID).Scan(&mine)
	})
	if mine != 1 {
		t.Errorf("misty keeps %d", mine)
	}
}

func TestSentences(t *testing.T) {
	long := strings.Repeat("word ", 60)
	doc := func(text string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{
			map[string]any{"type": "text", "text": text},
			map[string]any{"type": "mention", "attrs": map[string]string{"personId": rainy.ID, "label": "Rainy"}},
			map[string]any{"type": "text", "text": " ok."},
		}}}})
		return b
	}
	for _, c := range []struct {
		doc  json.RawMessage
		want string
	}{
		{doc("One. Two, "), "Two, @Rainy ok."},
		{doc("Hi "), "Hi @Rainy ok."},
		{mentionDoc(misty), ""},
	} {
		if got := notify.MentionSentence(c.doc, rainy.ID); got != c.want {
			t.Errorf("MentionSentence: %q, want %q", got, c.want)
		}
	}
	if got := notify.MentionSentence(doc(long), rainy.ID); !strings.HasSuffix(got, "@Rainy ok.") && !strings.Contains(got, "@Rainy") {
		t.Errorf("a long sentence loses the mention: %q", got)
	}
	if got := notify.Excerpt(tracker.PlainDoc(long)); len([]rune(got)) > notify.MaxText || !strings.HasSuffix(got, "…") {
		t.Errorf("Excerpt: %q", got)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
