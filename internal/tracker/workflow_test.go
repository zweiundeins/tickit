package tracker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zweiundeins/lith/events/eventstest"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
)

// FR-36.1: statuses added, renamed, reordered and recategorized, each
// with the board columns that follow them (FR-36.3).
func TestWorkflowDeciders(t *testing.T) {
	var id string
	add := eventstest.For(addStatusDecider).Given(webCreated)
	add.When(addStatus{AddStatus{Name: " In QA ", Category: InProgress}, &id}).
		Then(t, ColumnAdded{Column: "c2", Name: "In QA"}, StatusAdded{Status: Status{ID: "in-qa", Name: "In QA", Category: InProgress}, Column: "c2"})
	add.When(addStatus{AddStatus{Name: "done", Category: Done}, &id}).ThenError(t, refuse("name", "The workflow has a status of that name."))
	add.When(addStatus{AddStatus{Name: "QA", Category: "closed"}, &id}).ThenError(t, refuse("category", "Pick To Do, In Progress or Done."))
	add.When(addStatus{AddStatus{Name: "", Category: ToDo}, &id}).ThenError(t, refuse("name", "A name has 1 to 40 characters."))
	// A removed status's id is never given again.
	add.Given(StatusRemoved{Status: StatusBlocked}).When(addStatus{AddStatus{Name: "Blocked", Category: InProgress}, &id}).
		Then(t, ColumnAdded{Column: "c3", Name: "Blocked"}, StatusAdded{Status: Status{ID: "blocked-2", Name: "Blocked", Category: InProgress}, Column: "c3"})
	add.When(addStatus{AddStatus{Name: "Ü ✓", Category: ToDo}, &id}).
		Then(t, ColumnAdded{Column: "c2", Name: "Ü ✓"}, StatusAdded{Status: Status{ID: "status", Name: "Ü ✓", Category: ToDo}, Column: "c2"})

	ren := eventstest.For(renameStatus).Given(webCreated)
	ren.When(RenameStatus{Status: StatusInReview, Name: "Code review"}).
		Then(t, StatusRenamed{Status: StatusInReview, Name: "Code review"}, ColumnRenamed{Column: StatusInReview, Name: "Code review"})
	ren.When(RenameStatus{Status: StatusBacklog, Name: "Ideas"}).Then(t, StatusRenamed{Status: StatusBacklog, Name: "Ideas"}) // no column
	ren.Given(ColumnRenamed{Column: StatusInReview, Name: "Review"}).When(RenameStatus{Status: StatusInReview, Name: "Code review"}).
		Then(t, StatusRenamed{Status: StatusInReview, Name: "Code review"}) // a column named otherwise keeps its name
	ren.When(RenameStatus{Status: StatusInReview, Name: "In Review"}).Then(t)
	ren.When(RenameStatus{Status: StatusInReview, Name: "to do"}).ThenError(t, refuse("name", "The workflow has a status of that name."))
	ren.When(RenameStatus{Status: "nope", Name: "x"}).ThenError(t, errNoStatus)

	cat := eventstest.For(setStatusCategory).Given(webCreated)
	cat.When(SetStatusCategory{Status: StatusBlocked, Category: ToDo}).Then(t, StatusCategorySet{Status: StatusBlocked, Category: ToDo})
	cat.When(SetStatusCategory{Status: StatusBlocked, Category: InProgress}).Then(t)

	mv := eventstest.For(moveStatus).Given(webCreated)
	order := []string{StatusBacklog, StatusToDo, StatusSelected, StatusInProgress, StatusInReview, StatusBlocked, StatusDone}
	mv.When(MoveStatus{Status: StatusToDo, Earlier: true}).
		Then(t, StatusesOrdered{Order: order}, ColumnsOrdered{Order: []string{StatusToDo, StatusSelected, StatusInProgress, StatusInReview, StatusBlocked, StatusDone}})
	mv.When(MoveStatus{Status: StatusBacklog, Earlier: true}).Then(t) // first already
	mv.When(MoveStatus{Status: StatusDone}).Then(t)                   // last already
	// Columns an Admin put out of workflow order stay where they are.
	custom := []string{StatusDone, StatusSelected, StatusToDo, StatusInProgress, StatusInReview, StatusBlocked}
	mv.Given(ColumnsOrdered{Order: custom}).When(MoveStatus{Status: StatusToDo, Earlier: true}).Then(t, StatusesOrdered{Order: order})
}

