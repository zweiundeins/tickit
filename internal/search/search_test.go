package search_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/dbtest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/search"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

var ctx = context.Background()

// A page stream as the wiki's hook would index it: a projector named
// search.PagesProjector that writes search.Tables(search.Page).
type pageEvent interface{ pageEvent() }

type pageSaved struct {
	Space, SpaceKey, Title, Body string
	Labels                       []string
}
type pageDeleted struct{}

func (pageSaved) pageEvent()   {}
func (pageDeleted) pageEvent() {}

var (
	pageStream  = events.NewStream[string, pageEvent]("page")
	pageSavedT  = events.Define(pageStream, "page.saved", func(e pageSaved) pageEvent { return e })
	pageDeleteT = events.Define(pageStream, "page.deleted", func(e pageDeleted) pageEvent { return e })
)

var pageSearch = events.NewProjector(pageStream, search.PagesProjector, search.PagesVersion,
	events.Handle(pageSavedT, func(tx *writer.Tx, r events.Record[string, pageSaved]) error {
		e := r.Data
		return search.Put(tx, search.Page, search.Doc{ID: r.ID, Scope: e.Space, Title: e.Title, Body: e.Body, Labels: e.Labels,
			Href: "/spaces/" + e.SpaceKey + "/pages/" + r.ID, Place: e.SpaceKey, People: []string{r.Actor}, Updated: r.At})
	}),
	events.Handle(pageDeleteT, func(tx *writer.Tx, r events.Record[string, pageDeleted]) error {
		return search.Remove(tx, search.Page, r.ID)
	}),
).Tables(search.Tables(search.Page)...)

var store = events.Must(events.New(append(tracker.Parts(), pageSearch)...))

func init() { tracker.Bind(store) }

type env struct {
	*dbtest.Env
	t        *testing.T
	n        int
	web, app string
	ids      map[string]string // issue ids by key, page ids by title
	ada, bo  people.Person
}

func (e *env) by(p people.Person) tabs.By {
	e.n++
	return tabs.By{Actor: p, Op: fmt.Sprint("op", e.n), At: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC).Add(time.Duration(e.n) * time.Minute)}
}

func (e *env) result(cmd writer.ResultCommand) string {
	e.t.Helper()
	b, err := e.W.DoValue(ctx, func(tx *writer.Tx) ([]byte, error) { return cmd.ApplyResult(tx) })
	if err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
	var m map[string]string
	json.Unmarshal(b, &m)
	return m["id"]
}

func (e *env) issue(key string, c tracker.CreateIssue) {
	e.t.Helper()
	c.By = e.by(e.ada)
	if c.Type == "" {
		c.Type = tracker.Task
	}
	e.ids[key] = e.result(c)
}

func (e *env) page(title, space, body string, labels ...string) string {
	e.t.Helper()
	id := tracker.NewID()
	e.Do(e.t, writer.Func(func(tx *writer.Tx) error {
		return events.Append(store, tx, pageStream.ID(id), events.NoStream, e.by(e.bo).Meta(),
			pageEvent(pageSaved{Space: space, SpaceKey: strings.ToUpper(space), Title: title, Body: body, Labels: labels}))
	}))
	e.ids[title] = id
	return id
}

// newEnv has the projects WEB and APP and the spaces eng and prod:
//
//	WEB-1 "Checkout flow broken"        body "The cart drops items."          label checkout, Ada reports, Bo assigned
//	WEB-2 "Login page"                  body "Users cannot log in at checkout" comment "zebra crossing"
//	WEB-3 "Release checklist review"    body "<b>bold</b> & zebra"
//	APP-1 "Search ranking"              body "Exact keys first, then titles"
//	page eng  "Release checklist"       body "Steps for a checkout release"   label release
//	page eng  "Checkout"                body "How checkout works"
//	page prod "Zebra handbook"          body "Stripes"
func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{Env: dbtest.Open(t), t: t, ids: map[string]string{}, ada: people.Seeded[0], bo: people.Seeded[1]}
	e.Do(t, people.SeedPeople{At: time.Now()})
	e.web = e.result(tracker.CreateProject{By: e.by(e.ada), Key: "WEB", Name: "Website", Open: true})
	e.app = e.result(tracker.CreateProject{By: e.by(e.ada), Key: "APP", Name: "App", Open: true})
	e.issue("WEB-1", tracker.CreateIssue{Project: e.web, Summary: "Checkout flow broken", Description: tracker.PlainDoc("The cart drops items."),
		Labels: []string{"checkout"}, Assignee: e.bo.ID})
	e.issue("WEB-2", tracker.CreateIssue{Project: e.web, Summary: "Login page", Description: tracker.PlainDoc("Users cannot log in at checkout")})
	e.issue("WEB-3", tracker.CreateIssue{Project: e.web, Summary: "Release checklist review", Description: tracker.PlainDoc("<b>bold</b> & zebra")})
	e.issue("APP-1", tracker.CreateIssue{Project: e.app, Summary: "Search ranking", Description: tracker.PlainDoc("Exact keys first, then titles")})
	e.ids["comment"] = e.result(tracker.AddComment{By: e.by(e.bo), Issue: e.ids["WEB-2"], Body: tracker.PlainDoc("zebra crossing")})
	e.page("Release checklist", "eng", "Steps for a checkout release", "release")
	e.page("Checkout", "eng", "How checkout works")
	e.page("Zebra handbook", "prod", "Stripes")
	return e
}

