package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/collab"
	"github.com/zweiundeins/tickit/internal/search"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/migrations"
)

func TestCreateSpace(t *testing.T) {
	e := newEnv(t)
	id, home := e.space("ENG", true)
	if got := e.row(`SELECT key || ' ' || name || ' ' || home_id FROM spaces WHERE id = ?`, id); got != "ENG ENG space "+home {
		t.Errorf("space row %q", got)
	}
	// FR-76.1: the creator is its Admin; the home page has the space's name.
	if r := e.row(`SELECT role FROM memberships WHERE scope_id = ? AND person_id = ?`, id, admin.ID); r != "admin" {
		t.Errorf("creator's role %q", r)
	}
	if p := e.pageState(home); !p.Home || p.Title != "ENG space" || p.Versions != 1 {
		t.Errorf("home page %+v", p)
	}
	// FR-76.2: a taken key, another case included.
	err := e.do(CreateSpace{By: e.by(sunny), Key: "eng", Name: "Again"})
	if msg := refusal(t, err); msg != "The key ENG is taken." {
		t.Errorf("taken key: %q", msg)
	}
	for _, key := range []string{"E", "ELEVENCHARS", "EN1"} {
		if msg := refusal(t, e.do(CreateSpace{By: e.by(sunny), Key: key, Name: "x"})); msg != "A key has 2 to 10 letters A to Z." {
			t.Errorf("key %q: %q", key, msg)
		}
	}
	// FR-77: only the Admin edits and archives.
	name := "Engineering"
	if err := e.do(EditSpace{By: e.by(sunny), Space: id, Name: &name}); !isDenied(err) {
		t.Errorf("a guest edits a space: %v", err)
	}
	e.must(EditSpace{By: e.by(admin), Space: id, Name: &name})
	e.must(ArchiveSpace{By: e.by(admin), Space: id, Archived: true})
	if msg := refusal(t, e.do(CreatePage{By: e.by(admin), Space: id, Title: "Late"})); msg != "This space is archived." {
		t.Errorf("archived: %q", msg)
	}
}

func isDenied(err error) bool {
	var d interface{ Denied() bool }
	return errors.As(err, &d) && d.Denied()
}

func TestPageTitles(t *testing.T) {
	e := newEnv(t)
	sp, home := e.space("ENG", true)
	a := e.page(sunny, CreatePage{Space: sp, Title: "Deploy checklist"})
	// FR-81.2: titles are unique per space without regard to case.
	if msg := refusal(t, e.do(CreatePage{By: e.by(rainy), Space: sp, Title: "deploy CHECKLIST"})); msg != `A page titled "deploy CHECKLIST" exists in this space.` {
		t.Errorf("duplicate title: %q", msg)
	}
	other, _ := e.space("OPS", true)
	e.page(sunny, CreatePage{Space: other, Title: "Deploy checklist"}) // another space: fine
	b := e.page(sunny, CreatePage{Space: sp, Parent: home, Title: "Runbooks"})
	// FR-86.2: a rename to a taken title is refused next to the title.
	err := e.do(RenamePage{By: e.by(rainy), Page: b, Title: "Deploy Checklist"})
	var field interface{ OnField() string }
	if !errors.As(err, &field) || field.OnField() != "title" {
		t.Errorf("rename to a taken title: %v", err)
	}
	e.must(RenamePage{By: e.by(rainy), Page: a, Title: "deploy checklist"}) // its own title in another case
	e.must(RenamePage{By: e.by(rainy), Page: b, Title: "Runbooks and drills"})
	e.page(sunny, CreatePage{Space: sp, Title: "Runbooks"}) // the old title is free again
	if s := e.pageState(a); s.Title != "deploy checklist" {
		t.Errorf("renamed: %q", s.Title)
	}
	if msg := refusal(t, e.do(CreatePage{By: e.by(rainy), Space: sp, Title: strings.Repeat("x", 256)})); msg != "A title has 1 to 255 characters." {
		t.Errorf("long title: %q", msg)
	}
}

