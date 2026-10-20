package markup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Write turns a document of tickit's schema, as JSON, back into markup,
// for editing an existing description or comment as text: Parse of what
// Write returns gives the same document for what the markup can say.
// Chips write as [[KEY]], mentions as @id, page links as [[page:Title]]
// with the title pageRef gives for the page's id (the id when it has
// none, or when pageRef is nil). Empty input writes "".
func Write(raw json.RawMessage, pageRef func(id string) (string, bool)) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var d wnode
	if err := json.Unmarshal(raw, &d); err != nil {
		return "", err
	}
	if d.Type != "doc" {
		return "", fmt.Errorf("markup: not a document but a %q", d.Type)
	}
	w := &writer{pageRef: pageRef}
	w.blocks(d.Content, "", "")
	return strings.TrimRight(w.b.String(), "\n"), nil
}

// wnode is a node as the JSON holds it, for Write.
type wnode struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs"`
	Content []wnode        `json:"content"`
	Marks   []wmark        `json:"marks"`
	Text    string         `json:"text"`
}

type wmark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

func (n wnode) str(key string) string {
	s, _ := n.Attrs[key].(string)
	return s
}

func (n wnode) num(key string) int {
	f, _ := n.Attrs[key].(float64)
	return int(f)
}

type writer struct {
	b       strings.Builder
	pageRef func(string) (string, bool)
}

func (w *writer) line(prefix, s string) {
	w.b.WriteString(strings.TrimRight(prefix+s, " ") + "\n")
}

// blocks writes blocks separated by blank lines; first prefixes the first
// line, rest every other (a list item's indentation, a quote's "> ").
func (w *writer) blocks(nodes []wnode, first, rest string) {
	for i, n := range nodes {
		if i > 0 {
			w.line(rest, "")
		}
		w.block(n, first, rest)
		first = rest
	}
}

func (w *writer) block(n wnode, first, rest string) {
	switch n.Type {
	case "paragraph":
		for i, l := range strings.Split(w.inline(n.Content), "\n") {
			p := rest
			if i == 0 {
				p = first
			}
			w.line(p, l)
		}
	case "heading":
		level := min(max(n.num("level"), 1), 3)
		w.line(first, strings.Repeat("#", level)+" "+strings.ReplaceAll(w.inline(n.Content), "\\\n", " "))
	case "blockquote":
		w.blocks(n.Content, first+"> ", rest+"> ")
	case "code_block":
		var text strings.Builder
		for _, c := range n.Content {
			text.WriteString(c.Text)
		}
		w.line(first, "```"+n.str("language"))
		if text.Len() > 0 {
			for _, l := range strings.Split(text.String(), "\n") {
				w.b.WriteString(rest + l + "\n")
			}
		}
		w.line(rest, "```")
	case "horizontal_rule":
		w.line(first, "---")
	case "image":
		w.line(first, "!["+strings.NewReplacer("]", "\\]", "[", "\\[").Replace(n.str("alt"))+"](lib:"+n.str("imageId")+")")
	case "bullet_list", "ordered_list", "task_list":
		order := n.num("order")
		if order < 1 {
			order = 1
		}
		for i, item := range n.Content {
			if i > 0 && (len(item.Content) > 1 || len(n.Content[i-1].Content) > 1) {
				w.line(rest, "")
			}
			marker := "- "
			switch {
			case n.Type == "ordered_list":
				marker = strconv.Itoa(order+i) + ". "
			case n.Type == "task_list" && item.Attrs["checked"] == true:
				marker = "- [x] "
			case n.Type == "task_list":
				marker = "- [ ] "
			}
			indent := "  "
			if n.Type == "ordered_list" {
				indent = strings.Repeat(" ", len(marker))
			}
			content := item.Content
			if len(content) > 0 && content[0].Type == "paragraph" && len(content[0].Content) == 0 && len(content) > 1 {
				w.line(p(i == 0, first, rest), strings.TrimRight(marker, " "))
				w.line(rest+indent, "")
				w.blocks(content[1:], rest+indent, rest+indent)
				continue
			}
			w.blocks(content, p(i == 0, first, rest)+marker, rest+indent)
		}
	case "table":
		for r, row := range n.Content {
			cells := make([]string, len(row.Content))
			for c, cell := range row.Content {
				var parts []string
				for _, para := range cell.Content {
					parts = append(parts, strings.ReplaceAll(w.inline(para.Content), "\\\n", " "))
				}
				cells[c] = strings.ReplaceAll(strings.Join(parts, " "), "|", "\\|")
			}
			w.line(p(r == 0, first, rest), "| "+strings.Join(cells, " | ")+" |")
			if r == 0 {
				seps := make([]string, len(cells))
				for c := range seps {
					seps[c] = "---"
				}
				w.line(rest, "| "+strings.Join(seps, " | ")+" |")
			}
		}
	default:
		// A block the markup cannot say: its text, as a paragraph.
		w.line(first, w.inline(n.Content))
	}
}

