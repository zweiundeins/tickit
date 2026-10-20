package tracker

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"
)

// IssueEvent is an event of an issue's stream.
type IssueEvent interface{ issueEvent() }

// IssueCreated starts an issue's stream (FR-10). The description is a
// document (PlainDoc); Watchers are the reporter, the assignee and the
// people the description mentions (FR-27.2), Mentions those it mentions.
type IssueCreated struct {
	Project     string          `json:"project"`
	Number      int64           `json:"number"`
	Key         string          `json:"key"`
	Type        IssueType       `json:"type"`
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description,omitempty"`
	Status      string          `json:"status"`
	Category    Category        `json:"category"`
	Priority    Priority        `json:"priority"`
	Assignee    string          `json:"assignee,omitempty"`
	Reporter    string          `json:"reporter"`
	Labels      []string        `json:"labels,omitempty"`
	Components  []string        `json:"components,omitempty"`
	Parent      string          `json:"parent,omitempty"`
	Due         string          `json:"due,omitempty"`
	Points      *int            `json:"points,omitempty"`
	Estimate    *int            `json:"estimate,omitempty"`
	Remaining   *int            `json:"remaining,omitempty"`
	Rank        string          `json:"rank"`
	Sprint      string          `json:"sprint,omitempty"`
	Watchers    []string        `json:"watchers,omitempty"`
	Mentions    []string        `json:"mentions,omitempty"`
}

// The changes of one field each, with the value before and after.
type (
	SummaryChanged struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	// DescriptionChanged is a version of the description (FR-15,
	// description.go): the document, the people it mentions for the
	// first time, whom notifications tell once (FR-67.3), the version of
	// the description's own stream it was cut at, and who edited it
	// since the version before.
	DescriptionChanged struct {
		To       json.RawMessage `json:"to,omitempty"`
		Mentions []string        `json:"mentions_added,omitempty"`
		Upto     int64           `json:"upto,omitempty"`
		Editors  []string        `json:"editors,omitempty"`
	}
	TypeChanged struct {
		From IssueType `json:"from"`
		To   IssueType `json:"to"`
	}
	// StatusChanged is a change of status from the issue's menu (FR-16).
	StatusChanged struct {
		From     string   `json:"from"`
		To       string   `json:"to"`
		Category Category `json:"category"`
	}
	// Moved is a move on the board or in the backlog (FR-41, FR-45): a new
	// rank, and a new status when it went to another column.
	Moved struct {
		FromStatus string   `json:"from_status"`
		Status     string   `json:"status"`
		Category   Category `json:"category"`
		FromRank   string   `json:"from_rank"`
		Rank       string   `json:"rank"`
	}
	PriorityChanged struct {
		From Priority `json:"from"`
		To   Priority `json:"to"`
	}
	AssigneeChanged struct {
		From string `json:"from,omitempty"`
		To   string `json:"to,omitempty"`
	}
	ParentChanged struct {
		From string `json:"from,omitempty"`
		To   string `json:"to,omitempty"`
	}
	DueChanged struct {
		From string `json:"from,omitempty"`
		To   string `json:"to,omitempty"`
	}
	PointsChanged struct {
		From *int `json:"from,omitempty"`
		To   *int `json:"to,omitempty"`
	}
	// EstimateChanged is the original estimate, in minutes (FR-26).
	EstimateChanged struct {
		From *int `json:"from,omitempty"`
		To   *int `json:"to,omitempty"`
	}
	// RemainingChanged is the remaining estimate, in minutes.
	RemainingChanged struct {
		From *int `json:"from,omitempty"`
		To   *int `json:"to,omitempty"`
	}
	SprintChanged struct {
		From string `json:"from,omitempty"`
		To   string `json:"to,omitempty"`
	}
	LabelsChanged struct {
		Added   []string `json:"added,omitempty"`
		Removed []string `json:"removed,omitempty"`
	}
	ComponentsChanged struct {
		Added   []string `json:"added,omitempty"`
		Removed []string `json:"removed,omitempty"`
	}
	WatchersAdded struct {
		People []string `json:"people"`
	}
	WatcherRemoved struct {
		Person string `json:"person"`
	}
	// LinkAdded is a link stored on both issues: Outward on the one that
	// names it, so that the other reads it inverted ("is blocked by").
	LinkAdded struct {
		Link    LinkType `json:"link"`
		Outward bool     `json:"outward"`
		Issue   string   `json:"issue"`
	}
	LinkRemoved struct {
		Link    LinkType `json:"link"`
		Outward bool     `json:"outward"`
		Issue   string   `json:"issue"`
	}
	// IssueDeleted deletes an issue (FR-28): its number, key and history
	// stay.
	IssueDeleted struct{}
	// ProjectChanged moves an issue to another project with a new number
	// (FR-29); its old key keeps resolving.
	ProjectChanged struct {
		FromProject string   `json:"from_project"`
		Project     string   `json:"project"`
		FromKey     string   `json:"from_key"`
		Key         string   `json:"key"`
		Number      int64    `json:"number"`
		Status      string   `json:"status"`
		Category    Category `json:"category"`
		Rank        string   `json:"rank"`
	}
)