// FR-78, FR-79: the tree's order and moves.
func TestMoves(t *testing.T) {
	e := newEnv(t)
	sp, home := e.space("ENG", true)
	a := e.page(sunny, CreatePage{Space: sp, Parent: home, Title: "A"})
	b := e.page(sunny, CreatePage{Space: sp, Parent: home, Title: "B"})
	c := e.page(sunny, CreatePage{Space: sp, Parent: home, After: a, Title: "C"})
	a1 := e.page(sunny, CreatePage{Space: sp, Parent: a, Title: "A1"})
	order := func(parent string) []string {
		var out []string
		e.View(t, func(tx *sql.Tx) error {
			nodes, err := Tree(context.Background(), tx, sp)
			var walk func([]*Node)
			walk = func(ns []*Node) {
				for _, n := range ns {
					if n.Parent == parent {
						out = append(out, n.Title)
					}
					walk(n.Children)
				}
			}
			walk(nodes)
			return err
		})
		return out
	}
	if got := order(home); !slices.Equal(got, []string{"A", "C", "B"}) {
		t.Errorf("order %v", got)
	}
	// FR-79.2: never under itself or a descendant.
	if msg := refusal(t, e.do(MovePage{By: e.by(rainy), Page: a, Parent: a1})); msg != "A page cannot move under one of its own pages." {
		t.Errorf("under a descendant: %q", msg)
	}
	if msg := refusal(t, e.do(MovePage{By: e.by(rainy), Page: a, Parent: a})); msg != "A page cannot move under itself." {
		t.Errorf("under itself: %q", msg)
	}
	if msg := refusal(t, e.do(MovePage{By: e.by(rainy), Page: home, Parent: a})); msg != "The home page stays at the top of its space." {
		t.Errorf("home: %q", msg)
	}
	e.must(MovePage{By: e.by(rainy), Page: b, Parent: home, Before: a})
	if got := order(home); !slices.Equal(got, []string{"B", "A", "C"}) {
		t.Errorf("after the move %v", got)
	}
	// FR-79.1: the children move with their page.
	e.must(MovePage{By: e.by(rainy), Page: a, Parent: c})
	if got := order(c); !slices.Equal(got, []string{"A"}) {
		t.Errorf("under C %v", got)
	}
	if got := order(a); !slices.Equal(got, []string{"A1"}) {
		t.Errorf("A keeps A1: %v", got)
	}
	// FR-79.3: a title the target space has is refused, the child's too.
	ops, opsHome := e.space("OPS", true)
	clash := e.page(sunny, CreatePage{Space: ops, Title: "A1"})
	if msg := refusal(t, e.do(MovePage{By: e.by(rainy), Page: a, Space: ops, Parent: opsHome})); msg != `A page titled "A1" exists in this space.` {
		t.Errorf("taken in the target: %q", msg)
	}
	e.must(RenamePage{By: e.by(sunny), Page: clash, Title: "A1 in OPS"})
	e.must(MovePage{By: e.by(rainy), Page: c, Space: ops, Parent: opsHome})
	for _, id := range []string{c, a, a1} {
		if s := e.pageState(id); s.Space != ops {
			t.Errorf("%s stayed in %s", s.Title, s.Space)
		}
	}
	if got := e.row(`SELECT count(*) FROM pages WHERE space_id = ?`, ops); got != "5" {
		t.Errorf("OPS has %s pages", got)
	}
}