// FR-36.2: a removal of a status issues hold names where they go; the
// start status passes on, and a column left empty goes with it.
func TestRemoveStatusDecider(t *testing.T) {
	rm := eventstest.For(removeStatusDecider).Given(webCreated)
	rm.When(removeStatus{RemoveStatus{Status: StatusBlocked}, 3}).ThenError(t, refuse("move_to", "Pick the status its 3 issues move to."))
	rm.When(removeStatus{RemoveStatus{Status: StatusBlocked}, 1}).ThenError(t, refuse("move_to", "Pick the status its 1 issue move to."))
	rm.When(removeStatus{RemoveStatus{Status: StatusBlocked, MoveTo: StatusBlocked}, 3}).ThenError(t, refuse("move_to", "Pick another status of this workflow."))
	rm.When(removeStatus{RemoveStatus{Status: StatusBlocked, MoveTo: "nope"}, 3}).ThenError(t, refuse("move_to", "Pick another status of this workflow."))
	rm.When(removeStatus{RemoveStatus{Status: StatusBlocked, MoveTo: StatusInProgress}, 3}).
		Then(t, StatusRemoved{Status: StatusBlocked, MoveTo: StatusInProgress}, ColumnRemoved{Column: StatusBlocked})
	rm.When(removeStatus{RemoveStatus{Status: StatusBacklog}, 0}).Then(t, StatusRemoved{Status: StatusBacklog}) // off the board: no column
	rm.When(removeStatus{RemoveStatus{Status: StatusToDo}, 0}).
		Then(t, StatusRemoved{Status: StatusToDo, Start: StatusBacklog}, ColumnRemoved{Column: StatusToDo})
	rm.When(removeStatus{RemoveStatus{Status: StatusToDo, MoveTo: StatusSelected}, 2}).
		Then(t, StatusRemoved{Status: StatusToDo, MoveTo: StatusSelected, Start: StatusSelected}, ColumnRemoved{Column: StatusToDo})
	// A column of two statuses keeps the other.
	rm.Given(StatusMapped{Status: StatusBlocked, Column: StatusInProgress}).When(removeStatus{RemoveStatus{Status: StatusInProgress, MoveTo: StatusBlocked}, 1}).
		Then(t, StatusRemoved{Status: StatusInProgress, MoveTo: StatusBlocked})
	rm.When(removeStatus{RemoveStatus{Status: "nope"}, 0}).Then(t)
	one := ProjectCreated{Key: "ONE", Name: "One", Workflow: Workflow{Statuses: []Status{{ID: "open", Name: "Open", Category: ToDo}}, Start: "open"}}
	eventstest.For(removeStatusDecider).Given(one).When(removeStatus{RemoveStatus{Status: "open"}, 0}).
		ThenError(t, refuse("status", "A workflow keeps one status at least."))
}