func (IssueCreated) issueEvent()       {}
func (SummaryChanged) issueEvent()     {}
func (DescriptionChanged) issueEvent() {}
func (TypeChanged) issueEvent()        {}
func (StatusChanged) issueEvent()      {}
func (Moved) issueEvent()              {}
func (PriorityChanged) issueEvent()    {}
func (AssigneeChanged) issueEvent()    {}
func (ParentChanged) issueEvent()      {}
func (DueChanged) issueEvent()         {}
func (PointsChanged) issueEvent()      {}
func (EstimateChanged) issueEvent()    {}
func (RemainingChanged) issueEvent()   {}
func (SprintChanged) issueEvent()      {}
func (LabelsChanged) issueEvent()      {}
func (ComponentsChanged) issueEvent()  {}
func (WatchersAdded) issueEvent()      {}
func (WatcherRemoved) issueEvent()     {}
func (LinkAdded) issueEvent()          {}
func (LinkRemoved) issueEvent()        {}
func (IssueDeleted) issueEvent()       {}
func (ProjectChanged) issueEvent()     {}

// IssueStream is the kind of the issues' streams, keyed by issue id.
var IssueStream = events.NewStream[string, IssueEvent]("issue")

func defineIssue[T IssueEvent](name string) events.Type[string, IssueEvent, T] {
	return events.Define(IssueStream, name, func(t T) IssueEvent { return t })
}

// The issue event types; the names are what the log stores.
var (
	IssueCreatedType       = defineIssue[IssueCreated]("issue.created")
	SummaryChangedType     = defineIssue[SummaryChanged]("issue.summary_changed")
	DescriptionChangedType = defineIssue[DescriptionChanged]("issue.description_changed")
	TypeChangedType        = defineIssue[TypeChanged]("issue.type_changed")
	StatusChangedType      = defineIssue[StatusChanged]("issue.status_changed")
	MovedType              = defineIssue[Moved]("issue.moved")
	PriorityChangedType    = defineIssue[PriorityChanged]("issue.priority_changed")
	AssigneeChangedType    = defineIssue[AssigneeChanged]("issue.assignee_changed")
	ParentChangedType      = defineIssue[ParentChanged]("issue.parent_changed")
	DueChangedType         = defineIssue[DueChanged]("issue.due_changed")
	PointsChangedType      = defineIssue[PointsChanged]("issue.points_changed")
	EstimateChangedType    = defineIssue[EstimateChanged]("issue.estimate_changed")
	RemainingChangedType   = defineIssue[RemainingChanged]("issue.remaining_changed")
	SprintChangedType      = defineIssue[SprintChanged]("issue.sprint_changed")
	LabelsChangedType      = defineIssue[LabelsChanged]("issue.labels_changed")
	ComponentsChangedType  = defineIssue[ComponentsChanged]("issue.components_changed")
	WatchersAddedType      = defineIssue[WatchersAdded]("issue.watchers_added")
	WatcherRemovedType     = defineIssue[WatcherRemoved]("issue.watcher_removed")
	LinkAddedType          = defineIssue[LinkAdded]("issue.link_added")
	LinkRemovedType        = defineIssue[LinkRemoved]("issue.link_removed")
	IssueDeletedType       = defineIssue[IssueDeleted]("issue.deleted")
	ProjectChangedType     = defineIssue[ProjectChanged]("issue.project_changed")
)

