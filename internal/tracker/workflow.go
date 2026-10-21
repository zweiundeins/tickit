package tracker

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo/words"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Workflows and board columns (FR-36, FR-37, FR-43). A project's Admin
// adds, renames, reorders and removes statuses, sets their categories and
// the rules of which status may follow which, and arranges the board's
// columns, each holding statuses of the workflow. Every change is an
// event of the project's stream. A status keeps its id for good, so a
// rename rewrites no issue; a removal moves the issues that hold the
// status in the same command, and a category change tells each of them.
// Each event says all it changes: the decider emits a column's removal or
// rename beside the change of status that causes it.

// The workflow's and the board's events.
type (
	// StatusAdded appends a status to the workflow, in the column Column
	// (ColumnAdded made it just before).
	StatusAdded struct {
		Status Status `json:"status"`
		Column string `json:"column,omitempty"`
	}
	StatusRenamed struct {
		Status string `json:"status"`
		Name   string `json:"name"`
	}
	// StatusCategorySet changes a status's category; the issues that
	// hold it get issue.category_changed.
	StatusCategorySet struct {
		Status   string   `json:"status"`
		Category Category `json:"category"`
	}
	// StatusesOrdered is the workflow's new order, every status once.
	StatusesOrdered struct {
		Order []string `json:"order"`
	}
	// StatusRemoved removes a status; the issues that held it went to
	// MoveTo, and Start is the new start status when it was the start.
	StatusRemoved struct {
		Status string `json:"status"`
		MoveTo string `json:"move_to,omitempty"`
		Start  string `json:"start,omitempty"`
	}
	// TransitionRuleSet says which statuses an issue may come to Status
	// from (FR-37): From, or any with Any.
	TransitionRuleSet struct {
		Status string   `json:"status"`
		Any    bool     `json:"any,omitempty"`
		From   []string `json:"from,omitempty"`
	}
	ColumnAdded struct {
		Column string `json:"column"`
		Name   string `json:"name"`
	}
	ColumnRenamed struct {
		Column string `json:"column"`
		Name   string `json:"name"`
	}
	ColumnsOrdered struct {
		Order []string `json:"order"`
	}
	// ColumnRemoved removes a column; its statuses leave the board.
	ColumnRemoved struct {
		Column string `json:"column"`
	}
	// StatusMapped puts a status in a column, out of the one it was in;
	// Column "" takes it off the board.
	StatusMapped struct {
		Status string `json:"status"`
		Column string `json:"column,omitempty"`
	}
)

func (StatusAdded) projectEvent()       {}
func (StatusRenamed) projectEvent()     {}
func (StatusCategorySet) projectEvent() {}
func (StatusesOrdered) projectEvent()   {}
func (StatusRemoved) projectEvent()     {}
func (TransitionRuleSet) projectEvent() {}
func (ColumnAdded) projectEvent()       {}
func (ColumnRenamed) projectEvent()     {}
func (ColumnsOrdered) projectEvent()    {}
func (ColumnRemoved) projectEvent()     {}
func (StatusMapped) projectEvent()      {}

// The workflow's event types; the names are what the log stores.
var (
	StatusAddedType       = defineProject[StatusAdded]("project.status_added")
	StatusRenamedType     = defineProject[StatusRenamed]("project.status_renamed")
	StatusCategorySetType = defineProject[StatusCategorySet]("project.status_category_set")
	StatusesOrderedType   = defineProject[StatusesOrdered]("project.statuses_ordered")
	StatusRemovedType     = defineProject[StatusRemoved]("project.status_removed")
	TransitionRuleSetType = defineProject[TransitionRuleSet]("project.transition_rule_set")
	ColumnAddedType       = defineProject[ColumnAdded]("project.column_added")
	ColumnRenamedType     = defineProject[ColumnRenamed]("project.column_renamed")
	ColumnsOrderedType    = defineProject[ColumnsOrdered]("project.columns_ordered")
	ColumnRemovedType     = defineProject[ColumnRemoved]("project.column_removed")
	StatusMappedType      = defineProject[StatusMapped]("project.status_mapped")
)

