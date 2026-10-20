package tracker

import (
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
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
)

// ProjectEvent is an event of a project's stream.
type ProjectEvent interface{ projectEvent() }

// ProjectCreated starts a project's stream (FR-02): its key, its
// settings and the workflow it starts with (FR-35).
type ProjectCreated struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Lead        string   `json:"lead,omitempty"`
	Open        bool     `json:"open"`
	Workflow    Workflow `json:"workflow"`
}

// ProjectEdited changes the settings it names (FR-03); the others stay.
type ProjectEdited struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Lead        *string `json:"lead,omitempty"`
}

// ProjectArchived and ProjectUnarchived (FR-04).
type ProjectArchived struct{}
type ProjectUnarchived struct{}

// ComponentDefined adds a component, or changes its description (FR-24).
type ComponentDefined struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ComponentRemoved removes a component.
type ComponentRemoved struct {
	Name string `json:"name"`
}

// NumberAssigned gives the next number of the project to an issue that
// is created in it or moved into it (FR-11, FR-29).
type NumberAssigned struct {
	Number int64  `json:"number"`
	Issue  string `json:"issue"`
}

// WIPLimitSet sets the work-in-progress limit of a status's column, 0
// for none (FR-43).
type WIPLimitSet struct {
	Status string `json:"status"`
	Limit  int    `json:"limit"`
}

// SprintCreated, SprintStarted and SprintCompleted (FR-46). Dates are
// YYYY-MM-DD.
type SprintCreated struct {
	Sprint string `json:"sprint"`
	Name   string `json:"name"`
	Goal   string `json:"goal,omitempty"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
}

type SprintStarted struct {
	Sprint string `json:"sprint"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
}

// SprintCompleted closes a sprint; its unfinished issues went to MoveTo,
// a future sprint, or to the backlog when it is empty.
type SprintCompleted struct {
	Sprint string `json:"sprint"`
	MoveTo string `json:"move_to,omitempty"`
}

func (ProjectCreated) projectEvent()    {}
func (ProjectEdited) projectEvent()     {}
func (ProjectArchived) projectEvent()   {}
func (ProjectUnarchived) projectEvent() {}
func (ComponentDefined) projectEvent()  {}
func (ComponentRemoved) projectEvent()  {}
func (NumberAssigned) projectEvent()    {}
func (WIPLimitSet) projectEvent()       {}
func (SprintCreated) projectEvent()     {}
func (SprintStarted) projectEvent()     {}
func (SprintCompleted) projectEvent()   {}

// ProjectStream is the kind of the projects' streams, keyed by project id.
var ProjectStream = events.NewStream[string, ProjectEvent]("project")

func defineProject[T ProjectEvent](name string) events.Type[string, ProjectEvent, T] {
	return events.Define(ProjectStream, name, func(t T) ProjectEvent { return t })
}

// The project event types; the names are what the log stores.
var (
	ProjectCreatedType    = defineProject[ProjectCreated]("project.created")
	ProjectEditedType     = defineProject[ProjectEdited]("project.edited")
	ProjectArchivedType   = defineProject[ProjectArchived]("project.archived")
	ProjectUnarchivedType = defineProject[ProjectUnarchived]("project.unarchived")
	ComponentDefinedType  = defineProject[ComponentDefined]("project.component_defined")
	ComponentRemovedType  = defineProject[ComponentRemoved]("project.component_removed")
	NumberAssignedType    = defineProject[NumberAssigned]("project.number_assigned")
	WIPLimitSetType       = defineProject[WIPLimitSet]("project.wip_limit_set")
	SprintCreatedType     = defineProject[SprintCreated]("project.sprint_created")
	SprintStartedType     = defineProject[SprintStarted]("project.sprint_started")
	SprintCompletedType   = defineProject[SprintCompleted]("project.sprint_completed")
)

