package docnodes

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/zweiundeins/prosemirror-go"
)

type mdWriter = prosemirror.MarkdownWriter[Resolver]

var esc = prosemirror.MarkdownEscape

func mdList(w *mdWriter, n *prosemirror.Node) {
	order, _ := n.AttrInt("order")
	for i, item := range n.Content().Nodes() {
		if i > 0 && item.ChildCount() > 1 {
			w.BlankLine()
		}
		marker := "- "
		switch {
		case n.Type().Name() == "ordered_list":
			marker = strconv.Itoa(order+i) + ". "
		case item.AttrBool("checked"):
			marker = "- [x] "
		case n.Type().Name() == "task_list":
			marker = "- [ ] "
		}
		w.Blocks(item.Content().Nodes(), marker, strings.Repeat(" ", len(marker)))
	}
}

// markdown writes CommonMark with GitHub's tables and task lists. Its own
// nodes are written for a reader of the file, not as directives: a chip
// as its key, a mention as @name, a page link as a link.
var markdown = &prosemirror.Markdown[Resolver]{
	Verbatim: []string{"code"},
	Nodes: map[string]func(w *mdWriter, n *prosemirror.Node){
		"paragraph": func(w *mdWriter, n *prosemirror.Node) {
			for _, l := range strings.Split(w.Inline(n), "\n") {
				w.Line(l)
			}
		},
		"heading": func(w *mdWriter, n *prosemirror.Node) {
			l, _ := n.AttrInt("level")
			w.Line(strings.Repeat("#", min(max(l, 1), 3)+1) + " " + strings.ReplaceAll(w.Inline(n), "\n", " "))
		},
		"blockquote": func(w *mdWriter, n *prosemirror.Node) { w.Blocks(n.Content().Nodes(), "> ", "> ") },
		"code_block": func(w *mdWriter, n *prosemirror.Node) {
			text := n.TextBetween(0, n.Content().Size(), "", nil)
			fence := "```"
			for strings.Contains(text, fence) {
				fence += "`"
			}
			w.Line(fence + n.AttrString("language"))
			for _, l := range strings.Split(text, "\n") {
				w.Line(l)
			}
			w.Line(fence)
		},
		"horizontal_rule": func(w *mdWriter, n *prosemirror.Node) { w.Line("---") },
		"bullet_list":     mdList,
		"ordered_list":    mdList,
		"task_list":       mdList,
		"table": func(w *mdWriter, n *prosemirror.Node) {
			for ri, row := range n.Content().Nodes() {
				cells := make([]string, row.ChildCount())
				for ci, c := range row.Content().Nodes() {
					var parts []string
					for _, b := range c.Content().Nodes() {
						if b.Type().InlineContent() {
							parts = append(parts, w.Inline(b))
						} else {
							parts = append(parts, esc(b.TextBetween(0, b.Content().Size(), " ", nil)))
						}
					}
					cells[ci] = strings.ReplaceAll(strings.Join(parts, " "), "\n", " ")
				}
				w.Line("| " + strings.Join(cells, " | ") + " |")
				if ri == 0 {
					w.Line("|" + strings.Repeat(" --- |", len(cells)))
				}
			}
		},
		"issue_list": func(w *mdWriter, n *prosemirror.Node) {
			l := IssueListOf(n)
			if l.SavedFilter != "" {
				w.Line("Issue list of a saved filter")
				return
			}
			v, _ := url.ParseQuery(l.Filter)
			w.Line("Issue list: [issues matching " + esc(l.Filter) + "](/issues?" + v.Encode() + ")")
		},
		"image": func(w *mdWriter, n *prosemirror.Node) {
			w.Line("![" + esc(n.AttrString("alt")) + "](/img/lib/" + url.PathEscape(n.AttrString("imageId")) + ")")
		},
		"hard_break": func(w *mdWriter, n *prosemirror.Node) { w.Write("\\\n") },
		"issue_chip": func(w *mdWriter, n *prosemirror.Node) { w.Write(esc(n.AttrString("key"))) },
		"mention": func(w *mdWriter, n *prosemirror.Node) {
			name, ok := w.R.Person(n.AttrString("personId"))
			if !ok {
				name = n.AttrString("label")
			}
			w.Write("@" + esc(name))
		},
		"page_link": func(w *mdWriter, n *prosemirror.Node) {
			id := n.AttrString("pageId")
			if title, ok := w.R.Page(id); ok {
				w.Write("[" + esc(title) + "](/pages/" + url.PathEscape(id) + ")")
			} else {
				w.Write("a removed page")
			}
		},
	},
	Marks: map[string]func(w *mdWriter, m *prosemirror.Mark, n *prosemirror.Node, s string) string{
		"strong": func(_ *mdWriter, _ *prosemirror.Mark, _ *prosemirror.Node, s string) string { return "**" + s + "**" },
		"em":     func(_ *mdWriter, _ *prosemirror.Mark, _ *prosemirror.Node, s string) string { return "*" + s + "*" },
		"strike": func(_ *mdWriter, _ *prosemirror.Mark, _ *prosemirror.Node, s string) string { return "~~" + s + "~~" },
		"code": func(_ *mdWriter, _ *prosemirror.Mark, n *prosemirror.Node, s string) string {
			if !n.IsText() {
				return s
			}
			t := n.Text()
			fence := "`"
			for strings.Contains(t, fence) {
				fence += "`"
			}
			if strings.HasPrefix(t, "`") || strings.HasSuffix(t, "`") {
				return fence + " " + t + " " + fence
			}
			return fence + t + fence
		},
		"link": func(_ *mdWriter, m *prosemirror.Mark, n *prosemirror.Node, s string) string {
			if href := m.AttrString("href"); n.IsText() {
				if _, ok := prosemirror.SafeHref(href); ok {
					return "[" + s + "](<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(href) + ">)"
				}
			}
			return s
		},
	},
}

