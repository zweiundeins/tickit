package tracker

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/demo/words"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
)

var (
	errNoIssue  = errors.New("tracker: no such issue")
	errDeleted  = &writer.Refusal{Msg: "This issue was deleted."}
	errArchived = &writer.Refusal{Msg: "This project is archived."}
)

// related is what a command reads of another issue it names: a parent.
type related struct {
	Exists  bool
	Self    bool
	Deleted bool
	Project string
	Type    IssueType
	Key     string
}

// assignCheck says whether a person may be assigned, and if not why.
type assignCheck struct {
	OK  bool
	Why string
}

// MaxDescriptionMentions is how many people a description may mention.
const MaxDescriptionMentions = 50

// CreateIssue creates an issue with the next number of its project
// (FR-10, FR-11). Its result is the issue's id and key as JSON:
// {"id": "...", "key": "WEB-12"}. Empty fields take their defaults: the
// workflow's start status, Medium, no assignee; the reporter is the actor.
type CreateIssue struct {
	By          tabs.By
	ID          string // optional: a new id when empty
	Project     string
	Type        IssueType
	Summary     string
	Description json.RawMessage // a document (PlainDoc for plain text), or empty
	Status      string
	Priority    Priority
	Assignee    string
	Labels      []string
	Components  []string
	Parent      string
	Due         string // YYYY-MM-DD
	Points      *int
	Estimate    *int // minutes
	Remaining   *int // minutes; the estimate when empty
	Sprint      string
}

func (c CreateIssue) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.ID != "" && !ValidID(c.ID) || !validPerson(c.Assignee) || c.Parent != "" && !ValidID(c.Parent) {
		return errors.New("tracker: a malformed issue")
	}
	return nil
}

func (c CreateIssue) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

// newIssue is CreateIssue with what its command read for the decision.
type newIssue struct {
	CreateIssue
	project ProjectState
	number  int64
	rank    string
	parent  related
	assign  assignCheck
}

var createIssue = issueDecider(decideCreate)

func decideCreate(c newIssue, s IssueState) ([]IssueEvent, error) {
	if s.Exists {
		return nil, errors.New("tracker: the issue exists")
	}
	p := c.project
	if p.Archived {
		return nil, errArchived
	}
	e := IssueCreated{Project: c.Project, Number: c.number, Key: IssueKey(p.Key, c.number), Type: c.Type,
		Reporter: c.By.Actor.ID, Rank: c.rank, Assignee: c.Assignee, Parent: c.Parent, Due: c.Due,
		Points: c.Points, Estimate: c.Estimate, Remaining: c.Remaining, Sprint: c.Sprint}
	if !c.Type.Valid() {
		return nil, tabs.Refuse(FieldType, "Pick one of Epic, Story, Task, Bug and Subtask.")
	}
	var ok bool
	if e.Summary, ok = text(c.Summary, 1, MaxSummary); !ok {
		return nil, tabs.Refuse(FieldSummary, "A summary has 1 to 255 characters.")
	}
	info, err := description(c.Description)
	if err != nil {
		return nil, err
	}
	e.Description, e.Mentions = canonical(c.Description), info.Mentions
	e.Status = c.Status
	if e.Status == "" {
		e.Status = p.Workflow.Start
	}
	st, ok := p.Workflow.Status(e.Status)
	if !ok {
		return nil, tabs.Refuse(FieldStatus, "This project's workflow has no such status.")
	}
	e.Category = st.Category
	e.Priority = c.Priority
	if e.Priority == "" {
		e.Priority = Medium
	}
	if !e.Priority.Valid() {
		return nil, tabs.Refuse(FieldPriority, "Pick Low, Medium, High or Critical.")
	}
	if c.Assignee != "" && !c.assign.OK {
		return nil, tabs.Refuse(FieldAssignee, c.assign.Why)
	}
	if e.Labels, err = labelSet(nil, c.Labels, nil); err != nil {
		return nil, err
	}
	for _, name := range c.Components {
		comp, ok := p.Component(name)
		if !ok {
			return nil, tabs.Refuse(FieldComponents, fmt.Sprintf("%s has no component %s.", p.Key, strings.TrimSpace(name)))
		}
		if !slices.Contains(e.Components, comp.Name) {
			e.Components = append(e.Components, comp.Name)
		}
	}
	if len(e.Components) > MaxComponents {
		return nil, tabs.Refuse(FieldComponents, "An issue is in 20 components at most.")
	}
	if err := parentRule(c.Type, c.Parent, c.parent, c.Project); err != nil {
		return nil, err
	}
	if !validDate(c.Due) {
		return nil, tabs.Refuse(FieldDue, "A due date looks like 2026-10-21.")
	}
	if err := pointsRule(c.Points); err != nil {
		return nil, err
	}
	if err := estimateRule(FieldEstimate, c.Estimate); err != nil {
		return nil, err
	}
	if e.Remaining == nil {
		e.Remaining = c.Estimate
	}
	if err := estimateRule(FieldRemaining, e.Remaining); err != nil {
		return nil, err
	}
	if err := sprintRule(c.Sprint, p.Sprints); err != nil {
		return nil, err
	}
	e.Watchers = watchersOf(nil, c.By.Actor.ID, c.Assignee)
	e.Watchers = watchersOf(e.Watchers, e.Mentions...)
	return []IssueEvent{e}, nil
}

func (c CreateIssue) ApplyResult(tx *writer.Tx) ([]byte, error) {
	p, err := projectState(tx, c.Project)
	if err != nil {
		return nil, err
	}
	if err := require(tx, c.By, access.EditIssues, c.Project); err != nil {
		return nil, err
	}
	if c.ID == "" {
		c.ID = NewID()
	}
	when := at(c.By)
	n := newIssue{CreateIssue: c, project: p, number: p.Last + 1}
	if c.Parent != "" {
		if n.parent, err = relatedIssue(tx, c.Parent, c.ID); err != nil {
			return nil, err
		}
	}
	if c.Assignee != "" {
		if err := acted(tx, c.Project, c.By.Actor.ID, when); err != nil {
			return nil, err
		}
		if n.assign, err = canAssign(tx, c.Project, p, c.Assignee, when); err != nil {
			return nil, err
		}
	}
	last, err := lastRank(tx, c.Project)
	if err != nil {
		return nil, err
	}
	if n.rank, err = RankBetween(last, ""); err != nil {
		return nil, err
	}
	m := meta(c.By)
	if err := events.Execute(Store(), tx, createIssue, IssueStream.ID(c.ID), events.NoStream, n, m); err != nil {
		return nil, err
	}
	_, key, err := assignNumber(tx, p, c.Project, c.ID, m)
	if err != nil {
		return nil, err
	}
	if err := setParentRow(tx, c.ID, c.Parent); err != nil {
		return nil, err
	}
	if err := acted(tx, c.Project, c.By.Actor.ID, when); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": c.ID, "key": key})
}

