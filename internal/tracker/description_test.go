package tracker

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

	"github.com/zweiundeins/tickit/internal/collab"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/people"
)

// insert is a step that inserts text at pos.
func insert(pos int, text string) string {
	b, _ := json.Marshal(text)
	return fmt.Sprintf(`{"stepType":"replace","from":%d,"to":%d,"slice":{"content":[{"type":"text","text":%s}]}}`, pos, pos, b)
}

func steps(s ...string) string { return "[" + strings.Join(s, ",") + "]" }

// descState is an issue's description stream's state.
func (e *env) descState(id string) collab.State {
	e.t.Helper()
	var s collab.State
	e.must(writer.Func(func(tx *writer.Tx) (err error) { s, err = descDocs.State(tx, id); return err }))
	return s
}

// commitDesc sends a commit of steps made on base as p's tab, and returns
// its answer.
func (e *env) commitDesc(p people.Person, issue string, base int64, ref, steps string) collab.Answer {
	e.t.Helper()
	ds := e.descState(issue)
	c := &CommitDescription{By: e.by(p), Session: "sid-" + p.ID, Tab: "tab-" + p.ID, Issue: issue, Epoch: ds.Epoch, Base: base, Ref: ref,
		Client: collab.Client("tab-" + p.ID), Steps: json.RawMessage(steps)}
	b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return c.ApplyResult(tx) })
	if err != nil {
		e.t.Fatalf("commit %s: %v", ref, err)
	}
	return collab.Answer(b[0])
}

// descHead is an issue's description as it is now, as text.
func (e *env) descHead(issue string) string {
	e.t.Helper()
	var h DescHead
	e.View(e.t, func(tx *sql.Tx) (err error) { h, err = DescriptionHead(context.Background(), tx, issue); return err })
	return doc.Text(h.Doc)
}

// FR-15, MP-05: an issue's description is a co-edited document from its
// creation; two people's commits on one version both land.
func TestDescriptionCommits(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	id := e.issue(sunny, CreateIssue{Project: p, Description: PlainDoc("Hello")})
	if ds := e.descState(id); ds.Version != 1 || ds.Epoch == "" {
		t.Fatalf("the description's stream %+v", ds)
	}
	if got := e.descHead(id); got != "Hello" {
		t.Fatalf("head %q", got)
	}
	// An issue created without one has a blank document to edit.
	blank := e.issue(sunny, CreateIssue{Project: p})
	if got := e.row(`SELECT doc FROM issue_doc WHERE issue_id = ?`, blank); got != collab.Blank {
		t.Errorf("blank %s", got)
	}
	if a := e.commitDesc(sunny, id, 1, "s1", steps(insert(6, " world"))); a != collab.Applied {
		t.Fatalf("first: %c", a)
	}
	if a := e.commitDesc(rainy, id, 1, "r1", steps(insert(1, ">"))); a != collab.Applied {
		t.Fatalf("rebased: %c", a)
	}
	if got := e.descHead(id); got != ">Hello world" {
		t.Errorf("MP-05.1: head %q", got)
	}
	// A resent commit applies once (NFR-43.2).
	if a := e.commitDesc(rainy, id, 1, "r1", steps(insert(1, ">"))); a != collab.Applied || e.descState(id).Version != 3 {
		t.Errorf("resent: %c, version %d", a, e.descState(id).Version)
	}
	// MP-14: a rule refuses (422), and the editing tab's row says why.
	e.must(OpenDescription{By: e.by(sunny), Session: "sid-" + sunny.ID, Tab: "tab-" + sunny.ID, Issue: id, Base: 3, Client: collab.Client("tab-" + sunny.ID)})
	if a := e.commitDesc(sunny, id, 3, "s2", steps(insert(1, strings.Repeat("x", MaxIssueText)))); a != collab.Refused {
		t.Errorf("too long: %c", a)
	}
	if got := e.row(`SELECT refusal FROM issue_editors WHERE tab_id = ?`, "tab-"+sunny.ID); !strings.Contains(got, `"ref":"s2"`) || !strings.Contains(got, "20,000") {
		t.Errorf("refusal %s", got)
	}
	// MP-13.2: a commit of another epoch is stale (409).
	c := &CommitDescription{By: e.by(sunny), Session: "s", Tab: "t", Issue: id, Epoch: "e-before", Base: 3, Ref: "old", Steps: json.RawMessage(steps(insert(1, "x")))}
	if b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return c.ApplyResult(tx) }); err != nil || collab.Answer(b[0]) != collab.Stale {
		t.Errorf("another epoch: %v %v", b, err)
	}
	// A Viewer may not edit (403), a deleted issue stops its editors (409).
	closed := e.project("SEC", false)
	sec := e.issue(member, CreateIssue{Project: closed})
	if a := e.commitDesc(viewer, sec, 1, "v1", steps(insert(1, "x"))); a != collab.Readonly {
		t.Errorf("a viewer: %c", a)
	}
	if err := e.do(OpenDescription{By: e.by(viewer), Session: "sv", Tab: "tv", Issue: sec, Client: "0123456789ab"}); !isDenied(err) {
		t.Errorf("a viewer opens: %v", err)
	}
	e.must(DeleteIssue{By: e.by(admin), Issue: blank})
	if a := e.commitDesc(sunny, blank, 1, "d1", steps(insert(1, "x"))); a != collab.Stale {
		t.Errorf("a deleted issue: %c", a)
	}
	if got := e.row(`SELECT count(*) FROM lith_events WHERE stream = 'issuedoc' AND (actor IS NULL OR actor = '' OR causation IS NULL OR at = 0)`); got != "0" {
		t.Errorf("FR-155.1: %s commits lack an actor, an op or a time", got)
	}
}