// FR-94: the trash.
func TestTrash(t *testing.T) {
	e := newEnv(t)
	sp, home := e.space("ENG", false)
	a := e.page(member, CreatePage{Space: sp, Parent: home, Title: "A"})
	a1 := e.page(member, CreatePage{Space: sp, Parent: a, Title: "A1"})
	a2 := e.page(member, CreatePage{Space: sp, Parent: a, Title: "A2"})
	z := e.page(member, CreatePage{Space: sp, Parent: home, Title: "Z"})
	// Only its creator, the space's Admin or a moderator.
	if err := e.do(TrashPage{By: e.by(viewer), Page: a}); !isDenied(err) {
		t.Errorf("a viewer trashes: %v", err)
	}
	if msg := refusal(t, e.do(TrashPage{By: e.by(admin), Page: home})); msg != "The home page of a space cannot go to the trash." {
		t.Errorf("home: %q", msg)
	}
	e.must(TrashPage{By: e.by(member), Page: a})
	// FR-94.1: the children move up, in its place.
	var titles []string
	e.View(t, func(tx *sql.Tx) error {
		nodes, err := Tree(context.Background(), tx, sp)
		for _, n := range nodes[0].Children {
			titles = append(titles, n.Title)
		}
		return err
	})
	if !slices.Equal(titles, []string{"A1", "A2", "Z"}) {
		t.Errorf("after the trash %v", titles)
	}
	_ = a1
	_ = a2
	_ = z
	if msg := refusal(t, e.do(RenamePage{By: e.by(member), Page: a, Title: "B"})); msg != "This page is in the trash." {
		t.Errorf("rename in the trash: %q", msg)
	}
	if msg := refusal(t, e.do(CreatePage{By: e.by(member), Space: sp, Title: "a"})); msg != `A page titled "a" is in this space's trash.` {
		t.Errorf("trashed title: %q", msg)
	}
	var trash []Page
	e.View(t, func(tx *sql.Tx) (err error) { trash, err = Trash(context.Background(), tx, sp); return err })
	if len(trash) != 1 || trash[0].TrashedBy != member.ID {
		t.Errorf("trash %+v", trash)
	}
	// FR-94.2: the Admin restores it to its old place, or deletes it.
	if err := e.do(RestorePage{By: e.by(member), Page: a}); !isDenied(err) {
		t.Errorf("a member restores: %v", err)
	}
	e.must(RestorePage{By: e.by(admin), Page: a})
	if s := e.pageState(a); s.State != Current || s.Parent != home {
		t.Errorf("restored %+v", s)
	}
	e.must(TrashPage{By: e.by(moderator), Page: a})
	e.must(DeletePage{By: e.by(admin), Page: a})
	e.page(member, CreatePage{Space: sp, Title: "A"})
	if err := e.do(RenamePage{By: e.by(admin), Page: a, Title: "x"}); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("a deleted page: %v", err)
	}
}

