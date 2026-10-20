package doc

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Chip is what an issue chip shows (FR-100): the issue as it is now.
type Chip struct {
	Summary  string
	Status   string
	Category string // the status category: todo, progress or done
	Type     string // the issue type: bug, task, story, epic
	Deleted  bool
}

// Resolver looks up what atoms show at render time; the document holds
// only ids.
type Resolver interface {
	// Issue returns the issue with the key; false when no issue has it.
	Issue(key string) (Chip, bool)
	// Person returns a person's display name; false when unknown.
	Person(id string) (string, bool)
	// Page returns a page's title; false when the page is gone.
	Page(id string) (string, bool)
}

// StaticResolver is a Resolver over maps, for tests and previews.
type StaticResolver struct {
	Issues map[string]Chip
	People map[string]string
	Pages  map[string]string
}

// Issue looks up a chip.
func (s StaticResolver) Issue(key string) (Chip, bool) { c, ok := s.Issues[key]; return c, ok }

// Person looks up a name.
func (s StaticResolver) Person(id string) (string, bool) { n, ok := s.People[id]; return n, ok }

// Page looks up a title.
func (s StaticResolver) Page(id string) (string, bool) { t, ok := s.Pages[id]; return t, ok }

type htmlWriter struct {
	b *strings.Builder
	r Resolver
}

func (w htmlWriter) s(s string) { w.b.WriteString(s) }

// esc writes s with &, <, >, " and ' escaped, for text and attributes.
func (w htmlWriter) esc(s string) { escapeHTML(w.b, s) }

func escapeHTML(b *strings.Builder, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		var rep string
		switch s[i] {
		case '&':
			rep = "&amp;"
		case '<':
			rep = "&lt;"
		case '>':
			rep = "&gt;"
		case '"':
			rep = "&#34;"
		case '\'':
			rep = "&#39;"
		default:
			continue
		}
		b.WriteString(s[start:i])
		b.WriteString(rep)
		start = i + 1
	}
	b.WriteString(s[start:])
}

// RenderHTML renders the document's blocks as the HTML of docs/editor.md,
// which the browser's toDOM produces too; the caller wraps it in
// <div class="prose">. Every text and attribute is escaped, links pass
// SafeHref, images come from the library by imageId alone (DM-06). A nil
// Resolver knows nothing.
func RenderHTML(d Doc, r Resolver) string {
	var b strings.Builder
	b.Grow(d.content.size * 2)
	renderBlocks(&b, d.content.nodes, r)
	return b.String()
}

// RenderBlock renders one block.
func RenderBlock(n *Node, r Resolver) string {
	var b strings.Builder
	htmlWriter{&b, resolverOrNone(r)}.block(n)
	return b.String()
}

func resolverOrNone(r Resolver) Resolver {
	if r == nil {
		return StaticResolver{}
	}
	return r
}

func renderBlocks(b *strings.Builder, nodes []*Node, r Resolver) {
	w := htmlWriter{b, resolverOrNone(r)}
	for _, n := range nodes {
		w.block(n)
	}
}

func (w htmlWriter) blocks(n *Node) {
	for _, c := range n.content.nodes {
		w.block(c)
	}
}

func (w htmlWriter) block(n *Node) { w.wrap(n, "", nil) }

// body renders a node's content the default way.
func (w htmlWriter) body(n *Node) {
	switch {
	case n.typ.code:
		for _, t := range n.content.nodes {
			w.esc(t.text)
		}
	case n.typ.inlineContent:
		w.inline(n.content.nodes)
	default:
		w.blocks(n)
	}
}

