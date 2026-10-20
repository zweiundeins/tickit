package ui

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// glyph is an icon as SVG path data: its main tone and its second.
type glyph struct {
	main, second string
	smooth       bool // drawn with curves: geometricPrecision, not crispEdges
}

var (
	icons      = map[string]glyph{}
	typePaths  = map[string]string{}
	prioPaths  = map[string]string{}
	catPaths   = map[string]string{}
	brandGlyph glyph
)

func init() {
	for name, s := range sprites {
		icons[name] = glyph{main: runs(name, s, 16, '#'), second: runs(name, s, 16, '+')}
	}
	for name, p := range lithIcons {
		icons[name] = glyph{main: p, smooth: name == "smooth"}
	}
	for alias, name := range aliases {
		g, ok := icons[name]
		if !ok {
			panic("ui: alias " + alias + " names no icon " + name)
		}
		icons[alias] = g
	}
	for name, s := range typeGlyphs {
		typePaths[name] = runs(name, s, 16, '#')
	}
	for name, s := range prioGlyphs {
		prioPaths[name] = runs(name, s, 16, '#')
	}
	for name, s := range catMarkers {
		catPaths[name] = runs(name, s, 10, '#')
	}
	brandGlyph = glyph{main: runs("brand", brand, 16, '#'), second: runs("brand", brand, 16, '+')}
}

// runs turns a sprite into path data: a rectangle per run of ch in a row.
// It panics on a sprite that is not size rows of size characters.
func runs(name, sprite string, size int, ch byte) string {
	rows := strings.Split(strings.TrimSpace(sprite), "\n")
	if len(rows) != size {
		panic(fmt.Sprintf("ui: sprite %s has %d rows, want %d", name, len(rows), size))
	}
	var b strings.Builder
	for y, row := range rows {
		row = strings.TrimSpace(row)
		if len(row) != size || strings.Trim(row, ".#+") != "" {
			panic(fmt.Sprintf("ui: sprite %s row %d is %q: want %d of . # +", name, y, row, size))
		}
		for x := 0; x < size; {
			if row[x] != ch {
				x++
				continue
			}
			w := 1
			for x+w < size && row[x+w] == ch {
				w++
			}
			n := strconv.Itoa(w)
			b.WriteString("M" + strconv.Itoa(x) + " " + strconv.Itoa(y) + "h" + n + "v1h-" + n + "z")
			x += w
		}
	}
	return b.String()
}