// description checks an issue's description (DM-05).
func description(raw json.RawMessage) (docInfo, error) {
	info, err := readDoc(raw)
	switch {
	case err != nil && err != errNotDoc:
		return info, tabs.Refuse(FieldDescription, "A description has at most 20,000 characters.")
	case err != nil:
		return info, tabs.Refuse(FieldDescription, "The description is not a document.")
	case info.Length > MaxIssueText:
		return info, tabs.Refuse(FieldDescription, "A description has at most 20,000 characters.")
	case len(info.Mentions) > MaxDescriptionMentions:
		return info, tabs.Refuse(FieldDescription, "A description mentions 50 people at most.")
	}
	for _, m := range info.Mentions {
		if !people.ValidID(m) {
			return info, tabs.Refuse(FieldDescription, "A mention names nobody.")
		}
	}
	return info, nil
}

// parentRule is section 4.2's rule of parents: an epic has none, a
// story, task or bug may have an epic, a subtask has a story, task or
// bug of its project.
func parentRule(t IssueType, parent string, p related, project string) error {
	switch {
	case parent == "" && t == Subtask:
		return tabs.Refuse(FieldParent, "A subtask needs a parent.")
	case parent == "":
		return nil
	case t == Epic:
		return tabs.Refuse(FieldParent, "An epic has no parent.")
	case p.Self:
		return tabs.Refuse(FieldParent, "An issue cannot be its own parent.")
	case !p.Exists:
		return tabs.Refuse(FieldParent, "That issue does not exist.")
	case p.Deleted:
		return tabs.Refuse(FieldParent, "That issue was deleted.")
	case p.Project != project:
		return tabs.Refuse(FieldParent, "The parent is in another project.")
	case t == Subtask && (p.Type == Epic || p.Type == Subtask):
		return tabs.Refuse(FieldParent, "A subtask's parent is a story, a task or a bug.")
	case t != Subtask && p.Type != Epic:
		return tabs.Refuse(FieldParent, fmt.Sprintf("The parent of a %s is an epic.", strings.ToLower(t.Name())))
	}
	return nil
}

func pointsRule(p *int) error {
	if p != nil && (*p < 0 || *p > MaxPoints) {
		return tabs.Refuse(FieldPoints, "Story points are 0 to 100.")
	}
	return nil
}

func estimateRule(field string, m *int) error {
	if m != nil && (*m < 0 || *m > MaxEstimate) {
		return tabs.Refuse(field, "An estimate is 0 to 1,000 hours.")
	}
	return nil
}

func sprintRule(id string, sprints []Sprint) error {
	if id == "" {
		return nil
	}
	i := slices.IndexFunc(sprints, func(s Sprint) bool { return s.ID == id })
	switch {
	case i < 0:
		return tabs.Refuse(FieldSprint, "That sprint does not exist.")
	case sprints[i].State == SprintClosed:
		return tabs.Refuse(FieldSprint, "That sprint is closed.")
	}
	return nil
}