// cloned copies the workflow and the board, so that an evolve changes no
// state it was given.
func (s ProjectState) cloned() ProjectState {
	s.Workflow.Statuses = slices.Clone(s.Workflow.Statuses)
	if s.Workflow.Follows != nil {
		m := make(map[string][]string, len(s.Workflow.Follows))
		for k, v := range s.Workflow.Follows {
			m[k] = slices.Clone(v)
		}
		s.Workflow.Follows = m
	}
	s.Board = slices.Clone(s.Board)
	for i := range s.Board {
		s.Board[i].Statuses = slices.Clone(s.Board[i].Statuses)
	}
	return s
}

func evolveWorkflow(s ProjectState, e ProjectEvent) ProjectState {
	s = s.cloned()
	w := &s.Workflow
	status := func(id string, f func(*Status)) {
		for i := range w.Statuses {
			if w.Statuses[i].ID == id {
				f(&w.Statuses[i])
			}
		}
	}
	column := func(id string, f func(*BoardColumn)) {
		for i := range s.Board {
			if s.Board[i].ID == id {
				f(&s.Board[i])
			}
		}
	}
	switch e := e.(type) {
	case StatusAdded:
		w.Statuses = append(w.Statuses, e.Status)
		if e.Column != "" {
			s.mapStatus(e.Status.ID, e.Column)
		}
	case StatusRenamed:
		status(e.Status, func(st *Status) { st.Name = e.Name })
	case StatusCategorySet:
		status(e.Status, func(st *Status) { st.Category = e.Category })
	case StatusesOrdered:
		var order []Status
		for _, id := range e.Order {
			if st, ok := w.Status(id); ok {
				order = append(order, st)
			}
		}
		w.Statuses = order
		for i := range s.Board {
			s.Board[i].Statuses = s.inOrder(s.Board[i].Statuses)
		}
	case StatusRemoved:
		w.Statuses = slices.DeleteFunc(w.Statuses, func(st Status) bool { return st.ID == e.Status })
		s.Retired = append(slices.Clone(s.Retired), e.Status)
		delete(w.Follows, e.Status)
		for k, v := range w.Follows {
			w.Follows[k] = slices.DeleteFunc(v, func(id string) bool { return id == e.Status })
		}
		if len(w.Follows) == 0 {
			w.Follows = nil
		}
		s.mapStatus(e.Status, "")
		if e.Start != "" {
			w.Start = e.Start
		}
	case TransitionRuleSet:
		if e.Any {
			delete(w.Follows, e.Status)
			if len(w.Follows) == 0 {
				w.Follows = nil
			}
			break
		}
		if w.Follows == nil {
			w.Follows = map[string][]string{}
		}
		w.Follows[e.Status] = slices.Clone(e.From)
	case ColumnAdded:
		s.Board = append(s.Board, BoardColumn{ID: e.Column, Name: e.Name})
	case ColumnRenamed:
		column(e.Column, func(c *BoardColumn) { c.Name = e.Name })
	case ColumnsOrdered:
		var order []BoardColumn
		for _, id := range e.Order {
			if c, ok := s.Column(id); ok {
				order = append(order, c)
			}
		}
		s.Board = order
	case ColumnRemoved:
		s.Board = slices.DeleteFunc(s.Board, func(c BoardColumn) bool { return c.ID == e.Column })
	case StatusMapped:
		s.mapStatus(e.Status, e.Column)
	}
	return s
}

// mapStatus puts a status in a column, out of any other; "" takes it off
// the board.
func (s *ProjectState) mapStatus(status, column string) {
	for i := range s.Board {
		c := &s.Board[i]
		c.Statuses = slices.DeleteFunc(c.Statuses, func(id string) bool { return id == status })
		if c.ID == column {
			c.Statuses = s.inOrder(append(c.Statuses, status))
		}
	}
}

// inOrder sorts status ids in workflow order.
func (s ProjectState) inOrder(ids []string) []string {
	slices.SortStableFunc(ids, func(a, b string) int { return cmp.Compare(s.Workflow.Order(a), s.Workflow.Order(b)) })
	return ids
}

// ColumnOf is the board column that shows a status, if any.
func (s ProjectState) ColumnOf(status string) (BoardColumn, bool) {
	for _, c := range s.Board {
		if slices.Contains(c.Statuses, status) {
			return c, true
		}
	}
	return BoardColumn{}, false
}