// IconNames lists every icon Icon draws, aliases included, sorted.
func IconNames() []string {
	names := make([]string, 0, len(icons))
	for name := range icons {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// HasIcon reports whether Icon draws name.
func HasIcon(name string) bool {
	_, ok := icons[name]
	return ok
}

// Icon is a decorative pixel icon (aria-hidden): the text beside it, or the
// control around it, names it. An unknown name draws nothing.
func Icon(name string) templ.Component {
	return raw(iconSVG(name, "tk-icon", ""))
}

// IconSized is Icon at a size class: "lg" (2rem) or "xl" (4rem), the
// integer multiples that keep pixels crisp.
func IconSized(name, size string) templ.Component {
	return raw(iconSVG(name, "tk-icon tk-icon--"+size, ""))
}

// LabeledIcon is an icon that names itself to assistive technology, for an
// icon with no text beside it. Give it a tooltip too (Tip) so sighted
// people read the name.
func LabeledIcon(name, label string) templ.Component {
	return raw(iconSVG(name, "tk-icon", label))
}

func iconSVG(name, class, label string) string {
	g, ok := icons[name]
	if !ok {
		return ""
	}
	shape := "crispEdges"
	if g.smooth {
		shape = "geometricPrecision"
	}
	var b strings.Builder
	b.WriteString(`<svg class="` + class + `" viewBox="0 0 16 16" fill="currentColor" shape-rendering="` + shape + `"`)
	if label != "" {
		b.WriteString(` role="img" aria-label="` + templ.EscapeString(label) + `"`)
	} else {
		b.WriteString(` aria-hidden="true"`)
	}
	b.WriteString(` focusable="false"><path d="` + g.main + `"/>`)
	if g.second != "" {
		b.WriteString(`<path class="tk-icon__2" d="` + g.second + `"/>`)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// brandSVG is the mark, decorative: the word beside it names the app.
func brandSVG(class string) string {
	return `<svg class="` + class + `" viewBox="0 0 16 16" shape-rendering="crispEdges" aria-hidden="true" focusable="false"><path class="tk-mark__body" d="` + brandGlyph.main + `"/><path class="tk-mark__stub" d="` + brandGlyph.second + `"/></svg>`
}

// MarkSVG is the mark as a standalone SVG file, for the favicon: its
// colours are fixed, since a file does not see the page's tokens.
func MarkSVG() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" shape-rendering="crispEdges"><path fill="#3fb7c9" d="` + brandGlyph.main + `"/><path fill="#1f7f92" d="` + brandGlyph.second + `"/></svg>`
}

// IssueType is one of the five types of FR-19.
type IssueType string

const (
	Epic    IssueType = "epic"
	Story   IssueType = "story"
	Task    IssueType = "task"
	Bug     IssueType = "bug"
	Subtask IssueType = "subtask"
)

// IssueTypes are the types in the order pickers list them.
var IssueTypes = []IssueType{Epic, Story, Task, Bug, Subtask}

// Name is the type's name as people read it.
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
	return "Unknown type"
}

func (t IssueType) known() bool { return typePaths[string(t)] != "" }

// Priority is one of the four priorities of FR-18.
type Priority string

const (
	Low      Priority = "low"
	Medium   Priority = "medium"
	High     Priority = "high"
	Critical Priority = "critical"
)

// Priorities are the priorities from lowest to highest.
var Priorities = []Priority{Low, Medium, High, Critical}

// Name is the priority's name as people read it.
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
	return "No priority"
}

// StatusCategory is the category a workflow status belongs to.
type StatusCategory string

const (
	ToDo       StatusCategory = "todo"
	InProgress StatusCategory = "progress"
	Done       StatusCategory = "done"
)

// Name is the category's name.
func (c StatusCategory) Name() string {
	switch c {
	case InProgress:
		return "In progress"
	case Done:
		return "Done"
	}
	return "To do"
}

func (c StatusCategory) attr() string {
	if catPaths[string(c)] == "" {
		return string(ToDo)
	}
	return string(c)
}

// typeSVG draws an issue type's tile; label "" makes it decorative.
func typeSVG(t IssueType, class, label string) string {
	glyph := typePaths[string(t)]
	var b strings.Builder
	b.WriteString(`<svg class="` + class + `" data-type="` + templ.EscapeString(string(t)) + `" viewBox="0 0 16 16" shape-rendering="crispEdges"`)
	if label != "" {
		b.WriteString(` role="img" aria-label="` + templ.EscapeString(label) + `"`)
	} else {
		b.WriteString(` aria-hidden="true"`)
	}
	b.WriteString(` focusable="false"><path class="tk-type__tile" d="` + typeTile + `"/>`)
	if glyph != "" {
		b.WriteString(`<path class="tk-type__glyph" d="` + glyph + `"/>`)
	} else {
		b.WriteString(`<path class="tk-type__glyph" d="M7 4h2v5H7zM7 11h2v2H7z"/>`) // "!": a type tickit does not know
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func prioSVG(p Priority, label string) string {
	glyph := prioPaths[string(p)]
	if glyph == "" {
		glyph = "M4 7h8v2H4z" // no priority: a dash
	}
	var b strings.Builder
	b.WriteString(`<svg class="tk-prio" data-prio="` + templ.EscapeString(string(p)) + `" viewBox="0 0 16 16" fill="currentColor" shape-rendering="crispEdges"`)
	if label != "" {
		b.WriteString(` role="img" aria-label="` + templ.EscapeString(label) + `"`)
	} else {
		b.WriteString(` aria-hidden="true"`)
	}
	b.WriteString(` focusable="false"><path d="` + glyph + `"/></svg>`)
	return b.String()
}

func catSVG(c StatusCategory) string {
	return `<svg class="tk-cat" viewBox="0 0 10 10" fill="currentColor" shape-rendering="crispEdges" aria-hidden="true" focusable="false"><path d="` + catPaths[c.attr()] + `"/></svg>`
}

// raw renders trusted markup built in this package.
func raw(s string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}