func (e *env) query(text string, mod ...func(*search.Query)) search.Results {
	e.t.Helper()
	q := search.Query{Text: text, Projects: []string{e.web, e.app}, Spaces: []string{"eng", "prod"}}
	for _, m := range mod {
		m(&q)
	}
	var res search.Results
	e.View(e.t, func(tx *sql.Tx) error {
		var err error
		res, err = search.Search(ctx, tx, q)
		return err
	})
	return res
}

// names are the hits as their keys or titles.
func names(res search.Results) []string {
	var out []string
	for _, h := range res.Hits {
		if h.Kind == search.Issue {
			out = append(out, h.Key)
		} else {
			out = append(out, h.Title)
		}
	}
	return out
}

// FR-57.3: an exact issue key or page title comes first, then title and
// key matches, then body and comment matches. FR-52.1: "web-2" puts WEB-2
// first.
func TestRanking(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct {
		q    string
		want []string
	}{
		{"checkout", []string{"Checkout", "WEB-1", "WEB-2", "Release checklist"}},
		{"web-2", []string{"WEB-2"}},
		{"WEB-1", []string{"WEB-1"}},
		{"release checklist", []string{"Release checklist", "WEB-3"}},
		{"zebra", []string{"Zebra handbook", "WEB-3", "WEB-2"}},
		{"zeb", []string{"Zebra handbook", "WEB-3", "WEB-2"}}, // the last word as a prefix while typing
		{"Checkout flow broken", []string{"WEB-1"}},
		{"  ", nil},
		{`"`, nil},
		{"nothing-here", nil},
	} {
		got := names(e.query(c.q))
		if !slices.Equal(got, c.want) {
			t.Errorf("%q: %v, want %v", c.q, got, c.want)
		}
	}
	res := e.query("web-2")
	if h := res.Hits[0]; !h.Exact || h.Href != "/WEB-2" {
		t.Errorf("web-2: %+v", h)
	}
	if res := e.query("zebra"); res.Hits[2].ViaComment == "" || !hit(res.Hits[2].Snippet, "zebra") {
		t.Errorf("WEB-2 is found by its comment, with the comment as its snippet: %+v", res.Hits[2])
	}
}

func hit(parts []search.Part, word string) bool {
	for _, p := range parts {
		if p.Hit && strings.EqualFold(p.Text, word) {
			return true
		}
	}
	return false
}

// FR-57.2: the snippet marks the matched words; its text stays as
// written for the page to escape.
func TestSnippet(t *testing.T) {
	e := newEnv(t)
	res := e.query("zebra", func(q *search.Query) { q.Kinds = []search.Kind{search.Issue}; q.People = []string{e.ada.ID} })
	if len(res.Hits) == 0 {
		t.Fatal("no hits")
	}
	var b strings.Builder
	for _, p := range res.Hits[0].Snippet {
		if p.Hit {
			b.WriteString("[" + p.Text + "]")
		} else {
			b.WriteString(p.Text)
		}
	}
	if got := b.String(); got != "<b>bold</b> & [zebra]" {
		t.Errorf("snippet %q", got)
	}
}