// docs/editor.md: the writer rebases a stale commit; MP-05.1.
func TestCommits(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Notes"})
	if a := e.commit(sunny, p, 1, "s1", steps(insert(1, "Hello"))); a != Applied {
		t.Fatalf("first commit: %c", a)
	}
	// Rainy typed on version 1 too: rebased over Sunny's.
	if a := e.commit(rainy, p, 1, "r1", steps(insert(1, "Hi "))); a != Applied {
		t.Fatalf("stale commit: %c", a)
	}
	if got, want := e.head(p), string(paras("HelloHi ")); got != want {
		t.Errorf("head %s, want %s", got, want)
	}
	// A resent commit applies once, by its ref among the commits since its
	// base, without an op id (NFR-43.2).
	resent := &Commit{By: e.by(rainy), Session: "sid-" + rainy.ID, Tab: "tab-" + rainy.ID, Page: p, Epoch: e.docState(p).Epoch, Base: 1, Ref: "r1",
		Client: Client("tab-" + rainy.ID), Steps: json.RawMessage(steps(insert(1, "Hi ")))}
	e.must(resent)
	if v := e.docState(p).Version; v != 3 {
		t.Errorf("version %d after a resend", v)
	}
	// A commit whose steps all drop still appends, to confirm its ref.
	e.commit(sunny, p, 3, "s2", steps(`{"stepType":"replace","from":1,"to":9}`))
	if a := e.commit(rainy, p, 3, "r2", steps(`{"stepType":"replace","from":2,"to":3}`)); a != Applied {
		t.Fatalf("drop: %c", a)
	}
	if got := e.row(`SELECT data FROM lith_events WHERE stream = 'pagedoc' AND stream_id = ? AND version = 5`, p); !strings.Contains(got, `"steps":[]`) {
		t.Errorf("the dropped commit: %s", got)
	}
	if v := e.docState(p).Version; v != 5 {
		t.Errorf("version %d", v)
	}
	// 422: a rule refuses, and the editor's row says why.
	e.must(OpenEditor{By: e.by(sunny), Session: "sid-" + sunny.ID, Tab: "tab-" + sunny.ID, Page: p, Base: 5, Client: Client("tab-" + sunny.ID)})
	bad := steps(`{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"issue_chip","attrs":{"key":"lowercase-1"}}]}}`)
	if a := e.commit(sunny, p, 5, "s3", bad); a != Refused {
		t.Errorf("a bad chip: %c", a)
	}
	if r := e.row(`SELECT refusal FROM page_editors WHERE tab_id = ?`, "tab-"+sunny.ID); !strings.Contains(r, `"ref":"s3"`) {
		t.Errorf("refusal %q", r)
	}
	// 409: another epoch (MP-13.2), and a trashed page.
	c := &Commit{By: e.by(sunny), Session: "x", Tab: "y", Page: p, Epoch: "e-old", Base: 5, Ref: "s4", Steps: json.RawMessage(steps(insert(1, "a")))}
	op := writer.OnceResult("commit:"+p+":s4", c)
	e.must(op)
	if CommitAnswer(op.Result()[0]) != Stale {
		t.Errorf("old epoch: %c", op.Result()[0])
	}
	// 403: a viewer.
	closed, _ := e.space("SEC", false)
	q := e.page(member, CreatePage{Space: closed, Title: "Secret"})
	if a := e.commit(viewer, q, 1, "v1", steps(insert(1, "a"))); a != Readonly {
		t.Errorf("a viewer commits: %c", a)
	}
	e.must(TrashPage{By: e.by(sunny), Page: p})
	if a := e.commit(sunny, p, 5, "s5", steps(insert(1, "a"))); a != Stale {
		t.Errorf("a trashed page: %c", a)
	}
}

// MP-05.1: every keystroke of several editors on stale bases ends in one
// document holding all of them.
func TestConcurrentCommits(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Race"})
	for i := range 30 {
		base := int64(1 + i/3*3) // three editors on the same base each round
		who := []string{"a", "b", "c"}[i%3]
		e.commit(sunny, p, base, who+string(rune('0'+i%10))+strings.Repeat("x", i/10), steps(insert(1, who)))
	}
	head := e.head(p)
	for _, c := range "abc" {
		if n := strings.Count(strings.Split(head, `"text":"`)[1], string(c)); n != 10 {
			t.Errorf("%c %d times in %s", c, n, head)
		}
	}
}

