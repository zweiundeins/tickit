package markup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Resolver answers the references a markup text makes while Parse converts
// it: the seed's content (seed.Content.Resolver), or the tracker and the
// people of the app for what people type.
type Resolver interface {
	// Issue reports whether an issue chip's key names an issue.
	Issue(key string) bool
	// Person returns the label of a mention ("Priya Raman") for a person id.
	Person(id string) (label string, ok bool)
	// Page returns the page id a page link names: a title in the current
	// space, or "ENG/Title" for a page of another space.
	Page(ref string) (pageID string, ok bool)
	// Image reports whether an image id is in the library.
	Image(id string) bool
}

// Limits of a document, from internal/doc/schema.json and DM-05.
const (
	maxTextLength = 2000
	maxAttrLength = 2048
	maxBlocks     = 2000
	maxDocBytes   = 204800
	maxTableRows  = 50
	maxTableCols  = 20
)

var (
	codeLanguages = []string{"", "text", "go", "js", "ts", "html", "css", "sql", "json", "yaml", "bash", "python", "java", "rust", "markdown", "diff"}
	linkSchemes   = []string{"http", "https", "mailto"}
	markRank      = map[string]int{"link": 0, "strong": 1, "em": 2, "strike": 3, "code": 4}

	issueKeyRe  = regexp.MustCompile(`^[A-Z]{2,6}-[1-9][0-9]*$`)
	issueKeyCI  = regexp.MustCompile(`^[A-Za-z]{2,6}-[1-9][0-9]*$`)
	mentionRe   = regexp.MustCompile(`^@(s-[a-z]+|g-[a-z2-7]{20}|a-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)
	imageLineRe = regexp.MustCompile(`^!\[([^\]]*)\]\(lib:([a-z0-9-]+)\)$`)
	headingRe   = regexp.MustCompile(`^(#{1,6}) (.*)$`)
	orderedRe   = regexp.MustCompile(`^([0-9]{1,3})\. `)
	sepCellRe   = regexp.MustCompile(`^:?-{3,}:?$`)
)

// Mode is how strictly a text is read.
type Mode int

const (
	// Strict refuses whatever the markup does not define: the seed's
	// content, which is checked once.
	Strict Mode = iota
	// Text is what people type into a description: what the markup does
	// not define stays literal text, an unknown reference stays as typed.
	// Only the document's limits refuse it.
	Text
	// Comment is Text without headings and tables, which comments do not
	// hold (requirements 4.2): those lines stay paragraphs.
	Comment
)

// Parse converts markup (see the package documentation) into a ProseMirror
// document of tickit's schema, as JSON. Every reference must resolve; an
// empty text gives a document with one empty paragraph.
func Parse(markup string, resolve Resolver) (json.RawMessage, error) {
	return ParseMode(markup, resolve, Strict)
}

// ParseMode converts markup in a mode. Text and Comment refuse a text only
// over the limits of a document; their errors read as a person's message
// ("The text is longer than ...").
func ParseMode(markup string, resolve Resolver, mode Mode) (json.RawMessage, error) {
	p := &parser{resolve: resolve, mode: mode}
	var lines []line
	for i, s := range strings.Split(strings.ReplaceAll(markup, "\r\n", "\n"), "\n") {
		lines = append(lines, line{strings.TrimRight(s, " "), i + 1})
	}
	blocks, err := p.blocks(lines)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		blocks = []*node{{Type: "paragraph"}}
	}
	doc := &node{Type: "doc", Content: blocks}
	if n := countBlocks(doc); n > maxBlocks {
		return nil, fmt.Errorf("the document has %d blocks, more than %d", n, maxBlocks)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	out := bytes.TrimRight(buf.Bytes(), "\n")
	if len(out) > maxDocBytes {
		return nil, fmt.Errorf("the document takes %d bytes, more than %d", len(out), maxDocBytes)
	}
	return json.RawMessage(out), nil
}

// node is a ProseMirror node in the field order of Node.toJSON.
type node struct {
	Type    string  `json:"type"`
	Attrs   attrs   `json:"attrs,omitempty"`
	Content []*node `json:"content,omitempty"`
	Marks   []mark  `json:"marks,omitempty"`
	Text    string  `json:"text,omitempty"`
}

type mark struct {
	Type  string `json:"type"`
	Attrs attrs  `json:"attrs,omitempty"`
}

// attrs keeps the order in which the schema declares a node's attributes.
type attrs []attr

type attr struct {
	key   string
	value any
}

func (a attrs) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, kv := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, _ := json.Marshal(kv.key)
		buf.Write(k)
		buf.WriteByte(':')
		var v bytes.Buffer
		enc := json.NewEncoder(&v)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(kv.value); err != nil {
			return nil, err
		}
		buf.Write(bytes.TrimRight(v.Bytes(), "\n"))
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func cellAttrs() attrs {
	return attrs{{"colspan", 1}, {"rowspan", 1}, {"colwidth", nil}}
}

type line struct {
	text string
	no   int
}

type parser struct {
	resolve Resolver
	mode    Mode
}

// lenient is true for what people type: what the markup does not define
// stays text.
func (p *parser) lenient() bool { return p.mode != Strict }

func errAt(l line, format string, args ...any) error {
	return fmt.Errorf("line %d: %s", l.no, fmt.Sprintf(format, args...))
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

func indentOf(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

type listKind int

const (
	notList listKind = iota
	bulletList
	taskList
	orderedList
)

// marker returns the kind of list item a line starts, the width of its
// marker, the item's text, and for a task whether it is checked.
func marker(s string) (kind listKind, width int, text string, checked bool) {
	switch {
	case strings.HasPrefix(s, "- [ ] ") || s == "- [ ]":
		return taskList, 2, strings.TrimPrefix(strings.TrimPrefix(s, "- [ ]"), " "), false
	case strings.HasPrefix(s, "- [x] ") || s == "- [x]":
		return taskList, 2, strings.TrimPrefix(strings.TrimPrefix(s, "- [x]"), " "), true
	case strings.HasPrefix(s, "- ") || s == "-":
		return bulletList, 2, strings.TrimPrefix(strings.TrimPrefix(s, "-"), " "), false
	}
	if m := orderedRe.FindStringSubmatch(s); m != nil {
		return orderedList, len(m[0]), s[len(m[0]):], false
	}
	return notList, 0, "", false
}

// startsBlock reports whether a line at the current indentation begins a
// block other than a paragraph, which ends a paragraph before it.
func startsBlock(s string) bool {
	if strings.HasPrefix(s, "```") || strings.HasPrefix(s, ">") || strings.HasPrefix(s, "|") || strings.HasPrefix(s, "![") {
		return true
	}
	if s == "---" || headingRe.MatchString(s) {
		return true
	}
	k, _, _, _ := marker(s)
	return k != notList
}

func (p *parser) blocks(ls []line) ([]*node, error) {
	var out []*node
	for i := 0; i < len(ls); {
		l := ls[i]
		s := l.text
		if blank(s) {
			i++
			continue
		}
		if indentOf(s) > 0 {
			if !p.lenient() {
				return nil, errAt(l, "unexpected indentation (indent a line only inside a list item)")
			}
			s = strings.TrimLeft(s, " ")
			l = line{s, l.no}
			ls = append(ls[:i:i], append([]line{l}, ls[i+1:]...)...)
		}
		switch {
		case strings.HasPrefix(s, "```"):
			n, next, err := p.fence(ls, i)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
			i = next
		case headingRe.MatchString(s) && p.mode != Comment:
			m := headingRe.FindStringSubmatch(s)
			level := len(m[1])
			if level > 3 {
				if !p.lenient() {
					return nil, errAt(l, "heading level %d (levels 1 to 3 only)", level)
				}
				level = 3
			}
			inl, err := p.inline(strings.TrimSpace(m[2]), l)
			if err != nil {
				return nil, err
			}
			if len(inl) == 0 {
				if !p.lenient() {
					return nil, errAt(l, "an empty heading")
				}
				i++
				continue
			}
			out = append(out, &node{Type: "heading", Attrs: attrs{{"level", level}}, Content: inl})
			i++
		case s == "---":
			out = append(out, &node{Type: "horizontal_rule"})
			i++
		case strings.HasPrefix(s, ">"):
			var inner []line
			for i < len(ls) && strings.HasPrefix(ls[i].text, ">") {
				t := strings.TrimPrefix(ls[i].text, ">")
				t = strings.TrimPrefix(t, " ")
				inner = append(inner, line{t, ls[i].no})
				i++
			}
			content, err := p.blocks(inner)
			if err != nil {
				return nil, err
			}
			if len(content) == 0 {
				if !p.lenient() {
					return nil, errAt(l, "an empty quote")
				}
				continue
			}
			out = append(out, &node{Type: "blockquote", Content: content})
		case strings.HasPrefix(s, "|") && p.mode != Comment:
			n, next, err := p.table(ls, i)
			if err != nil {
				if !p.lenient() {
					return nil, err
				}
				if n, err = p.paragraph([]line{l}); err != nil {
					return nil, err
				}
				next = i + 1
			}
			out = append(out, n)
			i = next
		case strings.HasPrefix(s, "![") && p.mode != Comment:
			n, err := p.image(l)
			if err != nil {
				if !p.lenient() {
					return nil, err
				}
				if n, err = p.paragraph([]line{l}); err != nil {
					return nil, err
				}
			}
			out = append(out, n)
			i++
		default:
			if k, _, _, _ := marker(s); k != notList {
				n, next, err := p.list(ls, i, k)
				if err != nil {
					return nil, err
				}
				out = append(out, n)
				i = next
				continue
			}
			var para []line
			for i < len(ls) && !blank(ls[i].text) && indentOf(ls[i].text) == 0 && (len(para) == 0 || !startsBlock(ls[i].text)) {
				para = append(para, ls[i])
				i++
			}
			n, err := p.paragraph(para)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
	}
	return out, nil
}

func (p *parser) image(l line) (*node, error) {
	m := imageLineRe.FindStringSubmatch(l.text)
	if m == nil {
		return nil, errAt(l, "an image is a line of its own: ![alt text](lib:image-id)")
	}
	alt, id := strings.TrimSpace(m[1]), m[2]
	if alt == "" {
		return nil, errAt(l, "image %s has no alt text", id)
	}
	if p.resolve != nil && !p.resolve.Image(id) {
		return nil, errAt(l, "unknown library image %q", id)
	}
	if len(alt) > maxAttrLength {
		return nil, errAt(l, "alt text longer than %d bytes", maxAttrLength)
	}
	return &node{Type: "image", Attrs: attrs{{"imageId", id}, {"src", "/img/lib/" + id}, {"alt", alt}}}, nil
}

func (p *parser) fence(ls []line, i int) (*node, int, error) {
	open := ls[i]
	lang := strings.TrimSpace(strings.TrimPrefix(open.text, "```"))
	if !slices.Contains(codeLanguages, lang) {
		if !p.lenient() {
			return nil, 0, errAt(open, "code language %q is not in the allowlist", lang)
		}
		lang = ""
	}
	var code []string
	for j := i + 1; j <= len(ls); j++ {
		end := j == len(ls)
		if end && p.lenient() || !end && strings.TrimSpace(ls[j].text) == "```" && indentOf(ls[j].text) == 0 {
			n := &node{Type: "code_block", Attrs: attrs{{"language", lang}}}
			if text := strings.Join(code, "\n"); text != "" {
				if utf8.RuneCountInString(text) > maxTextLength {
					return nil, 0, errAt(open, "a code block longer than %d characters", maxTextLength)
				}
				n.Content = []*node{{Type: "text", Text: text}}
			}
			return n, min(j+1, len(ls)), nil
		}
		if end {
			break
		}
		code = append(code, ls[j].text)
	}
	return nil, 0, errAt(open, "a code block without its closing ```")
}

func (p *parser) list(ls []line, i int, kind listKind) (*node, int, error) {
	first := ls[i]
	var items []*node
	order := 1
	for i < len(ls) {
		s := ls[i].text
		if blank(s) {
			// A blank line continues the list when the next line is an item
			// of the same kind or belongs to the current item.
			j := i
			for j < len(ls) && blank(ls[j].text) {
				j++
			}
			if j == len(ls) {
				return wrapList(kind, items, order), j, nil
			}
			if k, _, _, _ := marker(ls[j].text); k == kind && indentOf(ls[j].text) == 0 {
				i = j
				continue
			}
			return wrapList(kind, items, order), i, nil
		}
		k, width, text, checked := marker(s)
		if k != kind || indentOf(s) != 0 {
			break
		}
		if kind == orderedList && len(items) == 0 {
			order, _ = strconv.Atoi(strings.TrimSuffix(s[:width], ". "))
		}
		if kind != orderedList {
			width = 2
		}
		itemLines := []line{{text, ls[i].no}}
		i++
		for i < len(ls) {
			if blank(ls[i].text) {
				// Blank lines stay in the item when its content goes on.
				j := i
				for j < len(ls) && blank(ls[j].text) {
					j++
				}
				if j == len(ls) || indentOf(ls[j].text) < width {
					break
				}
				for ; i < j; i++ {
					itemLines = append(itemLines, line{"", ls[i].no})
				}
				continue
			}
			if indentOf(ls[i].text) < width {
				break
			}
			itemLines = append(itemLines, line{ls[i].text[width:], ls[i].no})
			i++
		}
		for i < len(ls) && !blank(ls[i].text) && indentOf(ls[i].text) > 0 && !p.lenient() {
			return nil, 0, errAt(ls[i], "list item content must be indented by %d spaces", width)
		}
		content, err := p.blocks(itemLines)
		if err != nil {
			return nil, 0, err
		}
		if len(content) == 0 || content[0].Type != "paragraph" {
			content = append([]*node{{Type: "paragraph"}}, content...)
		}
		item := &node{Type: "list_item", Content: content}
		if kind == taskList {
			item.Type = "task_item"
			item.Attrs = attrs{{"checked", checked}}
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, 0, errAt(first, "an empty list")
	}
	return wrapList(kind, items, order), i, nil
}

func wrapList(kind listKind, items []*node, order int) *node {
	switch kind {
	case taskList:
		return &node{Type: "task_list", Content: items}
	case orderedList:
		return &node{Type: "ordered_list", Attrs: attrs{{"order", order}}, Content: items}
	}
	return &node{Type: "bullet_list", Content: items}
}

func (p *parser) table(ls []line, i int) (*node, int, error) {
	head := ls[i]
	header, err := splitRow(head)
	if err != nil {
		return nil, 0, err
	}
	if i+1 >= len(ls) {
		return nil, 0, errAt(head, "a table needs a separator row after its header")
	}
	sep, err := splitRow(ls[i+1])
	if err != nil {
		return nil, 0, err
	}
	if len(sep) != len(header) {
		return nil, 0, errAt(ls[i+1], "the separator row has %d cells, the header %d", len(sep), len(header))
	}
	for _, c := range sep {
		if !sepCellRe.MatchString(strings.TrimSpace(c)) {
			return nil, 0, errAt(ls[i+1], "a table needs a separator row like | --- | --- | after its header")
		}
	}
	if len(header) > maxTableCols {
		return nil, 0, errAt(head, "a table has %d columns, more than %d", len(header), maxTableCols)
	}
	rows := [][]string{header}
	lines := []line{head}
	j := i + 2
	for ; j < len(ls) && strings.HasPrefix(ls[j].text, "|"); j++ {
		cells, err := splitRow(ls[j])
		if err != nil {
			return nil, 0, err
		}
		if len(cells) != len(header) {
			return nil, 0, errAt(ls[j], "a row has %d cells, the header %d", len(cells), len(header))
		}
		rows = append(rows, cells)
		lines = append(lines, ls[j])
	}
	if len(rows) > maxTableRows {
		return nil, 0, errAt(head, "a table has %d rows, more than %d", len(rows), maxTableRows)
	}
	table := &node{Type: "table"}
	for r, cells := range rows {
		row := &node{Type: "table_row"}
		for _, c := range cells {
			inl, err := p.inline(strings.TrimSpace(c), lines[r])
			if err != nil {
				return nil, 0, err
			}
			cellType := "table_cell"
			if r == 0 {
				cellType = "table_header"
			}
			row.Content = append(row.Content, &node{Type: cellType, Attrs: cellAttrs(), Content: []*node{{Type: "paragraph", Content: inl}}})
		}
		table.Content = append(table.Content, row)
	}
	return table, j, nil
}

// splitRow splits "| a | b \| c |" into its cells; "\|" is a literal pipe.
func splitRow(l line) ([]string, error) {
	s := strings.TrimSpace(l.text)
	if !strings.HasPrefix(s, "|") || !strings.HasSuffix(s, "|") || len(s) < 2 {
		return nil, errAt(l, "a table row starts and ends with |")
	}
	s = s[1 : len(s)-1]
	var cells []string
	var cur strings.Builder
	for k := 0; k < len(s); k++ {
		switch {
		case s[k] == '\\' && k+1 < len(s) && s[k+1] == '|':
			cur.WriteByte('|')
			k++
		case s[k] == '|':
			cells = append(cells, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(s[k])
		}
	}
	return append(cells, cur.String()), nil
}

func (p *parser) paragraph(ls []line) (*node, error) {
	var b strings.Builder
	for k, l := range ls {
		t := l.text
		if strings.ContainsRune(t, '\t') {
			if !p.lenient() {
				return nil, errAt(l, "a tab outside a code block")
			}
			t = strings.ReplaceAll(t, "\t", "    ")
		}
		t = strings.TrimSpace(t)
		hard := strings.HasSuffix(t, `\`) && !strings.HasSuffix(t, `\\`)
		if hard {
			t = strings.TrimRight(strings.TrimSuffix(t, `\`), " ")
		}
		b.WriteString(t)
		if k < len(ls)-1 {
			if hard {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		} else if hard && !p.lenient() {
			return nil, errAt(l, "a hard break (\\ at the end of a line) ends a paragraph")
		}
	}
	inl, err := p.inline(b.String(), ls[0])
	if err != nil {
		return nil, err
	}
	return &node{Type: "paragraph", Content: inl}, nil
}

// inline converts the text of one paragraph, heading or cell. "\n" in it
// stands for a hard break.
func (p *parser) inline(s string, at line) ([]*node, error) {
	nodes, err := p.spans(s, nil, at)
	if err != nil {
		return nil, err
	}
	return merge(nodes, at)
}

func withMark(marks []mark, m mark) []mark {
	for _, x := range marks {
		if x.Type == m.Type {
			return marks
		}
	}
	out := append(slices.Clone(marks), m)
	slices.SortStableFunc(out, func(a, b mark) int { return markRank[a.Type] - markRank[b.Type] })
	return out
}

func hasMark(marks []mark, name string) bool {
	return slices.ContainsFunc(marks, func(m mark) bool { return m.Type == name })
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isPunct(c byte) bool {
	return c < 0x80 && unicode.IsPunct(rune(c)) || strings.IndexByte("$+<=>^`|~", c) >= 0
}

// closing finds the delimiter that closes the one ending at from, skipping
// escapes, code spans, chips and nested strong pairs.
func closing(s string, from int, delim string) int {
	for k := from; k < len(s); {
		switch {
		case s[k] == '\\' && k+1 < len(s):
			k += 2
		case s[k] == '`':
			n := runLen(s, k, '`')
			end := strings.Index(s[k+n:], strings.Repeat("`", n))
			if end < 0 {
				return -1
			}
			k += n + end + n
		case strings.HasPrefix(s[k:], "[["):
			end := strings.Index(s[k:], "]]")
			if end < 0 {
				return -1
			}
			k += end + 2
		case delim == "*" && strings.HasPrefix(s[k:], "**"):
			end := closing(s, k+2, "**")
			if end < 0 {
				return -1
			}
			k = end + 2
		case strings.HasPrefix(s[k:], delim):
			if delim == "_" && k+1 < len(s) && isWordByte(s[k+1]) {
				k++
				continue
			}
			return k
		default:
			k++
		}
	}
	return -1
}

func runLen(s string, k int, c byte) int {
	n := 0
	for k+n < len(s) && s[k+n] == c {
		n++
	}
	return n
}

func (p *parser) spans(s string, marks []mark, at line) ([]*node, error) {
	var out []*node
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, &node{Type: "text", Marks: marks, Text: buf.String()})
			buf.Reset()
		}
	}
	emph := func(i int, delim, name string) (int, error) {
		j := closing(s, i+len(delim), delim)
		if j < 0 {
			return 0, errAt(at, "%q without its closing %q (escape it as \\%s)", delim, delim, delim[:1])
		}
		if j == i+len(delim) {
			return 0, errAt(at, "empty %s", name)
		}
		if p.lenient() && (s[i+len(delim)] == ' ' || s[j-1] == ' ') {
			return 0, errAt(at, "%q next to a space", delim) // as in Markdown: 2 * 3 * 4 stays text
		}
		flush()
		inner, err := p.spans(s[i+len(delim):j], withMark(marks, mark{Type: name}), at)
		if err != nil {
			return 0, err
		}
		out = append(out, inner...)
		return j + len(delim), nil
	}
	// literal keeps what a lenient parse does not read as markup: the
	// first n bytes at i stay text.
	literal := func(i, n int, err error) (int, error) {
		if !p.lenient() {
			return 0, err
		}
		buf.WriteString(s[i : i+n])
		return i + n, nil
	}
	for i := 0; i < len(s); {
		c := s[i]
		var err error
		switch {
		case c == '\\' && i+1 < len(s) && isPunct(s[i+1]):
			buf.WriteByte(s[i+1])
			i += 2
		case c == '\n':
			flush()
			out = append(out, &node{Type: "hard_break"})
			i++
		case c == '`':
			n := runLen(s, i, '`')
			end := strings.Index(s[i+n:], strings.Repeat("`", n))
			if end < 0 {
				i, err = literal(i, n, errAt(at, "inline code without its closing backtick"))
				break
			}
			code := s[i+n : i+n+end]
			if len(code) > 1 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.Trim(code, " ") != "" {
				code = code[1 : len(code)-1]
			}
			if code == "" {
				i, err = literal(i, 2*n+end, errAt(at, "empty inline code"))
				break
			}
			flush()
			out = append(out, &node{Type: "text", Marks: withMark(marks, mark{Type: "code"}), Text: code})
			i += n + end + n
		case strings.HasPrefix(s[i:], "**"):
			if next, e := emph(i, "**", "strong"); e != nil {
				i, err = literal(i, 2, e)
			} else {
				i = next
			}
		case strings.HasPrefix(s[i:], "~~"):
			if next, e := emph(i, "~~", "strike"); e != nil {
				i, err = literal(i, 2, e)
			} else {
				i = next
			}
		case c == '*':
			if next, e := emph(i, "*", "em"); e != nil {
				i, err = literal(i, 1, e)
			} else {
				i = next
			}
		case c == '_' && (i == 0 || !isWordByte(s[i-1])) && i+1 < len(s) && s[i+1] != ' ':
			if next, e := emph(i, "_", "em"); e != nil {
				i, err = literal(i, 1, e)
			} else {
				i = next
			}
		case strings.HasPrefix(s[i:], "[["):
			end := strings.Index(s[i+2:], "]]")
			if end < 0 {
				i, err = literal(i, 2, errAt(at, "[[ without its closing ]]"))
				break
			}
			ref := s[i+2 : i+2+end]
			n, e := p.reference(ref, at)
			if e != nil {
				i, err = literal(i, end+4, e)
				break
			}
			flush()
			n.Marks = marks
			out = append(out, n)
			i += end + 4
		case c == '[':
			n, next, e := p.link(s, i, marks, at)
			if e != nil {
				i, err = literal(i, 1, e)
				break
			}
			flush()
			out = append(out, n...)
			i = next
		case c == '@' && (i == 0 || !isWordByte(s[i-1])) && mentionRe.MatchString(s[i:]):
			id := mentionRe.FindStringSubmatch(s[i:])[1]
			if end := i + 1 + len(id); end < len(s) && isWordByte(s[end]) {
				i, err = literal(i, 1, errAt(at, "mention @%s runs into the next word", id))
				break
			}
			label := id
			if p.resolve != nil {
				var ok bool
				if label, ok = p.resolve.Person(id); !ok {
					i, err = literal(i, 1, errAt(at, "unknown person @%s", id))
					break
				}
			}
			flush()
			out = append(out, &node{Type: "mention", Attrs: attrs{{"personId", id}, {"label", label}}, Marks: marks})
			i += 1 + len(id)
		case c == '!' && strings.HasPrefix(s[i:], "!["):
			i, err = literal(i, 1, errAt(at, "an image is a line of its own"))
		default:
			buf.WriteByte(c)
			i++
		}
		if err != nil {
			return nil, err
		}
	}
	flush()
	return out, nil
}

func (p *parser) reference(ref string, at line) (*node, error) {
	if title, ok := strings.CutPrefix(ref, "page:"); ok {
		title = strings.TrimSpace(title)
		id := title
		if p.resolve != nil {
			if id, ok = p.resolve.Page(title); !ok {
				return nil, errAt(at, "unknown page %q", title)
			}
		}
		return &node{Type: "page_link", Attrs: attrs{{"pageId", id}}}, nil
	}
	if p.lenient() && issueKeyCI.MatchString(ref) {
		ref = strings.ToUpper(ref)
	}
	if !issueKeyRe.MatchString(ref) {
		return nil, errAt(at, "[[%s]] is neither an issue key nor page:Title", ref)
	}
	if p.resolve != nil && !p.resolve.Issue(ref) {
		return nil, errAt(at, "unknown issue %s", ref)
	}
	return &node{Type: "issue_chip", Attrs: attrs{{"key", ref}}}, nil
}

func (p *parser) link(s string, i int, marks []mark, at line) ([]*node, int, error) {
	if hasMark(marks, "link") {
		return nil, 0, errAt(at, "a link inside a link")
	}
	depth := 0
	textEnd := -1
	for k := i + 1; k < len(s) && textEnd < 0; k++ {
		switch s[k] {
		case '\\':
			k++
		case '[':
			depth++
		case ']':
			if depth == 0 {
				textEnd = k
			}
			depth--
		}
	}
	if textEnd < 0 || textEnd+1 >= len(s) || s[textEnd+1] != '(' {
		return nil, 0, errAt(at, "a [ that starts no link (escape it as \\[)")
	}
	hrefEnd := strings.IndexByte(s[textEnd+2:], ')')
	if hrefEnd < 0 {
		return nil, 0, errAt(at, "a link without its closing )")
	}
	href := s[textEnd+2 : textEnd+2+hrefEnd]
	u, err := url.Parse(href)
	if err != nil || !slices.Contains(linkSchemes, u.Scheme) || (u.Scheme != "mailto" && u.Host == "") {
		return nil, 0, errAt(at, "link %q is not an http, https or mailto URL", href)
	}
	if len(href) > maxAttrLength {
		return nil, 0, errAt(at, "a link longer than %d bytes", maxAttrLength)
	}
	text := s[i+1 : textEnd]
	if strings.TrimSpace(text) == "" {
		return nil, 0, errAt(at, "a link without text")
	}
	inner, err := p.spans(text, withMark(marks, mark{Type: "link", Attrs: attrs{{"href", href}}}), at)
	if err != nil {
		return nil, 0, err
	}
	return inner, textEnd + 2 + hrefEnd + 1, nil
}

// merge joins neighbouring text nodes with the same marks, as ProseMirror
// normalises them, and checks the length of each.
func merge(nodes []*node, at line) ([]*node, error) {
	var out []*node
	for _, n := range nodes {
		if k := len(out) - 1; k >= 0 && n.Type == "text" && out[k].Type == "text" && sameMarks(out[k].Marks, n.Marks) {
			out[k] = &node{Type: "text", Marks: n.Marks, Text: out[k].Text + n.Text}
			continue
		}
		out = append(out, n)
	}
	for _, n := range out {
		if n.Type == "text" && utf8.RuneCountInString(n.Text) > maxTextLength {
			return nil, errAt(at, "a text longer than %d characters", maxTextLength)
		}
	}
	return out, nil
}

func sameMarks(a, b []mark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type {
			return false
		}
		if a[i].Type == "link" && a[i].Attrs[0].value != b[i].Attrs[0].value {
			return false
		}
	}
	return true
}

func countBlocks(n *node) int {
	c := 0
	switch n.Type {
	case "doc", "text", "hard_break", "issue_chip", "mention", "page_link":
	default:
		c = 1
	}
	for _, k := range n.Content {
		c += countBlocks(k)
	}
	return c
}
