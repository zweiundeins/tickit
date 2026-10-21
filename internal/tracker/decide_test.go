package tracker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zweiundeins/lith/events/eventstest"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/tabs"
)

func refuse(field, msg string) error { return tabs.Refuse(field, msg) }

func ptr(n int) *int { return &n }

var webCreated = ProjectCreated{Key: "WEB", Name: "Web shop", Open: true, Workflow: DefaultWorkflow()}

func TestProjectDeciders(t *testing.T) {
	eventstest.For(createProject).When(webCreated).Then(t, webCreated)
	eventstest.For(createProject).Given(webCreated).When(webCreated).ThenError(t, errorString("tracker: the project exists"))

	web := eventstest.For(editProject).Given(webCreated)
	name, same, long, desc, lead := "Shop", "Web shop", strings.Repeat("x", 101), "All of it", "s-priya"
	web.When(EditProject{Name: &name}).Then(t, ProjectEdited{Name: &name})
	web.When(EditProject{Name: &same}).Then(t)                                                         // FR-14.3: nothing changes
	web.When(EditProject{Name: &long}).ThenError(t, refuse("name", "A name has 1 to 100 characters.")) // FR-02
	web.When(EditProject{Description: &desc, Lead: &lead}).Then(t, ProjectEdited{Description: &desc, Lead: &lead})
	eventstest.For(editProject).When(EditProject{Name: &name}).ThenError(t, errNoProject)

	arch := eventstest.For(archiveProject).Given(webCreated)
	arch.When(ArchiveProject{Archived: true}).Then(t, ProjectArchived{})
	arch.When(ArchiveProject{Archived: false}).Then(t)
	arch.Given(ProjectArchived{}).When(ArchiveProject{Archived: false}).Then(t, ProjectUnarchived{})

	comp := eventstest.For(defineComponent).Given(webCreated)
	comp.When(DefineComponent{Name: " Search ", Description: "The box"}).Then(t, ComponentDefined{Name: "Search", Description: "The box"})
	comp.Given(ComponentDefined{Name: "Search"}).When(DefineComponent{Name: "search"}).Then(t)
	comp.Given(ComponentDefined{Name: "Search"}).When(DefineComponent{Name: "search", Description: "New"}).Then(t, ComponentDefined{Name: "Search", Description: "New"})
	comp.When(DefineComponent{Name: ""}).ThenError(t, refuse("name", "A component's name has 1 to 50 characters."))
	comp.When(DefineComponent{Name: "x", Description: strings.Repeat("y", 2001)}).ThenError(t, refuse("description", "A description has at most 2,000 characters."))

	rm := eventstest.For(removeComponent).Given(webCreated, ComponentDefined{Name: "Search"})
	rm.When(RemoveComponent{Name: "SEARCH"}).Then(t, ComponentRemoved{Name: "Search"})
	rm.When(RemoveComponent{Name: "Cart"}).Then(t)

	wip := eventstest.For(setWIPLimit).Given(webCreated)
	wip.When(SetWIPLimit{Status: StatusInReview, Limit: 3}).Then(t, WIPLimitSet{Status: StatusInReview, Limit: 3})
	wip.When(SetWIPLimit{Status: StatusInReview, Limit: 0}).Then(t)
	wip.When(SetWIPLimit{Status: StatusBacklog, Limit: 3}).ThenError(t, refuse("status", "The board has no such column."))
	wip.When(SetWIPLimit{Status: StatusDone, Limit: 1000}).ThenError(t, refuse("limit", "A limit is 0 (none) to 999."))

	sp := eventstest.For(createSprint).Given(webCreated)
	sp.When(CreateSprint{ID: "s1", Name: " Sprint 1 ", Start: "2026-10-19", End: "2026-11-01"}).
		Then(t, SprintCreated{Sprint: "s1", Name: "Sprint 1", Start: "2026-10-19", End: "2026-11-01"})
	sp.When(CreateSprint{ID: "s1"}).ThenError(t, refuse("name", "A sprint's name has 1 to 60 characters."))
	sp.When(CreateSprint{ID: "s1", Name: "x", Goal: strings.Repeat("g", 501)}).ThenError(t, refuse("goal", "A goal has at most 500 characters."))
	sp.When(CreateSprint{ID: "s1", Name: "x", Start: "19.10.2026"}).ThenError(t, refuse("dates", "A date looks like 2026-10-21."))
	sp.When(CreateSprint{ID: "s1", Name: "x", Start: "2026-10-19", End: "2026-10-18"}).ThenError(t, refuse("dates", "A sprint ends after it starts."))

	start := eventstest.For(startSprint).Given(webCreated, SprintCreated{Sprint: "s1", Name: "One"}, SprintCreated{Sprint: "s2", Name: "Two"})
	start.When(StartSprint{Sprint: "s1"}).Then(t, SprintStarted{Sprint: "s1"})
	start.When(StartSprint{Sprint: "s9"}).ThenError(t, refuse("sprint", "That sprint does not exist."))
	start.Given(SprintStarted{Sprint: "s1"}).When(StartSprint{Sprint: "s1"}).Then(t)
	start.Given(SprintStarted{Sprint: "s1"}).When(StartSprint{Sprint: "s2"}).ThenError(t, refuse("sprint", "Complete the active sprint first."))
	start.Given(SprintStarted{Sprint: "s1"}, SprintCompleted{Sprint: "s1"}).When(StartSprint{Sprint: "s1"}).ThenError(t, refuse("sprint", "That sprint is closed."))
	start.When(StartSprint{Sprint: "s1", End: "soon"}).ThenError(t, refuse("dates", "A date looks like 2026-10-21."))

	done := eventstest.For(completeSprint).Given(webCreated, SprintCreated{Sprint: "s1", Name: "One"}, SprintCreated{Sprint: "s2", Name: "Two"})
	done.When(CompleteSprint{Sprint: "s1"}).ThenError(t, refuse("sprint", "Only the active sprint can be completed."))
	done.When(CompleteSprint{Sprint: "s9"}).ThenError(t, refuse("sprint", "That sprint does not exist."))
	active := done.Given(SprintStarted{Sprint: "s1"})
	active.When(CompleteSprint{Sprint: "s1", MoveTo: "s2"}).Then(t, SprintCompleted{Sprint: "s1", MoveTo: "s2"})
	active.When(CompleteSprint{Sprint: "s1", MoveTo: "s1"}).ThenError(t, refuse("move_to", "Move the open issues to a future sprint or to the backlog."))
	active.Given(SprintCompleted{Sprint: "s1"}).When(CompleteSprint{Sprint: "s1"}).Then(t)

	// FR-11.1: numbers start at 1 and follow the highest given.
	num := eventstest.For(numberDecider).Given(webCreated)
	num.When(numberIssue{Issue: "i1"}).Then(t, NumberAssigned{Number: 1, Issue: "i1"})
	num.Given(NumberAssigned{Number: 1, Issue: "i1"}, NumberAssigned{Number: 2, Issue: "i2"}).When(numberIssue{Issue: "i3"}).Then(t, NumberAssigned{Number: 3, Issue: "i3"})
	num.Given(ProjectArchived{}).When(numberIssue{Issue: "i1"}).ThenError(t, errArchived) // FR-04.1
	eventstest.For(numberDecider).When(numberIssue{Issue: "i1"}).ThenError(t, errNoProject)
}

