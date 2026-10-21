package web_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// unlimited is the list's server without rate limits.
func unlimited(t *testing.T) *listEnv {
	return newListOn(t, webtest.StartWith(t, webtest.Options{Configure: web.Unlimited}))
}

func (e *listEnv) detail(t *testing.T, key string) tracker.Detail {
	t.Helper()
	d, err := tracker.IssueDetail(context.Background(), e.DB.R, e.ids[key])
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// bulk posts a bulk edit from a listener's tab and returns its status.
func (e *listEnv) bulk(t *testing.T, l *listener, op, field, value, ref string, keys ...string) int {
	t.Helper()
	var ids []string
	for _, k := range keys {
		ids = append(ids, `"`+e.ids[k]+`"`)
	}
	body := fmt.Sprintf(`{"tab":%q,"op":%q,"issues":[%s],"field":%q,"value":%q,"ref":%q}`, l.tab, op, strings.Join(ids, ","), field, value, ref)
	code, _ := webtest.Post(t, l.c, e.URL+"/cmd/issues/bulk", body)
	return code
}

// FR-58.1, FR-58.2, FR-16.1, FR-16.2: a guest changes the status of three
// issues from the list; the one in an archived project is refused and
// named in the outcome, and another person's open list shows the others'
// new status in its next frame.
func TestBulkStatus(t *testing.T) {
	e := newList(t)
	e.Do(t, tracker.ArchiveProject{By: e.by(e.ada), Project: e.app, Archived: true})
	me := e.open(t, webtest.Client(t), "tab1", "")
	other := e.open(t, webtest.Client(t), "tab2", "status=inprogress")

	if code := e.bulk(t, me, "op1", "status", tracker.StatusInProgress, "", "WEB-1", "WEB-2", "APP-1"); code != http.StatusNoContent {
		t.Fatalf("the bulk edit: %d", code)
	}
	o := webtest.Next(t, me.events, `"op1":{`)
	if !strings.Contains(o, `"op1":{"ok":true,"msg":"2 issues changed. 1 was not:\nAPP-1: This project is archived."`) ||
		!strings.Contains(o, `"refused":[{"id":"`+e.ids["APP-1"]+`","key":"APP-1"`) {
		t.Errorf("the outcome: %s", o)
	}
	frame := webtest.Next(t, other.events, "WEB-1")
	if got := keys(frame); !slices.Equal(got, []string{"WEB-2", "WEB-1"}) { // an archived project's issues leave the list
		t.Errorf("the other list: %v", got)
	}
}

// FR-58.1: assignee ("me"), priority, a label and a sprint for several
// issues; FR-28.1: deleting them needs a project's Admin, so a guest's
// delete is a denial (403) that names each issue and changes none.
func TestBulkFields(t *testing.T) {
	e := unlimited(t)
	c := webtest.Client(t)
	_, home := webtest.Get(t, c, e.URL+"/")
	guest := guestID.FindStringSubmatch(home)[1]
	l := e.open(t, c, "tab1", "")
	sprint := e.result(tracker.CreateSprint{By: e.by(e.ada), Project: e.web, Name: "Sprint 1"})
	for i, x := range []struct{ field, value, ref string }{
		{"assignee", "me", ""},
		{"priority", "low", ""},
		{"labels", "Safari", "add"},
		{"labels", "ux", "remove"},
		{"sprint", sprint, ""},
	} {
		op := fmt.Sprint("op", i)
		if code := e.bulk(t, l, op, x.field, x.value, x.ref, "WEB-1", "WEB-2"); code != http.StatusNoContent {
			t.Fatalf("%s: %d", x.field, code)
		}
		if o := webtest.Next(t, l.events, `"`+op+`":{`); !strings.Contains(o, `"`+op+`":{"ok":true,"msg":"2 issues changed."`) {
			t.Errorf("%s: %s", x.field, o)
		}
	}
	for _, k := range []string{"WEB-1", "WEB-2"} {
		d := e.detail(t, k)
		if d.Assignee != guest || d.Priority != tracker.Low || !slices.Contains(d.Labels, "safari") || slices.Contains(d.Labels, "ux") || d.Sprint != sprint {
			t.Errorf("%s: %s %s %v %s", k, d.Assignee, d.Priority, d.Labels, d.Sprint)
		}
	}
	before := e.Digests(t, bookkeeping...)
	if code := e.bulk(t, l, "del", "delete", "", "", "WEB-1", "WEB-2"); code != http.StatusForbidden {
		t.Errorf("a guest's delete: %d", code)
	}
	if o := webtest.Next(t, l.events, `"del":{`); !strings.Contains(o, `"msg":"No issue was deleted.\nWEB-1: Only admins of this project can delete issues.\nWEB-2: Only admins`) {
		t.Errorf("the delete's outcome: %s", o)
	}
	if diff := changed(before, e.Digests(t, bookkeeping...)); len(diff) > 0 {
		t.Errorf("a refused delete changed %v", diff)
	}
}

// DM-04: a bulk edit is an action of its own class, 3 at once a session;
// FR-58: of at most 100 issues, each once; a malformed one answers 400.
func TestBulkLimits(t *testing.T) {
	e := newList(t)
	l := e.open(t, webtest.Client(t), "tab1", "")
	for i := range 3 {
		if code := e.bulk(t, l, fmt.Sprint("op", i), "priority", "high", "", "WEB-1"); code != http.StatusNoContent {
			t.Fatalf("bulk edit %d: %d", i, code)
		}
	}
	if code := e.bulk(t, l, "op4", "priority", "high", "", "WEB-1"); code != http.StatusTooManyRequests {
		t.Errorf("a fourth at once: %d", code)
	}
	e = unlimited(t)
	l = e.open(t, webtest.Client(t), "tab1", "")
	many := make([]string, tracker.MaxBatch+1)
	for i := range many {
		many[i] = `"` + tracker.NewID() + `"`
	}
	for name, body := range map[string]string{
		"101 issues":   `"issues":[` + strings.Join(many, ",") + `],"field":"status","value":"done"`,
		"none":         `"issues":[],"field":"status","value":"done"`,
		"twice":        `"issues":["` + e.ids["WEB-1"] + `","` + e.ids["WEB-1"] + `"],"field":"status","value":"done"`,
		"a key":        `"issues":["WEB-1"],"field":"status","value":"done"`,
		"no field":     `"issues":["` + e.ids["WEB-1"] + `"],"field":"summary","value":"x"`,
		"no priority":  `"issues":["` + e.ids["WEB-1"] + `"],"field":"priority","value":"urgent"`,
		"a label verb": `"issues":["` + e.ids["WEB-1"] + `"],"field":"labels","value":"ux","ref":"toggle"`,
	} {
		code, _ := webtest.Post(t, l.c, e.URL+"/cmd/issues/bulk", `{"tab":"tab1","op":"x`+strings.ReplaceAll(name, " ", "")+`",`+body+`}`)
		if code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
}

// FR-58.1: the list's rows have their boxes and the page its bar; Delete
// shows only to those who may delete issues somewhere.
func TestBulkBar(t *testing.T) {
	e := newList(t)
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/issues")
	for _, want := range []string{`id="list-bulk"`, `class="tk-check__box list-check-all"`, `aria-label="Select WEB-1" data-i="` + e.ids["WEB-1"] + `"`, `data-bulk="status" data-value="inprogress"`} {
		if !strings.Contains(page, want) {
			t.Errorf("no %s in the list's page", want)
		}
	}
	if strings.Contains(page, `commandfor="bulk-delete"`) {
		t.Error("a guest is offered Delete")
	}
}