func p(isFirst bool, first, rest string) string {
	if isFirst {
		return first
	}
	return rest
}

// inline writes a paragraph's content. A hard break writes "\" and a new
// line, which Write's caller indents with the block.
func (w *writer) inline(nodes []wnode) string {
	// Hard breaks at the end of a paragraph cannot be written.
	for len(nodes) > 0 && nodes[len(nodes)-1].Type == "hard_break" {
		nodes = nodes[:len(nodes)-1]
	}
	var b strings.Builder
	var open []wmark // the marks open now, outermost first
	closeTo := func(n int) {
		for len(open) > n {
			m := open[len(open)-1]
			open = open[:len(open)-1]
			switch m.Type {
			case "link":
				href, _ := m.Attrs["href"].(string)
				b.WriteString("](" + strings.NewReplacer(")", "%29", " ", "%20").Replace(href) + ")")
			default:
				b.WriteString(delim(m.Type))
			}
		}
	}
	lineStart := true
	for _, n := range nodes {
		marks := sortedMarks(n.Marks)
		var code bool
		if k := slices.IndexFunc(marks, func(m wmark) bool { return m.Type == "code" }); k >= 0 {
			code = true
			marks = slices.Delete(slices.Clone(marks), k, k+1)
		}
		// Keep the longest run of open marks that the node shares.
		same := 0
		for same < len(open) && same < len(marks) && sameMark(open[same], marks[same]) {
			same++
		}
		closeTo(same)
		for _, m := range marks[same:] {
			if m.Type == "link" {
				b.WriteString("[")
			} else {
				b.WriteString(delim(m.Type))
			}
			open = append(open, m)
		}
		switch n.Type {
		case "text":
			if code {
				b.WriteString(codeSpan(n.Text))
			} else {
				b.WriteString(escape(n.Text, lineStart && len(marks) == 0))
			}
		case "hard_break":
			closeTo(0)
			b.WriteString("\\\n")
			lineStart = true
			continue
		case "mention":
			b.WriteString("@" + n.str("personId"))
		case "issue_chip":
			b.WriteString("[[" + n.str("key") + "]]")
		case "page_link":
			ref := n.str("pageId")
			if w.pageRef != nil {
				if t, ok := w.pageRef(ref); ok {
					ref = t
				}
			}
			b.WriteString("[[page:" + ref + "]]")
		default:
			b.WriteString(escape(n.Text, lineStart))
		}
		lineStart = false
	}
	closeTo(0)
	return b.String()
}

func delim(mark string) string {
	switch mark {
	case "strong":
		return "**"
	case "em":
		return "_"
	case "strike":
		return "~~"
	}
	return ""
}

func sortedMarks(ms []wmark) []wmark {
	out := slices.Clone(ms)
	slices.SortStableFunc(out, func(a, b wmark) int { return markRank[a.Type] - markRank[b.Type] })
	return out
}

func sameMark(a, b wmark) bool {
	if a.Type != b.Type {
		return false
	}
	if a.Type == "link" {
		return a.Attrs["href"] == b.Attrs["href"]
	}
	return true
}

// codeSpan writes inline code with enough backticks around it.
func codeSpan(s string) string {
	n := 1
	for strings.Contains(s, strings.Repeat("`", n)) {
		n++
	}
	fence := strings.Repeat("`", n)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") || strings.HasPrefix(s, " ") && strings.HasSuffix(s, " ") {
		s = " " + s + " "
	}
	return fence + s + fence
}

// escape writes text so that Parse reads it back as text: punctuation the
// markup reads gets a backslash, and so does what would start a block at
// the start of a line.
func escape(s string, lineStart bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '*', '`', '[', ']', '~':
			b.WriteByte('\\')
		case '_':
			if i == 0 || i == len(s)-1 || !isWordByte(s[i-1]) || !isWordByte(s[i+1]) {
				b.WriteByte('\\')
			}
		case '@':
			if mentionRe.MatchString(s[i:]) {
				b.WriteByte('\\')
			}
		case '!':
			if i+1 < len(s) && s[i+1] == '[' {
				b.WriteByte('\\')
			}
		}
		b.WriteByte(c)
	}
	out := b.String()
	if !lineStart || out == "" {
		return out
	}
	if m := orderedRe.FindString(out); m != "" {
		return m[:len(m)-2] + "\\" + m[len(m)-2:] + out[len(m):]
	}
	switch out[0] {
	case '#', '>', '-', '|':
		return "\\" + out
	}
	return out
}