type errorString string

func (e errorString) Error() string { return string(e) }

// webState is the WEB project as an issue's commands see it.
func webState() ProjectState {
	return eventstest.For(createProject).Given(webCreated, ComponentDefined{Name: "Search"},
		SprintCreated{Sprint: "s1", Name: "One"}, SprintCreated{Sprint: "s2", Name: "Two"}, SprintStarted{Sprint: "s2"}, SprintCompleted{Sprint: "s2"}).State()
}

func TestCreateIssue(t *testing.T) {
	p := webState()
	c := func(m func(*newIssue)) newIssue {
		n := newIssue{CreateIssue: CreateIssue{By: tabs.By{Actor: sunny}, Project: "p1", Type: Story, Summary: "  Fix the login  "},
			project: p, number: 12, rank: "a5"}
		if m != nil {
			m(&n)
		}
		return n
	}
	base := IssueCreated{Project: "p1", Number: 12, Key: "WEB-12", Type: Story, Summary: "Fix the login", Status: StatusToDo,
		Category: ToDo, Priority: Medium, Reporter: sunny.ID, Rank: "a5", Watchers: []string{sunny.ID}}
	none := eventstest.For(createIssue)
	none.When(c(nil)).Then(t, base) // FR-10.2: To Do, Medium, the actor reports

	desc := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hi "},{"type":"mention","attrs":{"personId":"s-priya","label":"Ada"}}]}]}`)
	full := base
	full.Type, full.Priority, full.Status, full.Category = Bug, High, StatusInProgress, InProgress
	full.Assignee, full.Labels, full.Components, full.Due = rainy.ID, []string{"checkout", "ux"}, []string{"Search"}, "2026-11-01"
	full.Points, full.Estimate, full.Remaining, full.Sprint = ptr(5), ptr(90), ptr(90), "s1"
	full.Description, full.Mentions = desc, []string{"s-priya"}
	full.Watchers = []string{sunny.ID, rainy.ID, "s-priya"} // FR-27.2
	none.When(c(func(n *newIssue) {
		n.Type, n.Priority, n.Status, n.Assignee, n.assign = Bug, High, StatusInProgress, rainy.ID, assignCheck{OK: true}
		n.Labels, n.Components, n.Due = []string{"Checkout", "ux", "checkout"}, []string{"search"}, "2026-11-01"
		n.Points, n.Estimate, n.Sprint, n.Description = ptr(5), ptr(90), "s1", desc
	})).Then(t, full)

	for _, x := range []struct {
		name string
		m    func(*newIssue)
		err  error
	}{
		{"type", func(n *newIssue) { n.Type = "feature" }, refuse(FieldType, "Pick one of Epic, Story, Task, Bug and Subtask.")},
		{"empty summary", func(n *newIssue) { n.Summary = "   " }, refuse(FieldSummary, "A summary has 1 to 255 characters.")}, // FR-10.2
		{"long summary", func(n *newIssue) { n.Summary = strings.Repeat("é", 256) }, refuse(FieldSummary, "A summary has 1 to 255 characters.")},
		{"description", func(n *newIssue) { n.Description = json.RawMessage(`{"type":"p"}`) }, refuse(FieldDescription, "The description is not a document.")},
		{"long description", func(n *newIssue) { n.Description = PlainDoc(strings.Repeat("x", 20001)) }, refuse(FieldDescription, "A description has at most 20,000 characters.")}, // DM-05
		{"mention", func(n *newIssue) {
			n.Description = json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"personId":"root"}}]}]}`)
		}, refuse(FieldDescription, "A mention names nobody.")},
		{"status", func(n *newIssue) { n.Status = "wontfix" }, refuse(FieldStatus, "This project's workflow has no such status.")}, // FR-35.2
		{"priority", func(n *newIssue) { n.Priority = "urgent" }, refuse(FieldPriority, "Pick Low, Medium, High or Critical.")},
		{"assignee", func(n *newIssue) {
			n.Assignee, n.assign = rainy.ID, assignCheck{Why: "Rainy cannot be assigned: they are not a member of WEB."}
		}, refuse(FieldAssignee, "Rainy cannot be assigned: they are not a member of WEB.")}, // FR-10.4
		{"label", func(n *newIssue) { n.Labels = []string{"two words"} }, refuse(FieldLabels, "A label has 1 to 50 characters and no spaces.")}, // FR-23
		{"long label", func(n *newIssue) { n.Labels = []string{strings.Repeat("l", 51)} }, refuse(FieldLabels, "A label has 1 to 50 characters and no spaces.")},
		{"component", func(n *newIssue) { n.Components = []string{"Cart"} }, refuse(FieldComponents, "WEB has no component Cart.")},
		{"epic's parent", func(n *newIssue) {
			n.Type, n.Parent, n.parent = Epic, "e1", related{Exists: true, Type: Epic, Project: "p1"}
		}, refuse(FieldParent, "An epic has no parent.")},
		{"subtask's parent", func(n *newIssue) { n.Type = Subtask }, refuse(FieldParent, "A subtask needs a parent.")},
		{"subtask under an epic", func(n *newIssue) {
			n.Type, n.Parent, n.parent = Subtask, "e1", related{Exists: true, Type: Epic, Project: "p1"}
		}, refuse(FieldParent, "A subtask's parent is a story, a task or a bug.")},
		{"story under a story", func(n *newIssue) {
			n.Parent, n.parent = "s1", related{Exists: true, Type: Story, Project: "p1"}
		}, refuse(FieldParent, "The parent of a story is an epic.")},
		{"parent elsewhere", func(n *newIssue) {
			n.Parent, n.parent = "e1", related{Exists: true, Type: Epic, Project: "p2"}
		}, refuse(FieldParent, "The parent is in another project.")},
		{"parent unknown", func(n *newIssue) { n.Parent = "e1" }, refuse(FieldParent, "That issue does not exist.")},
		{"parent deleted", func(n *newIssue) {
			n.Parent, n.parent = "e1", related{Exists: true, Deleted: true, Type: Epic, Project: "p1"}
		}, refuse(FieldParent, "That issue was deleted.")},
		{"parent itself", func(n *newIssue) { n.Parent, n.parent = "x", related{Exists: true, Self: true} }, refuse(FieldParent, "An issue cannot be its own parent.")},
		{"due", func(n *newIssue) { n.Due = "2026-02-30" }, refuse(FieldDue, "A due date looks like 2026-10-21.")},
		{"points", func(n *newIssue) { n.Points = ptr(101) }, refuse(FieldPoints, "Story points are 0 to 100.")}, // FR-26
		{"estimate", func(n *newIssue) { n.Estimate = ptr(-1) }, refuse(FieldEstimate, "An estimate is 0 to 1,000 hours.")},
		{"remaining", func(n *newIssue) { n.Remaining = ptr(MaxEstimate + 1) }, refuse(FieldRemaining, "An estimate is 0 to 1,000 hours.")},
		{"sprint", func(n *newIssue) { n.Sprint = "s9" }, refuse(FieldSprint, "That sprint does not exist.")},
		{"closed sprint", func(n *newIssue) { n.Sprint = "s2" }, refuse(FieldSprint, "That sprint is closed.")},
		{"archived", func(n *newIssue) { n.project.Archived = true }, errArchived}, // FR-04.1
	} {
		t.Run(x.name, func(t *testing.T) { none.When(c(x.m)).ThenError(t, x.err) })
	}
	none.Given(base).When(c(nil)).ThenError(t, errorString("tracker: the issue exists"))
}

// story is WEB-12 as most issue deciders find it: a story in To Do,
// created at version 1 by Sunny.
var story = IssueCreated{Project: "p1", Number: 12, Key: "WEB-12", Type: Story, Summary: "Fix the login", Status: StatusToDo,
	Category: ToDo, Priority: Medium, Reporter: sunny.ID, Rank: "a5", Watchers: []string{sunny.ID}}

func ed(field, value string, seen int64) edit {
	return edit{Field: field, Text: value, Seen: seen, wf: DefaultWorkflow(), sprints: webState().Sprints}
}

// FR-15, FR-112: a version of a co-edited description is cut once per
// version of its document, and tells only the people it mentions first.
func TestCutDescription(t *testing.T) {
	doc := PlainDoc("Steps to reproduce")
	mention := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"personId":"s-priya"}}]}]}`)
	s := eventstest.For(cutDescDecider).Given(story)
	cut := DescriptionChanged{To: doc, Upto: 4, Editors: []string{sunny.ID, rainy.ID}}
	s.When(cut).Then(t, cut)
	s.When(DescriptionChanged{To: mention, Upto: 2}).Then(t, DescriptionChanged{To: mention, Upto: 2, Mentions: []string{"s-priya"}},
		WatchersAdded{People: []string{"s-priya"}}) // FR-27.2
	s.When(DescriptionChanged{To: doc, Upto: 1}).Then(t) // the version the issue was created with
	again := eventstest.For(cutDescDecider).Given(story, DescriptionChanged{To: mention, Upto: 2, Mentions: []string{"s-priya"}}, WatchersAdded{People: []string{"s-priya"}})
	again.When(DescriptionChanged{To: doc, Upto: 2}).Then(t)
	again.When(DescriptionChanged{To: mention, Upto: 3}).Then(t, DescriptionChanged{To: mention, Upto: 3}) // FR-67.3: mentioned before
	if st := again.State(); st.DescUpto != 2 {
		t.Errorf("DescUpto %d", st.DescUpto)
	}
	eventstest.For(cutDescDecider).Given(story, IssueDeleted{}).When(cut).ThenError(t, errDeleted)
}