// wrap writes a block's element with extra attributes (written verbatim)
// around its content; body nil renders the content the default way.
func (w htmlWriter) wrap(n *Node, extra string, body func()) {
	if body == nil {
		body = func() { w.body(n) }
	}
	tag := func(name string) {
		w.s("<" + name + extra + ">")
		body()
		w.s("</" + name + ">")
	}
	switch n.typ.name {
	case "paragraph":
		tag("p")
	case "heading":
		l, _ := n.AttrInt("level")
		tag("h" + strconv.Itoa(min(max(l, 1), 3)+1))
	case "blockquote":
		tag("blockquote")
	case "code_block":
		w.s("<pre")
		if l := n.AttrString("language"); l != "" {
			w.s(` data-language="`)
			w.esc(l)
			w.s(`"`)
		}
		w.s(extra + "><code>")
		body()
		w.s("</code></pre>")
	case "horizontal_rule":
		w.s("<hr" + extra + ">")
	case "bullet_list":
		tag("ul")
	case "ordered_list":
		if o, ok := n.AttrInt("order"); ok && o != 1 {
			w.s(`<ol start="` + strconv.Itoa(o) + `"` + extra + `>`)
		} else {
			w.s("<ol" + extra + ">")
		}
		body()
		w.s("</ol>")
	case "list_item":
		tag("li")
	case "task_list":
		w.s(`<ul class="task-list"` + extra + `>`)
		body()
		w.s("</ul>")
	case "task_item":
		if n.AttrBool("checked") {
			w.s(`<li class="task-item" data-checked="true"` + extra + `><input type="checkbox" aria-label="Done" disabled checked><div>`)
		} else {
			w.s(`<li class="task-item" data-checked="false"` + extra + `><input type="checkbox" aria-label="Done" disabled><div>`)
		}
		body()
		w.s("</div></li>")
	case "table":
		w.s("<table" + extra + "><tbody>")
		body()
		w.s("</tbody></table>")
	case "table_row":
		tag("tr")
	case "table_cell", "table_header":
		name := "td"
		if n.typ.name == "table_header" {
			name = "th"
		}
		w.s("<" + name)
		if c, ok := n.AttrInt("colspan"); ok && c != 1 {
			w.s(` colspan="` + strconv.Itoa(c) + `"`)
		}
		if r, ok := n.AttrInt("rowspan"); ok && r != 1 {
			w.s(` rowspan="` + strconv.Itoa(r) + `"`)
		}
		w.s(extra + ">")
		body()
		w.s("</" + name + ">")
	case "image":
		w.s(`<figure class="doc-image"` + extra + `><img src="/img/lib/`)
		w.esc(url.PathEscape(n.AttrString("imageId")))
		w.s(`" alt="`)
		w.esc(n.AttrString("alt"))
		w.s(`" loading="lazy"></figure>`)
	default:
		// A node type the renderer does not know renders its content.
		body()
	}
}

type openMark struct {
	m     *Mark
	close string
}

// inline renders inline content with DOMSerializer's mark nesting: marks
// shared with the previous node stay open. An atom never sits inside a
// link's <a> (it is a link of its own, and anchors do not nest).
func (w htmlWriter) inline(nodes []*Node) {
	var active []openMark
	for _, n := range nodes {
		marks := n.marks
		if !n.typ.isText {
			marks = slices.DeleteFunc(slices.Clone(marks), func(m *Mark) bool { return m.typ.name == "link" })
		}
		keep := 0
		for keep < len(active) && keep < len(marks) && marks[keep].Eq(active[keep].m) {
			keep++
		}
		for len(active) > keep {
			w.s(active[len(active)-1].close)
			active = active[:len(active)-1]
		}
		for _, m := range marks[keep:] {
			active = append(active, openMark{m, w.openMark(m)})
		}
		w.leaf(n)
	}
	for i := len(active) - 1; i >= 0; i-- {
		w.s(active[i].close)
	}
}

