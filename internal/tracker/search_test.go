package tracker

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
)

// searchEnv has WEB and APP (open), CLO (closed, Sunny may not read it)
// and ARC (archived), and these issues:
//
//	WEB-1 story  "Fix the login"          todo     high     Sunny   reporter Rainy  labels ux       component Search  due 2026-10-20
//	WEB-2 bug    "Cart 100% broken"       inreview critical Rainy   reporter Sunny  labels checkout                   due 2026-11-02
//	WEB-3 task   "under_score and Über"   done     low      -       reporter Sunny                   sprint S1
//	WEB-4 task   "Comment holds the word" backlog  medium   Ada     reporter Sunny  comment "zebra"
//	APP-1 task   "WEB-1 mentioned here"   todo     medium   -       reporter Rainy  description "unicorn"
//	CLO-1 task   "Secret"                 todo     medium
//	ARC-1 task   "Old"                    todo     medium
type searchEnv struct {
	*env
	web, app, clo, arc string
	sprint             string
	ids                map[string]string // by key
}

func newSearch(t *testing.T) *searchEnv {
	e := &searchEnv{env: newEnv(t), ids: map[string]string{}}
	e.web, e.app = e.project("WEB", true), e.project("APP", true)
	e.clo, e.arc = e.project("CLO", false), e.project("ARC", true)
	e.must(DefineComponent{By: e.by(admin), Project: e.web, Name: "Search"})
	e.sprint = e.result(CreateSprint{By: e.by(sunny), Project: e.web, Name: "S1"})["id"]
	add := func(key string, p people.Person, c CreateIssue) {
		id := e.issue(p, c)
		e.ids[key] = id
	}
	// Created a day apart, from 11 October.
	day := func(n int) { e.now = time.Date(2026, 10, 9+n, 9, 0, 0, 0, time.UTC) }
	day(2)
	add("WEB-1", rainy, CreateIssue{Project: e.web, Type: Story, Summary: "Fix the login", Priority: High, Assignee: rainy.ID,
		Labels: []string{"ux"}, Components: []string{"Search"}, Due: "2026-10-20"})
	day(3)
	add("WEB-2", sunny, CreateIssue{Project: e.web, Type: Bug, Summary: "Cart 100% broken", Status: StatusInReview, Priority: Critical,
		Assignee: rainy.ID, Labels: []string{"checkout"}, Due: "2026-11-02"})
	day(4)
	add("WEB-3", sunny, CreateIssue{Project: e.web, Summary: "under_score and Über", Status: StatusDone, Priority: Low, Sprint: e.sprint})
	day(5)
	add("WEB-4", sunny, CreateIssue{Project: e.web, Summary: "Comment holds the word", Status: StatusBacklog, Assignee: "s-priya"})
	e.must(DeleteIssue{By: e.by(admin), Issue: e.issue(sunny, CreateIssue{Project: e.web, Summary: "gone"})})
	e.result(AddComment{By: e.by(rainy), Issue: e.ids["WEB-4"], Body: PlainDoc("A ZEBRA crossed")})
	day(6)
	add("APP-1", rainy, CreateIssue{Project: e.app, Summary: "WEB-1 mentioned here", Description: PlainDoc("A Unicorn")})
	add("CLO-1", member, CreateIssue{Project: e.clo, Summary: "Secret"})
	add("ARC-1", admin, CreateIssue{Project: e.arc, Summary: "Old"})
	e.must(ArchiveProject{By: e.by(admin), Project: e.arc, Archived: true})
	// Sunny takes WEB-1 over.
	e.must(Assign{By: e.by(rainy), Issue: e.ids["WEB-1"], Seen: 1 << 40, Assignee: sunny.ID})
	return e
}