// Component is a project's component.
type Component struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Sprint is a project's sprint.
type Sprint struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Goal  string `json:"goal,omitempty"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
	State string `json:"state"`
}

// ProjectState is what a project's commands decide on, folded from its
// stream.
type ProjectState struct {
	Exists      bool           `json:"exists"`
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Lead        string         `json:"lead,omitempty"`
	Open        bool           `json:"open"`
	Archived    bool           `json:"archived,omitempty"`
	Workflow    Workflow       `json:"workflow"`
	Last        int64          `json:"last"` // the highest number given
	WIP         map[string]int `json:"wip,omitempty"`
	Components  []Component    `json:"components,omitempty"`
	Sprints     []Sprint       `json:"sprints,omitempty"`
	Version     int64          `json:"version"`
}

// Component returns the component with the name, if the project has it.
func (s ProjectState) Component(name string) (Component, bool) {
	i := slices.IndexFunc(s.Components, func(c Component) bool { return strings.EqualFold(c.Name, name) })
	if i < 0 {
		return Component{}, false
	}
	return s.Components[i], true
}

// Sprint returns the sprint with the id, if the project has it.
func (s ProjectState) Sprint(id string) (Sprint, bool) {
	i := slices.IndexFunc(s.Sprints, func(x Sprint) bool { return x.ID == id })
	if i < 0 {
		return Sprint{}, false
	}
	return s.Sprints[i], true
}

// projectFold is the version of evolveProject and ProjectState.
const projectFold = 1

func evolveProject(s ProjectState, e ProjectEvent) ProjectState {
	s.Version++
	switch e := e.(type) {
	case ProjectCreated:
		s.Exists, s.Key, s.Name, s.Description, s.Lead, s.Open, s.Workflow = true, e.Key, e.Name, e.Description, e.Lead, e.Open, e.Workflow
	case ProjectEdited:
		if e.Name != nil {
			s.Name = *e.Name
		}
		if e.Description != nil {
			s.Description = *e.Description
		}
		if e.Lead != nil {
			s.Lead = *e.Lead
		}
	case ProjectArchived:
		s.Archived = true
	case ProjectUnarchived:
		s.Archived = false
	case ComponentDefined:
		i := slices.IndexFunc(s.Components, func(c Component) bool { return strings.EqualFold(c.Name, e.Name) })
		if i < 0 {
			s.Components = append(s.Components, Component(e))
		} else {
			s.Components[i] = Component(e)
		}
	case ComponentRemoved:
		s.Components = slices.DeleteFunc(s.Components, func(c Component) bool { return strings.EqualFold(c.Name, e.Name) })
	case NumberAssigned:
		s.Last = max(s.Last, e.Number)
	case WIPLimitSet:
		if s.WIP == nil {
			s.WIP = map[string]int{}
		}
		if e.Limit == 0 {
			delete(s.WIP, e.Status)
		} else {
			s.WIP[e.Status] = e.Limit
		}
	case SprintCreated:
		s.Sprints = append(s.Sprints, Sprint{ID: e.Sprint, Name: e.Name, Goal: e.Goal, Start: e.Start, End: e.End, State: SprintFuture})
	case SprintStarted:
		for i := range s.Sprints {
			if s.Sprints[i].ID == e.Sprint {
				s.Sprints[i].State = SprintActive
				if e.Start != "" {
					s.Sprints[i].Start = e.Start
				}
				if e.End != "" {
					s.Sprints[i].End = e.End
				}
			}
		}
	case SprintCompleted:
		for i := range s.Sprints {
			if s.Sprints[i].ID == e.Sprint {
				s.Sprints[i].State = SprintClosed
			}
		}
	}
	return s
}

func projectDecider[C any](decide func(C, ProjectState) ([]ProjectEvent, error)) events.Decider[C, ProjectEvent, ProjectState] {
	return events.Decider[C, ProjectEvent, ProjectState]{Decide: decide, Evolve: evolveProject, Version: projectFold}
}

var peekProjectDecider = peekDecider(ProjectState{}, evolveProject, projectFold)

// projectState reads a project's state inside a command; a project that
// does not exist answers access.ErrNotFound.
func projectState(tx *writer.Tx, id string) (ProjectState, error) {
	var s ProjectState
	if !ValidID(id) {
		return s, notFound("project", id)
	}
	if err := events.Execute(Store(), tx, peekProjectDecider, ProjectStream.ID(id), events.Any, peek[ProjectState]{&s}, events.Meta{}); err != nil {
		return s, err
	}
	if !s.Exists {
		return s, notFound("project", id)
	}
	return s, nil
}

var errNoProject = errors.New("tracker: no such project")

// text checks a one-line text: trimmed, n runes at most, no control
// characters.
func text(s string, min, max int) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	return s, n >= min && n <= max && strings.IndexFunc(s, unicode.IsControl) < 0
}

// longText checks a text of several lines.
func longText(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	return s, utf8.RuneCountInString(s) <= max && strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) < 0
}

// validDate reports whether s is a date YYYY-MM-DD, or empty.
func validDate(s string) bool {
	if s == "" {
		return true
	}
	t, err := time.Parse(time.DateOnly, s)
	return err == nil && t.Format(time.DateOnly) == s
}

func validPerson(id string) bool { return id == "" || people.ValidID(id) }

// CreateProject creates a project with the default workflow, and makes
// its creator its Admin (FR-02). Its result is the project's id and key
// as JSON: {"id": "...", "key": "WEB"}.
type CreateProject struct {
	By          tabs.By
	ID          string // optional: a new id when empty
	Key         string
	Name        string
	Description string
	Lead        string // a person id, or empty
	Open        bool   // everyone has a Member's rights (Q14)
}

func (c CreateProject) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.ID != "" && !ValidID(c.ID) || !validPerson(c.Lead) {
		return errors.New("tracker: a malformed project")
	}
	return nil
}

func (c CreateProject) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

var createProject = projectDecider(func(c ProjectCreated, s ProjectState) ([]ProjectEvent, error) {
	if s.Exists {
		return nil, errors.New("tracker: the project exists")
	}
	return []ProjectEvent{c}, nil
})

func (c CreateProject) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if c.By.Actor.Kind != people.Seed {
		if err := access.Require(tx, c.By.Actor, access.CreateScope, access.Scope{}); err != nil {
			return nil, err
		}
	}
	key, ok := NormalizeKey(c.Key)
	if !ok {
		return nil, tabs.Refuse("key", "A key has 2 to 6 letters A to Z.")
	}
	name, ok := text(c.Name, 1, MaxProjectName)
	if !ok {
		return nil, tabs.Refuse("name", "A name has 1 to 100 characters.")
	}
	desc, ok := longText(c.Description, MaxDescription)
	if !ok {
		return nil, tabs.Refuse("description", "A description has at most 2,000 characters.")
	}
	id := c.ID
	if id == "" {
		id = NewID()
	}
	when := at(c.By)
	if err := access.AddScope(tx, access.Project, id, key, c.Open, c.By.Actor.ID, when); err != nil {
		return nil, err
	}
	e := ProjectCreated{Key: key, Name: name, Description: desc, Lead: c.Lead, Open: c.Open, Workflow: DefaultWorkflow()}
	if err := events.Execute(Store(), tx, createProject, ProjectStream.ID(id), events.NoStream, e, meta(c.By)); err != nil {
		return nil, err
	}
	if err := acted(tx, id, c.By.Actor.ID, when); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": id, "key": key})
}

// EditProject changes a project's name, description or lead, those that
// are set (FR-03). The key stays (FR-07).
type EditProject struct {
	By          tabs.By
	Project     string
	Name        *string
	Description *string
	Lead        *string
}

func (c EditProject) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.Lead != nil && !validPerson(*c.Lead) {
		return errors.New("tracker: a malformed lead")
	}
	return nil
}

var editProject = projectDecider(func(c EditProject, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	var e ProjectEdited
	if c.Name != nil {
		name, ok := text(*c.Name, 1, MaxProjectName)
		if !ok {
			return nil, tabs.Refuse("name", "A name has 1 to 100 characters.")
		}
		if name != s.Name {
			e.Name = &name
		}
	}
	if c.Description != nil {
		desc, ok := longText(*c.Description, MaxDescription)
		if !ok {
			return nil, tabs.Refuse("description", "A description has at most 2,000 characters.")
		}
		if desc != s.Description {
			e.Description = &desc
		}
	}
	if c.Lead != nil && *c.Lead != s.Lead {
		e.Lead = c.Lead
	}
	if e == (ProjectEdited{}) {
		return nil, nil
	}
	return []ProjectEvent{e}, nil
})

func (c EditProject) Apply(tx *writer.Tx) error {
	return projectCommand(tx, c.By, c.Project, access.Configure, editProject, c)
}

// projectCommand runs a project's command after its access check.
func projectCommand[C any](tx *writer.Tx, by tabs.By, project string, r access.Right, d events.Decider[C, ProjectEvent, ProjectState], c C) error {
	if _, err := projectState(tx, project); err != nil {
		return err
	}
	if err := require(tx, by, r, project); err != nil {
		return err
	}
	if err := events.Execute(Store(), tx, d, ProjectStream.ID(project), events.Any, c, meta(by)); err != nil {
		return err
	}
	return acted(tx, project, by.Actor.ID, at(by))
}

// ArchiveProject archives a project, or takes it out of the archive
// (FR-04). An archived project's issues are read-only.
type ArchiveProject struct {
	By       tabs.By
	Project  string
	Archived bool
}

func (c ArchiveProject) Validate() error { return checkBy(c.By) }

var archiveProject = projectDecider(func(c ArchiveProject, s ProjectState) ([]ProjectEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoProject
	case c.Archived == s.Archived:
		return nil, nil
	case c.Archived:
		return []ProjectEvent{ProjectArchived{}}, nil
	}
	return []ProjectEvent{ProjectUnarchived{}}, nil
})

func (c ArchiveProject) Apply(tx *writer.Tx) error {
	if err := projectCommand(tx, c.By, c.Project, access.ArchiveScope, archiveProject, c); err != nil {
		return err
	}
	return access.SetArchived(tx, access.ProjectScope(c.Project), c.Archived)
}

// DefineComponent adds a component to a project or changes its
// description (FR-24).
type DefineComponent struct {
	By          tabs.By
	Project     string
	Name        string
	Description string
}

func (c DefineComponent) Validate() error { return checkBy(c.By) }

var defineComponent = projectDecider(func(c DefineComponent, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	name, ok := text(c.Name, 1, MaxComponent)
	if !ok {
		return nil, tabs.Refuse("name", "A component's name has 1 to 50 characters.")
	}
	desc, ok := longText(c.Description, MaxDescription)
	if !ok {
		return nil, tabs.Refuse("description", "A description has at most 2,000 characters.")
	}
	if old, ok := s.Component(name); ok {
		if old.Description == desc {
			return nil, nil
		}
		name = old.Name
	}
	return []ProjectEvent{ComponentDefined{Name: name, Description: desc}}, nil
})

func (c DefineComponent) Apply(tx *writer.Tx) error {
	return projectCommand(tx, c.By, c.Project, access.Configure, defineComponent, c)
}

// RemoveComponent removes a component from a project and from its
// issues.
type RemoveComponent struct {
	By      tabs.By
	Project string
	Name    string
}

func (c RemoveComponent) Validate() error { return checkBy(c.By) }

var removeComponent = projectDecider(func(c RemoveComponent, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	old, ok := s.Component(c.Name)
	if !ok {
		return nil, nil
	}
	return []ProjectEvent{ComponentRemoved{Name: old.Name}}, nil
})

func (c RemoveComponent) Apply(tx *writer.Tx) error {
	p, err := projectState(tx, c.Project)
	if err != nil {
		return err
	}
	old, ok := p.Component(c.Name)
	if !ok {
		return nil
	}
	if err := projectCommand(tx, c.By, c.Project, access.Configure, removeComponent, c); err != nil {
		return err
	}
	ids, err := issueIDs(tx, `SELECT i.id FROM issues i, json_each(i.components) c
		WHERE i.project_id = ? AND c.value = ? AND i.deleted_at IS NULL ORDER BY i.id`, c.Project, old.Name)
	if err != nil {
		return err
	}
	for _, id := range ids {
		cmd := setComponents{remove: []string{old.Name}, force: true}
		if err := events.Execute(Store(), tx, componentsDecider, IssueStream.ID(id), events.Any, cmd, meta(c.By)); err != nil {
			return err
		}
	}
	return nil
}

// SetWIPLimit sets the work-in-progress limit of a board column, 0 for
// none (FR-43).
type SetWIPLimit struct {
	By      tabs.By
	Project string
	Status  string
	Limit   int
}

func (c SetWIPLimit) Validate() error { return checkBy(c.By) }

var setWIPLimit = projectDecider(func(c SetWIPLimit, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	if _, ok := s.Workflow.Status(c.Status); !ok || !OnBoard(c.Status) {
		return nil, tabs.Refuse("status", "The board has no such column.")
	}
	if c.Limit < 0 || c.Limit > MaxWIP {
		return nil, tabs.Refuse("limit", "A limit is 0 (none) to 999.")
	}
	if s.WIP[c.Status] == c.Limit {
		return nil, nil
	}
	return []ProjectEvent{WIPLimitSet{Status: c.Status, Limit: c.Limit}}, nil
})

func (c SetWIPLimit) Apply(tx *writer.Tx) error {
	return projectCommand(tx, c.By, c.Project, access.Configure, setWIPLimit, c)
}

// CreateSprint adds a future sprint to a project (FR-46). Its result is
// the sprint's id as JSON: {"id": "..."}.
type CreateSprint struct {
	By      tabs.By
	Project string
	ID      string // optional
	Name    string
	Goal    string
	Start   string // YYYY-MM-DD, optional
	End     string
}

func (c CreateSprint) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.ID != "" && !ValidID(c.ID) {
		return errors.New("tracker: a malformed sprint id")
	}
	return nil
}

var createSprint = projectDecider(func(c CreateSprint, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	name, ok := text(c.Name, 1, MaxSprintName)
	if !ok {
		return nil, tabs.Refuse("name", "A sprint's name has 1 to 60 characters.")
	}
	goal, ok := longText(c.Goal, MaxSprintGoal)
	if !ok {
		return nil, tabs.Refuse("goal", "A goal has at most 500 characters.")
	}
	if !validDate(c.Start) || !validDate(c.End) {
		return nil, tabs.Refuse("dates", "A date looks like 2026-10-21.")
	}
	if c.Start != "" && c.End != "" && c.End < c.Start {
		return nil, tabs.Refuse("dates", "A sprint ends after it starts.")
	}
	if _, dup := s.Sprint(c.ID); dup {
		return nil, nil
	}
	return []ProjectEvent{SprintCreated{Sprint: c.ID, Name: name, Goal: goal, Start: c.Start, End: c.End}}, nil
})

func (c CreateSprint) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c CreateSprint) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if c.ID == "" {
		c.ID = NewID()
	}
	if err := projectCommand(tx, c.By, c.Project, access.EditIssues, createSprint, c); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": c.ID})
}

// StartSprint starts a future sprint; a project has one active sprint at
// most.
type StartSprint struct {
	By      tabs.By
	Project string
	Sprint  string
	Start   string // optional: the dates it was created with stay
	End     string
}

func (c StartSprint) Validate() error { return checkBy(c.By) }

var startSprint = projectDecider(func(c StartSprint, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	sp, ok := s.Sprint(c.Sprint)
	switch {
	case !ok:
		return nil, tabs.Refuse("sprint", "That sprint does not exist.")
	case sp.State == SprintActive:
		return nil, nil
	case sp.State == SprintClosed:
		return nil, tabs.Refuse("sprint", "That sprint is closed.")
	case slices.ContainsFunc(s.Sprints, func(x Sprint) bool { return x.State == SprintActive }):
		return nil, tabs.Refuse("sprint", "Complete the active sprint first.")
	case !validDate(c.Start) || !validDate(c.End):
		return nil, tabs.Refuse("dates", "A date looks like 2026-10-21.")
	}
	return []ProjectEvent{SprintStarted{Sprint: c.Sprint, Start: c.Start, End: c.End}}, nil
})

func (c StartSprint) Apply(tx *writer.Tx) error {
	return projectCommand(tx, c.By, c.Project, access.EditIssues, startSprint, c)
}

// CompleteSprint closes the active sprint and moves its unfinished
// issues to MoveTo, a future sprint, or to the backlog when it is empty
// (FR-46.1).
type CompleteSprint struct {
	By      tabs.By
	Project string
	Sprint  string
	MoveTo  string
}

func (c CompleteSprint) Validate() error { return checkBy(c.By) }

var completeSprint = projectDecider(func(c CompleteSprint, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	sp, ok := s.Sprint(c.Sprint)
	switch {
	case !ok:
		return nil, tabs.Refuse("sprint", "That sprint does not exist.")
	case sp.State == SprintClosed:
		return nil, nil
	case sp.State != SprintActive:
		return nil, tabs.Refuse("sprint", "Only the active sprint can be completed.")
	}
	if c.MoveTo != "" {
		to, ok := s.Sprint(c.MoveTo)
		if !ok || to.State != SprintFuture {
			return nil, tabs.Refuse("move_to", "Move the open issues to a future sprint or to the backlog.")
		}
	}
	return []ProjectEvent{SprintCompleted{Sprint: c.Sprint, MoveTo: c.MoveTo}}, nil
})

func (c CompleteSprint) Apply(tx *writer.Tx) error {
	if err := projectCommand(tx, c.By, c.Project, access.EditIssues, completeSprint, c); err != nil {
		return err
	}
	ids, err := issueIDs(tx, `SELECT id FROM issues WHERE project_id = ? AND sprint = ? AND status_category != 'done' AND deleted_at IS NULL ORDER BY id`, c.Project, c.Sprint)
	if err != nil {
		return err
	}
	for _, id := range ids {
		cmd := edit{Field: FieldSprint, Text: c.MoveTo, Actor: c.By.Actor.ID, Seen: 1 << 62}
		if err := executeEdit(tx, c.By, id, cmd); err != nil {
			return err
		}
	}
	return nil
}

// issueIDs runs a query of issue ids inside a command.
func issueIDs(tx *writer.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// numberIssue gives the next number of a project to an issue, on the
// project's stream.
type numberIssue struct{ Issue string }

var numberDecider = projectDecider(func(c numberIssue, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	if s.Archived {
		return nil, &writer.Refusal{Msg: "This project is archived."}
	}
	return []ProjectEvent{NumberAssigned{Number: s.Last + 1, Issue: c.Issue}}, nil
})

// assignNumber gives an issue the next number of a project and records
// its key, which stays the issue's for good (FR-11, FR-12).
func assignNumber(tx *writer.Tx, p ProjectState, project, issue string, m events.Meta) (int64, string, error) {
	number := p.Last + 1
	if err := events.Execute(Store(), tx, numberDecider, ProjectStream.ID(project), events.Exactly(p.Version), numberIssue{Issue: issue}, m); err != nil {
		return 0, "", err
	}
	key := IssueKey(p.Key, number)
	if _, err := tx.Exec(`INSERT INTO issue_keys (key, project_id, number, issue_id) VALUES (?, ?, ?, ?)`, key, project, number, issue); err != nil {
		return 0, "", fmt.Errorf("tracker: the key %s: %w", key, err)
	}
	return number, key, nil
}