func TestEditIssue(t *testing.T) {
	s := eventstest.For(editDecider).Given(story)
	for _, x := range []struct {
		name string
		c    edit
		want []IssueEvent
	}{
		{"summary", ed(FieldSummary, " Fix logins ", 1), []IssueEvent{SummaryChanged{From: "Fix the login", To: "Fix logins"}}},
		{"same summary", ed(FieldSummary, "Fix the login ", 1), nil}, // FR-14.3
		{"type", ed(FieldType, "bug", 1), []IssueEvent{TypeChanged{From: Story, To: Bug}}},
		{"status", ed(FieldStatus, StatusInReview, 1), []IssueEvent{StatusChanged{From: StatusToDo, To: StatusInReview, Category: InProgress}}},
		{"same status", ed(FieldStatus, StatusToDo, 1), nil},
		{"priority", ed(FieldPriority, "high", 1), []IssueEvent{PriorityChanged{From: Medium, To: High}}},
		{"assignee", edit{Field: FieldAssignee, Text: rainy.ID, Seen: 1, assign: assignCheck{OK: true}},
			[]IssueEvent{AssigneeChanged{To: rainy.ID}, WatchersAdded{People: []string{rainy.ID}}}}, // FR-17.3
		{"assign the reporter", edit{Field: FieldAssignee, Text: sunny.ID, Seen: 1, assign: assignCheck{OK: true}}, []IssueEvent{AssigneeChanged{To: sunny.ID}}},
		{"parent", edit{Field: FieldParent, Text: "e1", Seen: 1, parent: related{Exists: true, Type: Epic, Project: "p1"}}, []IssueEvent{ParentChanged{To: "e1"}}},
		{"due", ed(FieldDue, "2026-11-01", 1), []IssueEvent{DueChanged{To: "2026-11-01"}}},
		{"points", edit{Field: FieldPoints, Num: ptr(3), Seen: 1}, []IssueEvent{PointsChanged{To: ptr(3)}}},
		{"same points", edit{Field: FieldPoints, Seen: 1}, nil},
		{"estimate", edit{Field: FieldEstimate, Num: ptr(150), Seen: 1}, []IssueEvent{EstimateChanged{To: ptr(150)}}},
		{"remaining", edit{Field: FieldRemaining, Num: ptr(0), Seen: 1}, []IssueEvent{RemainingChanged{To: ptr(0)}}},
		{"sprint", ed(FieldSprint, "s1", 1), []IssueEvent{SprintChanged{To: "s1"}}},
	} {
		t.Run(x.name, func(t *testing.T) { s.When(x.c).Then(t, x.want...) })
	}
	for _, x := range []struct {
		name string
		c    edit
		err  error
	}{
		{"summary", ed(FieldSummary, "", 1), refuse(FieldSummary, "A summary has 1 to 255 characters.")}, // FR-14.2
		{"type", ed(FieldType, "chore", 1), refuse(FieldType, "Pick one of Epic, Story, Task, Bug and Subtask.")},
		{"epic with children", edit{Field: FieldType, Text: "task", Seen: 1, children: 4}, nil},
		{"story with subtasks to epic", edit{Field: FieldType, Text: "epic", Seen: 1, children: 4}, refuse(FieldType, "Move the 4 child issues first.")}, // FR-19.2
		{"story with a subtask to subtask", edit{Field: FieldType, Text: "subtask", Seen: 1, children: 1}, refuse(FieldType, "Move the child issue first.")},
		{"subtask without parent", ed(FieldType, "subtask", 1), refuse(FieldType, "A subtask needs a parent.")},
		{"status", ed(FieldStatus, "closed", 1), refuse(FieldStatus, "This project's workflow has no such status.")},
		{"priority", ed(FieldPriority, "urgent", 1), refuse(FieldPriority, "Pick Low, Medium, High or Critical.")},
		{"assignee", edit{Field: FieldAssignee, Text: rainy.ID, Seen: 1, assign: assignCheck{Why: "Nope."}}, refuse(FieldAssignee, "Nope.")}, // FR-17.2
		{"parent", edit{Field: FieldParent, Text: "t1", Seen: 1, parent: related{Exists: true, Type: Task, Project: "p1"}}, refuse(FieldParent, "The parent of a story is an epic.")},
		{"due", ed(FieldDue, "tomorrow", 1), refuse(FieldDue, "A due date looks like 2026-10-21.")},
		{"points", edit{Field: FieldPoints, Num: ptr(-1), Seen: 1}, refuse(FieldPoints, "Story points are 0 to 100.")},
		{"estimate", edit{Field: FieldEstimate, Num: ptr(MaxEstimate + 1), Seen: 1}, refuse(FieldEstimate, "An estimate is 0 to 1,000 hours.")},
		{"sprint", ed(FieldSprint, "s2", 1), refuse(FieldSprint, "That sprint is closed.")},
		{"field", ed("colour", "red", 1), errorString(`tracker: no field "colour"`)},
	} {
		t.Run(x.name, func(t *testing.T) {
			if x.err == nil {
				s.When(x.c).Then(t, TypeChanged{From: Story, To: Task}) // story, task and bug swap with children
				return
			}
			s.When(x.c).ThenError(t, x.err)
		})
	}
	eventstest.For(editDecider).When(ed(FieldSummary, "x", 0)).ThenError(t, errNoIssue)
	s.Given(IssueDeleted{}).When(ed(FieldSummary, "x", 2)).ThenError(t, errDeleted)

	// A subtask's type: its parent decides.
	sub := eventstest.For(editDecider).Given(IssueCreated{Project: "p1", Key: "WEB-13", Type: Subtask, Parent: "s1", Status: StatusToDo, Priority: Medium})
	sub.When(edit{Field: FieldType, Text: "task", Seen: 1, parent: related{Exists: true, Type: Story, Project: "p1"}}).
		ThenError(t, refuse(FieldType, "The parent of a task is an epic."))
	sub.When(edit{Field: FieldParent, Text: "", Seen: 1}).ThenError(t, refuse(FieldParent, "A subtask needs a parent."))
	eventstest.For(editDecider).Given(IssueCreated{Project: "p1", Type: Story, Parent: "e1", Status: StatusToDo}).
		When(edit{Field: FieldType, Text: "epic", Seen: 1, parent: related{Exists: true, Type: Epic, Project: "p1"}}).
		ThenError(t, refuse(FieldType, "An epic has no parent."))
}

