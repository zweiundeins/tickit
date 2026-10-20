package seed

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Limits of DM-05 that the seed keeps.
const (
	MaxSummary     = 255
	MaxDescription = 20000
	MaxComment     = 10000
	MaxLabel       = 50
	MaxName        = 30
	MaxMentions    = 10
)

var (
	personIDRe   = regexp.MustCompile(`^s-[a-z]+$`)
	projectKeyRe = regexp.MustCompile(`^[A-Z]{2,6}$`)
	spaceKeyRe   = regexp.MustCompile(`^[A-Z]{2,10}$`)
	labelRe      = regexp.MustCompile(`^[^\s]{1,50}$`)
	imageIDRe    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	mentionAnyRe = regexp.MustCompile(`(^|[^\w])@s-[a-z]+`)
)

// Check checks what the content refers to and that it keeps the rules of
// the domain model and the limits of DM-05: every reference resolves, every
// issue's history leads to its final state, every markup text parses.
func (c *Content) Check() error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	person := func(id string) bool { return c.Person(id) != nil }

	seen := map[string]bool{}
	for _, p := range c.People {
		if !personIDRe.MatchString(p.ID) || seen[p.ID] {
			fail("person %q: bad or repeated id", p.ID)
		}
		seen[p.ID] = true
		if n := utf8.RuneCountInString(p.Name); n < 2 || n > MaxName || p.Role == "" {
			fail("person %s: name of 2 to %d characters and a role", p.ID, MaxName)
		}
		if p.Hue < 0 || p.Hue > 15 {
			fail("person %s: hue %d outside 0 to 15", p.ID, p.Hue)
		}
	}
	keys := map[string]bool{}
	for _, p := range c.Projects {
		if !projectKeyRe.MatchString(p.Key) || keys[p.Key] {
			fail("project %q: bad or repeated key", p.Key)
		}
		keys[p.Key] = true
		if n := utf8.RuneCountInString(p.Name); n < 1 || n > 100 {
			fail("project %s: name of 1 to 100 characters", p.Key)
		}
		if !person(p.Lead) {
			fail("project %s: unknown lead %q", p.Key, p.Lead)
		}
		if hasDup(p.Labels) || hasDup(names(p.Components)) {
			fail("project %s: repeated label or component", p.Key)
		}
		for _, l := range p.Labels {
			if !labelRe.MatchString(l) {
				fail("project %s: bad label %q", p.Key, l)
			}
		}
	}
	for _, s := range c.Spaces {
		if !spaceKeyRe.MatchString(s.Key) || keys[s.Key] {
			fail("space %q: bad or repeated key", s.Key)
		}
		keys[s.Key] = true
	}
	for _, im := range c.Images {
		if !imageIDRe.MatchString(im.ID) || keys[im.ID] {
			fail("image %q: bad or repeated id", im.ID)
		}
		keys[im.ID] = true
		if !slices.Contains(ImageKinds, im.Kind) || im.Width <= 0 || im.Height <= 0 || im.Title == "" || im.Alt == "" {
			fail("image %s: kind, size, title and alt", im.ID)
		}
	}

	for i := range c.Issues {
		errs = append(errs, c.checkIssue(&c.Issues[i])...)
	}
	for i := 1; i < len(c.Issues); i++ {
		a, b := c.Issues[i-1], c.Issues[i]
		if a.Project == b.Project && b.Created.Before(a.Created) {
			fail("%s is created before %s; numbers follow creation", b.Key, a.Key)
		}
	}
	pairs := map[string]string{}
	for _, is := range c.Issues {
		for _, l := range is.Links {
			a, b := is.Key, l.Key
			if b < a {
				a, b = b, a
			}
			if prev, dup := pairs[a+" "+b]; dup {
				fail("%s: a second link between %s and %s (%s, %s)", is.Key, a, b, prev, l.Type)
			}
			pairs[a+" "+b] = l.Type
		}
	}

	errs = append(errs, c.checkPages()...)

	for _, t := range c.Templates {
		if t.Name == "" || t.Description == "" {
			fail("template %q: name and description", t.Name)
		}
		if _, err := Parse(t.Body, c.Resolver("", nil)); err != nil {
			fail("template %s: %v", t.Name, err)
		}
	}
	if hasDup(templateNames(c.Templates)) {
		fail("repeated template name")
	}
	return errors.Join(errs...)
}

