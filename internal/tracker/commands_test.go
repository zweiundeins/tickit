package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

var ctx = context.Background()

// FR-11.2, NFR-18: issues created at the same moment in one project get
// different numbers, 1 to n.
func TestNumbersUnderConcurrency(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	const n = 60
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		who := []people.Person{sunny, rainy, member}[i%3]
		wg.Go(func() {
			errs <- e.do(CreateIssue{By: tabs.By{Actor: who, Op: fmt.Sprint("c", i), At: t0}, Project: p, Type: Task, Summary: fmt.Sprint("Issue ", i)})
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := e.DB.R.Query(`SELECT number, key FROM issues ORDER BY number`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := int64(1)
	for rows.Next() {
		var num int64
		var key string
		if err := rows.Scan(&num, &key); err != nil {
			t.Fatal(err)
		}
		if num != want || key != fmt.Sprint("WEB-", want) {
			t.Fatalf("issue %d is %d %s", want, num, key)
		}
		want++
	}
	if want != n+1 || e.row(`SELECT count(*) FROM issue_keys`) != fmt.Sprint(n) {
		t.Errorf("%d issues, %s keys", want-1, e.row(`SELECT count(*) FROM issue_keys`))
	}
	if got := e.row(`SELECT json_extract(state, '$.last') FROM lith_event_state WHERE stream = 'project'`); got != fmt.Sprint(n) {
		t.Errorf("the project's last number is %s", got)
	}
}

// FR-11.3, FR-12, FR-28.2, FR-29.1: keys resolve without regard to case;
// a deleted issue's key says who deleted it and its number is not given
// again; a moved issue's old key leads to its new one.
func TestKeys(t *testing.T) {
	e := newEnv(t)
	web, app := e.project("WEB", true), e.project("APP", true)
	a := e.issue(sunny, CreateIssue{Project: web})
	b := e.issue(sunny, CreateIssue{Project: web})
	l, err := IssueByKey(ctx, e.DB.R, " web-2 ")
	if err != nil || l.Issue != b || l.Key != "WEB-2" || l.Moved || l.Deleted {
		t.Fatalf("web-2: %+v %v", l, err)
	}
	for _, k := range []string{"WEB-3", "web-02", "WEB", "W-1", "WEB-x", ""} {
		if _, err := IssueByKey(ctx, e.DB.R, k); !errors.Is(err, access.ErrNotFound) {
			t.Errorf("%q: %v", k, err)
		}
	}
	by := e.by(admin)
	e.must(DeleteIssue{By: by, Issue: b})
	l, _ = IssueByKey(ctx, e.DB.R, "WEB-2")
	if !l.Deleted || l.DeletedBy != admin.ID || !l.DeletedAt.Equal(by.At) {
		t.Errorf("a deleted key: %+v", l)
	}
	c := e.issue(sunny, CreateIssue{Project: web})
	if got := e.state(c).Key; got != "WEB-3" {
		t.Errorf("after a delete the next key is %s", got)
	}
	res := e.result(MoveToProject{By: e.by(sunny), Issue: a, Project: app})
	if res["key"] != "APP-1" {
		t.Errorf("moved: %v", res)
	}
	l, _ = IssueByKey(ctx, e.DB.R, "web-1")
	if l.Issue != a || l.Key != "APP-1" || !l.Moved || l.Project != app {
		t.Errorf("an old key: %+v", l)
	}
	if s := e.state(a); !slices.Equal(s.Keys, []string{"WEB-1", "APP-1"}) || s.Number != 1 || s.Project != app {
		t.Errorf("the moved state: %+v", s)
	}
	d, err := IssueDetail(ctx, e.DB.R, a)
	if err != nil || !slices.Equal(d.Keys, []string{"WEB-1", "APP-1"}) || d.ProjectKey != "APP" {
		t.Errorf("the moved detail: %+v %v", d.Issue, err)
	}
	if got := e.issue(sunny, CreateIssue{Project: web}); e.state(got).Key != "WEB-4" {
		t.Error("a moved issue's number was given again")
	}
}

// act runs cmd as a page does, in tabs.Act, and returns its outcome.
func (e *env) act(cmd writer.Command, by tabs.By) (tabs.Outcome, tabs.Result, error) {
	e.t.Helper()
	o := writer.OnceResult("s:"+by.Op, tabs.Act{Session: "s", Tab: "t", By: by, Cmd: cmd})
	if err := e.W.Do(ctx, o); err != nil {
		return tabs.Outcome{}, tabs.Result{}, err
	}
	var s tabs.State
	var err error
	if s, err = tabs.Get(ctx, e.DB.R, "s", "t"); err != nil {
		e.t.Fatal(err)
	}
	out, _ := s.Outcome(by.Op)
	return out, tabs.ParseResult(o.Result()), nil
}

// MP-08.2, MP-08.3: the second editor of a field is refused with who set
// what how long ago, and nothing is stored; "Use Critical anyway" resends
// against the new version. Changes to other fields apply.
func TestFieldConflicts(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	id := e.issue(rainy, CreateIssue{Project: p, Summary: "Fix the login"})
	seen := e.state(id).Version
	first := e.by(sunny)
	e.must(SetPriority{By: first, Issue: id, Seen: seen, Priority: High})

	second := e.by(rainy)
	second.At = first.At.Add(5 * time.Second)
	o, res, err := e.act(SetPriority{By: second, Issue: id, Seen: seen, Priority: Critical}, second)
	if err != nil || res.OK || res.Denied {
		t.Fatalf("the second change: %+v %+v %v", o, res, err)
	}
	want := sunny.Name + " changed the priority to High 5 seconds ago. Your Critical was not saved."
	if o.Msg != want || o.Field != FieldPriority {
		t.Errorf("the refusal: %q at %q, want %q", o.Msg, o.Field, want)
	}
	var c Conflict
	if err := json.Unmarshal(o.Data, &c); err != nil {
		t.Fatal(err)
	}
	if c.Kind != "conflict" || c.By != sunny.ID || c.Name != sunny.Name || c.Theirs != "High" || c.Yours != "Critical" ||
		c.Version != seen+1 || c.At != first.At.UnixMilli() || c.Label != "priority" {
		t.Errorf("the conflict's data: %s", o.Data)
	}
	if s := e.state(id); s.Priority != High || s.Version != seen+1 {
		t.Errorf("the refused change was stored: %+v", s)
	}
	// MP-08.2: another field changes from the old version.
	e.must(SetSummary{By: e.by(rainy), Issue: id, Seen: seen, Summary: "Fix the login on Safari"})
	// "Use Critical anyway": the version the conflict named.
	e.must(SetPriority{By: e.by(rainy), Issue: id, Seen: c.Version, Priority: Critical})
	if s := e.state(id); s.Priority != Critical || s.Summary != "Fix the login on Safari" {
		t.Errorf("after both: %+v", s)
	}
	// One's own change never conflicts, from another tab either.
	e.must(SetPriority{By: e.by(rainy), Issue: id, Seen: seen, Priority: Low})
	// FR-14.3: the same value stores nothing.
	v := e.state(id).Version
	e.must(SetPriority{By: e.by(sunny), Issue: id, Seen: 1, Priority: Low})
	if e.state(id).Version != v {
		t.Error("the same value was stored")
	}
	// The summary's and the description's sentences.
	err = e.do(SetSummary{By: e.by(sunny), Issue: id, Seen: seen, Summary: "Mine"})
	if got := refusal(t, err); got != rainy.Name+` changed the summary to "Fix the login on Safari" 4 seconds ago. Your "Mine" was not saved.` {
		t.Errorf("the summary's conflict: %q", got)
	}
	// FR-15.2: a description is co-edited, never refused as a conflict.
	e.must(SetDescription{By: e.by(rainy), Issue: id, Description: PlainDoc("One")})
	e.must(SetDescription{By: e.by(sunny), Issue: id, Description: PlainDoc("Two")})
	if got := DocText(e.state(id).Description); got != "Two" {
		t.Errorf("the description %q", got)
	}
	e.must(Assign{By: e.by(rainy), Issue: id, Seen: v, Assignee: "s-priya"})
	err = e.do(Assign{By: e.by(sunny), Issue: id, Seen: v, Assignee: sunny.ID})
	if got := refusal(t, err); got != rainy.Name+" changed the assignee to Priya Raman 1 second ago. Your "+sunny.Name+" was not saved." {
		t.Errorf("the assignee's conflict: %q", got)
	}
}

// MP-04.3, MP-04.4: of two moves of one card the first wins, and the
// second hears who moved it where; card order is one order.
func TestMoveConflicts(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	var ids []string
	for range 4 {
		ids = append(ids, e.issue(sunny, CreateIssue{Project: p}))
	}
	seen := e.state(ids[3]).Version
	// Sunny moves WEB-4 to In Review, between nothing: the column is empty.
	e.must(Move{By: e.by(sunny), Issue: ids[3], Status: StatusInReview, Seen: seen})
	o, _, err := e.act(Move{By: e.by(rainy), Issue: ids[3], Status: StatusDone, Seen: seen}, e.by(rainy))
	if err != nil || o.OK {
		t.Fatalf("the second move: %+v %v", o, err)
	}
	if want := sunny.Name + " moved WEB-4 to In Review first."; o.Msg != want || o.Field != FieldMove {
		t.Errorf("the refusal: %q at %q", o.Msg, o.Field)
	}
	if s := e.state(ids[3]); s.Status != StatusInReview {
		t.Errorf("the second move was stored: %s", s.Status)
	}
	// Rank moves: WEB-4 back to To Do between WEB-1 and WEB-2, WEB-3 to the top.
	e.must(Move{By: e.by(rainy), Issue: ids[3], Status: StatusToDo, After: ids[0], Before: ids[1], Seen: e.state(ids[3]).Version})
	e.must(Move{By: e.by(rainy), Issue: ids[2], Before: ids[0], Seen: e.state(ids[2]).Version})
	b, err := Board(ctx, e.DB.R, p, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, c := range b.Columns[1].Cards {
		keys = append(keys, c.Key)
	}
	if b.Columns[1].ID != StatusToDo || !slices.Equal(keys, []string{"WEB-3", "WEB-1", "WEB-4", "WEB-2"}) {
		t.Errorf("To Do: %v", keys)
	}
	// A page that saw an old order: After wins, and ranks stay distinct.
	e.must(Move{By: e.by(rainy), Issue: ids[1], After: ids[0], Before: ids[2], Seen: e.state(ids[1]).Version})
	if r1, r2 := e.state(ids[1]).Rank, e.state(ids[3]).Rank; r1 == r2 || r1 < e.state(ids[0]).Rank || r1 > r2 {
		t.Errorf("WEB-2 at %s, WEB-1 at %s, WEB-4 at %s", r1, e.state(ids[0]).Rank, r2)
	}
}

// NFR-01.1, section 3.2: rights by role in an open and a closed project,
// and archived projects take no changes (FR-04.1).
func TestAccess(t *testing.T) {
	e := newEnv(t)
	closed, open := e.project("CLO", false), e.project("OPN", true)
	ci := e.issue(member, CreateIssue{Project: closed})
	oi := e.issue(sunny, CreateIssue{Project: open})
	comment := func(issue string) string {
		return e.result(AddComment{By: e.by(member), Issue: issue, Body: PlainDoc("Hi")})["id"]
	}
	cc, oc := comment(ci), comment(oi)

	type outcome int
	const (
		ok outcome = iota
		denied
		hidden // 404: the project is not readable
	)
	check := func(name string, p people.Person, want outcome, cmd func(tabs.By) writer.Command) {
		t.Helper()
		by := e.by(p)
		o, res, err := e.act(cmd(by), by)
		got := ok
		switch {
		case errors.Is(err, access.ErrNotFound):
			got = hidden
		case err != nil:
			t.Fatalf("%s as %s: %v", name, p.Name, err)
		case res.Denied:
			got = denied
		case !o.OK:
			t.Fatalf("%s as %s: refused %q", name, p.Name, o.Msg)
		}
		if got != want {
			t.Errorf("%s as %s: %d, want %d (%s)", name, p.Name, got, want, o.Msg)
		}
	}
	prio := func(issue string) func(tabs.By) writer.Command {
		pr := []Priority{Low, High}
		n := 0
		return func(by tabs.By) writer.Command {
			n++
			return SetPriority{By: by, Issue: issue, Seen: 1 << 40, Priority: pr[n%2]}
		}
	}
	for _, c := range []struct {
		p              people.Person
		closed, inOpen outcome
	}{
		{viewer, denied, ok}, {member, ok, ok}, {admin, ok, ok}, {moderator, denied, ok}, {siteAdmin, ok, ok}, {sunny, hidden, ok},
	} {
		check("edit", c.p, c.closed, prio(ci))
		check("edit in an open project", c.p, c.inOpen, prio(oi))
	}
	for _, c := range []struct {
		p    people.Person
		want outcome
	}{{viewer, ok}, {member, ok}, {admin, ok}, {moderator, ok}, {siteAdmin, ok}, {sunny, hidden}} {
		check("comment", c.p, c.want, func(by tabs.By) writer.Command { return AddComment{By: by, Issue: ci, Body: PlainDoc("x")} })
	}
	for _, c := range []struct {
		p    people.Person
		want outcome
	}{{viewer, denied}, {member, denied}, {admin, ok}, {moderator, ok}, {siteAdmin, ok}, {sunny, hidden}} {
		check("hide a comment", c.p, c.want, func(by tabs.By) writer.Command { return HideComment{By: by, Comment: cc, Hidden: true} })
		e.must(HideComment{By: e.by(siteAdmin), Comment: cc, Hidden: false}) // a moderator's hide, only staff undo (DM-07.1)
	}
	for _, c := range []struct {
		p    people.Person
		want outcome
	}{{viewer, denied}, {admin, denied}, {moderator, ok}, {siteAdmin, ok}, {member, ok}} {
		check("edit someone's comment", c.p, c.want, func(by tabs.By) writer.Command {
			return EditComment{By: by, Comment: cc, Body: PlainDoc("Edited by " + by.Op)}
		})
	}
	check("edit someone's comment in an open project", sunny, denied, func(by tabs.By) writer.Command {
		return EditComment{By: by, Comment: oc, Body: PlainDoc("Mine now")}
	})
	for _, c := range []struct {
		p    people.Person
		want outcome
	}{{viewer, denied}, {member, denied}, {moderator, denied}, {admin, ok}, {siteAdmin, ok}} {
		check("configure", c.p, c.want, func(by tabs.By) writer.Command {
			return DefineComponent{By: by, Project: closed, Name: "C", Description: by.Op}
		})
	}
	check("configure an open project", sunny, denied, func(by tabs.By) writer.Command { return DefineComponent{By: by, Project: open, Name: "C"} })
	for _, c := range []struct {
		p    people.Person
		want outcome
	}{{viewer, denied}, {member, denied}, {moderator, ok}, {sunny, denied}} {
		check("delete", c.p, c.want, func(by tabs.By) writer.Command {
			return DeleteIssue{By: by, Issue: e.issue(admin, CreateIssue{Project: open})}
		})
	}
	check("create in a closed project", sunny, hidden, func(by tabs.By) writer.Command {
		return CreateIssue{By: by, Project: closed, Type: Task, Summary: "x"}
	})
	check("create as a viewer", viewer, denied, func(by tabs.By) writer.Command {
		return CreateIssue{By: by, Project: closed, Type: Task, Summary: "x"}
	})
	check("archive as a member", member, denied, func(by tabs.By) writer.Command { return ArchiveProject{By: by, Project: closed, Archived: true} })
	check("archive", admin, ok, func(by tabs.By) writer.Command { return ArchiveProject{By: by, Project: open, Archived: true} })

	// FR-04.1: an archived project's issues are read-only.
	for _, cmd := range []func(tabs.By) writer.Command{
		prio(oi),
		func(by tabs.By) writer.Command { return CreateIssue{By: by, Project: open, Type: Task, Summary: "x"} },
		func(by tabs.By) writer.Command { return AddComment{By: by, Issue: oi, Body: PlainDoc("x")} },
		func(by tabs.By) writer.Command { return Move{By: by, Issue: oi, Status: StatusDone} },
	} {
		by := e.by(admin)
		o, res, err := e.act(cmd(by), by)
		if err != nil || !res.Denied || o.Msg != "This project is archived." {
			t.Errorf("a change in an archived project: %+v %+v %v", o, res, err)
		}
	}
	check("unarchive", admin, ok, func(by tabs.By) writer.Command { return ArchiveProject{By: by, Project: open, Archived: false} })
	check("edit after unarchiving", sunny, ok, prio(oi))
}

// FR-17: who may be assigned.
func TestAssign(t *testing.T) {
	e := newEnv(t)
	closed := e.project("CLO", false)
	id := e.issue(member, CreateIssue{Project: closed})
	try := func(who people.Person, by people.Person) error {
		return e.do(Assign{By: e.by(by), Issue: id, Seen: 1 << 40, Assignee: who.ID})
	}
	for _, p := range []people.Person{member, admin, siteAdmin, ada} {
		if err := try(p, member); err != nil {
			t.Errorf("assign %s: %v", p.Name, err)
		}
	}
	if err := try(member, member); err != nil { // "Assign to me"
		t.Error(err)
	}
	if got := refusal(t, try(viewer, member)); got != "Vera Viewer cannot be assigned: they are not a member of CLO." {
		t.Errorf("a viewer: %q", got)
	}
	if got := refusal(t, try(people.Person{ID: "s-nobody"}, member)); got != "That person does not exist." {
		t.Errorf("nobody: %q", got)
	}
	// A guest who acted in the project in the last 24 hours; FR-17.3: they watch.
	open := e.project("OPN", true)
	oid := e.issue(member, CreateIssue{Project: open})
	assignIn := func(issue string, who people.Person) error {
		return e.do(Assign{By: e.by(member), Issue: issue, Seen: 1 << 40, Assignee: who.ID})
	}
	if got := refusal(t, assignIn(oid, sunny)); got != sunny.Name+" cannot be assigned: they are not a member of OPN." {
		t.Errorf("a stranger: %q", got)
	}
	e.must(Watch{By: e.by(sunny), Issue: oid, Watch: true}) // reads: not acting
	e.must(SetLabels{By: e.by(sunny), Issue: oid, Add: []string{"x"}})
	if err := assignIn(oid, sunny); err != nil {
		t.Errorf("a guest who acted: %v", err)
	}
	ws, _ := Watchers(ctx, e.DB.R, oid)
	if !slices.Contains(ws, sunny.ID) {
		t.Errorf("watchers %v", ws)
	}
	got, err := Assignable(ctx, e.DB.R, open, "", e.now)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if !slices.Contains(names, sunny.Name) || !slices.Contains(names, "Priya Raman") || !slices.Contains(names, "Adam Admin") || slices.Contains(names, rainy.Name) {
		t.Errorf("assignable: %v", names)
	}
	if got, _ := Assignable(ctx, e.DB.R, open, "", e.now.Add(25*time.Hour)); slices.ContainsFunc(got, func(p people.Person) bool { return p.ID == sunny.ID }) {
		t.Error("a guest is assignable a day after acting")
	}
	if got, _ := Assignable(ctx, e.DB.R, open, "BRUNNER", e.now); len(got) != 1 || got[0].ID != "s-lena" {
		t.Errorf("assignable brunner: %v", got)
	}
	e.now = e.now.Add(25 * time.Hour)
	e.must(Assign{By: e.by(member), Issue: oid, Seen: 1 << 40})
	if got := refusal(t, assignIn(oid, sunny)); got != sunny.Name+" cannot be assigned: they are not a member of OPN." {
		t.Errorf("a day later: %q", got)
	}
}

// FR-20, FR-21, FR-19.2: epics, children and their rules.
func TestEpicsAndSubtasks(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	epic := e.issue(sunny, CreateIssue{Project: p, Type: Epic, Summary: "Checkout"})
	st := e.issue(sunny, CreateIssue{Project: p, Type: Story, Parent: epic})
	sub := e.issue(sunny, CreateIssue{Project: p, Type: Subtask, Parent: st})
	other := e.issue(sunny, CreateIssue{Project: p, Type: Bug, Parent: epic})
	e.must(SetStatus{By: e.by(sunny), Issue: other, Seen: 1, Status: StatusDone})
	if got := e.row(`SELECT epic_id FROM issues WHERE id = ?`, sub); got != epic {
		t.Errorf("the subtask's epic is %q", got)
	}
	d, err := IssueDetail(ctx, e.DB.R, epic)
	if err != nil || len(d.Children) != 2 || d.Progress != (Progress{Done: 1, All: 2}) {
		t.Errorf("the epic: %+v %+v %v", d.Children, d.Progress, err)
	}
	d, _ = IssueDetail(ctx, e.DB.R, sub)
	if d.Parent == nil || d.Parent.ID != st || d.Epic == nil || d.Epic.ID != epic {
		t.Errorf("the subtask's parent and epic: %+v %+v", d.Parent, d.Epic)
	}
	if got := refusal(t, e.do(SetType{By: e.by(sunny), Issue: epic, Seen: 1 << 40, Type: Story})); got != "Move the 2 child issues first." {
		t.Errorf("an epic with children: %q", got)
	}
	if got := refusal(t, e.do(DeleteIssue{By: e.by(admin), Issue: st})); got != "Delete or move its child issue first." {
		t.Errorf("deleting a parent: %q", got)
	}
	if got := refusal(t, e.do(SetParent{By: e.by(sunny), Issue: st, Seen: 1 << 40, Parent: other})); got != "The parent of a story is an epic." {
		t.Errorf("a story under a bug: %q", got)
	}
	// The story leaves the epic: its subtask's epic goes too.
	e.must(SetParent{By: e.by(sunny), Issue: st, Seen: 1 << 40})
	if got := e.row(`SELECT epic_id FROM issues WHERE id = ?`, sub); got != "" {
		t.Errorf("the subtask's epic after: %q", got)
	}
	e.must(DeleteIssue{By: e.by(admin), Issue: sub})
	e.must(SetType{By: e.by(sunny), Issue: st, Seen: 1 << 40, Type: Epic})
	if got := refusal(t, e.do(MoveToProject{By: e.by(sunny), Issue: epic, Project: e.project("APP", true)})); got != "Move the child issue first." {
		t.Errorf("moving an epic with a child: %q", got)
	}
}

// FR-22: links on both issues with the inverse wording.
func TestLinks(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	a, b := e.issue(sunny, CreateIssue{Project: p, Summary: "A"}), e.issue(sunny, CreateIssue{Project: p, Summary: "B"})
	e.must(LinkIssues{By: e.by(sunny), Issue: a, Type: Blocks, Other: b})
	e.must(LinkIssues{By: e.by(sunny), Issue: b, Type: RelatesTo, Other: a})
	da, _ := IssueDetail(ctx, e.DB.R, a)
	db, _ := IssueDetail(ctx, e.DB.R, b)
	words := func(d Detail) (out []string) {
		for _, l := range d.Links {
			out = append(out, l.Wording+" "+l.Issue.Key)
		}
		return out
	}
	if !slices.Equal(words(da), []string{"blocks WEB-2", "relates to WEB-2"}) || !slices.Equal(words(db), []string{"is blocked by WEB-1", "relates to WEB-1"}) {
		t.Errorf("links: %v and %v", words(da), words(db))
	}
	for _, c := range []struct {
		cmd  LinkIssues
		want string
	}{
		{LinkIssues{Issue: a, Type: Blocks, Other: b}, "These issues are linked so already."},
		{LinkIssues{Issue: b, Type: Blocks, Other: a}, "These issues are linked so already."},
		{LinkIssues{Issue: a, Type: Blocks, Other: a}, "An issue cannot link to itself."},
		{LinkIssues{Issue: a, Type: "clones", Other: b}, "Pick blocks, relates to or duplicates."},
		{LinkIssues{Issue: a, Type: Duplicates, Other: NewID()}, "That issue does not exist."},
	} {
		c.cmd.By = e.by(sunny)
		if got := refusal(t, e.do(c.cmd)); got != c.want {
			t.Errorf("%+v: %q", c.cmd, got)
		}
	}
	e.must(UnlinkIssues{By: e.by(sunny), Issue: b, Type: Blocks, Other: a})
	if got := e.row(`SELECT count(*) FROM issue_links`); got != "2" {
		t.Errorf("%s link rows after unlinking one", got)
	}
}

// FR-65 to FR-67: comments, their watchers and mentions.
func TestComments(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	id := e.issue(sunny, CreateIssue{Project: p})
	body := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Ask "},{"type":"mention","attrs":{"personId":"s-priya","label":"Ada"}}]}]}`)
	c := e.result(AddComment{By: e.by(rainy), Issue: id, Body: body})["id"]
	ws, _ := Watchers(ctx, e.DB.R, id)
	if !slices.Equal(ws, []string{sunny.ID, rainy.ID, "s-priya"}) {
		t.Errorf("watchers %v", ws)
	}
	if got := refusal(t, e.do(EditComment{By: e.by(sunny), Comment: c, Body: PlainDoc("No")})); got != "Only moderators can change other people's comments." {
		t.Errorf("someone else edits: %q", got)
	}
	e.must(EditComment{By: e.by(rainy), Comment: c, Body: PlainDoc("Asked")})
	e.must(DeleteComment{By: e.by(rainy), Comment: c})
	c2 := e.result(AddComment{By: e.by(sunny), Issue: id, Body: PlainDoc("Second")})["id"]
	e.must(HideComment{By: e.by(moderator), Comment: c2, Hidden: true, Reason: "spam"})
	cs, err := Comments(ctx, e.DB.R, id)
	if err != nil || len(cs) != 2 {
		t.Fatalf("comments %+v %v", cs, err)
	}
	if !cs[0].IsDeleted() || cs[0].Body != nil || cs[0].Edited.IsZero() || cs[0].Author != rainy.ID {
		t.Errorf("the deleted comment: %+v", cs[0])
	}
	if !cs[1].IsHidden() || cs[1].Body != nil || string(cs[1].HiddenBody) != string(PlainDoc("Second")) || cs[1].HiddenBy != moderator.ID {
		t.Errorf("the hidden comment: %+v", cs[1])
	}
	if _, err := Comments(ctx, e.DB.R, NewID()); err != nil {
		t.Error(err)
	}
	if err := e.do(EditComment{By: e.by(rainy), Comment: NewID(), Body: PlainDoc("x")}); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("an unknown comment: %v", err)
	}
}

// FR-68, FR-70: history from the stream, interleaved with comments; the
// issue as of a version.
func TestHistory(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	id := e.issue(sunny, CreateIssue{Project: p, Summary: "Old"})
	e.must(SetPriority{By: e.by(rainy), Issue: id, Seen: 1, Priority: Critical})
	e.must(Assign{By: e.by(rainy), Issue: id, Seen: 2, Assignee: "s-priya"})
	e.result(AddComment{By: e.by(rainy), Issue: id, Body: PlainDoc("Done soon")})
	e.must(Move{By: e.by(sunny), Issue: id, Status: StatusInReview, Seen: 4})
	e.must(SetEstimate{By: e.by(sunny), Issue: id, Seen: 4, Estimate: ptr(150)})
	h, err := History(ctx, e.DB.R, id)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, x := range h {
		lines = append(lines, fmt.Sprintf("%d %s %s: %s -> %s", x.Version, x.Actor[:4], x.Field, x.Old, x.New))
	}
	want := []string{
		"1 " + sunny.ID[:4] + " created:  -> WEB-1",
		"2 " + rainy.ID[:4] + " priority: Medium -> Critical",
		"3 " + rainy.ID[:4] + " assignee:  -> Priya Raman",
		"4 " + rainy.ID[:4] + " watchers:  -> Priya Raman",
		"5 " + rainy.ID[:4] + " watchers:  -> " + rainy.Name, // the comment's author watches
		"6 " + sunny.ID[:4] + " status: To Do -> In Review",
		"7 " + sunny.ID[:4] + " estimate:  -> 2h 30m",
	}
	if !slices.Equal(lines, want) {
		t.Errorf("history:\n%q\nwant\n%q", lines, want)
	}
	a, err := Activity(ctx, e.DB.R, id)
	if err != nil || len(a) != len(h)+1 || a[5].Comment == nil {
		t.Errorf("activity: %d items, %v", len(a), err)
	}
	s, v, err := StateAt(ctx, e.DB.R, id, 2)
	if err != nil || v != 2 || s.Priority != Critical || s.Assignee != "" {
		t.Errorf("as of 2: %+v %d %v", s, v, err)
	}
	if _, v, _ := StateAt(ctx, e.DB.R, id, 0); v != 7 {
		t.Errorf("now is version %d", v)
	}
	if _, _, err := StateAt(ctx, e.DB.R, NewID(), 0); !errors.Is(err, access.ErrNotFound) {
		t.Error(err)
	}
}

// FR-40, FR-42, FR-43, FR-45, FR-46: the board, the backlog and sprints.
func TestBoardAndBacklog(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	a := e.issue(sunny, CreateIssue{Project: p, Summary: "A", Labels: []string{"ux"}, Points: ptr(3)})
	b := e.issue(sunny, CreateIssue{Project: p, Summary: "B", Status: StatusBacklog})
	c := e.issue(sunny, CreateIssue{Project: p, Summary: "C", Type: Bug, Assignee: sunny.ID, Points: ptr(2)})
	d := e.issue(sunny, CreateIssue{Project: p, Summary: "D", Status: StatusDone})
	e.must(SetWIPLimit{By: e.by(admin), Project: p, Status: StatusToDo, Limit: 1})
	bd, err := Board(ctx, e.DB.R, p, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bd.Columns) != 6 || bd.Columns[0].ID != StatusSelected || bd.Total != 3 {
		t.Fatalf("columns %d, first %s, %d cards", len(bd.Columns), bd.Columns[0].ID, bd.Total)
	}
	todo := bd.Columns[1]
	if len(todo.Cards) != 2 || todo.Cards[0].ID != a || !todo.Over || todo.WIP != 1 || todo.Points != 5 {
		t.Errorf("To Do: %+v", todo)
	}
	q, _ := Board(ctx, e.DB.R, p, Filter{Assignees: []string{Me}}.Normalize(sunny.ID))
	if q.Total != 1 || q.Columns[1].Cards[0].ID != c {
		t.Errorf("only my issues: %d", q.Total)
	}
	q, _ = Board(ctx, e.DB.R, p, Filter{Labels: []string{"UX"}})
	if q.Total != 1 {
		t.Errorf("label ux: %d", q.Total)
	}
	bl, err := Backlog(ctx, e.DB.R, p)
	if err != nil || len(bl.Cards) != 3 || bl.Cards[1].ID != b {
		t.Errorf("backlog %+v %v", bl.Cards, err)
	}
	// A sprint with A and B, started: the board shows its issues alone.
	sp := e.result(CreateSprint{By: e.by(sunny), Project: p, Name: "Sprint 1"})["id"]
	next := e.result(CreateSprint{By: e.by(sunny), Project: p, Name: "Sprint 2"})["id"]
	for _, id := range []string{a, b} {
		e.must(SetSprint{By: e.by(sunny), Issue: id, Seen: 1 << 40, Sprint: sp})
	}
	e.must(SetSprint{By: e.by(sunny), Issue: c, Seen: 1 << 40, Sprint: next})
	bl, _ = Backlog(ctx, e.DB.R, p)
	if len(bl.Sprints) != 2 || len(bl.Sprints[0].Cards) != 2 || len(bl.Sprints[1].Cards) != 1 || len(bl.Cards) != 0 {
		t.Errorf("backlog with sprints: %+v", bl)
	}
	e.must(StartSprint{By: e.by(sunny), Project: p, Sprint: sp})
	bd, _ = Board(ctx, e.DB.R, p, Filter{})
	if bd.Sprint == nil || bd.Sprint.ID != sp || bd.Total != 1 {
		t.Errorf("the sprint's board: %+v, %d cards", bd.Sprint, bd.Total)
	}
	bl, _ = Backlog(ctx, e.DB.R, p)
	if bl.Active == nil || len(bl.Sprints) != 1 || len(bl.Sprints[0].Cards) != 1 {
		t.Errorf("backlog with an active sprint: %+v", bl)
	}
	e.must(SetStatus{By: e.by(sunny), Issue: a, Seen: 1 << 40, Status: StatusDone})
	e.must(CompleteSprint{By: e.by(sunny), Project: p, Sprint: sp, MoveTo: next})
	if e.state(b).Sprint != next || e.state(a).Sprint != sp {
		t.Errorf("after the sprint: a in %s, b in %s", e.state(a).Sprint, e.state(b).Sprint)
	}
	if e.row(`SELECT resolved_at IS NOT NULL FROM issues WHERE id = ?`, a) != "1" || e.row(`SELECT resolved_at IS NOT NULL FROM issues WHERE id = ?`, d) != "1" {
		t.Error("FR-38: done issues have a resolved time")
	}
	e.must(SetStatus{By: e.by(sunny), Issue: a, Seen: 1 << 40, Status: StatusInProgress})
	if e.row(`SELECT resolved_at IS NULL FROM issues WHERE id = ?`, a) != "1" {
		t.Error("FR-38: leaving Done clears the resolved time")
	}
}

// FR-24: components; removing one takes it off its issues.
func TestComponents(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	e.must(DefineComponent{By: e.by(admin), Project: p, Name: "Search"})
	e.must(DefineComponent{By: e.by(admin), Project: p, Name: "Cart"})
	id := e.issue(sunny, CreateIssue{Project: p, Components: []string{"search"}})
	e.must(SetComponents{By: e.by(sunny), Issue: id, Add: []string{"Cart"}})
	if got := e.row(`SELECT components FROM issues`); got != `["Search","Cart"]` {
		t.Errorf("components %s", got)
	}
	e.must(RemoveComponent{By: e.by(admin), Project: p, Name: "search"})
	if got := e.row(`SELECT components FROM issues`); got != `["Cart"]` {
		t.Errorf("after removing Search: %s", got)
	}
	cs, _ := Components(ctx, e.DB.R, p)
	if len(cs) != 1 || cs[0].Name != "Cart" {
		t.Errorf("components %+v", cs)
	}
}

// FR-02: projects and their keys.
func TestProjects(t *testing.T) {
	e := newEnv(t)
	res := e.result(CreateProject{By: e.by(sunny), Key: " web ", Name: "Web shop", Open: true})
	if res["key"] != "WEB" {
		t.Fatalf("%v", res)
	}
	for _, c := range []struct {
		cmd  CreateProject
		want string
	}{
		{CreateProject{Key: "web", Name: "Again"}, "The key WEB is taken."}, // FR-02.2
		{CreateProject{Key: "W", Name: "x"}, "A key has 2 to 6 letters A to Z."},
		{CreateProject{Key: "WEB2", Name: "x"}, "A key has 2 to 6 letters A to Z."},
		{CreateProject{Key: "ABC", Name: " "}, "A name has 1 to 100 characters."},
	} {
		c.cmd.By = e.by(sunny)
		if got := refusal(t, e.do(c.cmd)); got != c.want {
			t.Errorf("%+v: %q", c.cmd, got)
		}
	}
	role, _ := access.RoleOf(ctx, e.DB.R, sunny.ID, access.ProjectScope(res["id"]))
	if role != access.Admin {
		t.Errorf("FR-02.3: the creator is %q", role)
	}
	pr, err := ProjectByKey(ctx, e.DB.R, "Web")
	if err != nil || pr.Name != "Web shop" || pr.CreatedBy != sunny.ID {
		t.Errorf("by key: %+v %v", pr, err)
	}
	cols, _ := Statuses(ctx, e.DB.R, pr.ID)
	if len(cols) != 7 || cols[6].Category != Done {
		t.Errorf("FR-35.1: %+v", cols)
	}
	name := "Shop"
	e.must(EditProject{By: e.by(sunny), Project: pr.ID, Name: &name})
	e.issue(sunny, CreateIssue{Project: pr.ID})
	arch := e.result(CreateProject{By: e.by(sunny), Key: "ARC", Name: "Archive", Open: true})["id"]
	e.must(ArchiveProject{By: e.by(sunny), Project: arch, Archived: true})
	readable, _ := Readable(ctx, e.DB.R, rainy)
	ps, err := Projects(ctx, e.DB.R, readable)
	if err != nil || len(ps) != 2 || ps[0].Name != "Shop" || ps[0].OpenIssues != 1 || !ps[1].Archived {
		t.Errorf("FR-01.1: %+v %v", ps, err)
	}
	if got := refusal(t, e.do(CreateProject{By: e.by(sunny), Key: "arc", Name: "x"})); got != "The key ARC is taken." {
		t.Errorf("an archived project's key: %q", got)
	}
}

// The seed runs commands as seeded people at past times.
func TestSeedCommands(t *testing.T) {
	e := newEnv(t)
	past := t0.AddDate(0, 0, -12)
	by := tabs.By{Actor: ada, Op: "seed:1", At: past}
	p := e.result(CreateProject{By: by, Key: "OPS", Name: "Platform", Open: true})["id"]
	id := e.result(CreateIssue{By: by, Project: p, Type: Task, Summary: "Seeded", Assignee: "s-tomasz"})["id"]
	e.must(SetStatus{By: tabs.By{Actor: people.Seeded[1], Op: "seed:2", At: past.Add(time.Hour)}, Issue: id, Seen: 1, Status: StatusDone})
	h, _ := History(ctx, e.DB.R, id)
	if len(h) != 2 || !h[0].At.Equal(past) || h[0].Actor != "s-priya" || h[1].Actor != "s-tomasz" {
		t.Errorf("seeded history: %+v", h)
	}
	if got := e.row(`SELECT created_at FROM issues`); got != fmt.Sprint(past.UnixMilli()) {
		t.Errorf("created at %s", got)
	}
}

// Commands touch the topics of the views they change.
func TestTopics(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	epic := e.issue(sunny, CreateIssue{Project: p, Type: Epic})
	id := e.issue(sunny, CreateIssue{Project: p, Parent: epic})
	other := e.issue(sunny, CreateIssue{Project: p})
	e.must(LinkIssues{By: e.by(sunny), Issue: id, Type: Blocks, Other: other})
	e.Touched()
	e.must(Assign{By: e.by(sunny), Issue: id, Seen: 1 << 40, Assignee: "s-priya"})
	_ = e.Touched()
	e.must(SetPriority{By: e.by(sunny), Issue: id, Seen: 1 << 40, Priority: High})
	got := e.Touched()
	for _, want := range []string{IssueTopic(id), IssueTopic(epic), IssueTopic(other), IssuesTopic, BoardTopic(p)} {
		if !slices.Contains(got, want) {
			t.Errorf("a priority change touched %v, not %s", got, want)
		}
	}
	e.must(Assign{By: e.by(sunny), Issue: id, Seen: 1 << 40, Assignee: sunny.ID})
	if got := e.Touched(); !slices.Contains(got, people.PersonTopic(sunny.ID)) {
		t.Errorf("an assignment touched %v", got)
	}
	e.must(DefineComponent{By: e.by(admin), Project: p, Name: "Search"})
	if got := e.Touched(); !slices.Contains(got, ProjectTopic(p)) || !slices.Contains(got, ProjectsTopic) {
		t.Errorf("a component touched %v", got)
	}
}