// MP-08.2, MP-08.3: a change to a field someone else changed after the
// version the page showed is refused; a change to another field applies.
func TestEditConflicts(t *testing.T) {
	s := eventstest.For(editDecider).Given(story, PriorityChanged{From: Medium, To: High}, SummaryChanged{From: "Fix the login", To: "Fix logins"})
	s.When(ed(FieldPriority, "critical", 1)).ThenError(t, &conflictErr{Field: FieldPriority, Version: 2, Yours: "critical"})
	s.When(ed(FieldSummary, "Fix it", 2)).ThenError(t, &conflictErr{Field: FieldSummary, Version: 3, Yours: "Fix it"})
	s.When(ed(FieldStatus, StatusDone, 1)).Then(t, StatusChanged{From: StatusToDo, To: StatusDone, Category: Done}) // MP-08.2
	s.When(ed(FieldPriority, "critical", 2)).Then(t, PriorityChanged{From: High, To: Critical})                     // seen it
	s.When(ed(FieldPriority, "high", 1)).Then(t)                                                                    // the same value: nothing, no conflict
	own := ed(FieldPriority, "critical", 1)
	own.own = []string{FieldPriority}
	s.When(own).Then(t, PriorityChanged{From: High, To: Critical}) // one's own change never conflicts
}

