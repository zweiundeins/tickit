package tracker

import (
	"bytes"
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/people"
)

// Entry is one change of an issue's history (FR-68.1): who changed which
// field when, from what to what, as a person reads the values (names,
// keys, "2h 30m"). Field is one of the Field constants, or "created" and
// "deleted". A description's change has no values, only the others who
// edited that version with the actor (New): "changed the description",
// compared at /issues/<id>/description?v=<Version>; a version of the
// co-edited description that changed nothing is left out. OldRaw and
// NewRaw are the values as stored: ids, dates.
type Entry struct {
	Version  int64
	Position int64
	Actor    string
	At       time.Time
	Type     string // the event type: "issue.priority_changed"
	Field    string
	Old      string
	New      string
	OldRaw   string
	NewRaw   string
}

// The Field of the entries that change no one field.
const (
	EntryCreated = "created"
	EntryDeleted = "deleted"
)

// pending is an entry whose values still need names.
type pending struct {
	Entry
	project string   // whose workflow names the statuses
	people  []string // for watchers: the ids of New, or of Old
	outward bool     // for links: how the issue holds it
}

// History reads an issue's history from its stream, oldest first.
func History(ctx context.Context, q sqlite.Querier, issue string) ([]Entry, error) {
	var out []pending
	var s IssueState
	personIDs, issueIDs, sprintIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for r, err := range events.Load(ctx, Store(), q, IssueStream.ID(issue), 0) {
		if err != nil {
			return nil, err
		}
		base := Entry{Version: r.Version, Position: r.Position, Actor: r.Actor, At: r.At, Type: r.Type}
		personIDs[r.Actor] = true
		add := func(field, old, new string) {
			e := base
			e.Field, e.OldRaw, e.NewRaw = field, old, new
			out = append(out, pending{Entry: e, project: s.Project})
		}
		switch e := r.Data.(type) {
		case IssueCreated:
			add(EntryCreated, "", e.Key)
		case SummaryChanged:
			add(FieldSummary, e.From, e.To)
		case DescriptionChanged:
			if bytes.Equal(canonical(e.To), canonical(s.Description)) {
				break // a version of the co-edited description that changed nothing
			}
			var others []string
			for _, p := range e.Editors {
				if p != r.Actor {
					others = append(others, p)
					personIDs[p] = true
				}
			}
			add(FieldDescription, "", strings.Join(others, " "))
			out[len(out)-1].people = others
		case TypeChanged:
			add(FieldType, string(e.From), string(e.To))
		case StatusChanged:
			add(FieldStatus, e.From, e.To)
		case CategoryChanged:
			add(FieldCategory, string(e.From), string(e.To))
		case Moved:
			if e.FromStatus != e.Status {
				add(FieldStatus, e.FromStatus, e.Status)
			} else {
				add(FieldRank, e.FromRank, e.Rank)
			}
		case PriorityChanged:
			add(FieldPriority, string(e.From), string(e.To))
		case AssigneeChanged:
			add(FieldAssignee, e.From, e.To)
			personIDs[e.From], personIDs[e.To] = true, true
		case ParentChanged:
			add(FieldParent, e.From, e.To)
			issueIDs[e.From], issueIDs[e.To] = true, true
		case DueChanged:
			add(FieldDue, e.From, e.To)
		case PointsChanged:
			add(FieldPoints, intString(e.From), intString(e.To))
		case EstimateChanged:
			add(FieldEstimate, intString(e.From), intString(e.To))
		case RemainingChanged:
			add(FieldRemaining, intString(e.From), intString(e.To))
		case SprintChanged:
			add(FieldSprint, e.From, e.To)
			sprintIDs[e.From], sprintIDs[e.To] = true, true
		case LabelsChanged:
			add(FieldLabels, strings.Join(e.Removed, ", "), strings.Join(e.Added, ", "))
		case ComponentsChanged:
			add(FieldComponents, strings.Join(e.Removed, ", "), strings.Join(e.Added, ", "))
		case WatchersAdded:
			add(FieldWatchers, "", strings.Join(e.People, " "))
			out[len(out)-1].people = e.People
			for _, p := range e.People {
				personIDs[p] = true
			}
		case WatcherRemoved:
			add(FieldWatchers, e.Person, "")
			out[len(out)-1].people = []string{e.Person}
			personIDs[e.Person] = true
		case LinkAdded:
			add(FieldLinks, "", string(e.Link)+" "+e.Issue)
			out[len(out)-1].outward = e.Outward
			issueIDs[e.Issue] = true
		case LinkRemoved:
			add(FieldLinks, string(e.Link)+" "+e.Issue, "")
			out[len(out)-1].outward = e.Outward
			issueIDs[e.Issue] = true
		case PageLinked:
			add(FieldPages, "", e.Title)
		case PageUnlinked:
			add(FieldPages, e.Title, "")
		case IssueDeleted:
			add(EntryDeleted, "", "")
		case ProjectChanged:
			add(FieldProject, e.FromKey, e.Key)
			out[len(out)-1].project = e.Project
		}
		s = evolveIssue(s, r.Data)
	}
	if len(out) == 0 {
		return nil, nil
	}
	names, err := lookupNames(ctx, q, personIDs, issueIDs, sprintIDs, out)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(out))
	for i, p := range out {
		e := p.Entry
		e.Old, e.New = names.show(p, p.OldRaw, false), names.show(p, p.NewRaw, true)
		entries[i] = e
	}
	return entries, nil
}

