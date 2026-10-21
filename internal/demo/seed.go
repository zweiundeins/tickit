package demo

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/demo/seed"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// The seed runner (DM-02) writes internal/demo/seed's content with the
// commands people use, each by a seeded person (people.Seed) at a time in
// the 30 days before the run: projects, issues as they were created, every
// change of their history, links, comments, watchers, and at the end the
// board order. The event log, the history tabs and the activity feeds then
// read like a month of work. The steps run in time order, a batch of them
// per writer command, so the log's positions follow the times.

// Run is one run of the seed: the content, when it runs, and the ids it
// gives. A PageSeeder reads it.
type Run struct {
	Content *seed.Content
	// At is when the seed runs, in Europe/Zurich: the content's moments
	// count from its date.
	At time.Time
	// Epoch is the demo's epoch the run belongs to (MP-13.2): page
	// streams start anew at every reset, so an editor that names the
	// epoch it opened in never applies an edit to the page that replaced
	// the one it edited.
	Epoch int64

	ops      int
	issues   map[string]int // issue key to its place in creation order
	comments map[*seed.Comment]int
	pages    map[string]int // space/title to the page's place in the content
	spaces   map[string]int
}

// Step is one thing the seed does at a time: a command by a seeded person
// whose By.At is At. Seed runs every step in time order, and steps of one
// time in the order Phase, then the order they were made in.
type Step struct {
	At    time.Time
	Phase int // PhaseCreate for steps that make what later steps of the same minute name
	Name  string
	Cmd   writer.Command
}

// Phases of steps at the same time.
const (
	PhaseScope  = 0 // projects and spaces
	PhaseCreate = 1 // issues and pages
	PhaseChange = 2 // everything else
)

// PageSeeder writes the wiki's part of the seed: spaces, pages with their
// versions, page comments and templates. The wiki registers one with
// RegisterPageSeeder; without one, Seed seeds no pages.
type PageSeeder interface {
	// PageSteps returns the steps that write the spaces, the pages and
	// their versions, the page comments and the templates of r.Content,
	// with r's ids (SpaceID, PageID, PageCommentID, TemplateID), people
	// (By) and documents (Doc). Seed runs them among the tracker's steps
	// in time order.
	PageSteps(r *Run) ([]Step, error)
}

var (
	pageSeederMu sync.Mutex
	pageSeeder   PageSeeder
)

// RegisterPageSeeder sets the wiki's part of the seed: the wiki's
// package calls it from an init function.
func RegisterPageSeeder(p PageSeeder) {
	pageSeederMu.Lock()
	defer pageSeederMu.Unlock()
	pageSeeder = p
}

func registeredPageSeeder() PageSeeder {
	pageSeederMu.Lock()
	defer pageSeederMu.Unlock()
	return pageSeeder
}

// Seeded counts what a seed run wrote.
type Seeded struct {
	Projects, Issues, Changes, Links, Comments, Moves, PageSteps, Steps int
	Took                                                                time.Duration
}

// NewRun prepares a run of content c at run.
func NewRun(c *seed.Content, run time.Time, epoch int64) *Run {
	r := &Run{Content: c, At: run.In(zurich), Epoch: epoch, issues: map[string]int{}, comments: map[*seed.Comment]int{}, pages: map[string]int{}, spaces: map[string]int{}}
	order := make([]*seed.Issue, len(c.Issues))
	for i := range c.Issues {
		order[i] = &c.Issues[i]
	}
	slices.SortStableFunc(order, func(a, b *seed.Issue) int { return whenCmp(a.Created, b.Created) })
	var comments []*seed.Comment
	for i, is := range order {
		r.issues[is.Key] = i
		for k := range is.Comments {
			comments = append(comments, &is.Comments[k])
		}
	}
	slices.SortStableFunc(comments, func(a, b *seed.Comment) int { return whenCmp(a.At, b.At) })
	for i, cm := range comments {
		r.comments[cm] = i
	}
	for i, p := range c.Pages {
		r.pages[p.Space+"/"+p.Title] = i
	}
	for i, s := range c.Spaces {
		r.spaces[s.Key] = i
	}
	return r
}

func whenCmp(a, b seed.When) int {
	return cmp.Or(cmp.Compare(a.Days, b.Days), cmp.Compare(a.Minutes, b.Minutes))
}