func TestSetDeciders(t *testing.T) {
	s := eventstest.For(labelsDecider).Given(story, LabelsChanged{Added: []string{"ux"}})
	s.When(setLabels{add: []string{"UX", "Checkout"}, remove: nil}).Then(t, LabelsChanged{Added: []string{"checkout"}})
	s.When(setLabels{remove: []string{"ux", "gone"}}).Then(t, LabelsChanged{Removed: []string{"ux"}})
	s.When(setLabels{add: []string{"ux"}}).Then(t)
	s.When(setLabels{add: []string{"a b"}}).ThenError(t, refuse(FieldLabels, "A label has 1 to 50 characters and no spaces."))
	many := make([]string, 20)
	for i := range many {
		many[i] = "l" + itoa(i)
	}
	s.When(setLabels{add: many}).ThenError(t, refuse(FieldLabels, "An issue has 20 labels at most."))
	eventstest.For(labelsDecider).Given(story, IssueDeleted{}).When(setLabels{add: []string{"x"}}).ThenError(t, errDeleted)

	defined := []Component{{Name: "Search"}, {Name: "Cart"}}
	c := eventstest.For(componentsDecider).Given(story)
	c.When(setComponents{add: []string{"search", "cart"}, defined: defined, key: "WEB"}).Then(t, ComponentsChanged{Added: []string{"Search", "Cart"}})
	c.When(setComponents{add: []string{"Emails"}, defined: defined, key: "WEB"}).ThenError(t, refuse(FieldComponents, "WEB has no component Emails."))
	c.Given(ComponentsChanged{Added: []string{"Search"}}).When(setComponents{remove: []string{"Search"}, force: true}).Then(t, ComponentsChanged{Removed: []string{"Search"}})
	c.When(setComponents{remove: []string{"Search"}, defined: defined}).Then(t)

	w := eventstest.For(watchDecider).Given(story)
	w.When(watch{add: []string{rainy.ID}}).Then(t, WatchersAdded{People: []string{rainy.ID}})
	w.When(watch{add: []string{sunny.ID}}).Then(t)
	w.When(watch{remove: sunny.ID}).Then(t, WatcherRemoved{Person: sunny.ID})
	w.When(watch{remove: rainy.ID}).Then(t)
	eventstest.For(watchDecider).Given(story, IssueDeleted{}).When(watch{add: []string{rainy.ID}}).ThenError(t, errDeleted)

	// FR-22.2: no second identical link, in either direction.
	l := eventstest.For(linkDecider).Given(story)
	l.When(link{Link: Link{Type: Blocks, Outward: true, Issue: "i2"}}).Then(t, LinkAdded{Link: Blocks, Outward: true, Issue: "i2"})
	linked := l.Given(LinkAdded{Link: Blocks, Outward: false, Issue: "i2"})
	linked.When(link{Link: Link{Type: Blocks, Outward: true, Issue: "i2"}}).ThenError(t, refuse(FieldLinks, "These issues are linked so already."))
	linked.When(link{Link: Link{Type: RelatesTo, Outward: true, Issue: "i2"}}).Then(t, LinkAdded{Link: RelatesTo, Outward: true, Issue: "i2"})
	linked.When(link{Link: Link{Type: Blocks, Issue: "i2"}, remove: true}).Then(t, LinkRemoved{Link: Blocks, Outward: false, Issue: "i2"})
	l.When(link{Link: Link{Type: Blocks, Issue: "i2"}, remove: true}).Then(t)
	eventstest.For(linkDecider).Given(story, IssueDeleted{}).When(link{Link: Link{Type: Blocks, Issue: "i2"}}).ThenError(t, errDeleted)

	d := eventstest.For(deleteDecider).Given(story)
	d.When(deleteIssue{}).Then(t, IssueDeleted{})
	d.Given(IssueDeleted{}).When(deleteIssue{}).Then(t)
	d.When(deleteIssue{children: 1}).ThenError(t, &writer.Refusal{Msg: "Delete or move its child issue first."})
	d.When(deleteIssue{children: 3}).ThenError(t, &writer.Refusal{Msg: "Delete or move its 3 child issues first."})
	eventstest.For(deleteDecider).When(deleteIssue{}).ThenError(t, errNoIssue)
}

