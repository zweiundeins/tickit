package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zweiundeins/lith/sqlite"

	"github.com/mbolli/tickit/internal/people"
)

// Filter values that stand for something other than an id.
const (
	Me         = "me"         // the person who looks, in Assignees and Reporters
	Unassigned = "unassigned" // no assignee, in Assignees
	NoSprint   = "none"       // in no sprint, in Sprints
)

// MaxFilterText is how long a filter's text may be.
const MaxFilterText = 200

// Filter is a list's filter (FR-51, FR-52): filters combine with AND,
// the values of one filter with OR, an empty one filters nothing. Its URL
// form (Values, ParseFilter) uses the parameters project (keys), type,
// status (ids), category, priority, assignee, reporter, label, component,
// sprint, due_from, due_to, created_from, created_to (YYYY-MM-DD,
// inclusive) and q (the text).
type Filter struct {
	Projects    []string // project keys
	Types       []IssueType
	Statuses    []string // status ids
	Categories  []Category
	Priorities  []Priority
	Assignees   []string // person ids, Me, Unassigned
	Reporters   []string // person ids, Me
	Labels      []string
	Components  []string
	Sprints     []string // sprint ids, NoSprint
	DueFrom     string
	DueTo       string
	CreatedFrom string
	CreatedTo   string
	Text        string // matches keys, summaries, descriptions and comments, without regard to case; % and _ match themselves
}

// Normalize returns the filter with values the model does not have left
// out (FR-51.2), each list sorted and each value once, and Me replaced by
// me unless me is empty. Two filters that select the same issues for the
// same person normalize to the same filter, and so to the same Key.
func (f Filter) Normalize(me string) Filter {
	keep := func(in []string, ok func(string) (string, bool)) []string {
		var out []string
		for _, v := range in {
			if n, good := ok(v); good && !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
		slices.Sort(out)
		return out
	}
	person := func(allow ...string) func(string) (string, bool) {
		return func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if v == Me && me != "" {
				return me, true
			}
			return v, slices.Contains(allow, v) || people.ValidID(v)
		}
	}
	var n Filter
	n.Projects = keep(f.Projects, NormalizeKey)
	n.Types = enums(f.Types)
	n.Statuses = keep(f.Statuses, func(v string) (string, bool) {
		v = strings.ToLower(strings.TrimSpace(v))
		return v, v != "" && len(v) <= 40 && strings.Trim(v, "abcdefghijklmnopqrstuvwxyz0123456789_-") == ""
	})
	n.Categories = enums(f.Categories)
	n.Priorities = enums(f.Priorities)
	n.Assignees = keep(f.Assignees, person(Me, Unassigned))
	n.Reporters = keep(f.Reporters, person(Me))
	n.Labels = keep(f.Labels, NormalizeLabel)
	n.Components = keep(f.Components, func(v string) (string, bool) { return text(v, 1, MaxComponent) })
	n.Sprints = keep(f.Sprints, func(v string) (string, bool) { return v, v == NoSprint || ValidID(v) })
	date := func(d string) string {
		d = strings.TrimSpace(d)
		if !validDate(d) {
			return ""
		}
		return d
	}
	n.DueFrom, n.DueTo, n.CreatedFrom, n.CreatedTo = date(f.DueFrom), date(f.DueTo), date(f.CreatedFrom), date(f.CreatedTo)
	n.Text = strings.Join(strings.Fields(f.Text), " ")
	if utf8.RuneCountInString(n.Text) > MaxFilterText {
		n.Text = string([]rune(n.Text)[:MaxFilterText])
	}
	return n
}