// columnsByWorkflow is the board's columns sorted by their first
// statuses, an empty column staying after the one before it.
func columnsByWorkflow(s ProjectState) []string {
	type keyed struct {
		id  string
		key int
	}
	var list []keyed
	last := 0
	for _, c := range s.Board {
		if len(c.Statuses) > 0 {
			last = s.Workflow.Order(c.Statuses[0])
		}
		list = append(list, keyed{c.ID, last})
	}
	slices.SortStableFunc(list, func(a, b keyed) int { return cmp.Compare(a.key, b.key) })
	out := make([]string, len(list))
	for i, k := range list {
		out[i] = k.id
	}
	return out
}

func columnIDs(board []BoardColumn) []string {
	out := make([]string, len(board))
	for i, c := range board {
		out[i] = c.ID
	}
	return out
}

func statusIDs(w Workflow) []string {
	out := make([]string, len(w.Statuses))
	for i, s := range w.Statuses {
		out[i] = s.ID
	}
	return out
}

// newStatusID makes a status's id of its name: "In QA" is "in-qa", and
// "in-qa-2" when the workflow had one before (ids are never given twice).
func newStatusID(s ProjectState, name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if b.Len() >= 30 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	base := b.String()
	if base == "" {
		base = "status"
	}
	taken := func(id string) bool {
		_, ok := s.Workflow.Status(id)
		return ok || slices.Contains(s.Retired, id)
	}
	id := base
	for n := 2; taken(id); n++ {
		id = base + "-" + strconv.Itoa(n)
	}
	return id
}

// statusName checks the name of a status or a column.
func statusName(name string) (string, error) {
	v, ok := text(name, 1, MaxStatusName)
	if !ok {
		return "", tabs.Refuse("name", "A name has 1 to 40 characters.")
	}
	if words.Blocked(v) {
		return "", tabs.Refuse("name", words.Refusal)
	}
	return v, nil
}

var errNoStatus = tabs.Refuse("status", "This workflow has no such status.")
var errNoColumn = tabs.Refuse("column", "The board has no such column.")

// workflowCommand runs a workflow's or a board's change: a project's
// Admin's (section 3.2).
func workflowCommand[C any](tx *writer.Tx, by tabs.By, project string, d events.Decider[C, ProjectEvent, ProjectState], c C) error {
	return projectCommand(tx, by, project, access.Configure, d, c)
}

// AddStatus appends a status to a project's workflow (FR-36.1), in a
// board column of its own (FR-36.3). Its result is the status's id as
// JSON: {"id": "in-qa"}.
type AddStatus struct {
	By       tabs.By
	Project  string
	Name     string
	Category Category
}

func (c AddStatus) Validate() error { return checkBy(c.By) }

// addStatus is AddStatus with the id it gives, for its result.
type addStatus struct {
	AddStatus
	id *string
}

var addStatusDecider = projectDecider(func(c addStatus, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	name, err := statusName(c.Name)
	if err != nil {
		return nil, err
	}
	if !c.Category.Valid() {
		return nil, tabs.Refuse("category", "Pick To Do, In Progress or Done.")
	}
	if slices.ContainsFunc(s.Workflow.Statuses, func(st Status) bool { return strings.EqualFold(st.Name, name) }) {
		return nil, tabs.Refuse("name", "The workflow has a status of that name.")
	}
	if len(s.Workflow.Statuses) >= MaxStatuses {
		return nil, tabs.Refuse("name", fmt.Sprintf("A workflow has at most %d statuses.", MaxStatuses))
	}
	st := Status{ID: newStatusID(s, name), Name: name, Category: c.Category}
	*c.id = st.ID
	if len(s.Board) >= MaxColumns {
		return []ProjectEvent{StatusAdded{Status: st}}, nil
	}
	col := "c" + strconv.FormatInt(s.Version+1, 10)
	return []ProjectEvent{ColumnAdded{Column: col, Name: name}, StatusAdded{Status: st, Column: col}}, nil
})

func (c AddStatus) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c AddStatus) ApplyResult(tx *writer.Tx) ([]byte, error) {
	var id string
	if err := workflowCommand(tx, c.By, c.Project, addStatusDecider, addStatus{c, &id}); err != nil {
		return nil, err
	}
	return []byte(`{"id":"` + id + `"}`), nil
}