// FR-41, MP-04.3: a move changes status and rank in one event; a card
// someone else moved after Seen is refused.
func TestMoveDecider(t *testing.T) {
	wf := DefaultWorkflow()
	s := eventstest.For(moveDecider).Given(story)
	s.When(move{Status: StatusInReview, Seen: 1, lower: "a1", upper: "a2", wf: wf}).
		Then(t, Moved{FromStatus: StatusToDo, Status: StatusInReview, Category: InProgress, FromRank: "a5", Rank: "a1V"})
	s.When(move{Status: StatusToDo, Seen: 1, lower: "a6", wf: wf}).Then(t, Moved{FromStatus: StatusToDo, Status: StatusToDo, Category: ToDo, FromRank: "a5", Rank: "a7"})
	s.When(move{Status: StatusInReview, Seen: 1, lower: "a4", upper: "a6", wf: wf}).
		Then(t, Moved{FromStatus: StatusToDo, Status: StatusInReview, Category: InProgress, FromRank: "a5", Rank: "a5"}) // the rank fits: it stays
	s.When(move{Status: StatusToDo, Seen: 1, lower: "a4", upper: "a6", wf: wf}).Then(t) // where it is: nothing
	s.When(move{Status: "closed", Seen: 1, wf: wf}).ThenError(t, refuse(FieldMove, "This project's workflow has no such status."))
	strict := wf
	strict.Follows = map[string][]string{StatusDone: nil}
	s.When(move{Status: StatusDone, Seen: 1, wf: strict}).ThenError(t, refuse(FieldMove, "Nothing can move to Done."))
	strict.Follows[StatusDone] = []string{StatusInReview}
	s.When(move{Status: StatusDone, Seen: 1, wf: strict}).ThenError(t, refuse(FieldMove, "Done can only follow In Review.")) // FR-37
	s.When(move{Status: StatusInProgress, Seen: 1, wf: strict, lower: "a6"}).Then(t, Moved{FromStatus: StatusToDo, Status: StatusInProgress, Category: InProgress, FromRank: "a5", Rank: "a7"})

	moved := s.Given(Moved{FromStatus: StatusToDo, Status: StatusInReview, Category: InProgress, FromRank: "a5", Rank: "a3"})
	moved.When(move{Status: StatusDone, Seen: 1, wf: wf}).ThenError(t, &conflictErr{Field: FieldMove, Version: 2, Yours: StatusDone})
	moved.When(move{Status: StatusDone, Seen: 2, wf: wf, lower: "a3"}).Then(t, Moved{FromStatus: StatusInReview, Status: StatusDone, Category: Done, FromRank: "a3", Rank: "a4"})
	ranked := s.Given(Moved{FromStatus: StatusToDo, Status: StatusToDo, Category: ToDo, FromRank: "a5", Rank: "a3"})
	ranked.When(move{Status: StatusDone, Seen: 1, wf: wf}).ThenError(t, &conflictErr{Field: FieldMove, Version: 2, Yours: StatusDone}) // reranked counts too
	s.Given(PriorityChanged{From: Medium, To: High}).When(move{Status: StatusDone, Seen: 1, lower: "a6", wf: wf}).
		Then(t, Moved{FromStatus: StatusToDo, Status: StatusDone, Category: Done, FromRank: "a5", Rank: "a7"}) // other fields do not
	s.Given(IssueDeleted{}).When(move{Status: StatusDone, wf: wf}).ThenError(t, errDeleted)
}