// FR-57.2: filters by kind, project, space, label and person; nothing of
// a scope the person may not read.
func TestFilters(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct {
		name string
		mod  func(*search.Query)
		want []string
	}{
		{"issues", func(q *search.Query) { q.Kinds = []search.Kind{search.Issue} }, []string{"WEB-1", "WEB-2"}},
		{"pages", func(q *search.Query) { q.Kinds = []search.Kind{search.Page} }, []string{"Checkout", "Release checklist"}},
		{"project", func(q *search.Query) { q.InProjects = []string{e.app} }, []string{"Checkout", "Release checklist"}},
		{"space", func(q *search.Query) { q.InSpaces = []string{"prod"} }, []string{"WEB-1", "WEB-2"}},
		{"label", func(q *search.Query) { q.Labels = []string{"checkout", "release"} }, []string{"WEB-1", "Release checklist"}},
		{"person", func(q *search.Query) { q.People = []string{e.bo.ID}; q.Kinds = []search.Kind{search.Issue} }, []string{"WEB-1"}},
		{"readable", func(q *search.Query) { q.Projects = []string{e.app}; q.Spaces = nil }, nil},
	} {
		if got := names(e.query("checkout", c.mod)); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	res := e.query("checkout", func(q *search.Query) { q.Kinds = []search.Kind{search.Page} })
	if res.Counts[search.Issue] != 2 || res.Counts[search.Page] != 2 || res.Total != 2 {
		t.Errorf("counts %v, total %d", res.Counts, res.Total)
	}
	if res := e.query("checkout", func(q *search.Query) { q.Limit, q.Offset = 1, 1 }); len(res.Hits) != 1 || res.Total != 4 || res.Hits[0].Key != "WEB-1" {
		t.Errorf("the second of 4: %+v", res)
	}
}

// MP-03.1: the index follows every change: summaries, descriptions,
// labels, moves, deletes, comments edited, hidden and deleted.
func TestFollowsChanges(t *testing.T) {
	e := newEnv(t)
	far := int64(1 << 40)
	e.Do(t, tracker.SetSummary{By: e.by(e.ada), Issue: e.ids["APP-1"], Seen: far, Summary: "Quokka ranking"})
	e.Do(t, tracker.SetDescription{By: e.by(e.ada), Issue: e.ids["WEB-1"], Description: tracker.PlainDoc("now about wombats")})
	if got := names(e.query("quokka")); !slices.Equal(got, []string{"APP-1"}) {
		t.Errorf("renamed: %v", got)
	}
	if got := names(e.query("wombats")); !slices.Equal(got, []string{"WEB-1"}) {
		t.Errorf("described: %v", got)
	}
	if got := names(e.query("cart")); len(got) != 0 {
		t.Errorf("the old description: %v", got)
	}
	e.Do(t, tracker.SetLabels{By: e.by(e.ada), Issue: e.ids["APP-1"], Add: []string{"perf"}})
	if got := names(e.query("quokka", func(q *search.Query) { q.Labels = []string{"perf"} })); !slices.Equal(got, []string{"APP-1"}) {
		t.Errorf("labelled: %v", got)
	}
	e.Do(t, tracker.MoveToProject{By: e.by(e.ada), Issue: e.ids["APP-1"], Project: e.web, Status: tracker.StatusToDo})
	if got := names(e.query("app-1")); !slices.Equal(got, []string{"WEB-4"}) {
		t.Errorf("the old key finds the moved issue: %v", got)
	}
	e.Do(t, tracker.HideComment{By: e.by(people.Person{ID: "a-x", Kind: people.Account, Role: people.Admin}), Comment: e.ids["comment"], Hidden: true})
	if got := names(e.query("crossing")); len(got) != 0 {
		t.Errorf("a hidden comment: %v", got)
	}
	e.Do(t, tracker.HideComment{By: e.by(people.Person{ID: "a-x", Kind: people.Account, Role: people.Admin}), Comment: e.ids["comment"], Hidden: false})
	if got := names(e.query("crossing")); !slices.Equal(got, []string{"WEB-2"}) {
		t.Errorf("shown again: %v", got)
	}
	e.Do(t, tracker.EditComment{By: e.by(e.bo), Comment: e.ids["comment"], Body: tracker.PlainDoc("giraffe")})
	if got := names(e.query("giraffe")); !slices.Equal(got, []string{"WEB-2"}) {
		t.Errorf("edited: %v", got)
	}
	e.Do(t, tracker.DeleteComment{By: e.by(e.bo), Comment: e.ids["comment"]})
	e.Do(t, tracker.DeleteIssue{By: e.by(e.ada), Issue: e.ids["WEB-3"]})
	if got := names(e.query("giraffe")); len(got) != 0 {
		t.Errorf("a deleted comment: %v", got)
	}
	if got := names(e.query("zebra")); !slices.Equal(got, []string{"Zebra handbook"}) {
		t.Errorf("a deleted issue: %v", got)
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		return events.Append(store, tx, pageStream.ID(e.ids["Zebra handbook"]), events.Any, e.by(e.bo).Meta(), pageEvent(pageDeleted{}))
	}))
	if got := names(e.query("zebra")); len(got) != 0 {
		t.Errorf("a deleted page: %v", got)
	}
}