// Markdown exports the document as CommonMark with GitHub's tables and
// task lists (FR-152). Headings move one level down, as the page's title
// is its level-one heading. Chips export as their key, mentions as
// @name, page links as links to the page; unsafe links as their text.
func Markdown(d prosemirror.Doc, r Resolver) string { return markdown.Render(d, resolverOrNone(r)) }

// leaves is what tickit's leaf nodes stand for in plain text.
var leaves = prosemirror.LeafText{
	"issue_chip": func(n *prosemirror.Node) string { return n.AttrString("key") },
	"mention": func(n *prosemirror.Node) string {
		if l := n.AttrString("label"); l != "" {
			return "@" + l
		}
		return ""
	},
	"hard_break": func(*prosemirror.Node) string { return "\n" },
	"image":      func(n *prosemirror.Node) string { return n.AttrString("alt") },
}

// Text returns the document's text for search: blocks separated by
// newlines, chips by key, mentions by their name in the document, hard
// breaks as newlines and images by their alt text.
func Text(d prosemirror.Doc) string { return leaves.Text(d, "\n") }

// Refs are the ids a document refers to, each once and sorted.
type Refs struct {
	Issues []string // issue keys of chips (FR-101)
	People []string // person ids of mentions
	Pages  []string // page ids of page links (FR-103)
	Images []string // image ids, for the hidden ones (DM-07.1)
}

// References returns the issue keys, person ids, page ids and images of
// the document's atoms.
func References(d prosemirror.Doc) Refs {
	var r Refs
	d.Descendants(func(n *prosemirror.Node, _ int, _ *prosemirror.Node, _ int) bool {
		switch n.Type().Name() {
		case "issue_chip":
			r.Issues = append(r.Issues, n.AttrString("key"))
		case "mention":
			r.People = append(r.People, n.AttrString("personId"))
		case "page_link":
			r.Pages = append(r.Pages, n.AttrString("pageId"))
		case "image":
			r.Images = append(r.Images, n.AttrString("imageId"))
		}
		return true
	})
	for _, s := range []*[]string{&r.Issues, &r.People, &r.Pages, &r.Images} {
		slices.Sort(*s)
		*s = slices.Compact(*s)
	}
	return r
}