// FR-29: a move to another project.
func TestProjectMoveDecider(t *testing.T) {
	app := eventstest.For(createProject).Given(ProjectCreated{Key: "APP", Name: "App", Workflow: DefaultWorkflow()}, NumberAssigned{Number: 6}).State()
	s := eventstest.For(projectMoveDecider).Given(story, ParentChanged{To: "e1"}, ComponentsChanged{Added: []string{"Search"}}, SprintChanged{To: "s1"})
	s.When(projectMove{target: app, project: "p2", rank: "a0"}).Then(t,
		ParentChanged{From: "e1"}, ComponentsChanged{Removed: []string{"Search"}}, SprintChanged{From: "s1"},
		ProjectChanged{FromProject: "p1", Project: "p2", FromKey: "WEB-12", Key: "APP-7", Number: 7, Status: StatusToDo, Category: ToDo, Rank: "a0"})
	s.When(projectMove{target: app, project: "p1"}).Then(t)
	s.When(projectMove{target: app, project: "p2", children: 2}).ThenError(t, refuse(FieldProject, "Move the 2 child issues first."))
	s.When(projectMove{target: app, project: "p2", children: 1}).ThenError(t, refuse(FieldProject, "Move the child issue first."))
	s.When(projectMove{target: app, project: "p2", status: "gone"}).ThenError(t, refuse(FieldStatus, "Pick a status of App's workflow.")) // FR-29.2
	arch := app
	arch.Archived = true
	s.When(projectMove{target: arch, project: "p2"}).ThenError(t, errArchived)
	eventstest.For(projectMoveDecider).Given(IssueCreated{Project: "p1", Type: Subtask, Parent: "x", Status: StatusToDo}).
		When(projectMove{target: app, project: "p2"}).ThenError(t, refuse(FieldProject, "A subtask stays in its parent's project."))
	s.Given(IssueDeleted{}).When(projectMove{target: app, project: "p2"}).ThenError(t, errDeleted)
	eventstest.For(projectMoveDecider).When(projectMove{project: "p2"}).ThenError(t, errNoIssue)
}

