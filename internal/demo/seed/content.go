package seed

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed data
var files embed.FS

// The default workflow (section 4.2), in order, and the other fixed lists.
var (
	Types      = []string{"Epic", "Story", "Task", "Bug", "Subtask"}
	Statuses   = []string{"Backlog", "Selected for Development", "To Do", "In Progress", "In Review", "Blocked", "Done"}
	Priorities = []string{"Low", "Medium", "High", "Critical"}
	LinkTypes  = []string{"blocks", "relates to", "duplicates"}
	ImageKinds = []string{"diagram", "screenshot", "photo"}
)

// Category returns the workflow category of a default status: "To Do",
// "In Progress" or "Done".
func Category(status string) string {
	switch status {
	case "Backlog", "Selected for Development", "To Do":
		return "To Do"
	case "In Progress", "In Review", "Blocked":
		return "In Progress"
	case "Done":
		return "Done"
	}
	return ""
}

// Content is everything the seed holds, in a fixed order: people, projects
// and spaces as company.json lists them, issues by project and number, pages
// by space and file name.
type Content struct {
	People       []Person
	Projects     []Project
	Spaces       []Space
	Issues       []Issue
	Pages        []Page
	PageComments []PageComment
	Templates    []Template
	Images       []Image
}

// Person is one of the six seeded people (DM-02.3).
type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
	Hue  int    `json:"hue"`
}

type Project struct {
	Key         string      `json:"key"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Lead        string      `json:"lead"`
	Components  []Component `json:"components"`
	Labels      []string    `json:"labels"`
}

type Component struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Space struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Issue holds an issue's final state; History leads to it from the state
// the issue was created in (see Initial).
type Issue struct {
	Key         string    `json:"key"`
	Project     string    `json:"-"`
	Number      int       `json:"-"`
	Rank        int       `json:"-"` // 1 is the top of the project's board and backlog
	Type        string    `json:"type"`
	Summary     string    `json:"summary"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	Assignee    string    `json:"assignee"` // a person id, or "" for unassigned
	Reporter    string    `json:"reporter"`
	Parent      string    `json:"parent"` // an issue key, or ""
	Labels      []string  `json:"labels"`
	Components  []string  `json:"components"`
	Due         *int      `json:"due"` // days after the run's date; negative is past
	Points      *int      `json:"points"`
	Estimate    Estimate  `json:"estimate"`  // original estimate
	Remaining   Estimate  `json:"remaining"` // remaining estimate
	Watchers    []string  `json:"watchers"`  // besides reporter, assignees, commenters and mentioned people
	Created     When      `json:"created"`
	Description Text      `json:"description"`
	History     []Change  `json:"history"`
	Links       []Link    `json:"links"`
	Comments    []Comment `json:"comments"`
}