// The fields of an issue that a change names, as the page's controls do
// and conflicts (MP-08) and history entries (FR-68) report them.
const (
	FieldSummary     = "summary"
	FieldDescription = "description"
	FieldType        = "type"
	FieldStatus      = "status"
	FieldPriority    = "priority"
	FieldAssignee    = "assignee"
	FieldParent      = "parent"
	FieldDue         = "due"
	FieldPoints      = "points"
	FieldEstimate    = "estimate"
	FieldRemaining   = "remaining"
	FieldSprint      = "sprint"
	FieldRank        = "rank"
	FieldProject     = "project"
	FieldLabels      = "labels"
	FieldComponents  = "components"
	FieldWatchers    = "watchers"
	FieldLinks       = "links"
	FieldMove        = "move" // a board move's refusals and conflicts
)

// Link is a link as one of its issues holds it.
type Link struct {
	Type    LinkType `json:"type"`
	Outward bool     `json:"outward"`
	Issue   string   `json:"issue"`
}

// IssueState is what an issue's commands decide on, folded from its
// stream; StateAt returns it as of a version (FR-70). Changed holds, per
// field, the version of the event that last changed it (MP-08).
type IssueState struct {
	Exists      bool             `json:"exists"`
	Deleted     bool             `json:"deleted,omitempty"`
	Hidden      bool             `json:"hidden,omitempty"` // by a moderator (moderation.go)
	Project     string           `json:"project"`
	Key         string           `json:"key"`
	Number      int64            `json:"number"`
	Keys        []string         `json:"keys"` // every key it had, the current last
	Type        IssueType        `json:"type"`
	Summary     string           `json:"summary"`
	Description json.RawMessage  `json:"description,omitempty"`
	Mentioned   []string         `json:"mentioned,omitempty"` // everyone its description ever mentioned
	DescUpto    int64            `json:"desc_upto,omitempty"` // the description's stream version its last version holds
	Status      string           `json:"status"`
	Category    Category         `json:"category"`
	Priority    Priority         `json:"priority"`
	Assignee    string           `json:"assignee,omitempty"`
	Reporter    string           `json:"reporter"`
	Parent      string           `json:"parent,omitempty"`
	Labels      []string         `json:"labels,omitempty"`
	Components  []string         `json:"components,omitempty"`
	Due         string           `json:"due,omitempty"`
	Points      *int             `json:"points,omitempty"`
	Estimate    *int             `json:"estimate,omitempty"`
	Remaining   *int             `json:"remaining,omitempty"`
	Rank        string           `json:"rank"`
	Sprint      string           `json:"sprint,omitempty"`
	Watchers    []string         `json:"watchers,omitempty"`
	Links       []Link           `json:"links,omitempty"`
	Version     int64            `json:"version"`
	Changed     map[string]int64 `json:"changed,omitempty"`
}

// issueFold is the version of evolveIssue and IssueState.
const issueFold = 2

func (s *IssueState) changed(field string) {
	m := maps.Clone(s.Changed)
	if m == nil {
		m = map[string]int64{}
	}
	m[field] = s.Version
	s.Changed = m
}