// FR-37.1: a rule names the statuses a status may follow.
func TestTransitionRuleDecider(t *testing.T) {
	r := eventstest.For(setTransitionRule).Given(webCreated)
	r.When(SetTransitionRule{Status: StatusDone, From: []string{StatusInReview, StatusDone}}).Then(t, TransitionRuleSet{Status: StatusDone, From: []string{StatusInReview}})
	r.When(SetTransitionRule{Status: StatusDone, From: []string{StatusDone, StatusInReview, StatusToDo}}).
		Then(t, TransitionRuleSet{Status: StatusDone, From: []string{StatusToDo, StatusInReview}}) // in workflow order
	r.When(SetTransitionRule{Status: StatusDone, Any: true}).Then(t)
	r.When(SetTransitionRule{Status: StatusDone, From: []string{"nope"}}).ThenError(t, refuse("from", "Pick statuses of this workflow."))
	r.When(SetTransitionRule{Status: "nope"}).ThenError(t, errNoStatus)
	ruled := r.Given(TransitionRuleSet{Status: StatusDone, From: []string{StatusInReview}})
	ruled.When(SetTransitionRule{Status: StatusDone, From: []string{StatusInReview}}).Then(t)
	ruled.When(SetTransitionRule{Status: StatusDone, Any: true}).Then(t, TransitionRuleSet{Status: StatusDone, Any: true})
	ruled.When(SetTransitionRule{Status: StatusDone}).Then(t, TransitionRuleSet{Status: StatusDone})

	var s ProjectState
	for _, e := range []ProjectEvent{webCreated, TransitionRuleSet{Status: StatusDone, From: []string{StatusInReview}}} {
		s = evolveProject(s, e)
	}
	if s.Workflow.Allows(StatusToDo, StatusDone) || !s.Workflow.Allows(StatusInReview, StatusDone) || !s.Workflow.Allows(StatusDone, StatusToDo) {
		t.Errorf("rules: %v", s.Workflow.Follows)
	}
	if got := followRule(s.Workflow, StatusDone); got != "Done can only follow In Review." {
		t.Errorf("the refusal says %q", got)
	}
	s = evolveProject(s, StatusRemoved{Status: StatusInReview, MoveTo: StatusInProgress})
	if got := followRule(s.Workflow, StatusDone); got != "Nothing can move to Done." {
		t.Errorf("after In Review went, the refusal says %q", got)
	}
}

// FR-43.1: columns added, edited, reordered, removed; statuses mapped.
func TestColumnDeciders(t *testing.T) {
	add := eventstest.For(addColumn).Given(webCreated)
	add.When(AddColumn{Name: "Ready"}).Then(t, ColumnAdded{Column: "c2", Name: "Ready"})
	add.When(AddColumn{Name: "done"}).ThenError(t, refuse("name", "The board has a column of that name."))

	ed := eventstest.For(editColumn).Given(webCreated)
	ed.When(EditColumn{Column: StatusInReview, Name: "Review", Limit: 3}).
		Then(t, ColumnRenamed{Column: StatusInReview, Name: "Review"}, WIPLimitSet{Status: StatusInReview, Limit: 3})
	ed.When(EditColumn{Column: StatusInReview, Name: "In Review"}).Then(t)
	ed.When(EditColumn{Column: StatusInReview, Name: "Done"}).ThenError(t, refuse("name", "The board has a column of that name."))
	ed.When(EditColumn{Column: StatusInReview, Name: "In Review", Limit: -1}).ThenError(t, refuse("limit", "A limit is 0 (none) to 999."))
	ed.When(EditColumn{Column: StatusBacklog, Name: "x"}).ThenError(t, errNoColumn)

	mv := eventstest.For(moveColumn).Given(webCreated)
	mv.When(MoveColumn{Column: StatusDone, Earlier: true}).
		Then(t, ColumnsOrdered{Order: []string{StatusSelected, StatusToDo, StatusInProgress, StatusInReview, StatusDone, StatusBlocked}})
	mv.When(MoveColumn{Column: StatusDone}).Then(t)

	rm := eventstest.For(removeColumn).Given(webCreated)
	rm.When(RemoveColumn{Column: StatusBlocked}).Then(t, ColumnRemoved{Column: StatusBlocked})
	rm.When(RemoveColumn{Column: "nope"}).Then(t)

	mp := eventstest.For(mapStatus).Given(webCreated)
	mp.When(MapStatus{Status: StatusBlocked, Column: StatusInProgress}).Then(t, StatusMapped{Status: StatusBlocked, Column: StatusInProgress})
	mp.When(MapStatus{Status: StatusBacklog, Column: StatusToDo}).Then(t, StatusMapped{Status: StatusBacklog, Column: StatusToDo})
	mp.When(MapStatus{Status: StatusBacklog}).Then(t)
	mp.When(MapStatus{Status: StatusDone, Column: "nope"}).ThenError(t, errNoColumn)

	var s ProjectState
	for _, e := range []ProjectEvent{webCreated, StatusMapped{Status: StatusBacklog, Column: StatusDone}, StatusMapped{Status: StatusBlocked, Column: StatusDone}} {
		s = evolveProject(s, e)
	}
	if c, _ := s.Column(StatusDone); !slices.Equal(c.Statuses, []string{StatusBacklog, StatusBlocked, StatusDone}) {
		t.Errorf("the column holds %v, in workflow order", c.Statuses)
	}
	if c, _ := s.Column(StatusBlocked); len(c.Statuses) != 0 {
		t.Errorf("the column Blocked still holds %v", c.Statuses)
	}
}

