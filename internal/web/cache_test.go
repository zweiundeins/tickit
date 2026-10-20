package web_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// The home page's projects and their open counts come from a cache: renders
// that ask at once share one read, and a commit that touches the project's
// board, as every change to one of its issues does, refreshes it.
func TestProjectsCache(t *testing.T) {
	e := webtest.Start(t)
	g := guest(t, e, webtest.Client(t))
	by := tabs.By{Actor: g, Op: "o", At: time.Now()}
	var project string
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		if err := people.Ensure(tx, g, time.Now()); err != nil {
			return err
		}
		b, err := tracker.CreateProject{By: by, Key: "WEB", Name: "Website", Open: true}.ApplyResult(tx)
		project = strings.Split(strings.Split(string(b), `"id":"`)[1], `"`)[0]
		return err
	}))
	open := func() int {
		list, err := web.HomeProjects(e.Server, context.Background(), e.DB.R, []string{project}, e.Hub.Seq())
		if err != nil || len(list) != 1 {
			t.Fatalf("%v, %v", list, err)
		}
		return list[0].OpenIssues
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if n := open(); n != 0 {
				t.Errorf("%d open issues, want 0", n)
			}
		})
	}
	wg.Wait()
	e.Do(t, tracker.CreateIssue{By: by, Project: project, Type: tracker.Task, Summary: "One"})
	if n := open(); n != 1 {
		t.Errorf("after an issue: %d open issues, want 1", n)
	}
}