func TestCommentDeciders(t *testing.T) {
	body := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Looks good "},{"type":"mention","attrs":{"personId":"s-priya","label":"Ada"}}]}]}`)
	add := eventstest.For(addComment)
	nc := func(b json.RawMessage) newComment {
		return newComment{AddComment: AddComment{Issue: "i1", Body: b}, project: "p1"}
	}
	add.When(nc(body)).Then(t, CommentAdded{Subject: SubjectIssue, SubjectID: "i1", Project: "p1", Body: body, Mentions: []string{"s-priya"}})
	add.When(nc(PlainDoc("  "))).ThenError(t, refuse("body", "Write something first.")) // FR-65.1
	add.When(nc(PlainDoc(strings.Repeat("x", 10001)))).ThenError(t, refuse("body", "A comment has at most 10,000 characters."))
	add.When(nc(json.RawMessage(`{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"H"}]}]}`))).
		ThenError(t, refuse("body", "A comment has no headings and no tables."))
	add.When(nc(json.RawMessage(`"text"`))).ThenError(t, refuse("body", "The comment is not a document."))
	var ms []string
	for i := range 11 {
		ms = append(ms, `{"type":"mention","attrs":{"personId":"s-p`+itoa(i)+`"}}`)
	}
	add.When(nc(json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[`+strings.Join(ms, ",")+`]}]}`))).
		ThenError(t, refuse("body", "A comment mentions 10 people at most.")) // DM-05
	gone := nc(body)
	gone.deleted = true
	add.When(gone).ThenError(t, errDeleted)

	c := CommentAdded{Subject: SubjectIssue, SubjectID: "i1", Project: "p1", Body: PlainDoc("First"), Mentions: []string{"s-priya"}}
	edit := eventstest.For(editCommentDecider).Given(c)
	both := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"personId":"s-priya"}},{"type":"mention","attrs":{"personId":"s-tomasz"}}]}]}`)
	edit.When(editComment{both}).Then(t, CommentEdited{Body: both, Mentions: []string{"s-tomasz"}})                                           // FR-67.3: Ada was told already
	edit.Given(CommentEdited{Body: PlainDoc("x")}).When(editComment{both}).Then(t, CommentEdited{Body: both, Mentions: []string{"s-tomasz"}}) // once, even when she comes back
	edit.When(editComment{PlainDoc("First")}).Then(t)
	edit.When(editComment{PlainDoc("")}).ThenError(t, refuse("body", "Write something first."))
	edit.Given(CommentDeleted{}).When(editComment{PlainDoc("x")}).ThenError(t, &writer.Refusal{Msg: "This comment was deleted."})

	del := eventstest.For(deleteCommentDecider).Given(c)
	del.When(DeleteComment{}).Then(t, CommentDeleted{})
	del.Given(CommentDeleted{}).When(DeleteComment{}).Then(t)

	hide := eventstest.For(hideCommentDecider).Given(c)
	hide.When(HideComment{Hidden: true, Reason: " spam "}).Then(t, CommentHidden{Reason: "spam"})
	hide.When(HideComment{Hidden: false}).Then(t)
	hide.When(HideComment{Hidden: true, Reason: strings.Repeat("r", 201)}).ThenError(t, refuse("reason", "A reason has at most 200 characters."))
	hide.Given(CommentHidden{}).When(HideComment{Hidden: false}).Then(t, CommentUnhidden{})
}