func (c *Content) checkIssue(is *Issue) []error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(is.Key+": "+format, args...))
	}
	person := func(id string) bool { return c.Person(id) != nil }
	proj := c.Project(is.Project)

	if !slices.Contains(Types, is.Type) || !slices.Contains(Statuses, is.Status) || !slices.Contains(Priorities, is.Priority) {
		fail("type %q, status %q or priority %q", is.Type, is.Status, is.Priority)
	}
	if n := utf8.RuneCountInString(is.Summary); n < 1 || n > MaxSummary || strings.TrimSpace(is.Summary) != is.Summary {
		fail("summary of 1 to %d characters, trimmed", MaxSummary)
	}
	if is.Assignee != "" && !person(is.Assignee) || !person(is.Reporter) {
		fail("unknown assignee %q or reporter %q", is.Assignee, is.Reporter)
	}
	for _, l := range is.Labels {
		if !slices.Contains(proj.Labels, l) {
			fail("label %q is not one of the project's", l)
		}
	}
	for _, k := range is.Components {
		if !slices.Contains(names(proj.Components), k) {
			fail("component %q is not one of the project's", k)
		}
	}
	if hasDup(is.Labels) || hasDup(is.Components) || hasDup(is.Watchers) {
		fail("repeated label, component or watcher")
	}
	for _, w := range is.Watchers {
		if !person(w) {
			fail("unknown watcher %q", w)
		}
	}
	errs = append(errs, c.checkParent(is, is.Type, is.Parent, "")...)
	if is.Points != nil && (*is.Points < 0 || *is.Points > 100) {
		fail("story points %d outside 0 to 100", *is.Points)
	}
	if is.Due != nil && (*is.Due < -30 || *is.Due > 120) {
		fail("due date %d days away", *is.Due)
	}
	for _, e := range []Estimate{is.Estimate, is.Remaining} {
		if _, err := e.Minutes(); err != nil {
			fail("%v", err)
		}
	}
	if !inWindow(is.Created) {
		fail("created %s outside the 30 days before the run", is.Created)
	}
	if utf8.RuneCountInString(string(is.Description)) > MaxDescription {
		fail("description longer than %d characters", MaxDescription)
	}
	if _, err := Parse(string(is.Description), c.Resolver("", nil)); err != nil {
		fail("description: %v", err)
	}

	// History: from the initial state to the final one, in time order.
	init := is.Initial()
	if Category(init["status"]) != "To Do" {
		fail("created in status %q, not one of the To Do category", init["status"])
	}
	if init["parent"] != "" {
		if p := c.Issue(init["parent"]); p != nil && is.Created.Before(p.Created) {
			fail("created before its parent %s", p.Key)
		}
	}
	cur := init
	last := is.Created
	for _, h := range is.History {
		if !slices.Contains(HistoryFields, h.Field) {
			fail("history of unknown field %q", h.Field)
			continue
		}
		if h.At.Before(last) || !inWindow(h.At) {
			fail("change of %s at %s out of order or outside the window", h.Field, h.At)
		}
		last = h.At
		if !person(h.By) {
			fail("change by unknown %q", h.By)
		}
		if h.From != cur[h.Field] {
			fail("change of %s from %q, but it was %q", h.Field, h.From, cur[h.Field])
		}
		if h.From == h.To {
			fail("change of %s to the same %q", h.Field, h.To)
		}
		if err := c.checkValue(is, h); err != nil {
			fail("%v", err)
		}
		cur[h.Field] = h.To
	}
	for _, f := range HistoryFields {
		if cur[f] != is.Value(f) {
			fail("history ends with %s %q, the issue has %q", f, cur[f], is.Value(f))
		}
	}
	if _, ok := is.Resolved(); ok != (is.Status == "Done") {
		fail("resolved does not match status %s", is.Status)
	}

	for k, cm := range is.Comments {
		if !person(cm.By) {
			fail("comment %d by unknown %q", k+1, cm.By)
		}
		if cm.At.Before(is.Created) || !inWindow(cm.At) || k > 0 && cm.At.Before(is.Comments[k-1].At) {
			fail("comment %d at %s out of order", k+1, cm.At)
		}
		errs = append(errs, checkComment(c, is.Key+fmt.Sprintf(" comment %d", k+1), cm.Body, "")...)
	}
	for _, l := range is.Links {
		other := c.Issue(l.Key)
		switch {
		case !slices.Contains(LinkTypes, l.Type):
			fail("link type %q", l.Type)
		case other == nil:
			fail("link to unknown %s", l.Key)
		case other.Key == is.Key:
			fail("link to itself")
		case l.At.Before(is.Created) || l.At.Before(other.Created) || !inWindow(l.At):
			fail("link to %s at %s, before one of them exists", l.Key, l.At)
		case !person(l.By):
			fail("link by unknown %q", l.By)
		}
	}
	return errs
}