// RenameStatus renames a status, and the board columns named as it was
// (FR-36.1, FR-36.3). Issues hold its id, so none changes.
type RenameStatus struct {
	By      tabs.By
	Project string
	Status  string
	Name    string
}

func (c RenameStatus) Validate() error { return checkBy(c.By) }

var renameStatus = projectDecider(func(c RenameStatus, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	st, ok := s.Workflow.Status(c.Status)
	if !ok {
		return nil, errNoStatus
	}
	name, err := statusName(c.Name)
	if err != nil {
		return nil, err
	}
	if name == st.Name {
		return nil, nil
	}
	if slices.ContainsFunc(s.Workflow.Statuses, func(o Status) bool { return o.ID != st.ID && strings.EqualFold(o.Name, name) }) {
		return nil, tabs.Refuse("name", "The workflow has a status of that name.")
	}
	evs := []ProjectEvent{StatusRenamed{Status: st.ID, Name: name}}
	for _, col := range s.Board {
		if col.Name == st.Name {
			evs = append(evs, ColumnRenamed{Column: col.ID, Name: name})
		}
	}
	return evs, nil
})

func (c RenameStatus) Apply(tx *writer.Tx) error {
	return workflowCommand(tx, c.By, c.Project, renameStatus, c)
}

// SetStatusCategory changes a status's category (FR-36.1): each issue
// that holds it changes category with it, so that the Done category's
// resolved time follows (FR-38).
type SetStatusCategory struct {
	By       tabs.By
	Project  string
	Status   string
	Category Category
}

func (c SetStatusCategory) Validate() error { return checkBy(c.By) }

var setStatusCategory = projectDecider(func(c SetStatusCategory, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	st, ok := s.Workflow.Status(c.Status)
	switch {
	case !ok:
		return nil, errNoStatus
	case !c.Category.Valid():
		return nil, tabs.Refuse("category", "Pick To Do, In Progress or Done.")
	case st.Category == c.Category:
		return nil, nil
	}
	return []ProjectEvent{StatusCategorySet{Status: st.ID, Category: c.Category}}, nil
})

func (c SetStatusCategory) Apply(tx *writer.Tx) error {
	if err := workflowCommand(tx, c.By, c.Project, setStatusCategory, c); err != nil {
		return err
	}
	ids, err := issueIDs(tx, `SELECT id FROM issues WHERE project_id = ? AND status = ? AND deleted_at IS NULL ORDER BY id`, c.Project, c.Status)
	if err != nil {
		return err
	}
	for _, id := range ids {
		cmd := recategorize{Status: c.Status, To: c.Category}
		if err := events.Execute(Store(), tx, recategorizeDecider, IssueStream.ID(id), events.Any, cmd, meta(c.By)); err != nil {
			return err
		}
	}
	return nil
}

type recategorize struct {
	Status string
	To     Category
}

var recategorizeDecider = issueDecider(func(c recategorize, s IssueState) ([]IssueEvent, error) {
	if !s.Exists || s.Status != c.Status || s.Category == c.To {
		return nil, nil
	}
	return []IssueEvent{CategoryChanged{Status: c.Status, From: s.Category, To: c.To}}, nil
})

// MoveStatus moves a status one place earlier or later in the workflow
// (FR-36.1). A board whose columns were in workflow order stays in it
// (FR-36.3).
type MoveStatus struct {
	By      tabs.By
	Project string
	Status  string
	Earlier bool
}

func (c MoveStatus) Validate() error { return checkBy(c.By) }

var moveStatus = projectDecider(func(c MoveStatus, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	order := statusIDs(s.Workflow)
	i := slices.Index(order, c.Status)
	if i < 0 {
		return nil, errNoStatus
	}
	j := i + 1
	if c.Earlier {
		j = i - 1
	}
	if j < 0 || j >= len(order) {
		return nil, nil
	}
	order[i], order[j] = order[j], order[i]
	evs := []ProjectEvent{StatusesOrdered{Order: order}}
	if slices.Equal(columnsByWorkflow(s), columnIDs(s.Board)) {
		after := evolveProject(s, evs[0])
		if cols := columnsByWorkflow(after); !slices.Equal(cols, columnIDs(s.Board)) {
			evs = append(evs, ColumnsOrdered{Order: cols})
		}
	}
	return evs, nil
})

