package tracker

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/tabs"
)

// bulk runs a bulk edit as a page's action does, with its note.
func (e *env) bulk(c BulkEdit) (tabs.Outcome, tabs.Result, BulkResult) {
	e.t.Helper()
	o := writer.OnceResult("s:"+c.By.Op, tabs.Act{Session: "s", Tab: "t", By: c.By, Cmd: c, Note: BulkNote})
	if err := e.W.Do(ctx, o); err != nil {
		e.t.Fatalf("bulk %s: %v", c.Field, err)
	}
	s, err := tabs.Get(ctx, e.DB.R, "s", "t")
	if err != nil {
		e.t.Fatal(err)
	}
	out, _ := s.Outcome(c.By.Op)
	var r BulkResult
	if len(out.Data) > 0 {
		if err := json.Unmarshal(out.Data, &r); err != nil {
			e.t.Fatalf("the outcome's data %s: %v", out.Data, err)
		}
	}
	return out, tabs.ParseResult(o.Result()), r
}

// FR-58.1, FR-16.1: a status, an assignee, a priority, a label and a
// sprint for several issues at once, each through its single edit.
func TestBulkEdit(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	a := e.issue(rainy, CreateIssue{Project: p})
	b := e.issue(rainy, CreateIssue{Project: p})
	c := e.issue(rainy, CreateIssue{Project: p, Labels: []string{"ux"}})
	all := []string{a, b, c}
	sprint := e.result(CreateSprint{By: e.by(sunny), Project: p, Name: "Sprint 1"})["id"]

	o, res, r := e.bulk(BulkEdit{By: e.by(sunny), Issues: all, Field: FieldStatus, Value: StatusInProgress})
	if !o.OK || !res.OK || o.Msg != "3 issues changed." || strings.Join(r.Changed, " ") != "WEB-1 WEB-2 WEB-3" || len(r.Refused) != 0 {
		t.Fatalf("the status: %+v %+v %+v", o, res, r)
	}
	for _, edit := range []BulkEdit{
		{Field: FieldAssignee, Value: sunny.ID},
		{Field: FieldPriority, Value: string(Critical)},
		{Field: FieldLabels, Value: "Safari"},
		{Field: FieldLabels, Value: "ux", Remove: true},
		{Field: FieldSprint, Value: sprint},
	} {
		edit.By, edit.Issues = e.by(sunny), all
		if o, _, _ := e.bulk(edit); !o.OK {
			t.Fatalf("%s: %+v", edit.Field, o)
		}
	}
	for _, id := range all {
		s := e.state(id)
		if s.Status != StatusInProgress || s.Assignee != sunny.ID || s.Priority != Critical || strings.Join(s.Labels, " ") != "safari" || s.Sprint != sprint {
			t.Errorf("%s: %+v", s.Key, s)
		}
	}
	// FR-16.2: the history has the change, by the person who made it.
	h, err := History(ctx, e.DB.R, a)
	if err != nil {
		t.Fatal(err)
	}
	if last := h[len(h)-1]; last.Field != FieldSprint || last.Actor != sunny.ID {
		t.Errorf("the history's last entry: %+v", last)
	}
	// The value a field holds stores nothing, and still counts as done.
	v := e.state(a).Version
	if o, _, _ := e.bulk(BulkEdit{By: e.by(sunny), Issues: all, Field: FieldPriority, Value: string(Critical)}); !o.OK || e.state(a).Version != v {
		t.Errorf("the same priority: %+v, version %d to %d", o, v, e.state(a).Version)
	}
	// One refusal for a label no issue may have.
	o, res, _ = e.bulk(BulkEdit{By: e.by(sunny), Issues: all, Field: FieldLabels, Value: "two words"})
	if o.OK || res.Denied || o.Msg != "A label has 1 to 50 characters and no spaces." {
		t.Errorf("a bad label: %+v %+v", o, res)
	}
}