// NormalizeLabel turns a label to lower case and checks it: 1 to 50
// characters without spaces (FR-23).
func NormalizeLabel(s string) (string, bool) {
	l := strings.ToLower(strings.TrimSpace(s))
	n := utf8.RuneCountInString(l)
	return l, n >= 1 && n <= MaxLabel && strings.IndexFunc(l, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}

// labelSet is set with add and without remove, the labels checked.
func labelSet(set, add, remove []string) ([]string, error) {
	norm := func(in []string) ([]string, error) {
		var out []string
		for _, l := range in {
			n, ok := NormalizeLabel(l)
			if !ok {
				return nil, tabs.Refuse(FieldLabels, "A label has 1 to 50 characters and no spaces.")
			}
			if words.Blocked(n) {
				return nil, tabs.Refuse(FieldLabels, words.Refusal)
			}
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
		return out, nil
	}
	a, err := norm(add)
	if err != nil {
		return nil, err
	}
	r, err := norm(remove)
	if err != nil {
		return nil, err
	}
	out := applySet(set, a, r)
	if len(out) > MaxLabels {
		return nil, tabs.Refuse(FieldLabels, "An issue has 20 labels at most.")
	}
	return out, nil
}

// watchersOf adds people to watchers, each once, none empty.
func watchersOf(watchers []string, ids ...string) []string {
	for _, id := range ids {
		if id != "" && !slices.Contains(watchers, id) {
			watchers = append(watchers, id)
		}
	}
	return watchers
}

// newWatchers is a WatchersAdded for those of ids who do not watch yet.
func newWatchers(s IssueState, ids ...string) []IssueEvent {
	var add []string
	for _, id := range ids {
		if id != "" && !slices.Contains(s.Watchers, id) && !slices.Contains(add, id) {
			add = append(add, id)
		}
	}
	if len(add) == 0 {
		return nil
	}
	return []IssueEvent{WatchersAdded{People: add}}
}

// edit is a change to one field of an issue: what the exported commands
// send, with what Apply read for the decision.
type edit struct {
	Field string
	Seen  int64
	Actor string
	Text  string          // summary, type, status, priority, assignee, parent, due, sprint
	Doc   json.RawMessage // description
	Num   *int            // points, estimate, remaining

	wf       Workflow
	sprints  []Sprint
	parent   related // the new parent; for a type change, the issue's parent
	children int
	assign   assignCheck
	own      []string // fields whose last change since Seen was the actor's own
}

var editDecider = issueDecider(decideEdit)

func decideEdit(c edit, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted:
		return nil, errDeleted
	}
	var evs []IssueEvent
	yours := c.Text
	switch c.Field {
	case FieldSummary:
		v, ok := text(c.Text, 1, MaxSummary)
		if !ok {
			return nil, tabs.Refuse(FieldSummary, "A summary has 1 to 255 characters.")
		}
		if v == s.Summary {
			return nil, nil
		}
		yours = v
		evs = []IssueEvent{SummaryChanged{From: s.Summary, To: v}}
	case FieldDescription:
		info, err := description(c.Doc)
		if err != nil {
			return nil, err
		}
		doc := canonical(c.Doc)
		if bytes.Equal(doc, canonical(s.Description)) {
			return nil, nil
		}
		evs = append([]IssueEvent{DescriptionChanged{To: doc, Mentions: added(s.Mentioned, info.Mentions)}}, newWatchers(s, info.Mentions...)...)
	case FieldType:
		t := IssueType(c.Text)
		if !t.Valid() {
			return nil, tabs.Refuse(FieldType, "Pick one of Epic, Story, Task, Bug and Subtask.")
		}
		if t == s.Type {
			return nil, nil
		}
		plain := func(t IssueType) bool { return t == Story || t == Task || t == Bug }
		if c.children > 0 && !(plain(s.Type) && plain(t)) {
			if c.children == 1 {
				return nil, tabs.Refuse(FieldType, "Move the child issue first.")
			}
			return nil, tabs.Refuse(FieldType, fmt.Sprintf("Move the %d child issues first.", c.children))
		}
		if err := parentRule(t, s.Parent, c.parent, s.Project); err != nil {
			var f *tabs.FieldRefusal
			if errors.As(err, &f) {
				return nil, tabs.Refuse(FieldType, f.Msg)
			}
			return nil, err
		}
		evs = []IssueEvent{TypeChanged{From: s.Type, To: t}}
	case FieldStatus:
		st, ok := c.wf.Status(c.Text)
		if !ok {
			return nil, tabs.Refuse(FieldStatus, "This project's workflow has no such status.")
		}
		if c.Text == s.Status {
			return nil, nil
		}
		if !c.wf.Allows(s.Status, c.Text) {
			return nil, tabs.Refuse(FieldStatus, followRule(c.wf, c.Text))
		}
		evs = []IssueEvent{StatusChanged{From: s.Status, To: c.Text, Category: st.Category}}
	case FieldPriority:
		p := Priority(c.Text)
		if !p.Valid() {
			return nil, tabs.Refuse(FieldPriority, "Pick Low, Medium, High or Critical.")
		}
		if p == s.Priority {
			return nil, nil
		}
		evs = []IssueEvent{PriorityChanged{From: s.Priority, To: p}}
	case FieldAssignee:
		if c.Text == s.Assignee {
			return nil, nil
		}
		if c.Text != "" && !c.assign.OK {
			return nil, tabs.Refuse(FieldAssignee, c.assign.Why)
		}
		evs = append([]IssueEvent{AssigneeChanged{From: s.Assignee, To: c.Text}}, newWatchers(s, c.Text)...)
	case FieldParent:
		if c.Text == s.Parent {
			return nil, nil
		}
		if err := parentRule(s.Type, c.Text, c.parent, s.Project); err != nil {
			return nil, err
		}
		evs = []IssueEvent{ParentChanged{From: s.Parent, To: c.Text}}
	case FieldDue:
		if !validDate(c.Text) {
			return nil, tabs.Refuse(FieldDue, "A due date looks like 2026-10-21.")
		}
		if c.Text == s.Due {
			return nil, nil
		}
		evs = []IssueEvent{DueChanged{From: s.Due, To: c.Text}}
	case FieldPoints, FieldEstimate, FieldRemaining:
		var err error
		old := s.Points
		switch c.Field {
		case FieldPoints:
			err = pointsRule(c.Num)
		case FieldEstimate:
			err, old = estimateRule(FieldEstimate, c.Num), s.Estimate
		default:
			err, old = estimateRule(FieldRemaining, c.Num), s.Remaining
		}
		if err != nil {
			return nil, err
		}
		if intString(old) == intString(c.Num) {
			return nil, nil
		}
		yours = intString(c.Num)
		switch c.Field {
		case FieldPoints:
			evs = []IssueEvent{PointsChanged{From: old, To: c.Num}}
		case FieldEstimate:
			evs = []IssueEvent{EstimateChanged{From: old, To: c.Num}}
		default:
			evs = []IssueEvent{RemainingChanged{From: old, To: c.Num}}
		}
	case FieldSprint:
		if c.Text == s.Sprint {
			return nil, nil
		}
		if err := sprintRule(c.Text, c.sprints); err != nil {
			return nil, err
		}
		evs = []IssueEvent{SprintChanged{From: s.Sprint, To: c.Text}}
	default:
		return nil, fmt.Errorf("tracker: no field %q", c.Field)
	}
	if v := s.Changed[c.Field]; v > c.Seen && !slices.Contains(c.own, c.Field) {
		return nil, &conflictErr{Field: c.Field, Version: v, Yours: yours}
	}
	return evs, nil
}

// followRule says which statuses a status may follow (FR-37.1): "Done
// can only follow In Review."
func followRule(w Workflow, to string) string {
	var from []string
	for _, s := range w.Statuses {
		if s.ID != to && w.Allows(s.ID, to) {
			from = append(from, s.Name)
		}
	}
	if len(from) == 0 {
		return fmt.Sprintf("Nothing can move to %s.", w.Name(to))
	}
	return fmt.Sprintf("%s can only follow %s.", w.Name(to), strings.Join(from, " or "))
}

// executeEdit runs a change to one field of an issue: it reads what the
// decision needs, decides, and turns a conflict into a *Conflict.
func executeEdit(tx *writer.Tx, by tabs.By, id string, c edit) error {
	s, err := issueState(tx, id)
	if err != nil {
		return err
	}
	if err := require(tx, by, access.EditIssues, s.Project); err != nil {
		return err
	}
	p, err := projectState(tx, s.Project)
	if err != nil {
		return err
	}
	when := at(by)
	if err := acted(tx, s.Project, by.Actor.ID, when); err != nil {
		return err
	}
	c.Actor, c.wf, c.sprints = by.Actor.ID, p.Workflow, p.Sprints
	switch c.Field {
	case FieldType:
		if c.children, err = childCount(tx, id); err != nil {
			return err
		}
		if s.Parent != "" {
			if c.parent, err = relatedIssue(tx, s.Parent, id); err != nil {
				return err
			}
		}
	case FieldParent:
		if c.Text != "" {
			if c.parent, err = relatedIssue(tx, c.Text, id); err != nil {
				return err
			}
		}
	case FieldAssignee:
		if c.Text != "" && c.Text != s.Assignee {
			if c.assign, err = canAssign(tx, s.Project, p, c.Text, when); err != nil {
				return err
			}
		}
	}
	if c.own, err = ownChanges(tx, by, id, s, c.Seen, c.Field); err != nil {
		return err
	}
	err = events.Execute(Store(), tx, editDecider, IssueStream.ID(id), events.Any, c, meta(by))
	var ce *conflictErr
	switch {
	case errors.As(err, &ce):
		return fieldConflict(tx, by, id, s, p, ce)
	case err != nil:
		return err
	case c.Field == FieldParent:
		return setParentRow(tx, id, c.Text)
	}
	return nil
}

// ownChanges returns the fields whose last change after seen the actor
// made: a change of one's own never conflicts.
func ownChanges(tx *writer.Tx, by tabs.By, id string, s IssueState, seen int64, fields ...string) ([]string, error) {
	var own []string
	for _, f := range fields {
		v := s.Changed[f]
		if v <= seen {
			continue
		}
		m, err := eventMeta(tx.Context(), tx, IssueStream.ID(id), v)
		if err != nil {
			return nil, err
		}
		if m.Actor == by.Actor.ID {
			own = append(own, f)
		}
	}
	return own, nil
}

// fieldConflict is MP-08.3's refusal.
func fieldConflict(tx *writer.Tx, by tabs.By, id string, s IssueState, p ProjectState, ce *conflictErr) error {
	ctx := tx.Context()
	m, err := eventMeta(ctx, tx, IssueStream.ID(id), ce.Version)
	if err != nil {
		return err
	}
	who, err := people.Get(ctx, tx, m.Actor)
	if err != nil {
		return err
	}
	d := displayer{tx: tx, wf: p.Workflow, sprints: p.Sprints}
	theirs, yours := d.show(ce.Field, rawValue(s, ce.Field)), d.show(ce.Field, ce.Yours)
	msg := conflictMessage(ce.Field, who.Name, theirs, yours, Ago(m.At, at(by)))
	return &Conflict{Kind: "conflict", Field: ce.Field, Label: FieldLabel(ce.Field), By: m.Actor, Name: who.Name,
		At: m.At.UnixMilli(), Theirs: theirs, Yours: yours, Version: s.Version, Refusal: writer.Refusal{Msg: msg}}
}

// rawValue is a field's value in a state, as an edit sends it.
func rawValue(s IssueState, field string) string {
	switch field {
	case FieldSummary:
		return s.Summary
	case FieldType:
		return string(s.Type)
	case FieldStatus:
		return s.Status
	case FieldPriority:
		return string(s.Priority)
	case FieldAssignee:
		return s.Assignee
	case FieldParent:
		return s.Parent
	case FieldDue:
		return s.Due
	case FieldPoints:
		return intString(s.Points)
	case FieldEstimate:
		return intString(s.Estimate)
	case FieldRemaining:
		return intString(s.Remaining)
	case FieldSprint:
		return s.Sprint
	case FieldRank:
		return s.Rank
	}
	return ""
}

// displayer shows a field's raw value as a person reads it.
type displayer struct {
	tx      *writer.Tx
	wf      Workflow
	sprints []Sprint
}

func (d displayer) show(field, raw string) string {
	switch field {
	case FieldType:
		return IssueType(raw).Name()
	case FieldStatus:
		return d.wf.Name(raw)
	case FieldPriority:
		return Priority(raw).Name()
	case FieldAssignee:
		if raw == "" {
			return "Unassigned"
		}
		if p, err := people.Get(d.tx.Context(), d.tx, raw); err == nil {
			return p.Name
		}
	case FieldParent:
		if raw == "" {
			return "none"
		}
		var key string
		if d.tx.QueryRow(`SELECT key FROM issues WHERE id = ?`, raw).Scan(&key) == nil {
			return key
		}
	case FieldDue:
		if raw == "" {
			return "none"
		}
		return Date(raw)
	case FieldEstimate, FieldRemaining:
		var m int
		if _, err := fmt.Sscan(raw, &m); err != nil {
			return "none"
		}
		return Minutes(m)
	case FieldPoints:
		if raw == "" {
			return "none"
		}
	case FieldSprint:
		if raw == "" {
			return "the backlog"
		}
		for _, s := range d.sprints {
			if s.ID == raw {
				return s.Name
			}
		}
	}
	return raw
}

// relatedIssue reads another issue a command names.
func relatedIssue(tx *writer.Tx, id, self string) (related, error) {
	if id == self {
		return related{Exists: true, Self: true}, nil
	}
	s, err := issueState(tx, id)
	if errors.Is(err, access.ErrNotFound) {
		return related{}, nil
	}
	if err != nil {
		return related{}, err
	}
	return related{Exists: true, Deleted: s.Deleted, Project: s.Project, Type: s.Type, Key: s.Key}, nil
}

// childCount counts an issue's children, from issue_parents.
func childCount(tx *writer.Tx, id string) (int, error) {
	var n int
	err := tx.QueryRow(`SELECT count(*) FROM issue_parents WHERE parent_id = ?`, id).Scan(&n)
	return n, err
}

// setParentRow keeps issue_parents, which the commands write: the
// children an issue has, for the rules of types (FR-19.2).
func setParentRow(tx *writer.Tx, id, parent string) error {
	if parent == "" {
		_, err := tx.Exec(`DELETE FROM issue_parents WHERE issue_id = ?`, id)
		return err
	}
	_, err := tx.Exec(`INSERT INTO issue_parents (issue_id, parent_id) VALUES (?, ?)
		ON CONFLICT (issue_id) DO UPDATE SET parent_id = excluded.parent_id`, id, parent)
	return err
}

// lastRank is the highest rank of a project's issues, "" without any.
func lastRank(tx *writer.Tx, project string) (string, error) {
	var r sql.NullString
	err := tx.QueryRow(`SELECT max(rank) FROM issues WHERE project_id = ?`, project).Scan(&r)
	return r.String, err
}

// canAssign says whether a person may be assigned in a project (FR-17.2):
// a member, someone who acted there in the last 24 hours, a seeded
// person, or a site admin anywhere.
func canAssign(tx *writer.Tx, project string, p ProjectState, person string, now time.Time) (assignCheck, error) {
	ctx := tx.Context()
	who, err := people.Get(ctx, tx, person)
	if err != nil {
		return assignCheck{}, err
	}
	switch {
	case who.Kind == people.Deleted:
		return assignCheck{Why: "That person does not exist."}, nil
	case who.Kind == people.Account && who.Inactive:
		return assignCheck{Why: who.Name + " is deactivated and cannot be assigned."}, nil
	case who.Kind == people.Seed, who.Kind == people.Account && who.Role == people.Admin:
		return assignCheck{OK: true}, nil
	}
	role, err := access.RoleOf(ctx, tx, person, access.ProjectScope(project))
	if err != nil {
		return assignCheck{}, err
	}
	if role == access.Member || role == access.Admin {
		return assignCheck{OK: true}, nil
	}
	var last int64
	err = tx.QueryRow(`SELECT at FROM tracker_actors WHERE project_id = ? AND person_id = ?`, project, person).Scan(&last)
	switch {
	case err == nil && last >= now.Add(-AssignableRecent*time.Hour).UnixMilli():
		return assignCheck{OK: true}, nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return assignCheck{}, err
	}
	return assignCheck{Why: fmt.Sprintf("%s cannot be assigned: they are not a member of %s.", who.Name, p.Key)}, nil
}

// The changes of one field. Seen is the issue's version the page showed
// (MP-08.1): a field someone else changed after it refuses the change
// with a *Conflict; a change to the value the field holds stores nothing.
type (
	SetSummary struct {
		By      tabs.By
		Issue   string
		Seen    int64
		Summary string
	}
	SetDescription struct {
		By          tabs.By
		Issue       string
		Seen        int64
		Description json.RawMessage
	}
	SetType struct {
		By    tabs.By
		Issue string
		Seen  int64
		Type  IssueType
	}
	// SetStatus changes the status from the issue's menu (FR-16); the
	// board moves with Move.
	SetStatus struct {
		By     tabs.By
		Issue  string
		Seen   int64
		Status string
	}
	SetPriority struct {
		By       tabs.By
		Issue    string
		Seen     int64
		Priority Priority
	}
	// Assign assigns a person, or nobody when Assignee is empty (FR-17).
	Assign struct {
		By       tabs.By
		Issue    string
		Seen     int64
		Assignee string
	}
	// SetParent sets an issue's epic or a subtask's parent (FR-20, FR-21),
	// or clears it when Parent is empty.
	SetParent struct {
		By     tabs.By
		Issue  string
		Seen   int64
		Parent string
	}
	// SetDue sets the due date, YYYY-MM-DD, or clears it (FR-25).
	SetDue struct {
		By    tabs.By
		Issue string
		Seen  int64
		Due   string
	}
	// SetPoints sets the story points, nil to clear them (FR-26).
	SetPoints struct {
		By     tabs.By
		Issue  string
		Seen   int64
		Points *int
	}
	// SetEstimate sets the original estimate in minutes, nil to clear it.
	SetEstimate struct {
		By       tabs.By
		Issue    string
		Seen     int64
		Estimate *int
	}
	// SetRemaining sets the remaining estimate in minutes.
	SetRemaining struct {
		By        tabs.By
		Issue     string
		Seen      int64
		Remaining *int
	}
	// SetSprint puts an issue in a sprint, or in the backlog when Sprint
	// is empty (FR-46).
	SetSprint struct {
		By     tabs.By
		Issue  string
		Seen   int64
		Sprint string
	}
)

func (c SetSummary) Validate() error     { return checkBy(c.By) }
func (c SetDescription) Validate() error { return checkBy(c.By) }
func (c SetType) Validate() error        { return checkBy(c.By) }
func (c SetStatus) Validate() error      { return checkBy(c.By) }
func (c SetPriority) Validate() error    { return checkBy(c.By) }
func (c SetDue) Validate() error         { return checkBy(c.By) }
func (c SetPoints) Validate() error      { return checkBy(c.By) }
func (c SetEstimate) Validate() error    { return checkBy(c.By) }
func (c SetRemaining) Validate() error   { return checkBy(c.By) }
func (c SetSprint) Validate() error      { return checkBy(c.By) }

func (c Assign) Validate() error {
	if !validPerson(c.Assignee) {
		return errors.New("tracker: a malformed assignee")
	}
	return checkBy(c.By)
}

func (c SetParent) Validate() error {
	if c.Parent != "" && !ValidID(c.Parent) {
		return errors.New("tracker: a malformed parent")
	}
	return checkBy(c.By)
}

func (c SetSummary) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldSummary, Seen: c.Seen, Text: c.Summary})
}