func (c MoveStatus) Apply(tx *writer.Tx) error {
	return workflowCommand(tx, c.By, c.Project, moveStatus, c)
}

// RemoveStatus removes a status from a project's workflow (FR-36.2). The
// issues that hold it move to MoveTo in the same command, each with a
// status change in its history; a removal of a status that issues hold
// without MoveTo is refused. A column it leaves empty goes too.
type RemoveStatus struct {
	By      tabs.By
	Project string
	Status  string
	MoveTo  string
}

func (c RemoveStatus) Validate() error { return checkBy(c.By) }

type removeStatus struct {
	RemoveStatus
	held int // the issues that hold the status
}

var removeStatusDecider = projectDecider(func(c removeStatus, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	if _, ok := s.Workflow.Status(c.Status); !ok {
		return nil, nil
	}
	if len(s.Workflow.Statuses) == 1 {
		return nil, tabs.Refuse("status", "A workflow keeps one status at least.")
	}
	moveTo := ""
	if c.held > 0 || c.MoveTo != "" {
		if _, ok := s.Workflow.Status(c.MoveTo); c.MoveTo == c.Status || c.MoveTo != "" && !ok {
			return nil, tabs.Refuse("move_to", "Pick another status of this workflow.")
		}
		if c.held > 0 && c.MoveTo == "" {
			n := "1 issue"
			if c.held > 1 {
				n = strconv.Itoa(c.held) + " issues"
			}
			return nil, tabs.Refuse("move_to", "Pick the status its "+n+" move to.")
		}
		moveTo = c.MoveTo
	}
	e := StatusRemoved{Status: c.Status, MoveTo: moveTo}
	if s.Workflow.Start == c.Status {
		e.Start = moveTo
		if e.Start == "" {
			e.Start = slices.DeleteFunc(statusIDs(s.Workflow), func(id string) bool { return id == c.Status })[0]
		}
	}
	evs := []ProjectEvent{e}
	if col, ok := s.ColumnOf(c.Status); ok && len(col.Statuses) == 1 {
		evs = append(evs, ColumnRemoved{Column: col.ID})
	}
	return evs, nil
})

func (c RemoveStatus) Apply(tx *writer.Tx) error {
	p, err := projectState(tx, c.Project)
	if err != nil {
		return err
	}
	ids, err := issueIDs(tx, `SELECT id FROM issues WHERE project_id = ? AND status = ? AND deleted_at IS NULL ORDER BY id`, c.Project, c.Status)
	if err != nil {
		return err
	}
	if err := workflowCommand(tx, c.By, c.Project, removeStatusDecider, removeStatus{c, len(ids)}); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	to, _ := p.Workflow.Status(c.MoveTo)
	for _, id := range ids {
		cmd := forceStatus{From: c.Status, To: to.ID, Category: to.Category}
		if err := events.Execute(Store(), tx, forceStatusDecider, IssueStream.ID(id), events.Any, cmd, meta(c.By)); err != nil {
			return err
		}
	}
	return nil
}

// forceStatus moves an issue out of a status that goes, whatever the
// rules say: the Admin chose where.
type forceStatus struct {
	From, To string
	Category Category
}

var forceStatusDecider = issueDecider(func(c forceStatus, s IssueState) ([]IssueEvent, error) {
	if !s.Exists || s.Status != c.From {
		return nil, nil
	}
	return []IssueEvent{StatusChanged{From: c.From, To: c.To, Category: c.Category}}, nil
})

// SetTransitionRule says which statuses an issue may come to Status from
// (FR-37.1): From, or any status with Any. Board moves, menus and bulk
// edits all decide by it in the issue's decider.
type SetTransitionRule struct {
	By      tabs.By
	Project string
	Status  string
	Any     bool
	From    []string
}

func (c SetTransitionRule) Validate() error { return checkBy(c.By) }