func evolveIssue(s IssueState, e IssueEvent) IssueState {
	s.Version++
	switch e := e.(type) {
	case IssueCreated:
		s = IssueState{Exists: true, Project: e.Project, Key: e.Key, Number: e.Number, Keys: []string{e.Key},
			Type: e.Type, Summary: e.Summary, Description: e.Description, Mentioned: e.Mentions,
			Status: e.Status, Category: e.Category, Priority: e.Priority, Assignee: e.Assignee, Reporter: e.Reporter,
			Parent: e.Parent, Labels: e.Labels, Components: e.Components, Due: e.Due, Points: e.Points,
			Estimate: e.Estimate, Remaining: e.Remaining, Rank: e.Rank, Sprint: e.Sprint, Watchers: e.Watchers,
			DescUpto: 1, Version: s.Version}
	case SummaryChanged:
		s.Summary = e.To
		s.changed(FieldSummary)
	case DescriptionChanged:
		s.Description = e.To
		s.DescUpto = max(s.DescUpto, e.Upto)
		s.Mentioned = slices.Clone(s.Mentioned)
		for _, m := range e.Mentions {
			if !slices.Contains(s.Mentioned, m) {
				s.Mentioned = append(s.Mentioned, m)
			}
		}
		s.changed(FieldDescription)
	case TypeChanged:
		s.Type = e.To
		s.changed(FieldType)
	case StatusChanged:
		s.Status, s.Category = e.To, e.Category
		s.changed(FieldStatus)
	case Moved:
		if e.Status != e.FromStatus {
			s.Status, s.Category = e.Status, e.Category
			s.changed(FieldStatus)
		}
		s.Rank = e.Rank
		s.changed(FieldRank)
	case PriorityChanged:
		s.Priority = e.To
		s.changed(FieldPriority)
	case AssigneeChanged:
		s.Assignee = e.To
		s.changed(FieldAssignee)
	case ParentChanged:
		s.Parent = e.To
		s.changed(FieldParent)
	case DueChanged:
		s.Due = e.To
		s.changed(FieldDue)
	case PointsChanged:
		s.Points = e.To
		s.changed(FieldPoints)
	case EstimateChanged:
		s.Estimate = e.To
		s.changed(FieldEstimate)
	case RemainingChanged:
		s.Remaining = e.To
		s.changed(FieldRemaining)
	case SprintChanged:
		s.Sprint = e.To
		s.changed(FieldSprint)
	case LabelsChanged:
		s.Labels = applySet(s.Labels, e.Added, e.Removed)
	case ComponentsChanged:
		s.Components = applySet(s.Components, e.Added, e.Removed)
	case WatchersAdded:
		s.Watchers = slices.Clone(s.Watchers)
		for _, p := range e.People {
			if !slices.Contains(s.Watchers, p) {
				s.Watchers = append(s.Watchers, p)
			}
		}
	case WatcherRemoved:
		s.Watchers = slices.DeleteFunc(slices.Clone(s.Watchers), func(p string) bool { return p == e.Person })
	case LinkAdded:
		s.Links = append(slices.Clone(s.Links), Link{Type: e.Link, Outward: e.Outward, Issue: e.Issue})
	case LinkRemoved:
		s.Links = slices.DeleteFunc(slices.Clone(s.Links), func(l Link) bool {
			return l.Type == e.Link && l.Outward == e.Outward && l.Issue == e.Issue
		})
	case IssueDeleted:
		s.Deleted = true
	case IssueHidden:
		s.Hidden = true
	case IssueUnhidden:
		s.Hidden = false
	case ProjectChanged:
		s.Project, s.Key, s.Number = e.Project, e.Key, e.Number
		s.Keys = append(slices.Clone(s.Keys), e.Key)
		if e.Status != s.Status {
			s.Status, s.Category = e.Status, e.Category
			s.changed(FieldStatus)
		}
		s.Rank = e.Rank
		s.changed(FieldRank)
		s.changed(FieldProject)
	}
	return s
}

// applySet adds and removes values of a set kept in order.
func applySet(set, add, remove []string) []string {
	out := slices.DeleteFunc(slices.Clone(set), func(v string) bool { return slices.Contains(remove, v) })
	for _, v := range add {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func issueDecider[C any](decide func(C, IssueState) ([]IssueEvent, error)) events.Decider[C, IssueEvent, IssueState] {
	return events.Decider[C, IssueEvent, IssueState]{Decide: decide, Evolve: evolveIssue, Version: issueFold}
}

var peekIssueDecider = peekDecider(IssueState{}, evolveIssue, issueFold)

// issueState reads an issue's state inside a command; one that does not
// exist answers access.ErrNotFound.
func issueState(tx *writer.Tx, id string) (IssueState, error) {
	var s IssueState
	if !ValidID(id) {
		return s, notFound("issue", id)
	}
	if err := events.Execute(Store(), tx, peekIssueDecider, IssueStream.ID(id), events.Any, peek[IssueState]{&s}, events.Meta{}); err != nil {
		return s, err
	}
	if !s.Exists {
		return s, notFound("issue", id)
	}
	return s, nil
}