// FR-43.1, FR-37.1: a drop into a column of several statuses picks the
// first the workflow allows; a column that allows none refuses with the
// rule of its first.
func TestMoveToColumn(t *testing.T) {
	wf := DefaultWorkflow()
	wf.Follows = map[string][]string{StatusInReview: {StatusInProgress}, StatusDone: {StatusInReview}}
	board := []BoardColumn{{ID: "work", Statuses: []string{StatusInProgress}}, {ID: "rev", Statuses: []string{StatusInReview, StatusBlocked}},
		{ID: "done", Statuses: []string{StatusDone}}, {ID: "empty"}}
	s := eventstest.For(moveDecider).Given(story)
	s.When(move{Column: "rev", Seen: 1, lower: "a6", wf: wf, board: board}).
		Then(t, Moved{FromStatus: StatusToDo, Status: StatusBlocked, Category: InProgress, FromRank: "a5", Rank: "a7"})
	s.When(move{Column: "done", Seen: 1, wf: wf, board: board}).ThenError(t, refuse(FieldMove, "Done can only follow In Review."))
	s.When(move{Column: "empty", Seen: 1, wf: wf, board: board}).ThenError(t, refuse(FieldMove, "This column holds no status."))
	s.When(move{Column: "nope", Seen: 1, wf: wf, board: board}).ThenError(t, refuse(FieldMove, "The board has no such column."))
	working := s.Given(Moved{FromStatus: StatusToDo, Status: StatusInProgress, Category: InProgress, FromRank: "a5", Rank: "a5"})
	working.When(move{Column: "rev", Seen: 2, lower: "a6", wf: wf, board: board}).
		Then(t, Moved{FromStatus: StatusInProgress, Status: StatusInReview, Category: InProgress, FromRank: "a5", Rank: "a7"})
	if got := Rules(wf, board); !slices.Equal(got[StatusToDo], []string{"work", "rev"}) || !slices.Equal(got[StatusInReview], []string{"work", "rev", "done"}) {
		t.Errorf("rules %v", got)
	}
	if Rules(DefaultWorkflow(), board) != nil {
		t.Error("a workflow without rules has rules for the board")
	}
}