// FR-58.2: each issue changes or is refused on its own, and the outcome
// names the refused ones and why; an issue the actor may not read is not
// named.
func TestBulkRefusals(t *testing.T) {
	e := newEnv(t)
	web := e.project("WEB", true)
	app := e.project("APP", true)
	sec := e.project("SEC", false)
	w1 := e.issue(rainy, CreateIssue{Project: web})
	w2 := e.issue(rainy, CreateIssue{Project: web})
	a1 := e.issue(rainy, CreateIssue{Project: app})
	s1 := e.issue(admin, CreateIssue{Project: sec})
	sprint := e.result(CreateSprint{By: e.by(sunny), Project: web, Name: "Sprint 1"})["id"]

	// A Viewer of SEC changes the open projects' issues, not SEC's.
	o, res, r := e.bulk(BulkEdit{By: e.by(viewer), Issues: []string{w1, s1, w2}, Field: FieldPriority, Value: string(High)})
	want := "2 issues changed. 1 was not:\nSEC-1: Only members of this project can change its issues."
	if !o.OK || !res.OK || o.Msg != want {
		t.Errorf("a Viewer: %+v %+v, want %q", o, res, want)
	}
	if len(r.Refused) != 1 || r.Refused[0].ID != s1 || r.Refused[0].Key != "SEC-1" || strings.Join(r.Changed, " ") != "WEB-1 WEB-2" {
		t.Errorf("a Viewer's result: %+v", r)
	}
	if e.state(s1).Priority == High || e.state(w2).Priority != High {
		t.Error("the refusal undid the others, or stored its own")
	}
	// A sprint is one project's: the other project's issue is refused.
	if _, _, r := e.bulk(BulkEdit{By: e.by(sunny), Issues: []string{w1, a1}, Field: FieldSprint, Value: sprint}); len(r.Refused) != 1 || r.Refused[0].Key != "APP-1" {
		t.Errorf("a sprint of WEB on APP-1: %+v", r)
	}
	// A guest does not read SEC: its issue is refused without its key.
	_, _, r = e.bulk(BulkEdit{By: e.by(rainy), Issues: []string{s1, w1}, Field: FieldAssignee, Value: ""})
	if len(r.Refused) != 1 || r.Refused[0].Key != "" || strings.Contains(r.Message(), "SEC") {
		t.Errorf("an issue the guest cannot read: %+v", r)
	}
	// FR-28.1: a Member may not delete; with every issue refused for want
	// of a right, the whole is a denial and nothing changes.
	events := e.row(`SELECT count(*) FROM lith_events`)
	o, res, r = e.bulk(BulkEdit{By: e.by(sunny), Issues: []string{w1, w2}, Field: BulkDelete})
	if o.OK || !res.Denied || len(r.Refused) != 2 || !strings.HasPrefix(o.Msg, "No issue was deleted.\nWEB-1: Only admins of this project can delete issues.") {
		t.Errorf("a Member's delete: %+v %+v %+v", o, res, r)
	}
	if e.row(`SELECT count(*) FROM lith_events`) != events || e.state(w1).Deleted {
		t.Error("a refused delete changed something")
	}
	// A project's Admin deletes them.
	if o, _, r := e.bulk(BulkEdit{By: e.by(admin), Issues: []string{w1, w2}, Field: BulkDelete}); !o.OK || o.Msg != "2 issues deleted." || len(r.Changed) != 2 {
		t.Errorf("an Admin's delete: %+v", o)
	}
	if !e.state(w1).Deleted || !e.state(w2).Deleted {
		t.Error("not deleted")
	}
	// Every issue refused by a rule: refused, not denied.
	o, res, _ = e.bulk(BulkEdit{By: e.by(sunny), Issues: []string{w1}, Field: FieldStatus, Value: StatusDone})
	if o.OK || res.Denied || o.Msg != "No issue was changed.\nWEB-1: This issue was deleted." {
		t.Errorf("a deleted issue's status: %+v %+v", o, res)
	}
}

// MP-08: a bulk edit writes the last value over a change it did not see,
// and the history records both.
func TestBulkLastWrite(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	id := e.issue(rainy, CreateIssue{Project: p})
	e.must(SetPriority{By: e.by(rainy), Issue: id, Seen: e.state(id).Version, Priority: High})
	if o, _, _ := e.bulk(BulkEdit{By: e.by(sunny), Issues: []string{id}, Field: FieldPriority, Value: string(Low)}); !o.OK {
		t.Fatalf("the bulk edit: %+v", o)
	}
	if s := e.state(id); s.Priority != Low {
		t.Errorf("the priority: %s", s.Priority)
	}
	h, err := History(ctx, e.DB.R, id)
	if err != nil {
		t.Fatal(err)
	}
	var who []string
	for _, x := range h {
		if x.Field == FieldPriority {
			who = append(who, x.Actor+" "+x.NewRaw)
		}
	}
	if strings.Join(who, ", ") != rainy.ID+" high, "+sunny.ID+" low" {
		t.Errorf("the priority's history: %v", who)
	}
}

// FR-58: at most 100 issues, each once, of the fields a bulk edit has.
func TestBulkValidate(t *testing.T) {
	by := tabs.By{Actor: sunny, Op: "op", At: t0}
	ids := make([]string, MaxBatch+1)
	for i := range ids {
		ids[i] = NewID()
	}
	for _, c := range []BulkEdit{
		{By: by, Field: FieldStatus, Value: StatusDone},
		{By: by, Issues: ids, Field: FieldStatus, Value: StatusDone},
		{By: by, Issues: []string{ids[0], ids[0]}, Field: FieldStatus, Value: StatusDone},
		{By: by, Issues: []string{"WEB-1"}, Field: FieldStatus, Value: StatusDone},
		{By: by, Issues: ids[:1], Field: FieldSummary, Value: "x"},
		{By: by, Issues: ids[:1], Field: FieldPriority, Value: "urgent"},
		{By: by, Issues: ids[:1], Field: FieldAssignee, Value: "nobody"},
		{By: by, Issues: ids[:1], Field: FieldSprint, Value: "next"},
		{By: by, Issues: ids[:1], Field: FieldStatus},
		{Issues: ids[:1], Field: BulkDelete},
	} {
		if c.Validate() == nil {
			t.Errorf("valid: %+v", c)
		}
	}
	if err := (BulkEdit{By: by, Issues: ids[:MaxBatch], Field: BulkDelete}).Validate(); err != nil {
		t.Errorf("100 issues: %v", err)
	}
}

// FR-58: a bulk edit of 100 issues is one writer command; it holds the
// writer for the time of 100 single edits.
func TestBulkHundred(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	e := newEnv(t)
	p := e.project("WEB", true)
	ids := make([]string, MaxBatch)
	for i := range ids {
		ids[i] = e.issue(rainy, CreateIssue{Project: p})
	}
	start := time.Now()
	o, _, r := e.bulk(BulkEdit{By: e.by(sunny), Issues: ids, Field: FieldStatus, Value: StatusInReview})
	took := time.Since(start)
	if !o.OK || len(r.Changed) != MaxBatch {
		t.Fatalf("100 issues: %+v", o)
	}
	t.Logf("100 status changes in one command: %v", took)
	if took > 2*time.Second {
		t.Errorf("100 status changes took %v", took)
	}
}
