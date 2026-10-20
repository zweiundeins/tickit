package web_test

import (
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/internal/web"
	"github.com/mbolli/tickit/internal/web/webtest"
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
}