// FR-87, FR-89: versions, restore.
func TestVersions(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Notes", Doc: paras("One")})
	e.commit(sunny, p, 1, "s1", steps(insert(4, " two")))
	if msg := refusal(t, e.do(CutVersion{By: e.by(rainy), Page: p, Message: strings.Repeat("m", 201)})); msg != "A message has at most 200 characters." {
		t.Errorf("long message: %q", msg)
	}
	e.must(CutVersion{By: e.by(rainy), Page: p, Message: "Second"})
	if msg := refusal(t, e.do(CutVersion{By: e.by(rainy), Page: p})); msg != "Nothing changed since version 2." {
		t.Errorf("nothing new: %q", msg)
	}
	var vs []VersionInfo
	e.View(t, func(tx *sql.Tx) (err error) { vs, err = Versions(context.Background(), tx, p); return err })
	if len(vs) != 2 || vs[0].Message != "Second" || !slices.Equal(vs[0].Editors, []string{sunny.ID}) || vs[0].Upto != 2 {
		t.Errorf("versions %+v", vs)
	}
	e.commit(rainy, p, 2, "r1", steps(insert(1, "Zero ")))
	e.must(RestoreVersion{By: e.by(rainy), Page: p, Number: 1})
	if got := e.head(p); got != string(paras("One")) {
		t.Errorf("restored %s", got)
	}
	e.View(t, func(tx *sql.Tx) (err error) { vs, err = Versions(context.Background(), tx, p); return err })
	if vs[0].Number != 3 || vs[0].Message != "Restored version 1" || vs[0].Reason != "restore" {
		t.Errorf("restore version %+v", vs[0])
	}
	// The cut job: idle, the last editor gone, every 10 minutes.
	e.commit(rainy, p, 4, "r2", steps(insert(1, "A")))
	editing := func(string) bool { return true }
	e.must(CutDue{At: e.now.Add(30 * time.Second), Editing: editing})
	if n := e.pageState(p).Versions; n != 3 {
		t.Errorf("cut after 30 s: version %d", n)
	}
	e.must(CutDue{At: e.now.Add(61 * time.Second), Editing: editing})
	if n := e.pageState(p).Versions; n != 4 {
		t.Errorf("idle: version %d", n)
	}
	e.commit(rainy, p, 5, "r3", steps(insert(1, "B")))
	e.must(CutDue{At: e.now, Editing: nil})
	if s := e.pageState(p); s.Versions != 5 {
		t.Errorf("left: version %d", s.Versions)
	}
	if r := e.row(`SELECT reason FROM page_versions WHERE page_id = ? AND number = 5`, p); r != "left" {
		t.Errorf("reason %q", r)
	}
}

// NFR-43: the fold leaves versions and the head as they were.
func TestFold(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Long"})
	for i := range 40 {
		e.commit(sunny, p, int64(i+1), "c"+strings.Repeat("x", i), steps(insert(1, "a")))
	}
	e.must(CutVersion{By: e.by(sunny), Page: p})
	for i := range 5 {
		e.commit(sunny, p, int64(i+41), "d"+strings.Repeat("x", i), steps(insert(1, "b")))
	}
	// NFR-43.3: the fold's bound is the first commit after the version.
	uncut := `SELECT at FROM collab_uncut WHERE kind = 'pagedoc' AND id = ?`
	if got, want := e.row(uncut, p), e.row(`SELECT at FROM lith_events WHERE stream = 'pagedoc' AND stream_id = ? AND version = 42`, p); got != want {
		t.Errorf("oldest uncut commit at %s, want %s", got, want)
	}
	before := e.head(p)
	// Too early: nothing folds.
	f := &Fold{At: e.now}
	e.must(f)
	if f.Deleted != 0 {
		t.Errorf("folded %d too early", f.Deleted)
	}
	f = &Fold{At: e.now.Add(FoldAfter + time.Minute)}
	e.must(f)
	if f.Deleted == 0 {
		t.Fatal("nothing folded")
	}
	// The commits after the version stay.
	if n := e.row(`SELECT count(*) FROM lith_events WHERE stream = 'pagedoc' AND stream_id = ? AND type = 'pagedoc.committed'`, p); n != "5" {
		t.Errorf("%s commits left", n)
	}
	collab.ResetCaches()
	if got := e.head(p); got != before {
		t.Errorf("head after the fold %s, want %s", got, before)
	}
	// A commit on a folded version starts the editor again (409).
	if a := e.commit(sunny, p, 10, "late", steps(insert(1, "z"))); a != Stale {
		t.Errorf("commit on a folded base: %c", a)
	}
	if a := e.commit(sunny, p, 45, "fresh", steps(insert(1, "z"))); a != Applied {
		t.Errorf("commit on the head: %c", a)
	}
	e.must(CutVersion{By: e.by(sunny), Page: p})
	if n := e.row(`SELECT count(*) FROM collab_uncut WHERE id = ?`, p); n != "0" {
		t.Errorf("%s uncut rows after a version of the head", n)
	}
}