var setTransitionRule = projectDecider(func(c SetTransitionRule, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	if _, ok := s.Workflow.Status(c.Status); !ok {
		return nil, errNoStatus
	}
	cur, restricted := s.Workflow.Follows[c.Status]
	if c.Any {
		if !restricted {
			return nil, nil
		}
		return []ProjectEvent{TransitionRuleSet{Status: c.Status, Any: true}}, nil
	}
	for _, id := range c.From {
		if _, ok := s.Workflow.Status(id); !ok {
			return nil, tabs.Refuse("from", "Pick statuses of this workflow.")
		}
	}
	var from []string
	for _, st := range s.Workflow.Statuses {
		if st.ID != c.Status && slices.Contains(c.From, st.ID) {
			from = append(from, st.ID)
		}
	}
	if restricted && slices.Equal(cur, from) {
		return nil, nil
	}
	return []ProjectEvent{TransitionRuleSet{Status: c.Status, From: from}}, nil
})

func (c SetTransitionRule) Apply(tx *writer.Tx) error {
	return workflowCommand(tx, c.By, c.Project, setTransitionRule, c)
}

// AddColumn appends an empty column to a project's board (FR-43.1).
type AddColumn struct {
	By      tabs.By
	Project string
	Name    string
}

func (c AddColumn) Validate() error { return checkBy(c.By) }

var addColumn = projectDecider(func(c AddColumn, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	name, err := statusName(c.Name)
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(s.Board, func(col BoardColumn) bool { return strings.EqualFold(col.Name, name) }) {
		return nil, tabs.Refuse("name", "The board has a column of that name.")
	}
	if len(s.Board) >= MaxColumns {
		return nil, tabs.Refuse("name", fmt.Sprintf("A board has at most %d columns.", MaxColumns))
	}
	return []ProjectEvent{ColumnAdded{Column: "c" + strconv.FormatInt(s.Version+1, 10), Name: name}}, nil
})

func (c AddColumn) Apply(tx *writer.Tx) error {
	return workflowCommand(tx, c.By, c.Project, addColumn, c)
}

// EditColumn renames a board column and sets its work-in-progress limit,
// 0 for none (FR-43.1).
type EditColumn struct {
	By      tabs.By
	Project string
	Column  string
	Name    string
	Limit   int
}

func (c EditColumn) Validate() error { return checkBy(c.By) }

var editColumn = projectDecider(func(c EditColumn, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	col, ok := s.Column(c.Column)
	if !ok {
		return nil, errNoColumn
	}
	name, err := statusName(c.Name)
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(s.Board, func(o BoardColumn) bool { return o.ID != col.ID && strings.EqualFold(o.Name, name) }) {
		return nil, tabs.Refuse("name", "The board has a column of that name.")
	}
	if c.Limit < 0 || c.Limit > MaxWIP {
		return nil, tabs.Refuse("limit", "A limit is 0 (none) to 999.")
	}
	var evs []ProjectEvent
	if name != col.Name {
		evs = append(evs, ColumnRenamed{Column: col.ID, Name: name})
	}
	if c.Limit != col.WIP {
		evs = append(evs, WIPLimitSet{Status: col.ID, Limit: c.Limit})
	}
	return evs, nil
})

func (c EditColumn) Apply(tx *writer.Tx) error {
	return workflowCommand(tx, c.By, c.Project, editColumn, c)
}

// MoveColumn moves a board column one place earlier or later (FR-43.1).
type MoveColumn struct {
	By      tabs.By
	Project string
	Column  string
	Earlier bool
}

func (c MoveColumn) Validate() error { return checkBy(c.By) }

var moveColumn = projectDecider(func(c MoveColumn, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	order := columnIDs(s.Board)
	i := slices.Index(order, c.Column)
	if i < 0 {
		return nil, errNoColumn
	}
	j := i + 1
	if c.Earlier {
		j = i - 1
	}
	if j < 0 || j >= len(order) {
		return nil, nil
	}
	order[i], order[j] = order[j], order[i]
	return []ProjectEvent{ColumnsOrdered{Order: order}}, nil
})

func (c MoveColumn) Apply(tx *writer.Tx) error {
	return workflowCommand(tx, c.By, c.Project, moveColumn, c)
}

// RemoveColumn removes a board column; its statuses leave the board, and
// their issues with them, until a column takes them again.
type RemoveColumn struct {
	By      tabs.By
	Project string
	Column  string
}

func (c RemoveColumn) Validate() error { return checkBy(c.By) }