// Change is one entry of an issue's history. Values are strings: a person
// id or "" for assignee, a key or "" for parent, a number for due and
// points, an Estimate for estimate and remaining.
type Change struct {
	At    When   `json:"at"`
	By    string `json:"by"`
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// HistoryFields are the fields a Change may name.
var HistoryFields = []string{"status", "assignee", "priority", "summary", "type", "parent", "due", "points", "estimate", "remaining"}

// Link goes from the issue that holds it to Key: "WEB-3 blocks OPS-12".
type Link struct {
	Type string `json:"type"`
	Key  string `json:"key"`
	By   string `json:"by"`
	At   When   `json:"at"`
}

type Comment struct {
	By   string `json:"by"`
	At   When   `json:"at"`
	Body Text   `json:"body"`
}

// Page is a page of a space. Versions holds every version, oldest first; the
// last one is the current Body.
type Page struct {
	Space    string
	File     string // the file it came from, inside the data directory
	Title    string
	Parent   string // the parent page's title, or "" for the home page and top-level pages
	Home     bool
	Position int // among its siblings, from 1
	Author   string
	Created  When
	Labels   []string
	Body     string
	Versions []Version
}

type Version struct {
	Number  int
	At      When
	By      string
	Message string
	Body    string
}

// PageComment is a comment below a page, with one level of replies.
type PageComment struct {
	Space   string        `json:"-"`
	Page    string        `json:"page"` // the page's title
	By      string        `json:"by"`
	At      When          `json:"at"`
	Body    Text          `json:"body"`
	Replies []PageComment `json:"replies"`
}

// Template is a page template for every space (FR-92).
type Template struct {
	Name        string
	Description string
	Body        string
}

// Image is an image of the library (FR-84), served at /img/lib/<ID> from
// internal/web/static/img/lib/<ID>.svg.
type Image struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Alt    string `json:"alt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Kind   string `json:"kind"`
}

// When is a moment relative to the seed run, written "-12d 09:40": 12 days
// before the run's date, at 09:40 in the run's time zone.
type When struct {
	Days    int
	Minutes int
}

var whenRe = regexp.MustCompile(`^(-?[0-9]+)d ([0-2][0-9]):([0-5][0-9])$`)

func ParseWhen(s string) (When, error) {
	m := whenRe.FindStringSubmatch(s)
	if m == nil {
		return When{}, fmt.Errorf("%q is not a moment like \"-12d 09:40\"", s)
	}
	d, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	min, _ := strconv.Atoi(m[3])
	if h > 23 {
		return When{}, fmt.Errorf("%q has no such hour", s)
	}
	return When{Days: d, Minutes: h*60 + min}, nil
}

func (w When) String() string {
	return fmt.Sprintf("%dd %02d:%02d", w.Days, w.Minutes/60, w.Minutes%60)
}

// Time is the moment for a run at run, in run's location (the demo uses
// Europe/Zurich).
func (w When) Time(run time.Time) time.Time {
	y, m, d := run.Date()
	return time.Date(y, m, d+w.Days, w.Minutes/60, w.Minutes%60, 0, 0, run.Location())
}

// Before reports whether w comes before v.
func (w When) Before(v When) bool {
	return w.Days < v.Days || w.Days == v.Days && w.Minutes < v.Minutes
}

func (w When) MarshalJSON() ([]byte, error) { return json.Marshal(w.String()) }

func (w *When) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseWhen(s)
	*w = v
	return err
}

// DueDate is the date of a due-date offset for a run at run: midnight of the
// day, in run's location.
func DueDate(run time.Time, days int) time.Time {
	y, m, d := run.Date()
	return time.Date(y, m, d+days, 0, 0, 0, 0, run.Location())
}

// Text is markup. JSON holds it as one string or as an array of lines.
type Text string

func (t *Text) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		var lines []string
		if err := json.Unmarshal(b, &lines); err != nil {
			return err
		}
		*t = Text(strings.Join(lines, "\n"))
		return nil
	}
	var s string
	err := json.Unmarshal(b, &s)
	*t = Text(s)
	return err
}

// Estimate is a duration in hours and minutes, written "1h 30m", "6h" or
// "45m"; "" is no estimate.
type Estimate string

var estimateRe = regexp.MustCompile(`^(?:([1-9][0-9]*)h)?(?: ?([1-5]?[0-9])m)?$`)

// Minutes returns the estimate in minutes.
func (e Estimate) Minutes() (int, error) {
	if e == "" {
		return 0, nil
	}
	m := estimateRe.FindStringSubmatch(string(e))
	if m == nil || m[1] == "" && m[2] == "" {
		return 0, fmt.Errorf("%q is not an estimate like \"1h 30m\"", e)
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	if m[2] != "" && min == 0 {
		return 0, fmt.Errorf("estimate %q has 0m", e)
	}
	return h*60 + min, nil
}

type company struct {
	People   []Person  `json:"people"`
	Projects []Project `json:"projects"`
	Spaces   []Space   `json:"spaces"`
}

type issueFile struct {
	Project string   `json:"project"`
	Rank    []string `json:"rank"`
	Issues  []Issue  `json:"issues"`
}

func decode(name string, v any) error {
	b, err := files.ReadFile(path.Join("data", name))
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Load reads the seed from the files embedded in the binary. It checks the
// syntax of every file; Check checks what the content refers to.
func Load() (*Content, error) {
	c := &Content{}
	var co company
	if err := decode("company.json", &co); err != nil {
		return nil, err
	}
	c.People, c.Projects, c.Spaces = co.People, co.Projects, co.Spaces
	if err := decode("images.json", &c.Images); err != nil {
		return nil, err
	}
	for _, p := range c.Projects {
		var f issueFile
		name := "issues-" + p.Key + ".json"
		if err := decode(name, &f); err != nil {
			return nil, err
		}
		if f.Project != p.Key {
			return nil, fmt.Errorf("%s: project %q, want %q", name, f.Project, p.Key)
		}
		rank := map[string]int{}
		for i, k := range f.Rank {
			if _, dup := rank[k]; dup {
				return nil, fmt.Errorf("%s: %s ranked twice", name, k)
			}
			rank[k] = i + 1
		}
		for i, is := range f.Issues {
			is.Project = p.Key
			is.Number = i + 1
			if want := fmt.Sprintf("%s-%d", p.Key, is.Number); is.Key != want {
				return nil, fmt.Errorf("%s: issue %d has key %q, want %q", name, i+1, is.Key, want)
			}
			r, ok := rank[is.Key]
			if !ok {
				return nil, fmt.Errorf("%s: %s missing from rank", name, is.Key)
			}
			is.Rank = r
			c.Issues = append(c.Issues, is)
		}
		if len(f.Rank) != len(f.Issues) {
			return nil, fmt.Errorf("%s: rank lists %d issues, the file holds %d", name, len(f.Rank), len(f.Issues))
		}
	}
	for _, s := range c.Spaces {
		dir := "pages-" + s.Key
		entries, err := fs.ReadDir(files, path.Join("data", dir))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			p, err := readPage(s.Key, path.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			c.Pages = append(c.Pages, p)
		}
		var comments []PageComment
		if err := decode(path.Join(dir, "comments.json"), &comments); err != nil {
			return nil, err
		}
		for i := range comments {
			comments[i].Space = s.Key
			for j := range comments[i].Replies {
				comments[i].Replies[j].Space = s.Key
				comments[i].Replies[j].Page = comments[i].Page
			}
		}
		c.PageComments = append(c.PageComments, comments...)
	}
	entries, err := fs.ReadDir(files, "data/templates")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := path.Join("templates", e.Name())
		fm, body, err := readFront(name)
		if err != nil {
			return nil, err
		}
		if err := fm.only("name", "description"); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		c.Templates = append(c.Templates, Template{Name: fm.str("name"), Description: fm.str("description"), Body: body})
	}
	return c, nil
}

func readFront(name string) (frontMatter, string, error) {
	b, err := files.ReadFile(path.Join("data", name))
	if err != nil {
		return nil, "", err
	}
	fm, body, err := parseFront(string(b))
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", name, err)
	}
	return fm, body, nil
}

func readPage(space, name string) (Page, error) {
	fm, body, err := readFront(name)
	if err != nil {
		return Page{}, err
	}
	fail := func(err error) (Page, error) { return Page{}, fmt.Errorf("%s: %w", name, err) }
	if err := fm.only("title", "parent", "home", "position", "author", "created", "labels", "edited", "editor", "message", "versions"); err != nil {
		return fail(err)
	}
	p := Page{Space: space, File: name, Title: fm.str("title"), Parent: fm.str("parent"), Author: fm.str("author"), Labels: fm.list("labels"), Body: body}
	p.Home = fm.str("home") == "true"
	if p.Position, err = strconv.Atoi(fm.str("position")); err != nil {
		return fail(fmt.Errorf("position: %w", err))
	}
	if p.Created, err = ParseWhen(fm.str("created")); err != nil {
		return fail(err)
	}
	for _, v := range fm.items("versions") {
		if err := v.only("at", "by", "message", "body"); err != nil {
			return fail(err)
		}
		at, err := ParseWhen(v.str("at"))
		if err != nil {
			return fail(err)
		}
		p.Versions = append(p.Versions, Version{Number: len(p.Versions) + 1, At: at, By: v.str("by"), Message: v.str("message"), Body: v.str("body")})
	}
	cur := Version{Number: len(p.Versions) + 1, At: p.Created, By: p.Author, Message: fm.str("message"), Body: body}
	if e := fm.str("edited"); e != "" {
		if cur.At, err = ParseWhen(e); err != nil {
			return fail(err)
		}
		cur.By = fm.str("editor")
	} else if len(p.Versions) > 0 {
		return fail(fmt.Errorf("a page with versions needs edited and editor"))
	}
	p.Versions = append(p.Versions, cur)
	return p, nil
}

// Project returns the project with the key, or nil.
func (c *Content) Project(key string) *Project {
	i := slices.IndexFunc(c.Projects, func(p Project) bool { return p.Key == key })
	if i < 0 {
		return nil
	}
	return &c.Projects[i]
}

// Issue returns the issue with the key, or nil.
func (c *Content) Issue(key string) *Issue {
	i := slices.IndexFunc(c.Issues, func(is Issue) bool { return is.Key == key })
	if i < 0 {
		return nil
	}
	return &c.Issues[i]
}

// Person returns the person with the id, or nil.
func (c *Content) Person(id string) *Person {
	i := slices.IndexFunc(c.People, func(p Person) bool { return p.ID == id })
	if i < 0 {
		return nil
	}
	return &c.People[i]
}

// Page returns the page of the space with the title, or nil.
func (c *Content) Page(space, title string) *Page {
	i := slices.IndexFunc(c.Pages, func(p Page) bool { return p.Space == space && p.Title == title })
	if i < 0 {
		return nil
	}
	return &c.Pages[i]
}

// Resolved returns when the issue entered Done for the last time, if it is
// done.
func (is *Issue) Resolved() (When, bool) {
	if is.Status != "Done" {
		return When{}, false
	}
	for i := len(is.History) - 1; i >= 0; i-- {
		if h := is.History[i]; h.Field == "status" && h.To == "Done" {
			return h.At, true
		}
	}
	return When{}, false
}

// Value returns the issue's final value of a history field, as a Change
// writes it.
func (is *Issue) Value(field string) string {
	switch field {
	case "status":
		return is.Status
	case "assignee":
		return is.Assignee
	case "priority":
		return is.Priority
	case "summary":
		return is.Summary
	case "type":
		return is.Type
	case "parent":
		return is.Parent
	case "due":
		return intValue(is.Due)
	case "points":
		return intValue(is.Points)
	case "estimate":
		return string(is.Estimate)
	case "remaining":
		return string(is.Remaining)
	}
	return ""
}

func intValue(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// Initial returns the values the issue was created with: per field, the
// From of its first change, or its final value when it never changed.
func (is *Issue) Initial() map[string]string {
	v := map[string]string{}
	for _, f := range HistoryFields {
		v[f] = is.Value(f)
	}
	for i := len(is.History) - 1; i >= 0; i-- {
		v[is.History[i].Field] = is.History[i].From
	}
	return v
}