// NFR-43.1: a page trashed while someone edits it holds back no other
// page's fold; restored, its commits hold the fold back again.
func TestFoldTrashedPage(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	uncut := `SELECT count(*) FROM collab_uncut WHERE kind = 'pagedoc' AND id = ?`
	b := e.page(sunny, CreatePage{Space: sp, Title: "Trashed"})
	e.commit(sunny, b, 1, "b1", steps(insert(1, "b")))
	e.must(TrashPage{By: e.by(sunny), Page: b})
	if n := e.row(uncut, b); n != "0" {
		t.Errorf("%s uncut rows of a trashed page", n)
	}
	e.must(RestorePage{By: e.by(admin), Page: b})
	if got, want := e.row(`SELECT at FROM collab_uncut WHERE kind = 'pagedoc' AND id = ?`, b), e.row(`SELECT at FROM lith_events WHERE stream = 'pagedoc' AND stream_id = ? AND version = 2`, b); got != want {
		t.Errorf("restored: oldest uncut commit at %q, want %q", got, want)
	}
	e.must(TrashPage{By: e.by(sunny), Page: b})

	e.now = e.now.Add(time.Hour)
	a := e.page(sunny, CreatePage{Space: sp, Title: "Edited"})
	for i := range 5 {
		e.commit(sunny, a, int64(i+1), "a"+strings.Repeat("x", i), steps(insert(1, "a")))
	}
	e.must(CutVersion{By: e.by(sunny), Page: a})
	e.commit(sunny, a, 6, "after", steps(insert(1, "c")))
	f := &Fold{At: e.now.Add(FoldAfter + time.Minute)}
	e.must(f)
	if n := e.row(`SELECT count(*) FROM lith_events WHERE stream = 'pagedoc' AND stream_id = ? AND type = 'pagedoc.committed'`, a); f.Deleted == 0 || n != "1" {
		t.Errorf("the fold deleted %d events, %s commits of the edited page left", f.Deleted, n)
	}

	// Restored after the fold took its commits, its next commits count.
	e.must(RestorePage{By: e.by(admin), Page: b})
	e.must(CutDue{At: e.now})
	if n := e.row(uncut, b); n != "0" {
		t.Errorf("%s uncut rows after a version of the head", n)
	}
	e.commit(sunny, b, e.docState(b).Version, "b2", steps(insert(1, "d")))
	if n := e.row(uncut, b); n != "1" {
		t.Errorf("%s uncut rows after a commit to a restored page", n)
	}
	e.must(TrashPage{By: e.by(sunny), Page: b})
	e.must(RestorePage{By: e.by(admin), Page: b})
	e.must(TrashPage{By: e.by(sunny), Page: b})
	e.must(DeletePage{By: e.by(admin), Page: b})
	if n := e.row(uncut, b); n != "0" {
		t.Errorf("%s uncut rows of a deleted page", n)
	}
}