var removeColumn = projectDecider(func(c RemoveColumn, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	if _, ok := s.Column(c.Column); !ok {
		return nil, nil
	}
	if len(s.Board) == 1 {
		return nil, tabs.Refuse("column", "A board keeps one column at least.")
	}
	return []ProjectEvent{ColumnRemoved{Column: c.Column}}, nil
})

func (c RemoveColumn) Apply(tx *writer.Tx) error {
	return workflowCommand(tx, c.By, c.Project, removeColumn, c)
}

// MapStatus puts a status in a board column (FR-43.1), or off the board
// with Column "".
type MapStatus struct {
	By      tabs.By
	Project string
	Status  string
	Column  string
}

func (c MapStatus) Validate() error { return checkBy(c.By) }

var mapStatus = projectDecider(func(c MapStatus, s ProjectState) ([]ProjectEvent, error) {
	if !s.Exists {
		return nil, errNoProject
	}
	if _, ok := s.Workflow.Status(c.Status); !ok {
		return nil, errNoStatus
	}
	if _, ok := s.Column(c.Column); c.Column != "" && !ok {
		return nil, errNoColumn
	}
	if cur, _ := s.ColumnOf(c.Status); cur.ID == c.Column {
		return nil, nil
	}
	return []ProjectEvent{StatusMapped{Status: c.Status, Column: c.Column}}, nil
})

func (c MapStatus) Apply(tx *writer.Tx) error {
	return workflowCommand(tx, c.By, c.Project, mapStatus, c)
}

// columnStatus is the status a card dropped in a column takes: the one it
// has when the column holds it, else the column's first status the
// workflow lets it go to (FR-43.1, FR-37.1).
func columnStatus(w Workflow, col BoardColumn, from string) (string, error) {
	if slices.Contains(col.Statuses, from) {
		return from, nil
	}
	if len(col.Statuses) == 0 {
		return "", tabs.Refuse(FieldMove, "This column holds no status.")
	}
	for _, st := range col.Statuses {
		if w.Allows(from, st) {
			return st, nil
		}
	}
	return "", tabs.Refuse(FieldMove, followRule(w, col.Statuses[0]))
}

// Rules is what a board needs to offer only the allowed moves (FR-37.1):
// per status, the columns a card of it may go to; nil when every move is
// allowed.
func Rules(w Workflow, board []BoardColumn) map[string][]string {
	if len(w.Follows) == 0 {
		return nil
	}
	out := map[string][]string{}
	for _, st := range w.Statuses {
		var to []string
		for _, col := range board {
			if _, err := columnStatus(w, col, st.ID); err == nil {
				to = append(to, col.ID)
			}
		}
		out[st.ID] = to
	}
	return out
}