// Time is the moment w of the run.
func (r *Run) Time(w seed.When) time.Time { return w.Time(r.At) }

// By is a seeded person acting at at, with an op id of its own.
func (r *Run) By(person string, at time.Time) tabs.By {
	r.ops++
	p := people.Person{ID: person, Kind: people.Seed}
	for _, s := range people.Seeded {
		if s.ID == person {
			p = s
		}
	}
	return tabs.By{Actor: p, Op: "seed-" + strconv.Itoa(r.ops), At: at}
}

// ProjectID is the id of the seed's project with key.
func (r *Run) ProjectID(key string) string {
	n := slices.IndexFunc(r.Content.Projects, func(p seed.Project) bool { return p.Key == key })
	return stableID(kindProject, n, "project "+key)
}

// IssueID is the id of the seed's issue with key.
func (r *Run) IssueID(key string) string { return stableID(kindIssue, r.issues[key], "issue "+key) }

func (r *Run) commentID(cm *seed.Comment, key string) string {
	return stableID(kindComment, r.comments[cm], fmt.Sprintf("comment %s %d", key, r.comments[cm]))
}

// SpaceID is the id of the seed's space with key.
func (r *Run) SpaceID(key string) string { return stableID(kindSpace, r.spaces[key], "space "+key) }

// PageID is the id of the seed's page of a space with a title: the same
// at every run, so issues link to it.
func (r *Run) PageID(space, title string) string {
	return stableID(kindPage, r.pages[space+"/"+title], "page "+space+"/"+title)
}

// PageCommentID is the id of the n-th page comment of the seed, in the
// order the PageSeeder makes them.
func (r *Run) PageCommentID(n int) string {
	return stableID(kindPageComment, n, "page comment "+strconv.Itoa(n))
}

// TemplateID is the id of the seed's template with name, the n-th.
func (r *Run) TemplateID(n int, name string) string {
	return stableID(kindTemplate, n, "template "+name)
}

// Doc is markup as a document of the editor's schema, its page links
// resolved to the seed's page ids; space is the space of a page's own
// markup, "" for issues and comments on them.
func (r *Run) Doc(markup string, space string) (json.RawMessage, error) {
	return seed.Parse(markup, r.Content.Resolver(space, r.PageID))
}

// The seed's names of statuses, as the default workflow's ids.
var statusIDs = map[string]string{
	"Backlog":                  tracker.StatusBacklog,
	"Selected for Development": tracker.StatusSelected,
	"To Do":                    tracker.StatusToDo,
	"In Progress":              tracker.StatusInProgress,
	"In Review":                tracker.StatusInReview,
	"Blocked":                  tracker.StatusBlocked,
	"Done":                     tracker.StatusDone,
}

var linkTypes = map[string]tracker.LinkType{"blocks": tracker.Blocks, "relates to": tracker.RelatesTo, "duplicates": tracker.Duplicates}

// Seed writes content c into an empty database as if it happened in the
// 30 days before run (DM-02): the people, the projects, the issues, and the
// wiki's pages when a PageSeeder is registered. The ids are the same at
// every run. It takes a few seconds; the reset runs it after it emptied
// the database, and `tickit seed` on a new one.
func Seed(ctx context.Context, w *writer.Writer, c *seed.Content, run time.Time, epoch int64) (Seeded, error) {
	start := time.Now()
	r := NewRun(c, run, epoch)
	steps, n, err := r.trackerSteps()
	if err != nil {
		return n, err
	}
	if ps := registeredPageSeeder(); ps != nil {
		more, err := ps.PageSteps(r)
		if err != nil {
			return n, fmt.Errorf("seed: pages: %w", err)
		}
		n.PageSteps = len(more)
		steps = append(steps, more...)
	}
	slices.SortStableFunc(steps, func(a, b Step) int { return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.Phase, b.Phase)) })
	steps = append(steps, r.dashboardSteps(steps[len(steps)-1].At)...)
	moves, err := r.rankSteps(steps[len(steps)-1].At)
	if err != nil {
		return n, err
	}
	n.Moves = len(moves)
	steps = append(steps, moves...)
	steps = append(steps, r.workflowSteps(steps[len(steps)-1].At)...)
	first := Step{At: r.At.AddDate(0, 0, -31), Name: "people", Cmd: people.SeedPeople{At: r.At.AddDate(0, 0, -31)}}
	steps = append([]Step{first}, steps...)
	n.Steps = len(steps)
	if err := runSteps(ctx, w, steps); err != nil {
		return n, err
	}
	n.Took = time.Since(start)
	return n, nil
}

