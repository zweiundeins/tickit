package doc

import (
	"net/url"
	"strconv"
	"strings"
)

// Markdown exports the document as CommonMark with GitHub's tables and
// task lists (FR-152). Headings move one level down, as the page's title
// is its level-one heading. Chips export as their key, mentions as
// @name, page links as links to the page; unsafe links as their text.
func Markdown(d Doc, r Resolver) string {
	m := &mdWriter{r: resolverOrNone(r)}
	m.blocks(d.content.nodes, "", "")
	return strings.TrimRight(m.b.String(), "\n") + "\n"
}

type mdWriter struct {
	b strings.Builder
	r Resolver
}

func (m *mdWriter) blocks(nodes []*Node, first, rest string) {
	for i, n := range nodes {
		if i > 0 {
			m.b.WriteString(strings.TrimRight(rest, " ") + "\n")
		}
		m.block(n, first, rest)
		first = rest
	}
}

func (m *mdWriter) line(prefix, s string) { m.b.WriteString(prefix + s + "\n") }

func (m *mdWriter) block(n *Node, first, rest string) {
	switch n.typ.name {
	case "paragraph":
		lines := strings.Split(m.inline(n.content.nodes), "\n")
		for i, l := range lines {
			p := rest
			if i == 0 {
				p = first
			}
			m.line(p, l)
		}
	case "heading":
		l, _ := n.AttrInt("level")
		m.line(first, strings.Repeat("#", min(max(l, 1), 3)+1)+" "+strings.ReplaceAll(m.inline(n.content.nodes), "\n", " "))
	case "blockquote":
		m.blocks(n.content.nodes, first+"> ", rest+"> ")
	case "code_block":
		text := n.TextBetween(0, n.content.size, "", nil)
		fence := "```"
		for strings.Contains(text, fence) {
			fence += "`"
		}
		m.line(first, fence+n.AttrString("language"))
		for _, l := range strings.Split(text, "\n") {
			m.line(rest, l)
		}
		m.line(rest, fence)
	case "horizontal_rule":
		m.line(first, "---")
	case "bullet_list", "ordered_list", "task_list":
		order, _ := n.AttrInt("order")
		for i, item := range n.content.nodes {
			if i > 0 && len(item.content.nodes) > 1 {
				m.b.WriteString(strings.TrimRight(rest, " ") + "\n")
			}
			marker := "- "
			switch {
			case n.typ.name == "ordered_list":
				marker = strconv.Itoa(order+i) + ". "
			case item.AttrBool("checked"):
				marker = "- [x] "
			case n.typ.name == "task_list":
				marker = "- [ ] "
			}
			p := first
			if i > 0 {
				p = rest
			}
			m.blocks(item.content.nodes, p+marker, rest+strings.Repeat(" ", len(marker)))
		}
	case "table":
		for ri, row := range n.content.nodes {
			cells := make([]string, len(row.content.nodes))
			for ci, c := range row.content.nodes {
				var parts []string
				for _, b := range c.content.nodes {
					if b.typ.inlineContent {
						parts = append(parts, m.inline(b.content.nodes))
					} else {
						parts = append(parts, mdEscape(b.TextBetween(0, b.content.size, " ", nil)))
					}
				}
				cells[ci] = strings.ReplaceAll(strings.Join(parts, " "), "\n", " ")
			}
			p := first
			if ri > 0 {
				p = rest
			}
			m.line(p, "| "+strings.Join(cells, " | ")+" |")
			if ri == 0 {
				m.line(rest, "|"+strings.Repeat(" --- |", len(cells)))
			}
		}
	case "image":
		m.line(first, "!["+mdEscape(n.AttrString("alt"))+"](/img/lib/"+url.PathEscape(n.AttrString("imageId"))+")")
	default:
		m.blocks(n.content.nodes, first, rest)
	}
}

var mdSpecial = strings.NewReplacer(`\`, `\\`, "`", "\\`", `*`, `\*`, `_`, `\_`, `[`, `\[`, `]`, `\]`, `<`, `\<`, `>`, `\>`, `#`, `\#`, `|`, `\|`, `~`, `\~`)

func mdEscape(s string) string { return mdSpecial.Replace(s) }

func (m *mdWriter) inline(nodes []*Node) string {
	var b strings.Builder
	for _, n := range nodes {
		var s string
		switch n.typ.name {
		case "text":
			s = mdEscape(n.text)
		case "hard_break":
			s = "\\\n"
		case "issue_chip":
			s = mdEscape(n.AttrString("key"))
		case "mention":
			name, ok := m.r.Person(n.AttrString("personId"))
			if !ok {
				name = n.AttrString("label")
			}
			s = "@" + mdEscape(name)
		case "page_link":
			id := n.AttrString("pageId")
			if title, ok := m.r.Page(id); ok {
				s = "[" + mdEscape(title) + "](/pages/" + url.PathEscape(id) + ")"
			} else {
				s = "a removed page"
			}
		}
		for i := len(n.marks) - 1; i >= 0; i-- {
			switch mk := n.marks[i]; mk.typ.name {
			case "strong":
				s = "**" + s + "**"
			case "em":
				s = "*" + s + "*"
			case "strike":
				s = "~~" + s + "~~"
			case "code":
				if n.typ.isText {
					t := n.text
					fence := "`"
					for strings.Contains(t, fence) {
						fence += "`"
					}
					s = fence + t + fence
					if strings.HasPrefix(t, "`") || strings.HasSuffix(t, "`") {
						s = fence + " " + t + " " + fence
					}
				}
			case "link":
				if href := mk.AttrString("href"); n.typ.isText {
					if _, ok := SafeHref(href); ok {
						s = "[" + s + "](<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(href) + ">)"
					}
				}
			}
		}
		b.WriteString(s)
	}
	return b.String()
}
