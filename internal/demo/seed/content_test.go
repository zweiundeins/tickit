package seed

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func load(t *testing.T) *Content {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// DM-02, DM-05: every reference resolves, every history ends in its issue's
// final state, every markup text parses, the limits hold.
func TestCheck(t *testing.T) {
	c := load(t)
	if err := c.Check(); err != nil {
		for _, e := range strings.Split(err.Error(), "\n") {
			t.Error(e)
		}
	}
}

// DM-02.1: three projects of 40 to 60 issues across every type, status and
// priority, with epics and children, subtasks, links, labels, overdue due
// dates and comments.
func TestIssueCounts(t *testing.T) {
	c := load(t)
	if len(c.Projects) != 3 {
		t.Fatalf("%d projects", len(c.Projects))
	}
	crossLinks := 0
	for _, p := range c.Projects {
		var issues []Issue
		for _, is := range c.Issues {
			if is.Project == p.Key {
				issues = append(issues, is)
			}
		}
		if n := len(issues); n < 40 || n > 60 {
			t.Errorf("%s: %d issues", p.Key, n)
		}
		types, statuses, priorities := map[string]int{}, map[string]int{}, map[string]int{}
		var children, subtasks, links, labelled, overdue, comments, points, estimates, unassigned, epics int
		parents := map[string]bool{}
		for _, is := range issues {
			types[is.Type]++
			statuses[is.Status]++
			priorities[is.Priority]++
			if is.Parent != "" {
				parents[is.Parent] = true
				if is.Type == "Subtask" {
					subtasks++
				} else {
					children++
				}
			}
			links += len(is.Links)
			for _, l := range is.Links {
				if !strings.HasPrefix(l.Key, p.Key+"-") {
					crossLinks++
				}
			}
			if len(is.Labels) > 0 {
				labelled++
			}
			if is.Due != nil && *is.Due < 0 && is.Status != "Done" {
				overdue++
			}
			if is.Type == "Story" && is.Points != nil {
				points++
			}
			if is.Estimate != "" {
				estimates++
			}
			if is.Assignee == "" {
				unassigned++
			}
			if len(is.Comments) > 6 {
				t.Errorf("%s: %d comments", is.Key, len(is.Comments))
			}
			comments += len(is.Comments)
		}
		for _, list := range []struct {
			name string
			want []string
			got  map[string]int
		}{{"type", Types, types}, {"status", Statuses, statuses}, {"priority", Priorities, priorities}} {
			for _, w := range list.want {
				if list.got[w] == 0 {
					t.Errorf("%s: no issue with %s %s", p.Key, list.name, w)
				}
			}
		}
		for _, is := range issues {
			if is.Type == "Epic" && parents[is.Key] {
				epics++
			}
		}
		if types["Epic"] < 3 || types["Epic"] > 5 || epics < 3 {
			t.Errorf("%s: %d epics, %d of them with children", p.Key, types["Epic"], epics)
		}
		for name, n := range map[string]int{"children of epics": children, "subtasks": subtasks, "links": links, "labelled": labelled, "overdue": overdue, "comments": comments, "story points": points, "estimates": estimates, "unassigned": unassigned} {
			if n == 0 {
				t.Errorf("%s: no %s", p.Key, name)
			}
		}
		t.Logf("%s: %d issues; types %v; statuses %v; priorities %v; %d comments, %d links, %d overdue", p.Key, len(issues), types, statuses, priorities, comments, links, overdue)
	}
	if crossLinks == 0 {
		t.Error("no link across projects")
	}
}

// DM-02.2: two spaces of 15 to 25 pages, three levels deep, with tables,
// code, library images, chips, page links, comments, and five pages with
// three or more versions.
func TestPageCounts(t *testing.T) {
	c := load(t)
	if len(c.Spaces) != 2 {
		t.Fatalf("%d spaces", len(c.Spaces))
	}
	versioned := 0
	for _, s := range c.Spaces {
		pages, depth := 0, 0
		var all strings.Builder
		for i := range c.Pages {
			p := &c.Pages[i]
			if p.Space != s.Key {
				continue
			}
			pages++
			depth = max(depth, c.Depth(p))
			if len(p.Versions) >= 3 {
				versioned++
			}
			all.WriteString(p.Body + "\n")
		}
		if pages < 15 || pages > 25 || depth != 3 {
			t.Errorf("%s: %d pages, %d deep", s.Key, pages, depth)
		}
		body := all.String()
		for what, re := range map[string]string{"table": `(?m)^\| --- `, "code": "(?m)^```[a-z]+$", "image": `\(lib:`, "chip": `\[\[[A-Z]+-[0-9]+\]\]`, "page link": `\[\[page:`, "mention": `@s-`, "task": `(?m)^- \[[ x]\] `} {
			if !regexp.MustCompile(re).MatchString(body) {
				t.Errorf("%s: no %s in any page", s.Key, what)
			}
		}
		comments := 0
		for _, pc := range c.PageComments {
			if pc.Space == s.Key {
				comments += 1 + len(pc.Replies)
			}
		}
		if comments == 0 {
			t.Errorf("%s: no page comments", s.Key)
		}
		t.Logf("%s: %d pages, %d page comments and replies", s.Key, pages, comments)
	}
	if versioned < 5 {
		t.Errorf("%d pages with three or more versions, want 5", versioned)
	}
	langs := map[string]bool{}
	for _, p := range c.Pages {
		for _, m := range regexp.MustCompile("(?m)^ *```([a-z]+)$").FindAllStringSubmatch(p.Body, -1) {
			langs[m[1]] = true
		}
	}
	for _, l := range []string{"bash", "sql", "go", "yaml"} {
		if !langs[l] {
			t.Errorf("no %s code block in the runbooks", l)
		}
	}
}

// DM-02.3, FR-84, FR-92: six people, about 40 images, five templates.
func TestPeopleImagesTemplates(t *testing.T) {
	c := load(t)
	if len(c.People) != 6 {
		t.Errorf("%d people", len(c.People))
	}
	if n := len(c.Images); n < 35 || n > 45 {
		t.Errorf("%d images", n)
	}
	kinds := map[string]int{}
	total := 0
	for _, im := range c.Images {
		kinds[im.Kind]++
		b, err := os.ReadFile(filepath.Join("..", "..", "web", "static", "img", "lib", im.ID+".svg"))
		if err != nil {
			t.Error(err)
			continue
		}
		total += len(b)
		if !bytes.Contains(b, []byte(`width="`)) || !utf8.Valid(b) {
			t.Errorf("%s: not an SVG with a size", im.ID)
		}
	}
	for _, k := range ImageKinds {
		if kinds[k] < 5 {
			t.Errorf("%d images of kind %s", kinds[k], k)
		}
	}
	if total > 1500*1024 {
		t.Errorf("the library takes %d bytes", total)
	}
	var names []string
	for _, tp := range c.Templates {
		names = append(names, tp.Name)
	}
	slices.Sort(names)
	if want := []string{"Decision log", "How-to", "Meeting notes", "Product requirements", "Retrospective"}; !slices.Equal(names, want) {
		t.Errorf("templates %v, want %v", names, want)
	}
}

// Every document the seed produces fits internal/doc/schema.json.
func TestDocumentsFitTheSchema(t *testing.T) {
	c := load(t)
	v := loadSchema(t)
	n := 0
	check := func(what, markup, space string) {
		doc, err := Parse(markup, c.Resolver(space, nil))
		if err != nil {
			t.Errorf("%s: %v", what, err)
			return
		}
		if err := v.validate(doc); err != nil {
			t.Errorf("%s: %v", what, err)
		}
		n++
	}
	for _, is := range c.Issues {
		check(is.Key, string(is.Description), "")
		for k, cm := range is.Comments {
			check(is.Key+" comment "+string(rune('1'+k)), string(cm.Body), "")
		}
	}
	for _, p := range c.Pages {
		for _, ver := range p.Versions {
			check(p.File, ver.Body, p.Space)
		}
	}
	for _, pc := range c.PageComments {
		check("comment on "+pc.Page, string(pc.Body), pc.Space)
		for _, r := range pc.Replies {
			check("reply on "+pc.Page, string(r.Body), pc.Space)
		}
	}
	for _, tp := range c.Templates {
		check(tp.Name, tp.Body, "")
	}
	t.Logf("%d documents", n)
}

// DM-02.4: the same content and the same documents on every run, with
// times relative to the run.
func TestDeterministic(t *testing.T) {
	a, b := load(t), load(t)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if !bytes.Equal(ja, jb) {
		t.Fatal("two loads differ")
	}
	for _, p := range a.Pages {
		d1, _ := Parse(p.Body, a.Resolver(p.Space, nil))
		d2, _ := Parse(p.Body, b.Resolver(p.Space, nil))
		if !bytes.Equal(d1, d2) {
			t.Fatalf("%s converts differently", p.File)
		}
	}
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skip("no time zone data")
	}
	run := time.Date(2026, 3, 29, 4, 0, 0, 0, zurich) // the night the clocks change
	if got := (When{Days: -1, Minutes: 9*60 + 40}).Time(run); got.Day() != 28 || got.Hour() != 9 || got.Minute() != 40 {
		t.Errorf("-1d 09:40 is %v", got)
	}
	for _, is := range a.Issues {
		if !is.Created.Time(run).Before(run) {
			t.Errorf("%s is created after the run", is.Key)
		}
	}
}