// checkParent checks the rules of section 4.2 for a type and a parent.
func (c *Content) checkParent(is *Issue, typ, parent, at string) []error {
	if parent == "" {
		if typ == "Subtask" {
			return []error{fmt.Errorf("%s: a subtask without a parent%s", is.Key, at)}
		}
		return nil
	}
	p := c.Issue(parent)
	switch {
	case p == nil:
		return []error{fmt.Errorf("%s: unknown parent %s", is.Key, parent)}
	case p.Project != is.Project:
		return []error{fmt.Errorf("%s: parent %s in another project", is.Key, parent)}
	case typ == "Epic":
		return []error{fmt.Errorf("%s: an epic with a parent", is.Key)}
	case typ == "Subtask" && (p.Type == "Subtask" || p.Type == "Epic"):
		return []error{fmt.Errorf("%s: a subtask under %s %s", is.Key, p.Type, p.Key)}
	case typ != "Subtask" && p.Type != "Epic":
		return []error{fmt.Errorf("%s: a %s under %s %s, not an epic", is.Key, typ, p.Type, p.Key)}
	}
	return nil
}

func (c *Content) checkValue(is *Issue, h Change) error {
	bad := func() error { return fmt.Errorf("%s: change of %s to %q", is.Key, h.Field, h.To) }
	for _, v := range []string{h.From, h.To} {
		switch h.Field {
		case "status":
			if !slices.Contains(Statuses, v) {
				return bad()
			}
		case "priority":
			if !slices.Contains(Priorities, v) {
				return bad()
			}
		case "type":
			if !slices.Contains(Types, v) {
				return bad()
			}
		case "assignee":
			if v != "" && c.Person(v) == nil {
				return bad()
			}
		case "parent":
			if v != "" && c.Issue(v) == nil {
				return bad()
			}
		case "due", "points":
			if _, err := strconv.Atoi(v); v != "" && err != nil {
				return bad()
			}
		case "estimate", "remaining":
			if _, err := Estimate(v).Minutes(); err != nil {
				return bad()
			}
		case "summary":
			if n := utf8.RuneCountInString(v); n < 1 || n > MaxSummary {
				return bad()
			}
		}
	}
	if h.Field == "parent" && h.To != "" {
		if p := c.Issue(h.To); h.At.Before(p.Created) {
			return fmt.Errorf("%s: parent %s set before it exists", is.Key, h.To)
		}
	}
	return nil
}

func checkComment(c *Content, what string, body Text, space string) []error {
	var errs []error
	s := string(body)
	if strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > MaxComment {
		errs = append(errs, fmt.Errorf("%s: 1 to %d characters", what, MaxComment))
	}
	if n := len(mentionAnyRe.FindAllString(s, -1)); n > MaxMentions {
		errs = append(errs, fmt.Errorf("%s: %d mentions, more than %d", what, n, MaxMentions))
	}
	if _, err := Parse(s, c.Resolver(space, nil)); err != nil {
		errs = append(errs, fmt.Errorf("%s: %v", what, err))
	}
	for _, l := range strings.Split(s, "\n") {
		if headingRe.MatchString(l) || strings.HasPrefix(l, "|") {
			errs = append(errs, fmt.Errorf("%s: comments hold no headings or tables", what))
			break
		}
	}
	return errs
}