func (e *searchEnv) keys(t *testing.T, f Filter, s Sort) []string {
	t.Helper()
	readable, err := Readable(ctx, e.DB.R, sunny)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Search(ctx, e.DB.R, Query{Readable: readable, Me: sunny.ID, Filter: f, Sort: s, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, i := range res.Issues {
		keys = append(keys, i.Key)
	}
	return keys
}

// FR-51, FR-52: every filter; AND across filters, OR within one.
func TestSearchFilters(t *testing.T) {
	e := newSearch(t)
	byKey := Sort{By: SortKey}
	for _, c := range []struct {
		name string
		f    Filter
		want string
	}{
		{"nothing: readable, not archived", Filter{}, "APP-1 WEB-1 WEB-2 WEB-3 WEB-4"},
		{"project", Filter{Projects: []string{"web"}}, "WEB-1 WEB-2 WEB-3 WEB-4"},
		{"an archived project by name", Filter{Projects: []string{"ARC"}}, "ARC-1"}, // FR-04.2
		{"a closed project", Filter{Projects: []string{"CLO"}}, ""},
		{"type", Filter{Types: []IssueType{Bug, Story}}, "WEB-1 WEB-2"},
		{"status", Filter{Statuses: []string{StatusInReview, StatusBacklog}}, "WEB-2 WEB-4"},
		{"category", Filter{Categories: []Category{Done}}, "WEB-3"},
		{"priority", Filter{Priorities: []Priority{Critical, Low}}, "WEB-2 WEB-3"},
		{"assignee me", Filter{Assignees: []string{Me}}, "WEB-1"},
		{"assignee unassigned", Filter{Assignees: []string{Unassigned}}, "APP-1 WEB-3"},
		{"assignee ids", Filter{Assignees: []string{rainy.ID, "s-priya"}}, "WEB-2 WEB-4"},
		{"reporter", Filter{Reporters: []string{Me}}, "WEB-2 WEB-3 WEB-4"},
		{"label", Filter{Labels: []string{"UX", "checkout"}}, "WEB-1 WEB-2"},
		{"component", Filter{Components: []string{"Search"}}, "WEB-1"},
		{"sprint", Filter{Sprints: []string{e.sprint}}, "WEB-3"},
		{"no sprint", Filter{Sprints: []string{NoSprint}, Projects: []string{"WEB"}}, "WEB-1 WEB-2 WEB-4"},
		{"due range", Filter{DueFrom: "2026-10-20", DueTo: "2026-10-31"}, "WEB-1"},
		{"due until", Filter{DueTo: "2026-11-02"}, "WEB-1 WEB-2"},
		{"created range", Filter{CreatedFrom: "2026-10-12", CreatedTo: "2026-10-13"}, "WEB-2 WEB-3"},
		{"AND across", Filter{Types: []IssueType{Task}, Projects: []string{"WEB"}, Priorities: []Priority{Medium}}, "WEB-4"},
		{"text in the summary", Filter{Text: "LOGIN"}, "WEB-1"},
		{"text in a key", Filter{Text: "app-"}, "APP-1"},
		{"text in the description", Filter{Text: "unicorn"}, "APP-1"},
		{"text in a comment", Filter{Text: "zebra"}, "WEB-4"},
		{"percent is literal", Filter{Text: "100%"}, "WEB-2"},
		{"percent matches no wildcard", Filter{Text: "%"}, "WEB-2"},
		{"underscore is literal", Filter{Text: "r_s"}, "WEB-3"},
		{"underscore matches no wildcard", Filter{Text: "a_d"}, ""},
		{"not ASCII, without regard to case", Filter{Text: "über"}, "WEB-3"},
		{"unknown values filter nothing", Filter{Types: []IssueType{"feature"}, Priorities: []Priority{"urgent"}}, "APP-1 WEB-1 WEB-2 WEB-3 WEB-4"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(e.keys(t, c.f, byKey), " "); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
	// FR-52.1: an exact key comes first, an old key too.
	if got := e.keys(t, Filter{Text: "web-1"}, byKey); !slices.Equal(got, []string{"WEB-1", "APP-1"}) {
		t.Errorf("web-1: %v", got)
	}
	if got := e.keys(t, Filter{Text: "web-1"}, Sort{By: SortKey, Desc: true}); !slices.Equal(got, []string{"WEB-1", "APP-1"}) {
		t.Errorf("web-1 by key down: %v", got)
	}
	// A deleted issue is in no list.
	if got := e.keys(t, Filter{Text: "gone"}, byKey); len(got) != 0 {
		t.Errorf("deleted: %v", got)
	}
}

// FR-53: every column sorts, priority by severity, status by workflow,
// people by name, ties by key; the default is created, newest first.
func TestSearchSorts(t *testing.T) {
	e := newSearch(t)
	web := Filter{Projects: []string{"WEB"}}
	for _, c := range []struct {
		s    Sort
		want string
	}{
		{DefaultSort, "WEB-4 WEB-3 WEB-2 WEB-1"},
		{Sort{By: SortCreated}, "WEB-1 WEB-2 WEB-3 WEB-4"},
		{Sort{By: SortPriority}, "WEB-3 WEB-4 WEB-1 WEB-2"},
		{Sort{By: SortPriority, Desc: true}, "WEB-2 WEB-1 WEB-4 WEB-3"},
		{Sort{By: SortStatus}, "WEB-4 WEB-1 WEB-2 WEB-3"}, // Backlog, To Do, In Review, Done
		{Sort{By: SortType}, "WEB-1 WEB-3 WEB-4 WEB-2"},   // story, task, task, bug; ties by key
		{Sort{By: SortSummary}, "WEB-2 WEB-4 WEB-1 WEB-3"},
		{Sort{By: SortAssignee}, "WEB-4 WEB-1 WEB-2 WEB-3"}, // by name, unassigned last
		{Sort{By: SortDue}, "WEB-1 WEB-2 WEB-3 WEB-4"},
		{Sort{By: SortDue, Desc: true}, "WEB-2 WEB-1 WEB-3 WEB-4"}, // none last both ways
		{Sort{By: SortKey, Desc: true}, "WEB-4 WEB-3 WEB-2 WEB-1"},
		{Sort{By: SortRank}, "WEB-1 WEB-2 WEB-3 WEB-4"},
		{Sort{By: "colour"}, "WEB-4 WEB-3 WEB-2 WEB-1"},
	} {
		want := c.want
		if c.s.By == SortAssignee {
			byName := []struct{ key, name string }{{"WEB-4", "Priya Raman"}, {"WEB-1", sunny.Name}, {"WEB-2", rainy.Name}}
			slices.SortStableFunc(byName, func(a, b struct{ key, name string }) int {
				return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
			})
			want = byName[0].key + " " + byName[1].key + " " + byName[2].key + " WEB-3"
		}
		if got := strings.Join(e.keys(t, web, c.s), " "); got != want {
			t.Errorf("%+v: %q, want %q", c.s, got, want)
		}
	}
	for by := range sortColumns {
		for _, desc := range []bool{false, true} {
			if got := e.keys(t, Filter{}, Sort{By: by, Desc: desc}); len(got) != 5 {
				t.Errorf("sorted by %s: %v", by, got)
			}
		}
	}
	if got := strings.Join(e.keys(t, Filter{}, Sort{By: SortProject}), " "); got != "APP-1 WEB-1 WEB-2 WEB-3 WEB-4" {
		t.Errorf("by project: %s", got)
	}
}

// FR-54: 25 a page, the total and the page count; a page past the end
// is the last one.
func TestSearchPages(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	for i := range 60 {
		e.issue(sunny, CreateIssue{Project: p, Summary: fmt.Sprint("Issue ", i+1)})
	}
	readable := []string{p}
	for _, c := range []struct{ page, want, first, n int }{{0, 1, 60, 25}, {1, 1, 60, 25}, {2, 2, 35, 25}, {3, 3, 10, 10}, {9, 3, 10, 10}} {
		res, err := Search(ctx, e.DB.R, Query{Readable: readable, Page: c.page})
		if err != nil {
			t.Fatal(err)
		}
		if res.Total != 60 || res.Pages != 3 || res.Page != c.want || len(res.Issues) != c.n || res.Issues[0].Number != int64(c.first) {
			t.Errorf("page %d: page %d of %d, %d issues from %d", c.page, res.Page, res.Pages, len(res.Issues), res.Issues[0].Number)
		}
	}
	res, _ := Search(ctx, e.DB.R, Query{Readable: readable, Filter: Filter{Text: "nothing"}, Page: 4})
	if res.Total != 0 || res.Pages != 1 || res.Page != 1 || len(res.Issues) != 0 {
		t.Errorf("no match: %+v", res)
	}
	res, _ = Search(ctx, e.DB.R, Query{Readable: nil})
	if res.Total != 0 {
		t.Errorf("nothing readable: %d", res.Total)
	}
}

// FR-55, MP-11.1: a filter's URL reads back as the filter, and filters
// that select the same issues for the same person share a key.
func TestFilterValues(t *testing.T) {
	f := Filter{Projects: []string{"app", "WEB", "WEB"}, Types: []IssueType{"bug", "Story"}, Statuses: []string{"inreview"},
		Categories: []Category{Done}, Priorities: []Priority{High, "urgent"}, Assignees: []string{Unassigned, Me, "nobody"},
		Reporters: []string{"s-priya"}, Labels: []string{"UX"}, Components: []string{" Search "}, Sprints: []string{NoSprint},
		DueFrom: "2026-10-01", DueTo: "2026-13-01", CreatedFrom: "2026-10-01", CreatedTo: "2026-10-31", Text: "  100%  done "}
	n := f.Normalize("")
	want := Filter{Projects: []string{"APP", "WEB"}, Types: []IssueType{Bug, Story}, Statuses: []string{"inreview"},
		Categories: []Category{Done}, Priorities: []Priority{High}, Assignees: []string{Me, Unassigned},
		Reporters: []string{"s-priya"}, Labels: []string{"ux"}, Components: []string{"Search"}, Sprints: []string{NoSprint},
		DueFrom: "2026-10-01", CreatedFrom: "2026-10-01", CreatedTo: "2026-10-31", Text: "100% done"}
	if fmt.Sprint(n) != fmt.Sprint(want) {
		t.Errorf("normalized:\n%+v\nwant\n%+v", n, want)
	}
	v := n.Values()
	if got := ParseFilter(v); fmt.Sprint(got) != fmt.Sprint(n) {
		t.Errorf("read back: %+v", got)
	}
	u, _ := url.ParseQuery(v.Encode())
	if ParseFilter(u).Key() != n.Key() {
		t.Error("the key changed through the URL")
	}
	shuffled := Filter{Projects: []string{"WEB", "APP"}, Types: []IssueType{Story, Bug}, Statuses: []string{"inreview"}, Categories: []Category{Done},
		Priorities: []Priority{High}, Assignees: []string{Unassigned, Me}, Reporters: []string{"s-priya"}, Labels: []string{"ux"},
		Components: []string{"Search"}, Sprints: []string{NoSprint}, DueFrom: "2026-10-01", CreatedFrom: "2026-10-01",
		CreatedTo: "2026-10-31", Text: "100% done"}
	if shuffled.Normalize("").Key() != n.Key() || (Filter{Text: "x"}).Key() == n.Key() {
		t.Error("keys")
	}
	if got := n.Normalize(sunny.ID).Assignees; !slices.Equal(got, []string{sunny.ID, Unassigned}) {
		t.Errorf("me: %v", got)
	}
	if !(Filter{}).IsZero() || n.IsZero() {
		t.Error("IsZero")
	}
	s := Sort{By: SortPriority, Desc: true}
	if got := ParseSort(s.Values()); got != s || ParseSort(url.Values{}) != DefaultSort || len(DefaultSort.Values()) != 0 {
		t.Errorf("sort: %+v", got)
	}
	if ParsePage(url.Values{"page": {"3"}}) != 3 || ParsePage(url.Values{"page": {"-1"}}) != 1 || ParsePage(nil) != 1 {
		t.Error("page")
	}
	q := Query{Readable: []string{"b", "a"}, Me: sunny.ID, Filter: Filter{Assignees: []string{Me}}, Page: 2}
	same := Query{Readable: []string{"a", "b"}, Me: sunny.ID, Filter: Filter{Assignees: []string{sunny.ID}}, Sort: DefaultSort, Page: 2}
	if q.Key() != same.Key() {
		t.Error("equal queries have different keys")
	}
	for _, other := range []Query{{Readable: []string{"a"}, Me: sunny.ID, Filter: q.Filter, Page: 2}, {Readable: q.Readable, Me: rainy.ID, Filter: q.Filter, Page: 2},
		{Readable: q.Readable, Me: sunny.ID, Filter: q.Filter, Page: 3}, {Readable: q.Readable, Me: sunny.ID, Filter: q.Filter, Page: 2, Sort: Sort{By: SortKey}}} {
		if other.Key() == q.Key() {
			t.Errorf("%+v shares the key", other)
		}
	}
	if got := q.Values().Encode(); got != "assignee=me&page=2" {
		t.Errorf("the URL: %s", got)
	}
}

// FR-23.1: labels by prefix, created on first use per project.
func TestLabels(t *testing.T) {
	e := newSearch(t)
	e.must(SetLabels{By: e.by(sunny), Issue: e.ids["WEB-3"], Add: []string{"Checkout-v2", "copy"}})
	got, err := Labels(ctx, e.DB.R, e.web, "CH")
	if err != nil || !slices.Equal(got, []string{"checkout", "checkout-v2"}) {
		t.Errorf("labels: %v %v", got, err)
	}
	if got, _ := Labels(ctx, e.DB.R, e.app, ""); len(got) != 0 {
		t.Errorf("APP's labels: %v", got)
	}
	if got, _ := Labels(ctx, e.DB.R, e.web, "%"); len(got) != 0 {
		t.Errorf("a wildcard: %v", got)
	}
}

// FR-56: saved filters.
func TestSavedFilters(t *testing.T) {
	e := newEnv(t)
	f := Filter{Assignees: []string{Me}, Priorities: []Priority{High}}
	id := e.result(SaveFilter{By: e.by(sunny), Name: "Mine", Filter: f, Sort: Sort{By: SortPriority, Desc: true}})["id"]
	other := e.result(SaveFilter{By: e.by(rainy), Name: "All bugs", Filter: Filter{Types: []IssueType{Bug}}})["id"]
	e.must(StarFilter{By: e.by(rainy), Filter: id, Star: true})
	got, err := SavedFilters(ctx, e.DB.R, rainy.ID)
	if err != nil || len(got) != 2 || got[0].ID != id || !got[0].Starred || got[0].Stars != 1 || got[0].Sort.By != SortPriority ||
		!slices.Equal(got[0].Filter.Assignees, []string{Me}) || got[1].ID != other {
		t.Fatalf("filters: %+v %v", got, err)
	}
	if got := refusal(t, e.do(RenameFilter{By: e.by(rainy), Filter: id, Name: "Theirs"})); got != "Only its owner can change this filter." {
		t.Errorf("renaming another's: %q", got)
	}
	if got := refusal(t, e.do(RenameFilter{By: e.by(sunny), Filter: id, Name: ""})); got != "A filter's name has 1 to 60 characters." {
		t.Errorf("an empty name: %q", got)
	}
	e.must(RenameFilter{By: e.by(sunny), Filter: id, Name: "My high ones"})
	e.must(DeleteFilter{By: e.by(sunny), Filter: id})
	if got, _ := SavedFilters(ctx, e.DB.R, rainy.ID); len(got) != 1 || e.row(`SELECT count(*) FROM saved_filter_stars`) != "0" {
		t.Errorf("after deleting: %+v", got)
	}
}

// FR-13: the issue page's issues to link are the project's 25 changed
// last, as the list sorted by update shows them, ties by number.
func TestRecent(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	for i := range 30 {
		e.issue(sunny, CreateIssue{Project: p, Summary: fmt.Sprint("Issue ", i+1)})
	}
	got, err := Recent(ctx, e.DB.R, p, PageSize)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Search(ctx, e.DB.R, Query{Readable: []string{p}, Filter: Filter{Projects: []string{"WEB"}}, Sort: Sort{By: SortUpdated, Desc: true}, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != PageSize || len(got) != len(res.Issues) {
		t.Fatalf("%d issues, the list %d", len(got), len(res.Issues))
	}
	for i := range got {
		if got[i].ID != res.Issues[i].ID {
			t.Errorf("issue %d: %s, the list %s", i, got[i].Key, res.Issues[i].Key)
		}
	}
}