func TestWhenAndEstimate(t *testing.T) {
	for _, s := range []string{"-12d 09:40", "-1d 00:00", "3d 23:59"} {
		w, err := ParseWhen(s)
		if err != nil || w.String() != s {
			t.Errorf("ParseWhen(%q) = %v, %v", s, w, err)
		}
	}
	for _, s := range []string{"-12d 9:40", "-12d 24:00", "12 09:40", ""} {
		if _, err := ParseWhen(s); err == nil {
			t.Errorf("ParseWhen(%q) accepted it", s)
		}
	}
	for s, want := range map[Estimate]int{"": 0, "45m": 45, "6h": 360, "1h 30m": 90} {
		if got, err := s.Minutes(); err != nil || got != want {
			t.Errorf("%q.Minutes() = %d, %v", s, got, err)
		}
	}
	for _, s := range []Estimate{"1d", "90", "1h 0m", "h"} {
		if _, err := s.Minutes(); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

// Check finds what it is meant to find.
func TestCheckFindsProblems(t *testing.T) {
	for name, spoil := range map[string]func(c *Content){
		"history":   func(c *Content) { c.Issue("WEB-6").Status = "Done" },
		"chip":      func(c *Content) { c.Issue("WEB-1").Description += "\n\nSee [[WEB-999]]." },
		"parent":    func(c *Content) { c.Issue("APP-10").Parent = "APP-1" },
		"page link": func(c *Content) { c.Pages[0].Versions[0].Body = "[[page:Nowhere]]" },
		"mention":   func(c *Content) { c.Issue("OPS-1").Comments[0].Body = "@s-nobody" },
		"order":     func(c *Content) { c.Issue("OPS-2").Created = When{Days: -1} },
	} {
		c := load(t)
		spoil(c)
		if err := c.Check(); err == nil {
			t.Errorf("a spoiled %s passed the check", name)
		}
	}
}