// FR-36, FR-37, FR-43 through the commands: rights, the issues a removal
// moves with their history, a rule every path obeys, a category that
// moves the resolved time, the board's columns.
func TestWorkflowCommands(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", false)
	a := e.issue(member, CreateIssue{Project: p})
	b := e.issue(member, CreateIssue{Project: p, Status: StatusBlocked})
	c := e.issue(member, CreateIssue{Project: p, Status: StatusBlocked})
	gone := e.issue(member, CreateIssue{Project: p, Status: StatusBlocked})
	e.must(DeleteIssue{By: e.by(admin), Issue: gone})

	// Section 3.2: the workflow is the Admin's.
	for _, cmd := range []writer.Command{
		AddStatus{By: e.by(member), Project: p, Name: "QA", Category: InProgress},
		RemoveStatus{By: e.by(member), Project: p, Status: StatusBlocked, MoveTo: StatusToDo},
		SetTransitionRule{By: e.by(member), Project: p, Status: StatusDone, From: []string{StatusInReview}},
		MapStatus{By: e.by(member), Project: p, Status: StatusBacklog, Column: StatusToDo},
		EditColumn{By: e.by(viewer), Project: p, Column: StatusDone, Name: "Shipped"},
	} {
		var d *access.Denial
		if err := e.do(cmd); !errors.As(err, &d) {
			t.Errorf("%T by a member: %v", cmd, err)
		}
	}

	// FR-36.2: one command removes the status and moves its issues.
	err := e.do(RemoveStatus{By: e.by(admin), Project: p, Status: StatusBlocked})
	if msg := refusal(t, err); msg != "Pick the status its 2 issues move to." {
		t.Errorf("removal without a target: %q", msg)
	}
	e.must(RemoveStatus{By: e.by(admin), Project: p, Status: StatusBlocked, MoveTo: StatusInProgress})
	for _, id := range []string{b, c} {
		if s := e.state(id); s.Status != StatusInProgress || s.Category != InProgress {
			t.Errorf("a moved issue is in %s (%s)", s.Status, s.Category)
		}
		if got := e.row(`SELECT status FROM issues WHERE id = ?`, id); got != StatusInProgress {
			t.Errorf("the list shows it in %s", got)
		}
	}
	if s := e.state(gone); s.Status != StatusBlocked {
		t.Errorf("a deleted issue moved to %s", s.Status)
	}
	h, err := History(context.Background(), e.DB.R, b)
	if err != nil {
		t.Fatal(err)
	}
	last := h[len(h)-1]
	if last.Field != FieldStatus || last.Old != "Blocked" || last.New != "In Progress" || last.Actor != admin.ID {
		t.Errorf("the history's last entry: %+v", last)
	}
	if hd, _ := History(context.Background(), e.DB.R, gone); hd[0].Field != EntryCreated {
		t.Errorf("history of the deleted issue: %+v", hd)
	}
	ws, err := Statuses(context.Background(), e.DB.R, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 6 || slices.ContainsFunc(ws, func(s WorkflowStatus) bool { return s.ID == StatusBlocked }) {
		t.Errorf("statuses after the removal: %v", ws)
	}
	cols, _ := BoardColumns(context.Background(), e.DB.R, p)
	if len(cols) != 5 {
		t.Errorf("columns after the removal: %v", cols)
	}

	// FR-36.1, FR-36.3: a new status with a column of its own.
	if id := e.result(AddStatus{By: e.by(admin), Project: p, Name: "In QA", Category: InProgress})["id"]; id != "in-qa" {
		t.Errorf("the new status's id is %q", id)
	}
	cols, _ = BoardColumns(context.Background(), e.DB.R, p)
	if l := cols[len(cols)-1]; l.Name != "In QA" || len(l.Statuses) != 1 || l.Statuses[0].ID != "in-qa" {
		t.Errorf("the new column: %+v", l)
	}
	e.must(RenameStatus{By: e.by(admin), Project: p, Status: StatusInReview, Name: "Code review"})
	if got := e.row(`SELECT status FROM issues WHERE id = ?`, a); got != StatusToDo {
		t.Errorf("a rename moved an issue to %s", got)
	}
	if got := e.row(`SELECT name FROM board_columns WHERE project_id = ? AND id = ?`, p, StatusInReview); got != "Code review" {
		t.Errorf("the column is named %q", got)
	}

	// FR-37.1: every path obeys the rule, in the issue's decider.
	e.must(SetTransitionRule{By: e.by(admin), Project: p, Status: StatusDone, From: []string{StatusInReview}})
	const rule = "Done can only follow Code review."
	if msg := refusal(t, e.do(SetStatus{By: e.by(member), Issue: a, Seen: 1 << 40, Status: StatusDone})); msg != rule {
		t.Errorf("the menu's change: %q", msg)
	}
	if msg := refusal(t, e.do(Move{By: e.by(member), Issue: a, Status: StatusDone, Seen: 1 << 40})); msg != rule {
		t.Errorf("the board's move: %q", msg)
	}
	if msg := refusal(t, e.do(Move{By: e.by(member), Issue: a, Column: StatusDone, Seen: 1 << 40})); msg != rule {
		t.Errorf("the board's move to a column: %q", msg)
	}
	e.must(Move{By: e.by(member), Issue: a, Column: StatusInReview, Seen: 1 << 40})
	e.must(SetStatus{By: e.by(member), Issue: a, Seen: 1 << 40, Status: StatusDone})
	if got := e.row(`SELECT follows FROM statuses WHERE project_id = ? AND id = ?`, p, StatusDone); got != `["inreview"]` {
		t.Errorf("the rule as stored: %s", got)
	}

	// FR-36.1, FR-38: a category change moves the issues' resolved time.
	e.must(SetStatus{By: e.by(member), Issue: b, Seen: 1 << 40, Status: "in-qa"})
	e.must(SetStatusCategory{By: e.by(admin), Project: p, Status: "in-qa", Category: Done})
	if s := e.state(b); s.Category != Done {
		t.Errorf("the issue's category is %s", s.Category)
	}
	if got := e.row(`SELECT resolved_at IS NOT NULL FROM issues WHERE id = ?`, b); got != "1" {
		t.Error("an issue whose status went to Done has no resolved time")
	}
	h, _ = History(context.Background(), e.DB.R, b)
	if last := h[len(h)-1]; last.Field != FieldCategory || last.Old != "In Progress" || last.New != "Done" {
		t.Errorf("the category's history entry: %+v", last)
	}
	e.must(SetStatusCategory{By: e.by(admin), Project: p, Status: "in-qa", Category: InProgress})
	if got := e.row(`SELECT resolved_at IS NULL FROM issues WHERE id = ?`, b); got != "1" {
		t.Error("the resolved time stays after the category left Done")
	}

	// FR-43.1: a column of two statuses; a drop picks the first allowed.
	e.must(MapStatus{By: e.by(admin), Project: p, Status: "in-qa", Column: StatusInReview})
	e.must(EditColumn{By: e.by(admin), Project: p, Column: StatusInReview, Name: "Review and QA", Limit: 2})
	e.must(RemoveColumn{By: e.by(admin), Project: p, Column: cols[len(cols)-1].ID})
	bd, err := Board(context.Background(), e.DB.R, p, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, col := range bd.Columns {
		names = append(names, col.Name)
	}
	if strings.Join(names, ", ") != "Selected for Development, To Do, In Progress, Review and QA, Done" {
		t.Errorf("the board's columns: %s", strings.Join(names, ", "))
	}
	rev := bd.Columns[3]
	if len(rev.Statuses) != 2 || rev.WIP != 2 || len(rev.Cards) != 1 || rev.Cards[0].ID != b {
		t.Errorf("the column of two statuses: %+v", rev)
	}
	if !slices.Equal(bd.Rules[StatusToDo], []string{StatusSelected, StatusToDo, StatusInProgress, StatusInReview}) {
		t.Errorf("rules of To Do: %v", bd.Rules[StatusToDo])
	}
	e.must(Move{By: e.by(member), Issue: c, Column: StatusInReview, Seen: 1 << 40})
	if s := e.state(c); s.Status != StatusInReview {
		t.Errorf("the drop took %s", s.Status)
	}
	// The default workflow's other projects stay as they were (FR-35.1).
	other := e.project("APP", true)
	if ws, _ := Statuses(context.Background(), e.DB.R, other); len(ws) != 7 || !ws[0].Any {
		t.Errorf("a new project's workflow: %v", ws)
	}
}