// NFR-43.1: migration 089 deletes the rows that trashed and deleted pages
// kept before TrashPage released them, and keeps those of current pages.
func TestMigrationReleasesGonePages(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	var ids []string
	for _, title := range []string{"Current", "Trashed", "Deleted"} {
		p := e.page(sunny, CreatePage{Space: sp, Title: title})
		e.commit(sunny, p, 1, title, steps(insert(1, "x")))
		ids = append(ids, p)
	}
	e.must(TrashPage{By: e.by(sunny), Page: ids[1]})
	e.must(TrashPage{By: e.by(sunny), Page: ids[2]})
	e.must(DeletePage{By: e.by(admin), Page: ids[2]})
	sqlText, err := migrations.FS.ReadFile("089_collab_uncut_gone.sql")
	if err != nil {
		t.Fatal(err)
	}
	e.must(writer.Func(func(tx *writer.Tx) error {
		for _, id := range ids[1:] { // as a commit before the trash left them
			if _, err := tx.Exec(`INSERT OR REPLACE INTO collab_uncut (kind, id, at) VALUES ('pagedoc', ?, 0)`, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(string(sqlText))
		return err
	}))
	if got := e.row(`SELECT group_concat(id) FROM collab_uncut WHERE kind = 'pagedoc'`); got != ids[0] {
		t.Errorf("uncut rows %q, want %q", got, ids[0])
	}
}

// FR-87.1: CutDue finds a page renamed since its last version as it
// finds one with commits, and a title changed back leaves nothing to cut.
func TestCutDueRenamed(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Plan"})
	uncut := `SELECT count(*) FROM collab_uncut WHERE kind = 'pagedoc' AND id = ?`
	e.must(RenamePage{By: e.by(sunny), Page: p, Title: "Plan B"})
	if n := e.row(uncut, p); n != "1" {
		t.Errorf("%s uncut rows after a rename", n)
	}
	e.must(CutDue{At: e.now})
	if s := e.pageState(p); s.Versions != 2 || s.CutTitle != "Plan B" {
		t.Errorf("after a rename: version %d titled %q", s.Versions, s.CutTitle)
	}
	e.must(RenamePage{By: e.by(sunny), Page: p, Title: "Plan C"})
	e.must(RenamePage{By: e.by(sunny), Page: p, Title: "Plan B"})
	e.must(CutDue{At: e.now})
	if n, v := e.row(uncut, p), e.pageState(p).Versions; n != "0" || v != 2 {
		t.Errorf("renamed back: %s uncut rows, version %d", n, v)
	}

	// Migration 090: a rename from before it gets its row.
	e.must(RenamePage{By: e.by(sunny), Page: p, Title: "Plan D"})
	sqlText, err := migrations.FS.ReadFile("090_collab_uncut_renames.sql")
	if err != nil {
		t.Fatal(err)
	}
	e.must(writer.Func(func(tx *writer.Tx) error {
		if _, err := tx.Exec(`DELETE FROM collab_uncut`); err != nil {
			return err
		}
		_, err := tx.Exec(string(sqlText))
		return err
	}))
	e.must(CutDue{At: e.now})
	if s := e.pageState(p); s.Versions != 3 || s.CutTitle != "Plan D" {
		t.Errorf("after migration 090: version %d titled %q", s.Versions, s.CutTitle)
	}
	// Renamed, trashed before its version, then restored.
	e.must(RenamePage{By: e.by(sunny), Page: p, Title: "Plan E"})
	e.must(TrashPage{By: e.by(sunny), Page: p})
	e.must(RestorePage{By: e.by(admin), Page: p})
	e.must(CutDue{At: e.now})
	if s := e.pageState(p); s.Versions != 4 || s.CutTitle != "Plan E" {
		t.Errorf("restored: version %d titled %q", s.Versions, s.CutTitle)
	}
}

// FR-90: comments and replies.
func TestComments(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", false)
	p := e.page(member, CreatePage{Space: sp, Title: "Notes"})
	c := e.result(AddComment{By: e.by(viewer), Page: p, Body: paras("Looks good")})["id"]
	r := e.result(AddComment{By: e.by(member), Page: p, Parent: c, Body: paras("Thanks")})["id"]
	if msg := refusal(t, e.do(AddComment{By: e.by(viewer), Page: p, Parent: r, Body: paras("Deeper")})); msg != "Replies go one level deep: reply to the comment above." {
		t.Errorf("second level: %q", msg)
	}
	if msg := refusal(t, e.do(AddComment{By: e.by(viewer), Page: p, Body: paras("")})); msg != "Write something first." {
		t.Errorf("empty: %q", msg)
	}
	if err := e.do(tracker.EditComment{By: e.by(member), Comment: c, Body: paras("Mine now")}); !isDenied(err) {
		t.Errorf("edit another's: %v", err)
	}
	e.must(tracker.EditComment{By: e.by(viewer), Comment: c, Body: paras("Looks great")})
	e.must(tracker.HideComment{By: e.by(admin), Comment: r, Hidden: true})
	e.must(tracker.DeleteComment{By: e.by(viewer), Comment: c})
	var list []Comment
	e.View(t, func(tx *sql.Tx) (err error) { list, err = Comments(context.Background(), tx, p); return err })
	if len(list) != 1 || !list[0].Deleted || len(list[0].Replies) != 1 || !list[0].Replies[0].Hidden {
		t.Errorf("comments %+v", list)
	}
}

// FR-100.2, FR-101: chips and the pages that mention an issue.
func TestChips(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	proj := e.result(tracker.CreateProject{By: e.by(admin), Key: "WEB", Name: "Web", Open: true})["id"]
	issue := e.result(tracker.CreateIssue{By: e.by(sunny), Project: proj, Type: tracker.Task, Summary: "Fix the login"})["id"]
	p := e.page(sunny, CreatePage{Space: sp, Title: "Notes"})
	e.Touched()
	chip := `{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"issue_chip","attrs":{"key":"WEB-1"}}]}}`
	e.commit(sunny, p, 1, "c1", steps(chip))
	got := e.Touched()
	for _, want := range []string{ChipsTopic(p), tracker.IssueTopic(issue), DocTopic(p)} {
		if !slices.Contains(got, want) {
			t.Errorf("chip added: %s not touched in %v", want, got)
		}
	}
	var refs []PageRef
	e.View(t, func(tx *sql.Tx) (err error) {
		refs, err = PagesMentioning(context.Background(), tx, "web-1")
		return err
	})
	if len(refs) != 1 || refs[0].ID != p || refs[0].SpaceKey != "ENG" {
		t.Errorf("mentioning %+v", refs)
	}
	e.must(tracker.SetSummary{By: e.by(rainy), Issue: issue, Seen: 1 << 40, Summary: "Fix the login for good"})
	if got := e.Touched(); !slices.Contains(got, ChipsTopic(p)) {
		t.Errorf("summary changed: chips not touched: %v", got)
	}
	e.commit(sunny, p, 2, "c2", steps(`{"stepType":"replace","from":1,"to":2}`))
	e.View(t, func(tx *sql.Tx) (err error) {
		refs, err = PagesMentioning(context.Background(), tx, "WEB-1")
		return err
	})
	if len(refs) != 0 {
		t.Errorf("FR-101.2: the page still listed %+v", refs)
	}
}

// FR-57: pages are found by title and by the text of their versions.
func TestPageSearch(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Deploy checklist", Doc: paras("Run the canary first")})
	e.commit(sunny, p, 1, "s1", steps(insert(1, "Zebra ")))
	var hits []search.Hit
	find := func(text string) int {
		e.View(t, func(tx *sql.Tx) error {
			res, err := search.Search(context.Background(), tx, search.Query{Text: text, Kinds: []search.Kind{search.Page}, Spaces: []string{sp}, Limit: 10})
			hits = res.Hits
			return err
		})
		return len(hits)
	}
	if find("checklist") != 1 || find("canary") != 1 {
		t.Fatalf("title or body not found: %+v", hits)
	}
	if find("zebra") != 0 {
		t.Error("a keystroke reached the index before a version")
	}
	e.must(CutVersion{By: e.by(sunny), Page: p})
	if find("zebra") != 1 {
		t.Error("the new version's text is not indexed")
	}
	e.must(TrashPage{By: e.by(sunny), Page: p})
	if find("checklist") != 0 {
		t.Error("a trashed page is found")
	}
}