func (w htmlWriter) openMark(m *Mark) string {
	switch m.typ.name {
	case "strong":
		w.s("<strong>")
		return "</strong>"
	case "em":
		w.s("<em>")
		return "</em>"
	case "strike":
		w.s("<s>")
		return "</s>"
	case "code":
		w.s("<code>")
		return "</code>"
	case "link":
		href := m.AttrString("href")
		if title, ok := SafeHref(href); ok {
			w.s(`<a href="`)
			w.esc(href)
			w.s(`" rel="nofollow ugc noopener" title="`)
			w.esc(title)
			w.s(`">`)
			return "</a>"
		}
		w.s(`<span class="link-unsafe">`)
		return "</span>"
	}
	w.s("<span>")
	return "</span>"
}

func (w htmlWriter) leaf(n *Node) {
	switch n.typ.name {
	case "text":
		w.esc(n.text)
	case "hard_break":
		w.s("<br>")
	case "issue_chip":
		w.chip(n.AttrString("key"))
	case "mention":
		id := n.AttrString("personId")
		name, ok := w.r.Person(id)
		if !ok || name == "" {
			name = n.AttrString("label")
		}
		w.s(`<a class="mention" data-person="`)
		w.esc(id)
		w.s(`" href="/people/`)
		w.esc(url.PathEscape(id))
		w.s(`">@`)
		w.esc(name)
		w.s(`</a>`)
	case "page_link":
		id := n.AttrString("pageId")
		title, ok := w.r.Page(id)
		if !ok {
			w.s(`<span class="page-link" data-state="gone">a removed page</span>`)
			return
		}
		w.s(`<a class="page-link" data-page="`)
		w.esc(id)
		w.s(`" href="/pages/`)
		w.esc(url.PathEscape(id))
		w.s(`">`)
		w.esc(title)
		w.s(`</a>`)
	}
}

// The issue type icons of the design system, as the editor's node view
// draws them (16 grid).
const typeTile = "M1 0h14v1h1v14h-1v1H1v-1H0V1h1z"

var typeGlyphs = map[string]string{
	"epic":    "M8 2h4v1h-4zM7 3h4v1h-4zM6 4h4v1h-4zM5 5h4v1h-4zM4 6h8v1h-8zM7 7h4v1h-4zM6 8h4v1h-4zM5 9h4v1h-4zM4 10h3v1h-3zM3 11h2v1h-2z",
	"story":   "M4 2h8v9h-8zM4 11h3v1h-3zM9 11h3v1h-3zM4 12h2v1h-2zM10 12h2v1h-2zM4 13h1v1h-1zM11 13h1v1h-1z",
	"task":    "M11 4h2v1h-2zM10 5h3v1h-3zM9 6h3v1h-3zM2 7h2v1h-2zM8 7h3v1h-3zM2 8h3v1h-3zM7 8h3v1h-3zM3 9h6v1h-6zM4 10h4v1h-4zM5 11h2v1h-2z",
	"bug":     "M5 2h1v1h-1zM10 2h1v1h-1zM6 3h4v1h-4zM4 4h3v1h-3zM9 4h3v1h-3zM1 5h1v1h-1zM3 5h4v1h-4zM9 5h4v1h-4zM14 5h1v1h-1zM2 6h5v1h-5zM9 6h5v1h-5zM3 7h4v1h-4zM9 7h4v1h-4zM1 8h1v1h-1zM3 8h4v1h-4zM9 8h4v1h-4zM14 8h1v1h-1zM2 9h5v1h-5zM9 9h5v1h-5zM3 10h4v1h-4zM9 10h4v1h-4zM4 11h3v1h-3zM9 11h3v1h-3zM6 12h1v1h-1zM9 12h1v1h-1z",
	"subtask": "M2 2h6v1h-6zM2 3h1v3h-1zM7 3h1v3h-1zM2 6h6v1h-6zM4 7h1v2h-1zM8 8h6v1h-6zM4 9h5v1h-5zM13 9h1v3h-1zM8 10h1v2h-1zM8 12h6v1h-6z",
}