// enums keeps the valid values of an enumeration, sorted, each once.
func enums[T interface {
	~string
	Valid() bool
}](in []T) []T {
	var out []T
	for _, v := range in {
		v = T(strings.ToLower(strings.TrimSpace(string(v))))
		if v.Valid() && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

// IsZero reports whether the filter filters nothing.
func (f Filter) IsZero() bool { return len(f.Values()) == 0 }

// Values is the filter as URL parameters (FR-55): ParseFilter reads them
// back.
func (f Filter) Values() url.Values {
	v := url.Values{}
	add := func(key string, vals []string) {
		for _, x := range vals {
			v.Add(key, x)
		}
	}
	add("project", f.Projects)
	add("type", strs(f.Types))
	add("status", f.Statuses)
	add("category", strs(f.Categories))
	add("priority", strs(f.Priorities))
	add("assignee", f.Assignees)
	add("reporter", f.Reporters)
	add("label", f.Labels)
	add("component", f.Components)
	add("sprint", f.Sprints)
	for key, x := range map[string]string{"due_from": f.DueFrom, "due_to": f.DueTo, "created_from": f.CreatedFrom, "created_to": f.CreatedTo, "q": f.Text} {
		if x != "" {
			v.Set(key, x)
		}
	}
	return v
}

func strs[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, x := range in {
		out[i] = string(x)
	}
	return out
}

func typed[T ~string](in []string) []T {
	out := make([]T, len(in))
	for i, x := range in {
		out[i] = T(x)
	}
	return out
}

// ParseFilter reads a filter from URL parameters (Values), normalized but
// with Me kept.
func ParseFilter(v url.Values) Filter {
	return Filter{
		Projects:    v["project"],
		Types:       typed[IssueType](v["type"]),
		Statuses:    v["status"],
		Categories:  typed[Category](v["category"]),
		Priorities:  typed[Priority](v["priority"]),
		Assignees:   v["assignee"],
		Reporters:   v["reporter"],
		Labels:      v["label"],
		Components:  v["component"],
		Sprints:     v["sprint"],
		DueFrom:     v.Get("due_from"),
		DueTo:       v.Get("due_to"),
		CreatedFrom: v.Get("created_from"),
		CreatedTo:   v.Get("created_to"),
		Text:        v.Get("q"),
	}.Normalize("")
}

// Key is a stable hash of the filter, for the keys of views (MP-11):
// normalize it first.
func (f Filter) Key() string { return hash(f.Values().Encode()) }

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// The columns a list sorts by (FR-53).
const (
	SortKey      = "key"
	SortType     = "type"
	SortSummary  = "summary"
	SortStatus   = "status"   // workflow order
	SortPriority = "priority" // severity
	SortAssignee = "assignee" // display name; unassigned last
	SortReporter = "reporter"
	SortProject  = "project" // name
	SortCreated  = "created"
	SortUpdated  = "updated"
	SortDue      = "due" // none last
	SortPoints   = "points"
	SortResolved = "resolved"
	SortRank     = "rank"
)

// Sort is a list's order: a column and its direction. Ties sort by key.
type Sort struct {
	By   string
	Desc bool
}

// DefaultSort is created, newest first (FR-53.1).
var DefaultSort = Sort{By: SortCreated, Desc: true}

// sortColumns are the ORDER BY expressions of the columns; those after a
// comma sort empty values last in both directions.
var sortColumns = map[string][]string{
	SortKey:      {"p.key", "i.number"},
	SortType:     {"CASE i.type WHEN 'epic' THEN 1 WHEN 'story' THEN 2 WHEN 'task' THEN 3 WHEN 'bug' THEN 4 ELSE 5 END"},
	SortSummary:  {"i.summary COLLATE NOCASE"},
	SortStatus:   {"coalesce(s.position, 0)"},
	SortPriority: {"i.priority_order"},
	SortAssignee: {"!i.assignee = ''", "coalesce(pa.display_name, i.assignee) COLLATE NOCASE"},
	SortReporter: {"coalesce(pr.display_name, i.reporter) COLLATE NOCASE"},
	SortProject:  {"p.name COLLATE NOCASE"},
	SortCreated:  {"i.created_at"},
	SortUpdated:  {"i.updated_at"},
	SortDue:      {"!i.due = ''", "i.due"},
	SortPoints:   {"!i.points IS NULL", "i.points"},
	SortResolved: {"!i.resolved_at IS NULL", "i.resolved_at"},
	SortRank:     {"i.rank"},
}

// Valid reports whether s sorts by a column there is.
func (s Sort) Valid() bool { _, ok := sortColumns[s.By]; return ok }

// orderBy is the ORDER BY clause of a sort, the exact key first.
func (s Sort) orderBy() string {
	if !s.Valid() {
		s = DefaultSort
	}
	parts := []string{"(i.id IN (SELECT issue_id FROM issue_keys WHERE key = ?17)) DESC"}
	dir := " ASC"
	if s.Desc {
		dir = " DESC"
	}
	for _, c := range sortColumns[s.By] {
		if e, ok := strings.CutPrefix(c, "!"); ok {
			parts = append(parts, "("+e+") ASC") // empty values last
			continue
		}
		parts = append(parts, c+dir)
	}
	parts = append(parts, "p.key ASC", "i.number ASC")
	return " ORDER BY " + strings.Join(parts, ", ")
}

// Values is the sort as URL parameters, none for the default.
func (s Sort) Values() url.Values {
	v := url.Values{}
	if s == DefaultSort || !s.Valid() {
		return v
	}
	v.Set("sort", s.By)
	if s.Desc {
		v.Set("dir", "desc")
	} else {
		v.Set("dir", "asc")
	}
	return v
}

// ParseSort reads a sort from URL parameters: sort and dir.
func ParseSort(v url.Values) Sort {
	s := Sort{By: v.Get("sort"), Desc: v.Get("dir") == "desc"}
	if !s.Valid() {
		return DefaultSort
	}
	return s
}

// ParsePage reads the page number from URL parameters: page, from 1.
func ParsePage(v url.Values) int {
	n, err := strconv.Atoi(v.Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// Query is a list as a person sees it: the projects they may read
// (Readable), who they are (for Me), the filter, the sort and the page.
type Query struct {
	Readable []string
	Me       string
	Filter   Filter
	Sort     Sort
	Page     int
}

// Key is a stable hash of everything the list's render depends on: the
// view key of a list (MP-11.1). Tabs of people who read the same projects
// and see the same list share it.
func (q Query) Key() string {
	r := slices.Sorted(slices.Values(q.Readable))
	s := q.Sort
	if !s.Valid() {
		s = DefaultSort
	}
	return hash(fmt.Sprintf("%s\n%s\n%s:%t\n%d", strings.Join(r, ","), q.Filter.Normalize(q.Me).Values().Encode(), s.By, s.Desc, max(q.Page, 1)))
}

// Values is the URL of the list: its filter, sort and page (FR-55).
func (q Query) Values() url.Values {
	v := q.Filter.Values()
	for k, x := range q.Sort.Values() {
		v[k] = x
	}
	if q.Page > 1 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	return v
}

// Results is a page of a list (FR-54): its issues, how many match, the
// page it is (a page past the end is the last one) and how many pages
// there are.
type Results struct {
	Issues []Issue
	Total  int
	Page   int
	Pages  int
}

// filterWhere selects the issues of a filter, with the parameters of
// filterArgs: one statement for every filter.
const filterWhere = ` WHERE i.deleted_at IS NULL
	AND i.project_id IN (SELECT value FROM json_each(?1))
	AND (?2 = '[]' OR p.key IN (SELECT value FROM json_each(?2)))
	AND (?2 != '[]' OR p.archived = 0)
	AND (?3 = '[]' OR i.type IN (SELECT value FROM json_each(?3)))
	AND (?4 = '[]' OR i.status IN (SELECT value FROM json_each(?4)))
	AND (?5 = '[]' OR i.status_category IN (SELECT value FROM json_each(?5)))
	AND (?6 = '[]' OR i.priority IN (SELECT value FROM json_each(?6)))
	AND (?7 = '[]' OR i.assignee IN (SELECT value FROM json_each(?7)))
	AND (?8 = '[]' OR i.reporter IN (SELECT value FROM json_each(?8)))
	AND (?9 = '[]' OR EXISTS (SELECT 1 FROM issue_labels l WHERE l.issue_id = i.id AND l.label IN (SELECT value FROM json_each(?9))))
	AND (?10 = '[]' OR EXISTS (SELECT 1 FROM json_each(i.components) c WHERE c.value IN (SELECT value FROM json_each(?10))))
	AND (?11 = '[]' OR i.sprint IN (SELECT value FROM json_each(?11)))
	AND (?12 = '' OR i.due != '' AND i.due >= ?12)
	AND (?13 = '' OR i.due != '' AND i.due <= ?13)
	AND (?14 = 0 OR i.created_at >= ?14)
	AND (?15 = 0 OR i.created_at < ?15)
	AND (?16 = '' OR instr(i.search, ?16) > 0 OR EXISTS (SELECT 1 FROM comments c
		WHERE c.subject_id = i.id AND c.deleted_at IS NULL AND c.hidden_at IS NULL AND instr(c.search, ?16) > 0))`

// byAssignee repeats filterWhere's assignee term on its own, and
// issueFromByAssignee reads issues by issues_assignee: "assigned to me",
// which the home page renders for every viewer on every change, then reads
// a person's issues, not every issue of the readable projects, which the
// planner prefers by its statistics.
const (
	byAssignee          = ` AND i.assignee IN (SELECT value FROM json_each(?7))`
	issueFromByAssignee = ` FROM issues i INDEXED BY issues_assignee JOIN projects p ON p.id = i.project_id
	LEFT JOIN statuses s ON s.project_id = i.project_id AND s.id = i.status`
)

func jsonList(v []string) string {
	if len(v) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// filterArgs are the parameters of filterWhere and orderBy (?17, the
// exact key) for a normalized filter.
func filterArgs(readable []string, f Filter) []any {
	day := func(d string, next bool) int64 {
		if d == "" {
			return 0
		}
		t, _ := time.Parse(time.DateOnly, d)
		if next {
			t = t.AddDate(0, 0, 1)
		}
		return t.UnixMilli()
	}
	assignees := slices.Clone(f.Assignees)
	for i, a := range assignees {
		if a == Unassigned {
			assignees[i] = ""
		}
	}
	sprints := slices.Clone(f.Sprints)
	for i, s := range sprints {
		if s == NoSprint {
			sprints[i] = ""
		}
	}
	exact := ""
	if k, n, ok := ParseIssueKey(f.Text); ok {
		exact = IssueKey(k, n)
	}
	return []any{jsonList(readable), jsonList(f.Projects), jsonList(strs(f.Types)), jsonList(f.Statuses),
		jsonList(strs(f.Categories)), jsonList(strs(f.Priorities)), jsonList(assignees), jsonList(f.Reporters),
		jsonList(f.Labels), jsonList(f.Components), jsonList(sprints), f.DueFrom, f.DueTo,
		day(f.CreatedFrom, false), day(f.CreatedTo, true), strings.ToLower(f.Text), exact}
}

// issueFrom are the tables of an issue's row.
const issueFrom = ` FROM issues i JOIN projects p ON p.id = i.project_id
	LEFT JOIN statuses s ON s.project_id = i.project_id AND s.id = i.status`

// Recent are the n issues of a project changed last, as Search shows them
// sorted by update, without counting the others: the issue page's issues
// to link by key or summary (issues_project_updated).
func Recent(ctx context.Context, q sqlite.Querier, project string, n int) ([]Issue, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+issueColumns+issueFrom+` WHERE i.project_id = ? AND i.deleted_at IS NULL
		ORDER BY i.updated_at DESC, i.number ASC LIMIT ?`, project, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Issue
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Search returns a page of the issues that match a query (FR-50 to
// FR-54): of the projects the person may read, archived ones only when
// the filter names them; an exact key first, then in the query's order,
// ties by key; 25 a page.
func Search(ctx context.Context, q sqlite.Querier, query Query) (Results, error) {
	f := query.Filter.Normalize(query.Me)
	args := filterArgs(query.Readable, f)
	var res Results
	from, where := issueFrom, filterWhere
	if len(f.Assignees) > 0 {
		from, where = issueFromByAssignee, filterWhere+byAssignee
	}
	if err := q.QueryRowContext(ctx, `SELECT count(*)`+from+where+` AND ?17 = ?17`, args...).Scan(&res.Total); err != nil {
		return res, err
	}
	res.Pages = max(1, (res.Total+PageSize-1)/PageSize)
	res.Page = min(max(query.Page, 1), res.Pages)
	joins := ``
	if query.Sort.By == SortAssignee || query.Sort.By == SortReporter {
		joins = ` LEFT JOIN people pa ON pa.id = i.assignee LEFT JOIN people pr ON pr.id = i.reporter`
	}
	rows, err := q.QueryContext(ctx, `SELECT `+issueColumns+from+joins+where+query.Sort.orderBy()+` LIMIT 25 OFFSET ?18`,
		append(args, (res.Page-1)*PageSize)...)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			return res, err
		}
		res.Issues = append(res.Issues, i)
	}
	return res, rows.Err()
}