func (c SetDescription) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldDescription, Seen: c.Seen, Doc: c.Description})
}

func (c SetType) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldType, Seen: c.Seen, Text: string(c.Type)})
}

func (c SetStatus) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldStatus, Seen: c.Seen, Text: c.Status})
}

func (c SetPriority) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldPriority, Seen: c.Seen, Text: string(c.Priority)})
}

func (c Assign) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldAssignee, Seen: c.Seen, Text: c.Assignee})
}

func (c SetParent) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldParent, Seen: c.Seen, Text: c.Parent})
}

func (c SetDue) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldDue, Seen: c.Seen, Text: strings.TrimSpace(c.Due)})
}

func (c SetPoints) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldPoints, Seen: c.Seen, Num: c.Points})
}

func (c SetEstimate) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldEstimate, Seen: c.Seen, Num: c.Estimate})
}

func (c SetRemaining) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldRemaining, Seen: c.Seen, Num: c.Remaining})
}

func (c SetSprint) Apply(tx *writer.Tx) error {
	return executeEdit(tx, c.By, c.Issue, edit{Field: FieldSprint, Seen: c.Seen, Text: c.Sprint})
}

// SetLabels adds and removes labels (FR-23); a new word becomes a label
// of the project. Sets merge, so it carries no Seen.
type SetLabels struct {
	By     tabs.By
	Issue  string
	Add    []string
	Remove []string
}

