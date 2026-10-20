package web_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// FR-38: the list shows when an issue was resolved, sorts by it and
// filters by the day it was resolved; an open issue matches no such day.
func TestListResolved(t *testing.T) {
	e := newList(t)
	c := webtest.Client(t)
	today := time.Now().UTC().Format(time.DateOnly)
	if got := e.list(c, "resolved_from="+today+"&resolved_to="+today); !slices.Equal(got, []string{"WEB-3"}) {
		t.Errorf("resolved today: %v, want WEB-3", got)
	}
	if got := e.list(c, "resolved_to=2000-01-01"); len(got) != 0 {
		t.Errorf("resolved before 2000: %v, want none", got)
	}
	if got := e.list(c, "sort=resolved&dir=asc"); len(got) != 4 || got[0] != "WEB-3" {
		t.Errorf("by resolved: %v, the resolved issue first and the open ones last", got)
	}
	_, page := webtest.Get(t, c, e.URL+"/issues?project=WEB")
	for _, want := range []string{">Resolved<", `name="resolved_from"`, `name="resolved_to"`, "Not resolved"} {
		if !strings.Contains(page, want) {
			t.Errorf("the list has no %q", want)
		}
	}
}

// FR-23.2, FR-25.2: labels in the list and on cards link to the list of
// their issues, and an overdue issue says "Overdue" in text beside its
// icon, not only in colour.
func TestLabelsAndOverdue(t *testing.T) {
	e := newList(t)
	e.issue("WEB-4", tracker.CreateIssue{Project: e.web, Summary: "Late & loud", Due: "2020-01-02", Labels: []string{"a&b"}})
	c := webtest.Client(t)
	_, list := webtest.Get(t, c, e.URL+"/issues?project=WEB")
	_, board := webtest.Get(t, c, e.URL+"/projects/WEB/board")
	for name, page := range map[string]string{"list": list, "board": board} {
		for _, want := range []string{
			`class="tk-label" data-hue="`, `href="/issues?label=ux"`, `href="/issues?label=a%26b"`,
			`<b>Overdue</b>`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: no %q", name, want)
			}
		}
	}
	if code, _ := webtest.Get(t, c, e.URL+"/issues?label=a%26b"); code != http.StatusOK {
		t.Errorf("the label's list: %d", code)
	}
	if got := e.list(c, "label=a%26b"); !slices.Equal(got, []string{"WEB-4"}) {
		t.Errorf("label a&b: %v", got)
	}
}