// batchSteps is how many steps one writer command applies: enough to keep
// the seed short, few enough that no action waits long behind one.
const batchSteps = 64

// runSteps applies the steps in order, batchSteps to a writer command.
func runSteps(ctx context.Context, w *writer.Writer, steps []Step) error {
	for len(steps) > 0 {
		batch := steps[:min(batchSteps, len(steps))]
		steps = steps[len(batch):]
		err := w.Do(ctx, writer.Func(func(tx *writer.Tx) error {
			for _, s := range batch {
				if v, ok := s.Cmd.(writer.Validator); ok {
					if err := v.Validate(); err != nil {
						return fmt.Errorf("seed: %s: %w", s.Name, err)
					}
				}
				if err := s.Cmd.Apply(tx); err != nil {
					return fmt.Errorf("seed: %s at %s: %w", s.Name, s.At.Format(time.DateTime), err)
				}
			}
			return nil
		}))
		if err != nil {
			return err
		}
	}
	return nil
}

// workflowSteps give OPS a transition rule once its history is written
// (FR-37): its issues reach Done only from In Review. The other projects
// keep the default workflow.
func (r *Run) workflowSteps(last time.Time) []Step {
	for _, p := range r.Content.Projects {
		if p.Key == "OPS" {
			at := last.Add(time.Second)
			return []Step{{At: at, Phase: PhaseChange, Name: "OPS rule of Done", Cmd: tracker.SetTransitionRule{By: r.By(p.Lead, at),
				Project: r.ProjectID(p.Key), Status: tracker.StatusDone, From: []string{tracker.StatusInReview}}}}
		}
	}
	return nil
}

// trackerSteps are the steps of the projects, issues and comments.
func (r *Run) trackerSteps() ([]Step, Seeded, error) {
	var steps []Step
	var n Seeded
	c := r.Content
	add := func(at time.Time, phase int, name string, cmd writer.Command) {
		steps = append(steps, Step{At: at, Phase: phase, Name: name, Cmd: cmd})
	}
	firstIssue := map[string]time.Time{}
	for _, is := range c.Issues {
		if t, ok := firstIssue[is.Project]; !ok || r.Time(is.Created).Before(t) {
			firstIssue[is.Project] = r.Time(is.Created)
		}
	}
	for i, p := range c.Projects {
		at := firstIssue[p.Key].Add(-time.Hour).Add(time.Duration(i) * time.Minute)
		id := r.ProjectID(p.Key)
		add(at, PhaseScope, p.Key, tracker.CreateProject{By: r.By(p.Lead, at), ID: id, Key: p.Key, Name: p.Name, Description: p.Description, Lead: p.Lead, Open: true})
		for k, comp := range p.Components {
			at := at.Add(time.Duration(k+1) * time.Second)
			add(at, PhaseScope, p.Key+" component "+comp.Name, tracker.DefineComponent{By: r.By(p.Lead, at), Project: id, Name: comp.Name, Description: comp.Description})
		}
		n.Projects++
	}
	for k := range c.Issues {
		is := &c.Issues[k]
		more, err := r.issueSteps(is, &n)
		if err != nil {
			return nil, n, err
		}
		steps = append(steps, more...)
	}
	return steps, n, nil
}

