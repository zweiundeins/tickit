package web_test

import (
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/dashboards"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// The ids of the probes' data.
const (
	probeWeb   = "01920000-0000-7000-8000-0000000000b1"
	probeApp   = "01920000-0000-7000-8000-0000000000b2"
	probeGone  = "01920000-0000-7000-8000-0000000000a2"
	probeMoved = "01920000-0000-7000-8000-0000000000a3"
)

// seedProbes writes what the issue routes' probes name: the open
// projects WEB and APP, WEB-1 (web.ProbeIssue), WEB-2 deleted and WEB-3
// moved to APP-1, as seeded Ada.
func seedProbes(t *testing.T, e *webtest.Env) {
	t.Helper()
	ada := people.Seeded[0]
	at := time.Now().Add(-time.Hour)
	n := 0
	by := func() tabs.By {
		n++
		return tabs.By{Actor: ada, Op: "probe" + string(rune('a'+n)), At: at}
	}
	e.Do(t, people.SeedPeople{At: at})
	e.Do(t, tracker.CreateProject{By: by(), ID: probeWeb, Key: "WEB", Name: "Website", Open: true})
	e.Do(t, tracker.CreateProject{By: by(), ID: probeApp, Key: "APP", Name: "App", Open: true})
	for _, id := range []string{web.ProbeIssue, probeGone, probeMoved} {
		e.Do(t, tracker.CreateIssue{By: by(), ID: id, Project: probeWeb, Type: tracker.Task, Summary: "Probe " + id[len(id)-2:],
			Description: tracker.PlainDoc("A description with **markup**.")})
	}
	e.Do(t, tracker.AddComment{By: by(), Issue: web.ProbeIssue, Body: tracker.PlainDoc("A comment.")})
	e.Do(t, tracker.DeleteIssue{By: by(), Issue: probeGone})
	e.Do(t, tracker.MoveToProject{By: by(), Issue: probeMoved, Project: probeApp})
	// A dashboard with a gadget of every kind (FR-60).
	e.Do(t, tracker.SaveFilter{By: by(), ID: "01920000-0000-7000-8000-0000000000f1", Name: "Mine", Filter: tracker.Filter{Assignees: []string{tracker.Me}}})
	e.Do(t, dashboards.Create{By: by(), ID: web.ProbeDashboard, Name: "Probe"})
	for _, g := range []dashboards.AddGadget{{Kind: dashboards.Filter, SavedFilter: "01920000-0000-7000-8000-0000000000f1"}, {Kind: dashboards.Status, Project: probeWeb},
		{Kind: dashboards.Assigned}, {Kind: dashboards.Activity}, {Kind: dashboards.Created, Project: probeWeb}} {
		g.By, g.Dashboard = by(), web.ProbeDashboard
		e.Do(t, g)
	}
}

func byOf(p people.Person, op string) tabs.By {
	return tabs.By{Actor: p, Op: op, At: time.Now().Add(-time.Minute)}
}
