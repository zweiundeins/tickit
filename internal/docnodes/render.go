package docnodes

import (
	"net/url"
	"strconv"
	"sync"

	"github.com/zweiundeins/prosemirror-go"
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

// FaceResolver is a Resolver that also draws people's faces: a mention
// then shows the face before the name, without the "@" (FR-67.2). Face
// returns trusted HTML, "" for none.
type FaceResolver interface {
	Face(id string) string
}

// IssueList is an issue list block (FR-104): a saved filter's id or a
// filter in the issue list's query syntax, and how many rows it shows.
type IssueList struct {
	Filter      string
	SavedFilter string
	Limit       int
}

// IssueListOf reads an issue_list node's attributes.
func IssueListOf(n *prosemirror.Node) IssueList {
	l, _ := n.AttrInt("limit")
	return IssueList{Filter: n.AttrString("filter"), SavedFilter: n.AttrString("savedFilter"), Limit: l}
}

// Key names the list by its attributes, as the editor's node view does.
func (l IssueList) Key() string {
	return l.SavedFilter + "|" + l.Filter + "|" + strconv.Itoa(l.Limit)
}

// ListResolver is a Resolver that fills issue lists: List returns
// trusted HTML for the inside of the list's element, its rows as the
// reader may see them now or what its filter is; "" for the default.
type ListResolver interface {
	List(l IssueList) string
}

// StaticResolver is a Resolver over maps, for tests and previews. Faces
// are trusted HTML by person id (FaceResolver).
type StaticResolver struct {
	Issues map[string]Chip
	People map[string]string
	Pages  map[string]string
	Faces  map[string]string `json:",omitempty"`
	// Hidden are the images a moderator hid (DM-07.1).
	Hidden map[string]bool `json:",omitempty"`
}

// Face looks up a face.
func (s StaticResolver) Face(id string) string { return s.Faces[id] }

// ImageHidden reports whether a moderator hid an image.
func (s StaticResolver) ImageHidden(id string) bool { return s.Hidden[id] }

// imageHider is a Resolver that knows hidden images: the renderer shows
// their place without them.
type imageHider interface{ ImageHidden(id string) bool }

// Issue looks up a chip.
func (s StaticResolver) Issue(key string) (Chip, bool) { c, ok := s.Issues[key]; return c, ok }

// Person looks up a name.
func (s StaticResolver) Person(id string) (string, bool) { n, ok := s.People[id]; return n, ok }

// Page looks up a title.
func (s StaticResolver) Page(id string) (string, bool) { t, ok := s.Pages[id]; return t, ok }

func resolverOrNone(r Resolver) Resolver {
	if r == nil {
		return StaticResolver{}
	}
	return r
}

type writer = prosemirror.HTMLWriter[Resolver]

func tag(name string) func(w *writer, n *prosemirror.Node) {
	return func(w *writer, n *prosemirror.Node) {
		w.Open(name)
		w.Content(n)
		w.Raw("</" + name + ">")
	}
}

func markTag(name string) prosemirror.MarkHTML[Resolver] {
	return prosemirror.MarkHTML[Resolver]{Open: func(w *writer, m *prosemirror.Mark) string {
		w.Raw("<" + name + ">")
		return "</" + name + ">"
	}}
}

func cell(name string) func(w *writer, n *prosemirror.Node) {
	return func(w *writer, n *prosemirror.Node) {
		var attrs []string
		if c, ok := n.AttrInt("colspan"); ok && c != 1 {
			attrs = append(attrs, "colspan", strconv.Itoa(c))
		}
		if r, ok := n.AttrInt("rowspan"); ok && r != 1 {
			attrs = append(attrs, "rowspan", strconv.Itoa(r))
		}
		w.Open(name, attrs...)
		w.Content(n)
		w.Raw("</" + name + ">")
	}
}

// html renders the HTML table of docs/editor.md, which the browser's toDOM
// produces too.
var html = &prosemirror.HTML[Resolver]{
	HighlightClass: "tk-ic", HighlightAttr: "data-c",
	Dynamic: []string{"issue_list"},
	Nodes: map[string]func(w *writer, n *prosemirror.Node){
		"paragraph":  tag("p"),
		"blockquote": tag("blockquote"),
		"heading": func(w *writer, n *prosemirror.Node) {
			l, _ := n.AttrInt("level")
			tag("h"+strconv.Itoa(min(max(l, 1), 3)+1))(w, n)
		},
		"code_block": func(w *writer, n *prosemirror.Node) {
			if l := n.AttrString("language"); l != "" {
				w.Open("pre", "data-language", l)
			} else {
				w.Open("pre")
			}
			w.Raw("<code>")
			w.Content(n)
			w.Raw("</code></pre>")
		},
		"horizontal_rule": func(w *writer, n *prosemirror.Node) { w.Open("hr") },
		"bullet_list":     tag("ul"),
		"ordered_list": func(w *writer, n *prosemirror.Node) {
			if o, ok := n.AttrInt("order"); ok && o != 1 {
				w.Open("ol", "start", strconv.Itoa(o))
			} else {
				w.Open("ol")
			}
			w.Content(n)
			w.Raw("</ol>")
		},
		"list_item": tag("li"),
		"task_list": func(w *writer, n *prosemirror.Node) {
			w.Open("ul", "class", "task-list")
			w.Content(n)
			w.Raw("</ul>")
		},
		"task_item": func(w *writer, n *prosemirror.Node) {
			if n.AttrBool("checked") {
				w.Open("li", "class", "task-item", "data-checked", "true")
				w.Raw(`<input type="checkbox" aria-label="Done" disabled checked><div>`)
			} else {
				w.Open("li", "class", "task-item", "data-checked", "false")
				w.Raw(`<input type="checkbox" aria-label="Done" disabled><div>`)
			}
			w.Content(n)
			w.Raw("</div></li>")
		},
		"table": func(w *writer, n *prosemirror.Node) {
			w.Open("table")
			w.Raw("<tbody>")
			w.Content(n)
			w.Raw("</tbody></table>")
		},
		"table_row":    tag("tr"),
		"table_cell":   cell("td"),
		"table_header": cell("th"),
		"image": func(w *writer, n *prosemirror.Node) {
			if h, ok := w.R.(imageHider); ok && h.ImageHidden(n.AttrString("imageId")) {
				w.Open("figure", "class", "doc-image doc-image--hidden")
				w.Raw(`<p>Image hidden by a moderator</p></figure>`)
				return
			}
			w.Open("figure", "class", "doc-image")
			w.Raw(`<img src="/img/lib/`)
			w.Text(url.PathEscape(n.AttrString("imageId")))
			w.Raw(`" alt="`)
			w.Text(n.AttrString("alt"))
			w.Raw(`" loading="lazy"></figure>`)
		},
		"issue_list": func(w *writer, n *prosemirror.Node) {
			l := IssueListOf(n)
			w.Open("div", "class", "issue-list", "data-filter", l.Filter, "data-saved-filter", l.SavedFilter, "data-limit", strconv.Itoa(l.Limit))
			inner := ""
			if lr, ok := w.R.(ListResolver); ok {
				inner = lr.List(l)
			}
			if inner == "" {
				inner = ListDescription(l, "")
			}
			w.Raw(inner + `</div>`)
		},
		"hard_break": func(w *writer, n *prosemirror.Node) { w.Raw("<br>") },
		"issue_chip": func(w *writer, n *prosemirror.Node) { chip(w, n.AttrString("key")) },
		"mention": func(w *writer, n *prosemirror.Node) {
			id := n.AttrString("personId")
			name, known := w.R.Person(id)
			if !known || name == "" {
				known, name = false, n.AttrString("label")
			}
			w.Raw(`<a class="mention" data-person="`)
			w.Text(id)
			w.Raw(`" href="/people/`)
			w.Text(url.PathEscape(id))
			face := ""
			if f, ok := w.R.(FaceResolver); ok && known {
				face = f.Face(id)
			}
			if face != "" {
				w.Raw(`" data-face>`)
				w.Raw(face)
			} else {
				w.Raw(`">@`)
			}
			w.Text(name)
			w.Raw(`</a>`)
		},
		"page_link": func(w *writer, n *prosemirror.Node) {
			id := n.AttrString("pageId")
			title, ok := w.R.Page(id)
			if !ok {
				w.Raw(`<span class="page-link" data-state="gone">a removed page</span>`)
				return
			}
			w.Raw(`<a class="page-link" data-page="`)
			w.Text(id)
			w.Raw(`" href="/pages/`)
			w.Text(url.PathEscape(id))
			w.Raw(`">`)
			w.Text(title)
			w.Raw(`</a>`)
		},
	},
	Marks: map[string]prosemirror.MarkHTML[Resolver]{
		"strong": markTag("strong"),
		"em":     markTag("em"),
		"strike": markTag("s"),
		"code":   markTag("code"),
		"link": {TextOnly: true, Open: func(w *writer, m *prosemirror.Mark) string {
			href := m.AttrString("href")
			if title, ok := prosemirror.SafeHref(href); ok {
				w.Raw(`<a href="`)
				w.Text(href)
				w.Raw(`" rel="nofollow ugc noopener" title="`)
				w.Text(title)
				w.Raw(`">`)
				return "</a>"
			}
			w.Raw(`<span class="link-unsafe">`)
			return "</span>"
		}},
	},
}

// RenderHTML renders the document's blocks as the HTML of docs/editor.md,
// which the browser's toDOM produces too; the caller wraps it in
// <div class="prose">. Every text and attribute is escaped, links pass
// SafeHref, images come from the library by imageId alone (DM-06). A nil
// Resolver knows nothing.
func RenderHTML(d prosemirror.Doc, r Resolver) string { return html.Render(d, resolverOrNone(r)) }

// RenderBlock renders one block.
func RenderBlock(n *prosemirror.Node, r Resolver) string {
	return html.RenderBlock(n, resolverOrNone(r))
}

// RenderHighlighted renders the document as RenderHTML does, with the text
// inside each highlight in <mark class="tk-ic" data-c="<ids>">, the ids of
// every highlight over it separated by spaces (FR-91.1).
func RenderHighlighted(d prosemirror.Doc, r Resolver, hs []prosemirror.Highlight) string {
	return html.RenderHighlighted(d, resolverOrNone(r), hs)
}

// ListDescription is what an issue list shows in place of its rows: in
// versions and comparisons, and without a ListResolver. name is the saved
// filter's name, "" when unknown.
func ListDescription(l IssueList, name string) string {
	b := []byte(`<p class="issue-list__desc">`)
	switch {
	case l.SavedFilter != "" && name != "":
		b = append(b, `Issues of the saved filter <strong>`...)
		b = append(b, prosemirror.EscapeHTML(name)...)
		b = append(b, `</strong>`...)
	case l.SavedFilter != "":
		b = append(b, `Issues of a saved filter`...)
	case l.Filter != "":
		b = append(b, `Issues matching <code>`...)
		b = append(b, prosemirror.EscapeHTML(l.Filter)...)
		b = append(b, `</code>`...)
	default:
		b = append(b, `An issue list without a filter`...)
	}
	b = append(b, `, at most `+strconv.Itoa(l.Limit)+`</p>`...)
	return string(b)
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
func chip(w *writer, key string) {
	c, ok := w.R.Issue(key)
	w.Raw(`<a class="issue-chip" data-key="`)
	w.Text(key)
	w.Raw(`" href="/`)
	w.Text(url.PathEscape(key))
	w.Raw(`"`)
	switch {
	case !ok:
		w.Raw(` data-state="unknown"><span class="issue-chip__key">`)
		w.Text(key)
		w.Raw(`</span><span class="issue-chip__summary">unknown issue</span></a>`)
		return
	case c.Deleted:
		w.Raw(` data-state="deleted"><span class="issue-chip__key">`)
		w.Text(key)
		w.Raw(`</span><span class="issue-chip__summary">deleted issue</span></a>`)
		return
	}
	w.Raw(`>`)
	if g, ok := typeGlyphs[c.Type]; ok {
		w.Raw(`<svg class="tk-type" data-type="` + c.Type + `" viewBox="0 0 16 16" aria-hidden="true"><path class="tk-type__tile" d="` + typeTile + `"></path><path class="tk-type__glyph" d="` + g + `"></path></svg>`)
	}
	w.Raw(`<span class="issue-chip__key">`)
	w.Text(key)
	w.Raw(`</span>`)
	if c.Summary != "" {
		w.Raw(`<span class="issue-chip__summary">`)
		w.Text(c.Summary)
		w.Raw(`</span>`)
	}
	if c.Status != "" {
		cat := c.Category
		if cat == "" {
			cat = "todo"
		}
		w.Raw(`<span class="tk-lozenge issue-chip__status" data-cat="`)
		w.Text(cat)
		w.Raw(`">`)
		w.Text(c.Status)
		w.Raw(`</span>`)
	}
	w.Raw(`</a>`)
}

// BlockCache renders documents block by block and keeps the HTML of each
// top-level block by node, so a commit re-renders only the blocks it
// changed; a block that holds an issue list renders every time. Reset it
// when what the Resolver returns changes (a chip's status, a page's
// title). Its zero value is ready; it is safe for concurrent use.
type BlockCache struct {
	once sync.Once
	c    *prosemirror.BlockCache[Resolver]
}

func (c *BlockCache) cache() *prosemirror.BlockCache[Resolver] {
	c.once.Do(func() { c.c = prosemirror.NewBlockCache(html) })
	return c.c
}

// Render renders the document, reusing the HTML of unchanged blocks.
func (c *BlockCache) Render(d prosemirror.Doc, r Resolver) string {
	return c.cache().Render(d, resolverOrNone(r))
}

// RenderHighlighted renders as Render does, with highlights.
func (c *BlockCache) RenderHighlighted(d prosemirror.Doc, r Resolver, hs []prosemirror.Highlight) string {
	return c.cache().RenderHighlighted(d, resolverOrNone(r), hs)
}

// Reset forgets every block.
func (c *BlockCache) Reset() { c.cache().Reset() }

// DocDiff is a comparison of two versions of a page (FR-88).
type DocDiff struct{ *prosemirror.DocDiff }

// Diff compares version a with version b.
func Diff(a, b prosemirror.Doc) DocDiff { return DocDiff{prosemirror.Diff(a, b)} }

// HTML renders the comparison: the newer version, with added text in
// <ins class="diff-ins"> and removed text in <del class="diff-del">; whole
// blocks added or removed carry data-diff="ins" or "del" (FR-88.1).
func (d DocDiff) HTML(r Resolver) string { return html.Diff(d.DocDiff, resolverOrNone(r)) }

// IssueLists returns the document's issue lists, in document order.
func IssueLists(d prosemirror.Doc) []IssueList {
	var out []IssueList
	d.Descendants(func(n *prosemirror.Node, _ int, _ *prosemirror.Node, _ int) bool {
		if n.Type().Name() == "issue_list" {
			out = append(out, IssueListOf(n))
		}
		return !n.Type().InlineContent()
	})
	return out
}