// issueSteps are an issue's creation and what happened to it after.
func (r *Run) issueSteps(is *seed.Issue, n *Seeded) ([]Step, error) {
	var steps []Step
	add := func(at time.Time, phase int, name string, cmd writer.Command) {
		steps = append(steps, Step{At: at, Phase: phase, Name: is.Key + " " + name, Cmd: cmd})
	}
	id := r.IssueID(is.Key)
	created := r.Time(is.Created)
	init := is.Initial()
	desc, err := r.Doc(string(is.Description), "")
	if err != nil {
		return nil, fmt.Errorf("seed: %s description: %w", is.Key, err)
	}
	ci := tracker.CreateIssue{
		By: r.By(is.Reporter, created), ID: id, Project: r.ProjectID(is.Project),
		Type: tracker.IssueType(strings.ToLower(init["type"])), Summary: init["summary"], Description: desc,
		Status: statusIDs[init["status"]], Priority: tracker.Priority(strings.ToLower(init["priority"])),
		Assignee: init["assignee"], Labels: is.Labels, Components: is.Components, Due: r.due(init["due"]),
	}
	if init["parent"] != "" {
		ci.Parent = r.IssueID(init["parent"])
	}
	if ci.Points, err = number(init["points"]); err != nil {
		return nil, fmt.Errorf("seed: %s points: %w", is.Key, err)
	}
	if ci.Estimate, err = minutes(init["estimate"]); err != nil {
		return nil, fmt.Errorf("seed: %s estimate: %w", is.Key, err)
	}
	if ci.Remaining, err = minutes(init["remaining"]); err != nil {
		return nil, fmt.Errorf("seed: %s remaining: %w", is.Key, err)
	}
	add(created, PhaseCreate, "created", ci)
	n.Issues++
	for _, w := range is.Watchers {
		add(created, PhaseChange, "watched by "+w, tracker.Watch{By: r.By(w, created), Issue: id, Watch: true})
	}
	for _, h := range is.History {
		at := r.Time(h.At)
		cmd, err := r.change(id, h, r.By(h.By, at))
		if err != nil {
			return nil, fmt.Errorf("seed: %s %s: %w", is.Key, h.Field, err)
		}
		add(at, PhaseChange, h.Field, cmd)
		n.Changes++
	}
	for _, l := range is.Links {
		at := r.Time(l.At)
		add(at, PhaseChange, "link "+l.Key, tracker.LinkIssues{By: r.By(l.By, at), Issue: id, Type: linkTypes[l.Type], Other: r.IssueID(l.Key)})
		n.Links++
	}
	for k := range is.Comments {
		cm := &is.Comments[k]
		at := r.Time(cm.At)
		body, err := r.Doc(string(cm.Body), "")
		if err != nil {
			return nil, fmt.Errorf("seed: %s comment %d: %w", is.Key, k+1, err)
		}
		add(at, PhaseChange, "comment "+strconv.Itoa(k+1), tracker.AddComment{By: r.By(cm.By, at), ID: r.commentID(cm, is.Key), Issue: id, Body: body})
		n.Comments++
	}
	return steps, nil
}

// change is the command of one entry of an issue's history. It reads the
// issue's version when it runs: the change saw everything before it.
func (r *Run) change(id string, h seed.Change, by tabs.By) (writer.Command, error) {
	var cmd func(seen int64) writer.Command
	switch h.Field {
	case "status":
		cmd = func(v int64) writer.Command {
			return tracker.SetStatus{By: by, Issue: id, Seen: v, Status: statusIDs[h.To]}
		}
	case "assignee":
		cmd = func(v int64) writer.Command { return tracker.Assign{By: by, Issue: id, Seen: v, Assignee: h.To} }
	case "priority":
		cmd = func(v int64) writer.Command {
			return tracker.SetPriority{By: by, Issue: id, Seen: v, Priority: tracker.Priority(strings.ToLower(h.To))}
		}
	case "summary":
		cmd = func(v int64) writer.Command { return tracker.SetSummary{By: by, Issue: id, Seen: v, Summary: h.To} }
	case "type":
		cmd = func(v int64) writer.Command {
			return tracker.SetType{By: by, Issue: id, Seen: v, Type: tracker.IssueType(strings.ToLower(h.To))}
		}
	case "parent":
		parent := ""
		if h.To != "" {
			parent = r.IssueID(h.To)
		}
		cmd = func(v int64) writer.Command { return tracker.SetParent{By: by, Issue: id, Seen: v, Parent: parent} }
	case "due":
		due := r.due(h.To)
		cmd = func(v int64) writer.Command { return tracker.SetDue{By: by, Issue: id, Seen: v, Due: due} }
	case "points":
		p, err := number(h.To)
		if err != nil {
			return nil, err
		}
		cmd = func(v int64) writer.Command { return tracker.SetPoints{By: by, Issue: id, Seen: v, Points: p} }
	case "estimate", "remaining":
		m, err := minutes(h.To)
		if err != nil {
			return nil, err
		}
		if h.Field == "estimate" {
			cmd = func(v int64) writer.Command { return tracker.SetEstimate{By: by, Issue: id, Seen: v, Estimate: m} }
		} else {
			cmd = func(v int64) writer.Command { return tracker.SetRemaining{By: by, Issue: id, Seen: v, Remaining: m} }
		}
	default:
		return nil, fmt.Errorf("no field %q", h.Field)
	}
	return seen(id, cmd), nil
}