func (c SetLabels) Validate() error { return checkBy(c.By) }

type setLabels struct{ add, remove []string }

var labelsDecider = issueDecider(func(c setLabels, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted:
		return nil, errDeleted
	}
	now, err := labelSet(s.Labels, c.add, c.remove)
	if err != nil {
		return nil, err
	}
	e := LabelsChanged{Added: added(s.Labels, now), Removed: added(now, s.Labels)}
	if len(e.Added) == 0 && len(e.Removed) == 0 {
		return nil, nil
	}
	return []IssueEvent{e}, nil
})

func (c SetLabels) Apply(tx *writer.Tx) error {
	s, err := issueState(tx, c.Issue)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.EditIssues, s.Project); err != nil {
		return err
	}
	if err := events.Execute(Store(), tx, labelsDecider, IssueStream.ID(c.Issue), events.Any, setLabels{c.Add, c.Remove}, meta(c.By)); err != nil {
		return err
	}
	return acted(tx, s.Project, c.By.Actor.ID, at(c.By))
}

// SetComponents puts an issue in components of its project and takes it
// out of others (FR-24).
type SetComponents struct {
	By     tabs.By
	Issue  string
	Add    []string
	Remove []string
}

func (c SetComponents) Validate() error { return checkBy(c.By) }

