package tracker

import (
	"fmt"
	"slices"
	"strings"
)

// IssueType is one of the five types of section 4.2 (FR-19).
type IssueType string

const (
	Epic    IssueType = "epic"
	Story   IssueType = "story"
	Task    IssueType = "task"
	Bug     IssueType = "bug"
	Subtask IssueType = "subtask"
)

// Types are the issue types in the order menus list them.
var Types = []IssueType{Epic, Story, Task, Bug, Subtask}

// Valid reports whether t is one of the types.
func (t IssueType) Valid() bool { return slices.Contains(Types, t) }

// Name is the type as pages show it: "Story".
func (t IssueType) Name() string {
	switch t {
	case Epic:
		return "Epic"
	case Story:
		return "Story"
	case Task:
		return "Task"
	case Bug:
		return "Bug"
	case Subtask:
		return "Subtask"
	}
	return string(t)
}

// Priority is one of the four priorities, in order of severity (FR-18).
type Priority string

const (
	Low      Priority = "low"
	Medium   Priority = "medium"
	High     Priority = "high"
	Critical Priority = "critical"
)

// Priorities are the priorities from the least to the most severe.
var Priorities = []Priority{Low, Medium, High, Critical}

// Valid reports whether p is one of the priorities.
func (p Priority) Valid() bool { return slices.Contains(Priorities, p) }

// Order is the priority's severity, 1 for Low to 4 for Critical: lists
// sort by it (FR-53.2).
func (p Priority) Order() int { return slices.Index(Priorities, p) + 1 }

// Name is the priority as pages show it: "High".
func (p Priority) Name() string {
	switch p {
	case Low:
		return "Low"
	case Medium:
		return "Medium"
	case High:
		return "High"
	case Critical:
		return "Critical"
	}
	return string(p)
}

// Category is a status's category (section 4.2).
type Category string

const (
	ToDo       Category = "todo"
	InProgress Category = "inprogress"
	Done       Category = "done"
)

// Categories in workflow order.
var Categories = []Category{ToDo, InProgress, Done}

// Valid reports whether c is one of the categories.
func (c Category) Valid() bool { return slices.Contains(Categories, c) }

// Name is the category as pages show it: "In Progress".
func (c Category) Name() string {
	switch c {
	case ToDo:
		return "To Do"
	case InProgress:
		return "In Progress"
	case Done:
		return "Done"
	}
	return string(c)
}

// Status is a status of a workflow: a stable id, which issues and URLs
// hold, a name, which pages show and an Admin may change later (FR-36),
// and its category.
type Status struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category Category `json:"category"`
}

// Workflow is a project's ordered statuses and the transitions between
// them (FR-35 to FR-37). Transitions nil allows every one.
type Workflow struct {
	Statuses    []Status            `json:"statuses"`
	Start       string              `json:"start"`
	Transitions map[string][]string `json:"transitions,omitempty"`
}

// The statuses of the default workflow (section 4.2), by id.
const (
	StatusBacklog    = "backlog"
	StatusSelected   = "selected"
	StatusToDo       = "todo"
	StatusInProgress = "inprogress"
	StatusInReview   = "inreview"
	StatusBlocked    = "blocked"
	StatusDone       = "done"
)

// DefaultWorkflow is the workflow every project starts with (FR-35.1):
// seven statuses in three categories, every transition allowed, new
// issues in To Do.
func DefaultWorkflow() Workflow {
	return Workflow{
		Statuses: []Status{
			{StatusBacklog, "Backlog", ToDo},
			{StatusSelected, "Selected for Development", ToDo},
			{StatusToDo, "To Do", ToDo},
			{StatusInProgress, "In Progress", InProgress},
			{StatusInReview, "In Review", InProgress},
			{StatusBlocked, "Blocked", InProgress},
			{StatusDone, "Done", Done},
		},
		Start: StatusToDo,
	}
}

// Status returns the status with the id, if the workflow has it.
func (w Workflow) Status(id string) (Status, bool) {
	for _, s := range w.Statuses {
		if s.ID == id {
			return s, true
		}
	}
	return Status{}, false
}