// seen runs the command that cmd makes with the issue's version.
func seen(id string, cmd func(seen int64) writer.Command) writer.Command {
	return writer.Func(func(tx *writer.Tx) error {
		v, err := issueVersion(tx, id)
		if err != nil {
			return err
		}
		return cmd(v).Apply(tx)
	})
}

func issueVersion(tx *writer.Tx, id string) (int64, error) {
	var v int64
	err := tx.QueryRow(`SELECT version FROM issues WHERE id = ?`, id).Scan(&v)
	return v, err
}

// rankSteps put each project's issues in the board order the seed holds,
// after everything else: its lead moves the fewest issues that need it
// (those outside the longest run that is in order already), each after
// the issue above it.
func (r *Run) rankSteps(after time.Time) ([]Step, error) {
	var steps []Step
	at := after.Add(time.Minute)
	for _, p := range r.Content.Projects {
		var list []*seed.Issue
		for k := range r.Content.Issues {
			if is := &r.Content.Issues[k]; is.Project == p.Key {
				list = append(list, is)
			}
		}
		// Created in number order, they stand in number order.
		slices.SortFunc(list, func(a, b *seed.Issue) int { return cmp.Compare(a.Rank, b.Rank) })
		numbers := make([]int, len(list))
		for i, is := range list {
			numbers[i] = is.Number
		}
		keep := increasing(numbers)
		for i, is := range list {
			if keep[i] {
				continue
			}
			above := ""
			if i > 0 {
				above = r.IssueID(list[i-1].Key)
			}
			by := r.By(p.Lead, at)
			id := r.IssueID(is.Key)
			cmd := seen(id, func(v int64) writer.Command {
				if above == "" {
					var top string
					return writer.Func(func(tx *writer.Tx) error {
						if err := tx.QueryRow(`SELECT id FROM issues WHERE project_id = ? AND id != ? ORDER BY rank LIMIT 1`, r.ProjectID(p.Key), id).Scan(&top); err != nil {
							return err
						}
						return tracker.Move{By: by, Issue: id, Before: top, Seen: v}.Apply(tx)
					})
				}
				return tracker.Move{By: by, Issue: id, After: above, Seen: v}
			})
			steps = append(steps, Step{At: at, Phase: PhaseChange, Name: is.Key + " rank", Cmd: cmd})
			at = at.Add(10 * time.Second)
		}
	}
	if len(steps) > 0 && !steps[len(steps)-1].At.Before(time.Date(r.At.Year(), r.At.Month(), r.At.Day(), 0, 0, 0, 0, r.At.Location())) {
		return nil, errors.New("seed: the board order would end on the day of the run")
	}
	return steps, nil
}

// increasing marks a longest increasing subsequence of s.
func increasing(s []int) []bool {
	tails, prev := []int{}, make([]int, len(s)) // tails: index of the smallest tail of each length
	for i, v := range s {
		k, _ := slices.BinarySearchFunc(tails, v, func(t, v int) int { return cmp.Compare(s[t], v) })
		prev[i] = -1
		if k > 0 {
			prev[i] = tails[k-1]
		}
		if k == len(tails) {
			tails = append(tails, i)
		} else {
			tails[k] = i
		}
	}
	keep := make([]bool, len(s))
	if len(tails) > 0 {
		for i := tails[len(tails)-1]; i >= 0; i = prev[i] {
			keep[i] = true
		}
	}
	return keep
}

// due is a due date of the seed, days after the run's date, as the
// tracker writes it; "" for none.
func (r *Run) due(days string) string {
	if days == "" {
		return ""
	}
	d, err := strconv.Atoi(days)
	if err != nil {
		return ""
	}
	return seed.DueDate(r.At, d).Format(time.DateOnly)
}

func number(s string) (*int, error) {
	if s == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(s)
	return &v, err
}

func minutes(s string) (*int, error) {
	if s == "" {
		return nil, nil
	}
	m, err := seed.Estimate(s).Minutes()
	return &m, err
}