// fixed are the queries whose results a rebuild keeps (NFR-44.1).
var fixed = []string{"checkout", "web-2", "zebra", "zeb", "release checklist", "stripes", "cart", "ranking"}

func (e *env) answers() string {
	var b strings.Builder
	for _, q := range fixed {
		res := e.query(q)
		fmt.Fprintf(&b, "%s: %d", q, res.Total)
		for _, h := range res.Hits {
			fmt.Fprintf(&b, " %s/%s/%s/%v", h.Kind, h.Key, h.Title, h.Snippet)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// NFR-44: a rebuild of the index's projectors, FTS5 tables included,
// gives the same results for a fixed set of queries; while it rebuilds,
// search answers from the old index and is never empty.
func TestRebuild(t *testing.T) {
	e := newEnv(t)
	for i := range 300 { // enough events for many batches
		e.issue(fmt.Sprint("WEB-", i+5), tracker.CreateIssue{Project: e.web, Summary: fmt.Sprint("Filler number ", i)})
	}
	before := e.answers()
	if !strings.Contains(before, "web-2: 2 issue/WEB-2/") {
		t.Fatalf("the test finds too little:\n%s", before)
	}
	// A new version of every search projector rebuilds its tables.
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE lith_projections SET version = version + 100 WHERE name LIKE 'search_%'`)
		return err
	}))
	// Readers ask while the runner rebuilds in batches of 10.
	var stop atomic.Bool
	var wg sync.WaitGroup
	var asked, wrong atomic.Int64
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				res := search.Results{}
				err := e.DB.View(ctx, func(tx *sql.Tx) error {
					var err error
					res, err = search.Search(ctx, tx, search.Query{Text: "filler", Projects: []string{e.web}, Limit: 1})
					return err
				})
				asked.Add(1)
				if err != nil || res.Total != 300 {
					wrong.Add(1)
				}
			}
		}()
	}
	r := events.NewRunner(store, e.DB, e.W, events.Options{Batch: 10})
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- r.Run(rctx) }()
	defer func() { cancel(); <-done }()
	rebuilt := false
	wait := 20 * time.Second
	if raceDetector {
		wait = 2 * time.Minute
	}
	for deadline := time.Now().Add(wait); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		ps, live, versions := r.Stats().Projectors, true, true
		for _, p := range ps {
			live = live && p.Status == "live"
			if strings.HasPrefix(p.Name, "search_") {
				versions = versions && p.Version < 100
			}
		}
		if len(ps) == len(tracker.Parts())+1 && live {
			var v int
			e.DB.R.QueryRow(`SELECT max(version) FROM lith_projections WHERE name LIKE 'search_%'`).Scan(&v)
			if v < 100 {
				rebuilt = true
				break
			}
		}
	}
	stop.Store(true)
	wg.Wait()
	if !rebuilt {
		t.Fatalf("not rebuilt: %+v", r.Stats().Projectors)
	}
	if n := wrong.Load(); n > 0 {
		t.Errorf("NFR-44.2: %d of %d searches during the rebuild answered wrong", n, asked.Load())
	}
	if after := e.answers(); after != before {
		t.Errorf("NFR-44.1: results differ after the rebuild:\n%s\nwere\n%s", after, before)
	}
	var leftovers int
	e.DB.R.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name LIKE 'search%' AND (instr(name, '__v') > 0 OR instr(name, '__retired') > 0 OR instr(name, '__hold') > 0)`).Scan(&leftovers)
	if leftovers != 0 {
		t.Errorf("%d tables of the rebuild left over", leftovers)
	}
	t.Logf("%d searches during the rebuild", asked.Load())
}