func (c *Content) checkPages() []error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	person := func(id string) bool { return c.Person(id) != nil }
	for _, s := range c.Spaces {
		titles := map[string]bool{}
		homes := 0
		positions := map[string][]int{}
		for _, p := range c.Pages {
			if p.Space != s.Key {
				continue
			}
			what := p.File
			lower := strings.ToLower(p.Title)
			if n := utf8.RuneCountInString(p.Title); n < 1 || n > MaxSummary || titles[lower] {
				fail("%s: title of 1 to %d characters, unique in the space", what, MaxSummary)
			}
			titles[lower] = true
			if p.Home {
				homes++
				if p.Parent != "" {
					fail("%s: the home page has a parent", what)
				}
			}
			if p.Parent != "" && c.Page(s.Key, p.Parent) == nil {
				fail("%s: unknown parent %q", what, p.Parent)
			}
			positions[p.Parent] = append(positions[p.Parent], p.Position)
			if !person(p.Author) || !inWindow(p.Created) {
				fail("%s: author %q or created %s", what, p.Author, p.Created)
			}
			for _, l := range p.Labels {
				if !labelRe.MatchString(l) {
					fail("%s: bad label %q", what, l)
				}
			}
			if d := c.Depth(&p); d < 1 || d > 3 {
				fail("%s: depth %d (a cycle, or deeper than 3)", what, d)
			}
			first := p.Versions[0]
			if first.At != p.Created || first.By != p.Author {
				fail("%s: version 1 is not at created by the author", what)
			}
			for k, v := range p.Versions {
				if !person(v.By) || !inWindow(v.At) || k > 0 && v.At.Before(p.Versions[k-1].At) {
					fail("%s: version %d by %q at %s", what, v.Number, v.By, v.At)
				}
				if _, err := Parse(v.Body, c.Resolver(s.Key, nil)); err != nil {
					fail("%s: version %d: %v", what, v.Number, err)
				}
				if k > 0 && v.Body == p.Versions[k-1].Body {
					fail("%s: version %d repeats the one before", what, v.Number)
				}
			}
		}
		if homes != 1 {
			fail("space %s: %d home pages", s.Key, homes)
		}
		for parent, ps := range positions {
			slices.Sort(ps)
			for k, pos := range ps {
				if pos != k+1 {
					fail("space %s: positions under %q are %v, not 1 to %d", s.Key, parent, ps, len(ps))
					break
				}
			}
		}
	}
	for k, pc := range c.PageComments {
		p := c.Page(pc.Space, pc.Page)
		what := fmt.Sprintf("page comment %d on %s/%q", k+1, pc.Space, pc.Page)
		if p == nil {
			fail("%s: unknown page", what)
			continue
		}
		if !person(pc.By) || pc.At.Before(p.Created) || !inWindow(pc.At) {
			fail("%s: author %q or time %s", what, pc.By, pc.At)
		}
		errs = append(errs, checkComment(c, what, pc.Body, pc.Space)...)
		last := pc.At
		for j, r := range pc.Replies {
			if !person(r.By) || r.At.Before(last) || !inWindow(r.At) || len(r.Replies) > 0 {
				fail("%s: reply %d by %q at %s", what, j+1, r.By, r.At)
			}
			last = r.At
			errs = append(errs, checkComment(c, fmt.Sprintf("%s reply %d", what, j+1), r.Body, pc.Space)...)
		}
	}
	return errs
}

// Depth returns 1 for a page without a parent, 2 for its children and so
// on, or 0 for a page in a cycle.
func (c *Content) Depth(p *Page) int {
	d := 1
	for p.Parent != "" {
		p = c.Page(p.Space, p.Parent)
		if p == nil || d > len(c.Pages) {
			return 0
		}
		d++
	}
	return d
}

// inWindow reports whether a moment lies in the 30 days before the run's
// date (DM-02.1), and before the run's date.
func inWindow(w When) bool { return w.Days >= -30 && w.Days <= -1 }

func hasDup(s []string) bool {
	seen := map[string]bool{}
	for _, v := range s {
		if seen[v] {
			return true
		}
		seen[v] = true
	}
	return false
}

func names(cs []Component) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func templateNames(ts []Template) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Name)
	}
	return out
}
