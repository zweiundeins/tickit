package docnodes

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/zweiundeins/prosemirror-go"
)

// Heading is a heading of a document's outline.
type Heading struct {
	Level int
	Text  string
	// ID is the id of the heading's element: h-, the document's name when
	// it is rendered As one and a hyphen, and a slug of its text, with -2,
	// -3 for the second and third of the same text.
	ID string
}

// Outline returns the document's headings in document order, wherever
// they are, with the ids RenderHTML gives their elements; a toc lists
// them.
func Outline(d prosemirror.Doc) []Heading { return newOutline(d, "").list }

// OutlineAs is Outline with the ids of a document rendered As doc.
func OutlineAs(d prosemirror.Doc, doc string) []Heading { return newOutline(d, doc).list }

// outline is a document's headings for one render: their ids by node,
// and a key of the whole list for the BlockCache of a toc.
type outline struct {
	list []Heading
	ids  map[*prosemirror.Node]string
	key  string
}

func newOutline(d prosemirror.Doc, doc string) *outline {
	prefix := "h-"
	if doc != "" {
		prefix = "h-" + doc + "-"
	}
	o := &outline{ids: map[*prosemirror.Node]string{}}
	if d == nil {
		return o
	}
	used := map[string]bool{}
	var key strings.Builder
	d.Descendants(func(n *prosemirror.Node, _ int, _ *prosemirror.Node, _ int) bool {
		if n.Type().Name() != "heading" {
			return !n.Type().InlineContent()
		}
		if _, seen := o.ids[n]; seen {
			return false
		}
		text := headingText(n)
		id := prefix + slug(text)
		for i := 2; used[id]; i++ {
			id = prefix + slug(text) + "-" + strconv.Itoa(i)
		}
		used[id] = true
		l := headingLevel(n)
		o.ids[n] = id
		o.list = append(o.list, Heading{Level: l, Text: text, ID: id})
		key.WriteString(strconv.Itoa(l) + id + "\x00" + text + "\x00")
		return false
	})
	o.key = key.String()
	return o
}

// id is the id of a heading's element: from the outline when the render
// has one, else from the heading's own text.
func (o *outline) id(n *prosemirror.Node) string {
	if o != nil {
		if id, ok := o.ids[n]; ok {
			return id
		}
	}
	return "h-" + slug(headingText(n))
}

func headingLevel(n *prosemirror.Node) int {
	l, _ := n.AttrInt("level")
	return min(max(l, 1), 6)
}

// headingText is a heading's text on one line, its atoms as Text writes
// them.
func headingText(n *prosemirror.Node) string {
	return strings.Join(strings.Fields(n.TextBetween(0, n.Content().Size(), " ", leaves.Leaf)), " ")
}

// slug is text in lower case with each run of characters other than
// letters and digits as one hyphen, at most 60 characters; "section"
// when nothing is left. The editor's toc writes the same (editor/src/schema.js).
func slug(text string) string {
	var b strings.Builder
	dash, n := false, 0
	for _, r := range strings.ToLower(text) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			dash = true
			continue
		}
		if dash && n > 0 {
			b.WriteByte('-')
			n++
		}
		dash = false
		b.WriteRune(r)
		if n++; n >= 60 {
			break
		}
	}
	if b.Len() == 0 {
		return "section"
	}
	return b.String()
}

// tocHTML writes a toc's list: a nested <ol> of links to the headings,
// a deeper level inside the item before it.
func tocHTML(w *writer, hs []Heading) {
	if len(hs) == 0 {
		w.Raw(`<p class="doc-toc__empty">No headings yet</p>`)
		return
	}
	var stack []int
	for _, h := range hs {
		switch {
		case len(stack) == 0:
			w.Raw("<ol>")
			stack = append(stack, h.Level)
		case h.Level > stack[len(stack)-1]:
			w.Raw("<ol>")
			stack = append(stack, h.Level)
		default:
			for len(stack) > 1 && h.Level < stack[len(stack)-1] {
				w.Raw("</li></ol>")
				stack = stack[:len(stack)-1]
			}
			if h.Level > stack[len(stack)-1] {
				w.Raw("<ol>")
				stack = append(stack, h.Level)
			} else {
				w.Raw("</li>")
			}
		}
		w.Raw(`<li><a href="#`)
		w.Text(h.ID)
		w.Raw(`">`)
		w.Text(h.Text)
		w.Raw(`</a>`)
	}
	for range stack {
		w.Raw("</li></ol>")
	}
}