type setComponents struct {
	add, remove []string
	defined     []Component
	key         string
	force       bool // removing a component the project no longer has
}

var componentsDecider = issueDecider(func(c setComponents, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted && !c.force:
		return nil, errDeleted
	}
	p := ProjectState{Components: c.defined}
	canon := func(names []string, check bool) ([]string, error) {
		var out []string
		for _, n := range names {
			comp, ok := p.Component(n)
			switch {
			case ok:
				n = comp.Name
			case check:
				return nil, tabs.Refuse(FieldComponents, fmt.Sprintf("%s has no component %s.", c.key, strings.TrimSpace(n)))
			}
			out = append(out, n)
		}
		return out, nil
	}
	add, err := canon(c.add, true)
	if err != nil {
		return nil, err
	}
	remove, _ := canon(c.remove, false)
	now := applySet(s.Components, add, remove)
	if len(now) > MaxComponents {
		return nil, tabs.Refuse(FieldComponents, "An issue is in 20 components at most.")
	}
	e := ComponentsChanged{Added: added(s.Components, now), Removed: added(now, s.Components)}
	if len(e.Added) == 0 && len(e.Removed) == 0 {
		return nil, nil
	}
	return []IssueEvent{e}, nil
})

func (c SetComponents) Apply(tx *writer.Tx) error {
	s, err := issueState(tx, c.Issue)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.EditIssues, s.Project); err != nil {
		return err
	}
	p, err := projectState(tx, s.Project)
	if err != nil {
		return err
	}
	cmd := setComponents{add: c.Add, remove: c.Remove, defined: p.Components, key: p.Key}
	if err := events.Execute(Store(), tx, componentsDecider, IssueStream.ID(c.Issue), events.Any, cmd, meta(c.By)); err != nil {
		return err
	}
	return acted(tx, s.Project, c.By.Actor.ID, at(c.By))
}

// Watch starts or stops the actor watching an issue (FR-27.1).
type Watch struct {
	By    tabs.By
	Issue string
	Watch bool
}

func (c Watch) Validate() error { return checkBy(c.By) }

type watch struct {
	add    []string
	remove string
}

var watchDecider = issueDecider(func(c watch, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted:
		return nil, errDeleted
	case c.remove != "":
		if !slices.Contains(s.Watchers, c.remove) {
			return nil, nil
		}
		return []IssueEvent{WatcherRemoved{Person: c.remove}}, nil
	}
	return newWatchers(s, c.add...), nil
})

func (c Watch) Apply(tx *writer.Tx) error {
	s, err := issueState(tx, c.Issue)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.Read, s.Project); err != nil {
		return err
	}
	cmd := watch{add: []string{c.By.Actor.ID}}
	if !c.Watch {
		cmd = watch{remove: c.By.Actor.ID}
	}
	return events.Execute(Store(), tx, watchDecider, IssueStream.ID(c.Issue), events.Any, cmd, meta(c.By))
}

// LinkIssues links an issue to another (FR-22): "WEB-3 blocks WEB-4" is
// LinkIssues{Issue: WEB-3, Type: Blocks, Other: WEB-4}. Both issues
// store the link.
type LinkIssues struct {
	By    tabs.By
	Issue string
	Type  LinkType
	Other string
}

// UnlinkIssues removes a link from both issues.
type UnlinkIssues struct {
	By    tabs.By
	Issue string
	Type  LinkType
	Other string
}

func (c LinkIssues) Validate() error   { return checkBy(c.By) }
func (c UnlinkIssues) Validate() error { return checkBy(c.By) }

type link struct {
	Link
	remove bool
}

var linkDecider = issueDecider(func(c link, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted && !c.remove:
		return nil, errDeleted
	}
	i := slices.IndexFunc(s.Links, func(l Link) bool { return l.Type == c.Type && l.Issue == c.Issue })
	if c.remove {
		if i < 0 {
			return nil, nil
		}
		l := s.Links[i]
		return []IssueEvent{LinkRemoved{Link: l.Type, Outward: l.Outward, Issue: l.Issue}}, nil
	}
	if i >= 0 {
		return nil, tabs.Refuse(FieldLinks, "These issues are linked so already.")
	}
	return []IssueEvent{LinkAdded{Link: c.Type, Outward: c.Outward, Issue: c.Issue}}, nil
})

// linkBoth reads both issues and checks the rights for a link between
// them.
func linkBoth(tx *writer.Tx, by tabs.By, id, other string, t LinkType) (IssueState, error) {
	if !t.Valid() {
		return IssueState{}, tabs.Refuse(FieldLinks, "Pick blocks, relates to or duplicates.")
	}
	s, err := issueState(tx, id)
	if err != nil {
		return s, err
	}
	if err := require(tx, by, access.EditIssues, s.Project); err != nil {
		return s, err
	}
	if other == id {
		return s, tabs.Refuse(FieldLinks, "An issue cannot link to itself.")
	}
	o, err := issueState(tx, other)
	if errors.Is(err, access.ErrNotFound) {
		return s, tabs.Refuse(FieldLinks, "That issue does not exist.")
	}
	if err != nil {
		return s, err
	}
	if o.Project != s.Project {
		if err := require(tx, by, access.Read, o.Project); err != nil {
			if errors.Is(err, access.ErrNotFound) {
				return s, tabs.Refuse(FieldLinks, "That issue does not exist.")
			}
			return s, err
		}
	}
	return s, nil
}