// workflowHandlers keep statuses and board_columns, in tracker_projects.
// A change of a status's name, category or order shows in lists too.
var workflowHandlers = []events.Handler[string, ProjectEvent]{
	events.Handle(StatusAddedType, func(tx *writer.Tx, r events.Record[string, StatusAdded]) error {
		st := r.Data.Status
		_, err := tx.Exec(`INSERT INTO statuses (project_id, id, name, category, position, board_column)
			VALUES (?1, ?2, ?3, ?4, (SELECT coalesce(max(position), 0) + 1 FROM statuses WHERE project_id = ?1), ?5)`,
			r.ID, st.ID, st.Name, string(st.Category), r.Data.Column)
		touchWorkflow(tx, r.ID)
		return err
	}),
	events.Handle(StatusRenamedType, func(tx *writer.Tx, r events.Record[string, StatusRenamed]) error {
		_, err := tx.Exec(`UPDATE statuses SET name = ? WHERE project_id = ? AND id = ?`, r.Data.Name, r.ID, r.Data.Status)
		touchWorkflow(tx, r.ID)
		return err
	}),
	events.Handle(StatusCategorySetType, func(tx *writer.Tx, r events.Record[string, StatusCategorySet]) error {
		_, err := tx.Exec(`UPDATE statuses SET category = ? WHERE project_id = ? AND id = ?`, string(r.Data.Category), r.ID, r.Data.Status)
		touchWorkflow(tx, r.ID)
		return err
	}),
	events.Handle(StatusesOrderedType, func(tx *writer.Tx, r events.Record[string, StatusesOrdered]) error {
		for i, id := range r.Data.Order {
			if _, err := tx.Exec(`UPDATE statuses SET position = ? WHERE project_id = ? AND id = ?`, i+1, r.ID, id); err != nil {
				return err
			}
		}
		touchWorkflow(tx, r.ID)
		return nil
	}),
	events.Handle(StatusRemovedType, func(tx *writer.Tx, r events.Record[string, StatusRemoved]) error {
		if _, err := tx.Exec(`UPDATE statuses SET removed = 1, board_column = '', follows = '' WHERE project_id = ? AND id = ?`, r.ID, r.Data.Status); err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT id, follows FROM statuses WHERE project_id = ? AND follows != '' ORDER BY id`, r.ID)
		if err != nil {
			return err
		}
		rules := map[string][]string{}
		for rows.Next() {
			var id, follows string
			var from []string
			if err := rows.Scan(&id, &follows); err != nil {
				rows.Close()
				return err
			}
			if json.Unmarshal([]byte(follows), &from) == nil && slices.Contains(from, r.Data.Status) {
				rules[id] = slices.DeleteFunc(from, func(s string) bool { return s == r.Data.Status })
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range slices.Sorted(maps.Keys(rules)) {
			if _, err := tx.Exec(`UPDATE statuses SET follows = ? WHERE project_id = ? AND id = ?`, followsJSON(rules[id]), r.ID, id); err != nil {
				return err
			}
		}
		touchWorkflow(tx, r.ID)
		return nil
	}),
	events.Handle(TransitionRuleSetType, func(tx *writer.Tx, r events.Record[string, TransitionRuleSet]) error {
		follows := ""
		if !r.Data.Any {
			follows = followsJSON(r.Data.From)
		}
		_, err := tx.Exec(`UPDATE statuses SET follows = ? WHERE project_id = ? AND id = ?`, follows, r.ID, r.Data.Status)
		touchWorkflow(tx, r.ID)
		return err
	}),
	events.Handle(ColumnAddedType, func(tx *writer.Tx, r events.Record[string, ColumnAdded]) error {
		_, err := tx.Exec(`INSERT INTO board_columns (project_id, id, name, position)
			VALUES (?1, ?2, ?3, (SELECT coalesce(max(position), 0) + 1 FROM board_columns WHERE project_id = ?1))`, r.ID, r.Data.Column, r.Data.Name)
		touchProject(tx, r.ID)
		return err
	}),
	events.Handle(ColumnRenamedType, func(tx *writer.Tx, r events.Record[string, ColumnRenamed]) error {
		_, err := tx.Exec(`UPDATE board_columns SET name = ? WHERE project_id = ? AND id = ?`, r.Data.Name, r.ID, r.Data.Column)
		touchProject(tx, r.ID)
		return err
	}),
	events.Handle(ColumnsOrderedType, func(tx *writer.Tx, r events.Record[string, ColumnsOrdered]) error {
		for i, id := range r.Data.Order {
			if _, err := tx.Exec(`UPDATE board_columns SET position = ? WHERE project_id = ? AND id = ?`, i+1, r.ID, id); err != nil {
				return err
			}
		}
		touchProject(tx, r.ID)
		return nil
	}),
	events.Handle(ColumnRemovedType, func(tx *writer.Tx, r events.Record[string, ColumnRemoved]) error {
		if _, err := tx.Exec(`DELETE FROM board_columns WHERE project_id = ? AND id = ?`, r.ID, r.Data.Column); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE statuses SET board_column = '' WHERE project_id = ? AND board_column = ?`, r.ID, r.Data.Column)
		touchProject(tx, r.ID)
		return err
	}),
	events.Handle(StatusMappedType, func(tx *writer.Tx, r events.Record[string, StatusMapped]) error {
		_, err := tx.Exec(`UPDATE statuses SET board_column = ? WHERE project_id = ? AND id = ?`, r.Data.Column, r.ID, r.Data.Status)
		touchProject(tx, r.ID)
		return err
	}),
}

// touchWorkflow touches what a status's change shows on: the project's
// pages and the lists, which name and sort by statuses.
func touchWorkflow(tx *writer.Tx, project string) {
	touchProject(tx, project)
	tx.Touch(IssuesTopic)
}

func followsJSON(from []string) string {
	b, _ := json.Marshal(nonNil(from))
	return string(b)
}