func isDenied(err error) bool {
	var d interface{ Denied() bool }
	return errors.As(err, &d) && d.Denied()
}

// FR-15, FR-68, FR-112: versions of a description are cut as
// issue.description_changed: after a minute without edits, when the last
// editor leaves, at once by SetDescription; each tells the people it
// mentions first, once.
func TestDescriptionVersions(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	id := e.issue(sunny, CreateIssue{Project: p, Description: PlainDoc("Hello")})
	e.commitDesc(sunny, id, 1, "s1", steps(insert(6, " world")))
	mention := `{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"mention","attrs":{"personId":"` + ada.ID + `","label":"Ada"}}]}}`
	e.commitDesc(rainy, id, 2, "r1", steps(mention))
	last := e.now
	editing := func(string) bool { return true }
	e.must(CutDescriptions{At: last.Add(30 * time.Second), Editing: editing})
	if s := e.state(id); s.DescUpto != 1 {
		t.Fatalf("cut too early: %+v", s.DescUpto)
	}
	e.must(CutDescriptions{At: last.Add(61 * time.Second), Editing: editing})
	s := e.state(id)
	if s.DescUpto != 3 || !strings.Contains(DocText(s.Description), "Hello world") {
		t.Fatalf("idle cut: upto %d, %s", s.DescUpto, s.Description)
	}
	got := e.row(`SELECT data FROM lith_events WHERE stream = 'issue' AND stream_id = ? AND type = 'issue.description_changed'`, id)
	if !strings.Contains(got, `"mentions_added":["`+ada.ID+`"]`) || !strings.Contains(got, `"editors":["`+sunny.ID+`","`+rainy.ID+`"]`) {
		t.Errorf("the version %s", got)
	}
	if actor := e.row(`SELECT actor FROM lith_events WHERE stream = 'issue' AND stream_id = ? AND type = 'issue.description_changed'`, id); actor != rainy.ID {
		t.Errorf("the version's actor %s", actor)
	}
	if !contains(e.state(id).Watchers, ada.ID) {
		t.Error("FR-27.2: the mentioned person does not watch")
	}
	if got := e.row(`SELECT description_text FROM issues WHERE id = ?`, id); !strings.Contains(got, "Hello world") {
		t.Errorf("the list's text %q", got)
	}
	if got := e.row(`SELECT count(*) FROM search_issues_fts WHERE search_issues_fts MATCH 'world'`); got != "1" {
		t.Errorf("FR-57: search finds %s", got)
	}
	hist, err := History(context.Background(), e.DB.R, id)
	if err != nil {
		t.Fatal(err)
	}
	var desc []Entry
	for _, h := range hist {
		if h.Field == FieldDescription {
			desc = append(desc, h)
		}
	}
	if len(desc) != 1 || desc[0].Actor != rainy.ID || desc[0].New != sunny.Name {
		t.Errorf("FR-68: %+v", desc)
	}
	// The as-of view shows the description at its versions (FR-70).
	at1, _, err := StateAt(context.Background(), e.DB.R, id, 1)
	if err != nil || DocText(at1.Description) != "Hello" {
		t.Errorf("as of version 1: %q %v", DocText(at1.Description), err)
	}
	// Nothing new: nothing cut. The last editor gone: cut at once.
	n := e.row(`SELECT count(*) FROM lith_events WHERE stream = 'issue' AND stream_id = ?`, id)
	e.must(CutDescriptions{At: last.Add(2 * time.Minute)})
	if again := e.row(`SELECT count(*) FROM lith_events WHERE stream = 'issue' AND stream_id = ?`, id); again != n {
		t.Errorf("cut without a change: %s events, was %s", again, n)
	}
	e.commitDesc(sunny, id, 3, "s2", steps(insert(1, "Oh ")))
	e.must(CutDescriptions{At: e.now, Editing: nil})
	if s := e.state(id); s.DescUpto != 4 {
		t.Errorf("left: upto %d", s.DescUpto)
	}
	// A version that mentions Ada again tells nobody (FR-67.3).
	if got := e.row(`SELECT data FROM lith_events WHERE stream = 'issue' AND stream_id = ? AND type = 'issue.description_changed' ORDER BY version DESC LIMIT 1`, id); strings.Contains(got, "mentions_added") {
		t.Errorf("mentioned again: %s", got)
	}
	// Ten minutes of edits without a pause cut one version.
	start := e.now
	for i := range 12 {
		e.commitDesc(rainy, id, e.descState(id).Version, fmt.Sprintf("i%d", i), steps(insert(1, "x")))
		e.now = e.now.Add(55 * time.Second)
		e.must(CutDescriptions{At: e.now, Editing: editing})
	}
	if v := e.state(id).DescUpto; v <= 4 || e.now.Sub(start) < CutInterval {
		t.Errorf("interval: upto %d", v)
	}
	// SetDescription cuts at once; the same text stores nothing (FR-14.3).
	e.must(SetDescription{By: e.by(member), Issue: id, Description: PlainDoc("Replaced")})
	s = e.state(id)
	if DocText(s.Description) != "Replaced" || s.DescUpto != e.descState(id).Version || e.descHead(id) != "Replaced" {
		t.Errorf("set: %s, upto %d", s.Description, s.DescUpto)
	}
	v := s.Version
	e.must(SetDescription{By: e.by(member), Issue: id, Description: PlainDoc("Replaced")})
	if e.state(id).Version != v {
		t.Error("the same description was stored")
	}
	if err := e.do(SetDescription{By: e.by(member), Issue: id, Description: json.RawMessage(`[1]`)}); refusal(t, err) != "The description is not a document." {
		t.Errorf("not a document: %v", err)
	}
}