// chip renders an issue chip as the editor's node view does: type icon,
// key, summary and status, or the key and the state of a deleted or
// unknown issue (FR-100).
func (w htmlWriter) chip(key string) {
	c, ok := w.r.Issue(key)
	w.s(`<a class="issue-chip" data-key="`)
	w.esc(key)
	w.s(`" href="/`)
	w.esc(url.PathEscape(key))
	w.s(`"`)
	switch {
	case !ok:
		w.s(` data-state="unknown"><span class="issue-chip__key">`)
		w.esc(key)
		w.s(`</span><span class="issue-chip__summary">unknown issue</span></a>`)
		return
	case c.Deleted:
		w.s(` data-state="deleted"><span class="issue-chip__key">`)
		w.esc(key)
		w.s(`</span><span class="issue-chip__summary">deleted issue</span></a>`)
		return
	}
	w.s(`>`)
	if g, ok := typeGlyphs[c.Type]; ok {
		w.s(`<svg class="tk-type" data-type="` + c.Type + `" viewBox="0 0 16 16" aria-hidden="true"><path class="tk-type__tile" d="` + typeTile + `"></path><path class="tk-type__glyph" d="` + g + `"></path></svg>`)
	}
	w.s(`<span class="issue-chip__key">`)
	w.esc(key)
	w.s(`</span>`)
	if c.Summary != "" {
		w.s(`<span class="issue-chip__summary">`)
		w.esc(c.Summary)
		w.s(`</span>`)
	}
	if c.Status != "" {
		cat := c.Category
		if cat == "" {
			cat = "todo"
		}
		w.s(`<span class="tk-lozenge issue-chip__status" data-cat="`)
		w.esc(cat)
		w.s(`">`)
		w.esc(c.Status)
		w.s(`</span>`)
	}
	w.s(`</a>`)
}

// BlockCache renders documents block by block and keeps the HTML of each
// top-level block by node, so a commit re-renders only the blocks it
// changed. Reset it when what the Resolver returns changes (a chip's
// status, a page's title). It is safe for concurrent use.
type BlockCache struct {
	mu sync.Mutex
	m  map[*Node]string
}

// Render renders the document, reusing the HTML of unchanged blocks, and
// forgets blocks the document no longer holds.
func (c *BlockCache) Render(d Doc, r Resolver) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := make(map[*Node]string, len(d.content.nodes))
	var b strings.Builder
	for _, n := range d.content.nodes {
		h, ok := c.m[n]
		if !ok {
			h = RenderBlock(n, r)
		}
		next[n] = h
		b.WriteString(h)
	}
	c.m = next
	return b.String()
}

// Reset forgets every block.
func (c *BlockCache) Reset() {
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}

// Text returns the document's text for search: blocks separated by
// newlines, chips by key, mentions by their name in the document, hard
// breaks as newlines and images by their alt text.
func Text(d Doc) string {
	return d.TextBetween(0, d.content.size, "\n", func(n *Node) string {
		switch n.typ.name {
		case "issue_chip":
			return n.AttrString("key")
		case "mention":
			if l := n.AttrString("label"); l != "" {
				return "@" + l
			}
		case "hard_break":
			return "\n"
		case "image":
			return n.AttrString("alt")
		}
		return ""
	})
}

// Refs are the ids a document refers to, each once and sorted.
type Refs struct {
	Issues []string // issue keys of chips (FR-101)
	People []string // person ids of mentions
	Pages  []string // page ids of page links (FR-103)
}

// References returns the issue keys, person ids and page ids of the
// document's atoms.
func References(d Doc) Refs {
	var r Refs
	d.Descendants(func(n *Node, _ int, _ *Node, _ int) bool {
		switch n.typ.name {
		case "issue_chip":
			r.Issues = append(r.Issues, n.AttrString("key"))
		case "mention":
			r.People = append(r.People, n.AttrString("personId"))
		case "page_link":
			r.Pages = append(r.Pages, n.AttrString("pageId"))
		}
		return true
	})
	for _, s := range []*[]string{&r.Issues, &r.People, &r.Pages} {
		slices.Sort(*s)
		*s = slices.Compact(*s)
	}
	return r
}