// names are what History's values show as.
type names struct {
	people    map[string]people.Person
	keys      map[string]string
	sprints   map[string]string
	workflows map[string]Workflow
}

func lookupNames(ctx context.Context, q sqlite.Querier, personIDs, issueIDs, sprintIDs map[string]bool, out []pending) (names, error) {
	n := names{keys: map[string]string{}, sprints: map[string]string{}, workflows: map[string]Workflow{}}
	var ids []string
	for id := range personIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	var err error
	if n.people, err = people.Lookup(ctx, q, ids...); err != nil {
		return n, err
	}
	for id := range issueIDs {
		if id == "" {
			continue
		}
		var key string
		if q.QueryRowContext(ctx, `SELECT key FROM issues WHERE id = ?`, id).Scan(&key) == nil {
			n.keys[id] = key
		}
	}
	for id := range sprintIDs {
		if id == "" {
			continue
		}
		var name string
		if q.QueryRowContext(ctx, `SELECT name FROM sprints WHERE id = ?`, id).Scan(&name) == nil {
			n.sprints[id] = name
		}
	}
	for _, p := range out {
		if _, ok := n.workflows[p.project]; ok || p.project == "" {
			continue
		}
		w, err := allStatuses(ctx, q, p.project)
		if err != nil {
			return n, err
		}
		n.workflows[p.project] = w
	}
	return n, nil
}

func (n names) show(p pending, raw string, isNew bool) string {
	if raw == "" {
		return ""
	}
	person := func(id string) string { return n.people[id].Name }
	switch p.Field {
	case FieldType:
		return IssueType(raw).Name()
	case FieldStatus:
		return n.workflows[p.project].Name(raw)
	case FieldCategory:
		return Category(raw).Name()
	case FieldPriority:
		return Priority(raw).Name()
	case FieldAssignee:
		return person(raw)
	case FieldParent:
		if k, ok := n.keys[raw]; ok {
			return k
		}
	case FieldDue:
		return Date(raw)
	case FieldEstimate, FieldRemaining:
		var m int
		for _, r := range raw {
			m = m*10 + int(r-'0')
		}
		return Minutes(m)
	case FieldSprint:
		if s, ok := n.sprints[raw]; ok {
			return s
		}
	case FieldWatchers, FieldDescription:
		var out []string
		for _, id := range p.people {
			out = append(out, person(id))
		}
		return strings.Join(out, ", ")
	case FieldLinks:
		t, id, _ := strings.Cut(raw, " ")
		key := n.keys[id]
		return LinkType(t).Wording(p.outward) + " " + key
	}
	return raw
}

// ActivityItem is an entry of an issue's Activity tab (FR-68.3): a change
// or a comment.
type ActivityItem struct {
	At      time.Time
	Entry   *Entry
	Comment *Comment
}

// Activity interleaves an issue's history and comments in time order;
// the history's "created" comes first.
func Activity(ctx context.Context, q sqlite.Querier, issue string) ([]ActivityItem, error) {
	hist, err := History(ctx, q, issue)
	if err != nil {
		return nil, err
	}
	comments, err := Comments(ctx, q, issue)
	if err != nil {
		return nil, err
	}
	var out []ActivityItem
	for i := range hist {
		out = append(out, ActivityItem{At: hist[i].At, Entry: &hist[i]})
	}
	for i := range comments {
		out = append(out, ActivityItem{At: comments[i].Created, Comment: &comments[i]})
	}
	slices.SortStableFunc(out, func(a, b ActivityItem) int { return cmp.Compare(a.At.UnixMilli(), b.At.UnixMilli()) })
	return out, nil
}

// StateAt returns an issue as it stood at a version of its stream
// (FR-70), and the version: the newest when version is 0 or beyond it.
// An issue without events answers access.ErrNotFound.
func StateAt(ctx context.Context, q sqlite.Querier, issue string, version int64) (IssueState, int64, error) {
	var s IssueState
	for r, err := range events.Load(ctx, Store(), q, IssueStream.ID(issue), 0) {
		if err != nil {
			return s, 0, err
		}
		if version > 0 && r.Version > version {
			break
		}
		s = evolveIssue(s, r.Data)
	}
	if !s.Exists {
		return s, 0, notFound("issue", issue)
	}
	return s, s.Version, nil
}

// allStatuses is a project's statuses, those removed too: the history
// names the statuses an issue held (FR-36.2).
func allStatuses(ctx context.Context, q sqlite.Querier, project string) (Workflow, error) {
	var w Workflow
	rows, err := q.QueryContext(ctx, `SELECT id, name, category FROM statuses WHERE project_id = ? ORDER BY removed, position`, project)
	if err != nil {
		return w, err
	}
	defer rows.Close()
	for rows.Next() {
		var s Status
		if err := rows.Scan(&s.ID, &s.Name, &s.Category); err != nil {
			return w, err
		}
		w.Statuses = append(w.Statuses, s)
	}
	return w, rows.Err()
}