// NFR-43: the commits up to a description's version are folded into a
// snapshot; the head stays, a commit on a folded version is stale.
func TestDescriptionFold(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	id := e.issue(sunny, CreateIssue{Project: p, Description: PlainDoc("Hello")})
	for i := range 5 {
		e.commitDesc(sunny, id, e.descState(id).Version, fmt.Sprintf("s%d", i), steps(insert(1, "x")))
	}
	before := e.descHead(id)
	f := &FoldDescriptions{At: e.now.Add(collab.FoldAfter + time.Minute)}
	e.must(f)
	if f.Deleted != 0 {
		t.Errorf("folded %d before a version", f.Deleted)
	}
	e.must(CutDescriptions{At: e.now})
	f = &FoldDescriptions{At: e.now.Add(collab.FoldAfter + time.Minute)}
	e.must(f)
	if f.Deleted == 0 {
		t.Fatal("nothing folded")
	}
	collab.ResetCaches()
	if got := e.descHead(id); got != before {
		t.Errorf("head after the fold %q, want %q", got, before)
	}
	if a := e.commitDesc(rainy, id, 2, "late", steps(insert(1, "z"))); a != collab.Stale {
		t.Errorf("a folded base: %c", a)
	}
	if a := e.commitDesc(rainy, id, e.descState(id).Version, "now", steps(insert(1, "z"))); a != collab.Applied {
		t.Errorf("after the fold: %c", a)
	}
	// As of version 1, the issue still has its first description (FR-70).
	if at1, _, _ := StateAt(context.Background(), e.DB.R, id, 1); DocText(at1.Description) != "Hello" {
		t.Errorf("as of 1: %s", at1.Description)
	}
}

// MP-13: a reset starts the description anew, in another epoch.
func TestDescriptionReset(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	id := e.issue(sunny, CreateIssue{Project: p, Description: PlainDoc("Hello")})
	old := e.descState(id).Epoch
	e.must(writer.Func(func(tx *writer.Tx) error {
		if _, err := events.Forget(Store(), tx, DescDocStream.ID(id)); err != nil {
			return err
		}
		return events.Execute(Store(), tx, collab.CreateDecider, DescDocStream.ID(id), events.NoStream, collab.Created{Epoch: "e-after", Doc: json.RawMessage(collab.Blank)}, meta(e.by(ada)))
	}))
	c := &CommitDescription{By: e.by(sunny), Session: "s", Tab: "t", Issue: id, Epoch: old, Base: 1, Ref: "x", Steps: json.RawMessage(steps(insert(1, "x")))}
	b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return c.ApplyResult(tx) })
	if err != nil || collab.Answer(b[0]) != collab.Stale {
		t.Errorf("MP-13.2: %v %v", b, err)
	}
}