func (c LinkIssues) Apply(tx *writer.Tx) error {
	s, err := linkBoth(tx, c.By, c.Issue, c.Other, c.Type)
	if err != nil {
		return err
	}
	m := meta(c.By)
	if err := events.Execute(Store(), tx, linkDecider, IssueStream.ID(c.Issue), events.Any, link{Link: Link{Type: c.Type, Outward: true, Issue: c.Other}}, m); err != nil {
		return err
	}
	if err := events.Execute(Store(), tx, linkDecider, IssueStream.ID(c.Other), events.Any, link{Link: Link{Type: c.Type, Outward: false, Issue: c.Issue}}, m); err != nil {
		if errors.Is(err, errDeleted) {
			return tabs.Refuse(FieldLinks, "That issue was deleted.")
		}
		return err
	}
	return acted(tx, s.Project, c.By.Actor.ID, at(c.By))
}

func (c UnlinkIssues) Apply(tx *writer.Tx) error {
	s, err := linkBoth(tx, c.By, c.Issue, c.Other, c.Type)
	if err != nil {
		return err
	}
	m := meta(c.By)
	for _, x := range [][2]string{{c.Issue, c.Other}, {c.Other, c.Issue}} {
		cmd := link{Link: Link{Type: c.Type, Issue: x[1]}, remove: true}
		if err := events.Execute(Store(), tx, linkDecider, IssueStream.ID(x[0]), events.Any, cmd, m); err != nil {
			return err
		}
	}
	return acted(tx, s.Project, c.By.Actor.ID, at(c.By))
}

// Move moves a card on the board or in the backlog (FR-41, FR-45): to
// Status (empty: the status it has), between the cards After and Before
// (issue ids; either may be empty: the top or the bottom). Status and
// rank change in one event. A card someone else moved after Seen refuses
// the move with a *Conflict of the field "move": "Sunny Otter moved
// WEB-12 to In Review first." (MP-04.3).
type Move struct {
	By     tabs.By
	Issue  string
	Status string
	Before string
	After  string
	Seen   int64
}

func (c Move) Validate() error { return checkBy(c.By) }

type move struct {
	Status       string
	Seen         int64
	lower, upper string
	wf           Workflow
	own          []string
}

var moveDecider = issueDecider(decideMove)

func decideMove(c move, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted:
		return nil, errDeleted
	}
	st, ok := c.wf.Status(c.Status)
	if !ok {
		return nil, tabs.Refuse(FieldMove, "This project's workflow has no such status.")
	}
	if !c.wf.Allows(s.Status, c.Status) {
		return nil, tabs.Refuse(FieldMove, followRule(c.wf, c.Status))
	}
	fits := s.Rank > c.lower && (c.upper == "" || s.Rank < c.upper)
	if fits && c.Status == s.Status {
		return nil, nil
	}
	var last int64
	for _, f := range []string{FieldStatus, FieldRank} {
		if v := s.Changed[f]; v > c.Seen && !slices.Contains(c.own, f) {
			last = max(last, v)
		}
	}
	if last > 0 {
		return nil, &conflictErr{Field: FieldMove, Version: last, Yours: c.Status}
	}
	rank := s.Rank
	if !fits {
		var err error
		if rank, err = RankBetween(c.lower, c.upper); err != nil {
			return nil, err
		}
	}
	return []IssueEvent{Moved{FromStatus: s.Status, Status: c.Status, Category: st.Category, FromRank: s.Rank, Rank: rank}}, nil
}

func (c Move) Apply(tx *writer.Tx) error {
	s, err := issueState(tx, c.Issue)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.EditIssues, s.Project); err != nil {
		return err
	}
	p, err := projectState(tx, s.Project)
	if err != nil {
		return err
	}
	m := move{Status: c.Status, Seen: c.Seen, wf: p.Workflow}
	if m.Status == "" {
		m.Status = s.Status
	}
	if m.lower, m.upper, err = neighbours(tx, s.Project, c.Issue, c.Before, c.After); err != nil {
		return err
	}
	if m.own, err = ownChanges(tx, c.By, c.Issue, s, c.Seen, FieldStatus, FieldRank); err != nil {
		return err
	}
	err = events.Execute(Store(), tx, moveDecider, IssueStream.ID(c.Issue), events.Any, m, meta(c.By))
	var ce *conflictErr
	switch {
	case errors.As(err, &ce):
		return moveConflict(tx, c.By, c.Issue, s, p, ce)
	case err != nil:
		return err
	}
	return acted(tx, s.Project, c.By.Actor.ID, at(c.By))
}

// moveConflict is MP-04.3's refusal.
func moveConflict(tx *writer.Tx, by tabs.By, id string, s IssueState, p ProjectState, ce *conflictErr) error {
	ctx := tx.Context()
	m, err := eventMeta(ctx, tx, IssueStream.ID(id), ce.Version)
	if err != nil {
		return err
	}
	who, err := people.Get(ctx, tx, m.Actor)
	if err != nil {
		return err
	}
	theirs := p.Workflow.Name(s.Status)
	msg := fmt.Sprintf("%s moved %s to %s first.", who.Name, s.Key, theirs)
	return &Conflict{Kind: "conflict", Field: FieldMove, Label: "move", By: m.Actor, Name: who.Name, At: m.At.UnixMilli(),
		Theirs: theirs, Yours: p.Workflow.Name(ce.Yours), Version: s.Version, Key: s.Key, Status: s.Status,
		Refusal: writer.Refusal{Msg: msg}}
}