// Order is the status's position in the workflow, from 1; 0 when the
// workflow lacks it.
func (w Workflow) Order(id string) int {
	return slices.IndexFunc(w.Statuses, func(s Status) bool { return s.ID == id }) + 1
}

// Name is the status's name, or its id when the workflow lacks it.
func (w Workflow) Name(id string) string {
	if s, ok := w.Status(id); ok {
		return s.Name
	}
	return id
}

// Allows reports whether the workflow lets an issue go from one status
// to another (FR-37).
func (w Workflow) Allows(from, to string) bool {
	if w.Transitions == nil || from == to {
		return true
	}
	return slices.Contains(w.Transitions[from], to)
}

// OnBoard reports whether a status has a column on the board: every one
// but Backlog (FR-40.1).
func OnBoard(status string) bool { return status != StatusBacklog }

// Limits of section 4.2 and DM-05.
const (
	MinKey           = 2
	MaxKey           = 6
	MaxProjectName   = 100
	MaxDescription   = 2000 // a project's description, in characters
	MaxSummary       = 255
	MaxIssueText     = 20000 // characters of text in an issue's description
	MaxCommentText   = 10000
	MaxDocBytes      = 256 << 10 // a description or comment as stored JSON
	MaxLabel         = 50
	MaxLabels        = 20 // per issue
	MaxComponents    = 20 // per issue
	MaxComponent     = 50 // a component's name
	MaxMentions      = 10 // per comment (DM-05)
	MaxPoints        = 100
	MaxEstimate      = 60000 // minutes: 1,000 hours
	MaxSprintName    = 60
	MaxSprintGoal    = 500
	MaxFilterName    = 60
	MaxWIP           = 999
	PageSize         = 25 // issues per page of a list (FR-54.1)
	AssignableRecent = 24 // hours a guest stays assignable after acting in a project (FR-17.1)
)

// NormalizeKey turns a project key to upper case and checks it: 2 to 6
// letters A to Z (FR-02.1).
func NormalizeKey(key string) (string, bool) {
	k := strings.ToUpper(strings.TrimSpace(key))
	if len(k) < MinKey || len(k) > MaxKey {
		return k, false
	}
	for _, r := range k {
		if r < 'A' || r > 'Z' {
			return k, false
		}
	}
	return k, true
}

// IssueKey is an issue's key: "WEB-12".
func IssueKey(projectKey string, number int64) string {
	return fmt.Sprintf("%s-%d", projectKey, number)
}

// ParseIssueKey splits "web-12" into "WEB" and 12, matching without
// regard to case (FR-12.1).
func ParseIssueKey(s string) (project string, number int64, ok bool) {
	k, n, found := strings.Cut(strings.TrimSpace(s), "-")
	if !found {
		return "", 0, false
	}
	project, ok = NormalizeKey(k)
	if !ok || n == "" || len(n) > 9 || n[0] == '0' {
		return "", 0, false
	}
	for _, r := range n {
		if r < '0' || r > '9' {
			return "", 0, false
		}
		number = number*10 + int64(r-'0')
	}
	return project, number, true
}

// LinkType is a kind of link between two issues (FR-22). Each is stored
// on both issues: outward on the one that names it ("blocks"), inward on
// the other ("is blocked by").
type LinkType string

const (
	Blocks     LinkType = "blocks"
	RelatesTo  LinkType = "relates"
	Duplicates LinkType = "duplicates"
)

// LinkTypes in the order menus list them.
var LinkTypes = []LinkType{Blocks, RelatesTo, Duplicates}

// Valid reports whether l is one of the link types.
func (l LinkType) Valid() bool { return slices.Contains(LinkTypes, l) }

// Wording is how the issue that holds a link names it: outward "blocks",
// inward "is blocked by".
func (l LinkType) Wording(outward bool) string {
	switch {
	case l == Blocks && outward:
		return "blocks"
	case l == Blocks:
		return "is blocked by"
	case l == RelatesTo:
		return "relates to"
	case l == Duplicates && outward:
		return "duplicates"
	case l == Duplicates:
		return "is duplicated by"
	}
	return string(l)
}

// Sprint states (FR-46).
const (
	SprintFuture = "future"
	SprintActive = "active"
	SprintClosed = "closed"
)