// neighbours finds the ranks a moved card goes between: after the card
// After and before the next card of the project, so that ranks stay
// distinct, or before the card Before, or at the bottom.
func neighbours(tx *writer.Tx, project, self, before, after string) (lower, upper string, err error) {
	rankOf := func(id string) (string, error) {
		if id == "" || id == self {
			return "", nil
		}
		var r string
		err := tx.QueryRow(`SELECT rank FROM issues WHERE id = ? AND project_id = ? AND deleted_at IS NULL`, id, project).Scan(&r)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return r, err
	}
	if lower, err = rankOf(after); err != nil {
		return "", "", err
	}
	if upper, err = rankOf(before); err != nil {
		return "", "", err
	}
	if lower != "" && upper != "" && lower >= upper {
		upper = "" // the page showed an order that changed since: After wins
	}
	var r sql.NullString
	switch {
	case lower != "":
		err = tx.QueryRow(`SELECT min(rank) FROM issues WHERE project_id = ? AND id != ? AND deleted_at IS NULL AND rank > ?`, project, self, lower).Scan(&r)
		if r.Valid && (upper == "" || r.String < upper) {
			upper = r.String
		}
	case upper != "":
		err = tx.QueryRow(`SELECT max(rank) FROM issues WHERE project_id = ? AND id != ? AND deleted_at IS NULL AND rank < ?`, project, self, upper).Scan(&r)
		lower = r.String
	default:
		err = tx.QueryRow(`SELECT max(rank) FROM issues WHERE project_id = ? AND id != ? AND deleted_at IS NULL`, project, self).Scan(&r)
		lower = r.String
	}
	return lower, upper, err
}

// DeleteIssue deletes an issue (FR-28): it leaves every list, board and
// search; its history, number and key stay, and the key answers that it
// was deleted (FR-12.2).
type DeleteIssue struct {
	By    tabs.By
	Issue string
}

func (c DeleteIssue) Validate() error { return checkBy(c.By) }

type deleteIssue struct{ children int }

var deleteDecider = issueDecider(func(c deleteIssue, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted:
		return nil, nil
	case c.children == 1:
		return nil, &writer.Refusal{Msg: "Delete or move its child issue first."}
	case c.children > 1:
		return nil, &writer.Refusal{Msg: fmt.Sprintf("Delete or move its %d child issues first.", c.children)}
	}
	return []IssueEvent{IssueDeleted{}}, nil
})

func (c DeleteIssue) Apply(tx *writer.Tx) error {
	s, err := issueState(tx, c.Issue)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.DeleteIssue, s.Project); err != nil {
		return err
	}
	n, err := childCount(tx, c.Issue)
	if err != nil {
		return err
	}
	if err := events.Execute(Store(), tx, deleteDecider, IssueStream.ID(c.Issue), events.Any, deleteIssue{n}, meta(c.By)); err != nil {
		return err
	}
	if err := setParentRow(tx, c.Issue, ""); err != nil {
		return err
	}
	return acted(tx, s.Project, c.By.Actor.ID, at(c.By))
}

// MoveToProject moves an issue to another project (FR-29): it gets the
// next number there, and its old key keeps opening it. Status is the
// status in the target's workflow, needed when that workflow lacks the
// issue's. The issue leaves its epic, components and sprint behind.
// Its result is the new key as JSON: {"id": "...", "key": "APP-7"}.
type MoveToProject struct {
	By      tabs.By
	Issue   string
	Project string
	Status  string
}

func (c MoveToProject) Validate() error { return checkBy(c.By) }

type projectMove struct {
	target   ProjectState
	project  string
	status   string
	rank     string
	children int
}

var projectMoveDecider = issueDecider(func(c projectMove, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted:
		return nil, errDeleted
	case c.project == s.Project:
		return nil, nil
	case c.target.Archived:
		return nil, errArchived
	case s.Type == Subtask:
		return nil, tabs.Refuse(FieldProject, "A subtask stays in its parent's project.")
	case c.children == 1:
		return nil, tabs.Refuse(FieldProject, "Move the child issue first.")
	case c.children > 1:
		return nil, tabs.Refuse(FieldProject, fmt.Sprintf("Move the %d child issues first.", c.children))
	}
	status := c.status
	if status == "" {
		status = s.Status
	}
	st, ok := c.target.Workflow.Status(status)
	if !ok {
		return nil, tabs.Refuse(FieldStatus, fmt.Sprintf("Pick a status of %s's workflow.", c.target.Name))
	}
	var evs []IssueEvent
	if s.Parent != "" {
		evs = append(evs, ParentChanged{From: s.Parent})
	}
	if len(s.Components) > 0 {
		evs = append(evs, ComponentsChanged{Removed: s.Components})
	}
	if s.Sprint != "" {
		evs = append(evs, SprintChanged{From: s.Sprint})
	}
	number := c.target.Last + 1
	return append(evs, ProjectChanged{FromProject: s.Project, Project: c.project, FromKey: s.Key,
		Key: IssueKey(c.target.Key, number), Number: number, Status: status, Category: st.Category, Rank: c.rank}), nil
})

func (c MoveToProject) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c MoveToProject) ApplyResult(tx *writer.Tx) ([]byte, error) {
	s, err := issueState(tx, c.Issue)
	if err != nil {
		return nil, err
	}
	if err := require(tx, c.By, access.EditIssues, s.Project); err != nil {
		return nil, err
	}
	if c.Project == s.Project {
		return json.Marshal(map[string]string{"id": c.Issue, "key": s.Key})
	}
	p, err := projectState(tx, c.Project)
	if err != nil {
		return nil, err
	}
	if err := require(tx, c.By, access.EditIssues, c.Project); err != nil {
		return nil, err
	}
	pm := projectMove{target: p, project: c.Project, status: c.Status}
	if pm.children, err = childCount(tx, c.Issue); err != nil {
		return nil, err
	}
	last, err := lastRank(tx, c.Project)
	if err != nil {
		return nil, err
	}
	if pm.rank, err = RankBetween(last, ""); err != nil {
		return nil, err
	}
	m := meta(c.By)
	if err := events.Execute(Store(), tx, projectMoveDecider, IssueStream.ID(c.Issue), events.Any, pm, m); err != nil {
		return nil, err
	}
	_, key, err := assignNumber(tx, p, c.Project, c.Issue, m)
	if err != nil {
		return nil, err
	}
	if err := setParentRow(tx, c.Issue, ""); err != nil {
		return nil, err
	}
	if err := acted(tx, c.Project, c.By.Actor.ID, at(c.By)); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": c.Issue, "key": key})
}
