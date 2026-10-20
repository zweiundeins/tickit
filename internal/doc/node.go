package doc

import (
	"math"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

// Node is a node of a document: immutable, so one node may be shared by
// many documents and read from many goroutines. Unchanged parts of a
// document keep their nodes across steps.
type Node struct {
	typ     *NodeType
	attrs   []any
	content Fragment
	marks   []*Mark
	text    string // text nodes: UTF-8
	size    int    // nodeSize: UTF-16 units for text

	blocks  atomic.Int32 // cached count of block nodes inside, plus one
	jsonLen atomic.Int64 // cached length of the node's JSON, plus one
}

// Doc is a document: a node of the schema's top type.
type Doc = *Node

func newNode(t *NodeType, attrs []any, content Fragment, marks []*Mark) *Node {
	n := &Node{typ: t, attrs: attrs, content: content, marks: marks}
	if t.contentMatch == emptyMatch {
		n.size = 1
	} else {
		n.size = content.size + 2
	}
	return n
}

func newText(t *NodeType, text string, units int, marks []*Mark) *Node {
	return &Node{typ: t, attrs: t.defaultAttrs, text: text, size: units, marks: marks}
}

// Type returns the node's type.
func (n *Node) Type() *NodeType { return n.typ }

// Text returns a text node's text.
func (n *Node) Text() string { return n.text }

// Marks returns the node's marks; the slice must not be changed.
func (n *Node) Marks() []*Mark { return n.marks }

// Content returns the node's children.
func (n *Node) Content() Fragment { return n.content }

// ChildCount is the number of children.
func (n *Node) ChildCount() int { return len(n.content.nodes) }

// Child returns the i-th child.
func (n *Node) Child(i int) *Node { return n.content.nodes[i] }

// NodeSize is the node's size in positions: UTF-16 units for text, 1 for
// other leaves, the content's size plus 2 for the rest.
func (n *Node) NodeSize() int { return n.size }

// IsText reports whether this is a text node.
func (n *Node) IsText() bool { return n.typ.isText }

// IsBlock reports whether this is a block node.
func (n *Node) IsBlock() bool { return n.typ.isBlock }

// IsInline reports whether this is an inline node.
func (n *Node) IsInline() bool { return !n.typ.isBlock }

// IsTextblock reports whether this is a block with inline content.
func (n *Node) IsTextblock() bool { return n.typ.IsTextblock() }

// IsLeaf reports whether the node allows no content.
func (n *Node) IsLeaf() bool { return n.typ.contentMatch == emptyMatch }

// IsAtom reports whether the node is a leaf or an atom.
func (n *Node) IsAtom() bool { return n.typ.IsAtom() }

// Attr returns the attribute called name, or Undefined.
func (n *Node) Attr(name string) any {
	if i := n.typ.attrIndex(name); i >= 0 {
		return n.attrs[i]
	}
	return Undefined
}

// AttrString returns a string attribute, or "".
func (n *Node) AttrString(name string) string { s, _ := n.Attr(name).(string); return s }

// AttrInt returns an integer attribute, and false when it is not one.
func (n *Node) AttrInt(name string) (int, bool) {
	f, ok := n.Attr(name).(float64)
	if !ok || f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		return 0, false
	}
	return int(f), true
}

// AttrBool returns a boolean attribute, or false.
func (n *Node) AttrBool(name string) bool { b, _ := n.Attr(name).(bool); return b }

// Eq reports whether two nodes are the same piece of document.
func (n *Node) Eq(o *Node) bool {
	if n == o {
		return true
	}
	if !n.sameMarkup(o) {
		return false
	}
	if n.typ.isText {
		return n.text == o.text
	}
	return n.content.Eq(o.content)
}

func (n *Node) sameMarkup(o *Node) bool { return n.hasMarkup(o.typ, o.attrs, o.marks) }

func (n *Node) hasMarkup(t *NodeType, attrs []any, marks []*Mark) bool {
	if n.typ != t {
		return false
	}
	if attrs == nil {
		attrs = t.defaultAttrs
	}
	if len(n.attrs) != len(attrs) {
		return false
	}
	for i := range attrs {
		if !compareDeep(n.attrs[i], attrs[i]) {
			return false
		}
	}
	return sameMarkSet(n.marks, marks)
}

func (n *Node) copy(content Fragment) *Node {
	if content.same(n.content) {
		return n
	}
	return newNode(n.typ, n.attrs, content, n.marks)
}

func (n *Node) mark(marks []*Mark) *Node {
	if sameSlice(marks, n.marks) {
		return n
	}
	if n.typ.isText {
		return newText(n.typ, n.text, n.size, marks)
	}
	return newNode(n.typ, n.attrs, n.content, marks)
}

func sameSlice(a, b []*Mark) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func (n *Node) withText(text string, units int) *Node {
	if text == n.text {
		return n
	}
	return newText(n.typ, text, units, n.marks)
}

func (n *Node) cutText(from, to int) *Node {
	if from == 0 && to == n.size {
		return n
	}
	a := byteOffset(n.text, n.size, from)
	b := byteOffset(n.text, n.size, to)
	return n.withText(n.text[a:b], to-from)
}

func (n *Node) cut(from, to int) *Node {
	if n.typ.isText {
		return n.cutText(from, to)
	}
	if from == 0 && to == n.content.size {
		return n
	}
	return n.copy(n.content.cut(from, to))
}

// NodesBetween calls fn for every descendant overlapping from to to.
func (n *Node) NodesBetween(from, to int, fn func(n *Node, pos int, parent *Node, index int) bool) {
	n.content.NodesBetween(from, to, fn, 0, n)
}

// Descendants calls fn for every descendant, descending unless it returns false.
func (n *Node) Descendants(fn func(n *Node, pos int, parent *Node, index int) bool) {
	n.NodesBetween(0, n.content.size, fn)
}

// nodeAt returns the node after pos, or nil.
func (n *Node) nodeAt(pos int) *Node {
	node := n
	for {
		index, offset := node.content.findIndex(pos)
		node = node.content.maybeChild(index)
		if node == nil {
			return nil
		}
		if offset == pos || node.typ.isText {
			return node
		}
		pos -= offset + 1
	}
}

// NodeAt returns the node directly after pos, or nil.
func (n *Node) NodeAt(pos int) (node *Node, err error) {
	defer recoverModel(&err)
	return n.nodeAt(pos), nil
}

func (n *Node) contentMatchAt(index int) *ContentMatch {
	m := n.typ.contentMatch.matchFragment(n.content, 0, index)
	if m == nil {
		panic(&ModelError{Msg: "Called contentMatchAt on a node with invalid content"})
	}
	return m
}

func (n *Node) canReplace(from, to int, repl Fragment, start, end int) bool {
	one := n.contentMatchAt(from).matchFragment(repl, start, end)
	var two *ContentMatch
	if one != nil {
		two = one.matchFragment(n.content, to, len(n.content.nodes))
	}
	if two == nil || !two.validEnd {
		return false
	}
	for i := start; i < end; i++ {
		if !n.typ.allowsMarks(repl.nodes[i].marks) {
			return false
		}
	}
	return true
}

// Check reports whether the node and its descendants conform to the schema,
// with prosemirror-model's messages.
func (n *Node) Check() (err error) {
	defer recoverModel(&err)
	n.check()
	return nil
}

func (n *Node) check() {
	if err := n.typ.checkContent(n.content); err != nil {
		panic(err)
	}
	for i, a := range n.typ.attrs {
		if err := a.check(n.typ.name, n.attrs[i]); err != nil {
			panic(err)
		}
	}
	var cp []*Mark
	for _, m := range n.marks {
		for i, a := range m.typ.attrs {
			if err := a.check(m.typ.name, m.attrs[i]); err != nil {
				panic(err)
			}
		}
		cp = m.addToSet(cp)
	}
	if !sameMarkSet(cp, n.marks) {
		names := make([]string, len(n.marks))
		for i, m := range n.marks {
			names[i] = m.typ.name
		}
		panic(modelErrorf("Invalid collection of marks for node %s: %s", n.typ.name, strings.Join(names, ",")))
	}
	for _, c := range n.content.nodes {
		c.check()
	}
}

// recoverModel turns a panic with a model error into an error.
func recoverModel(err *error) {
	switch r := recover().(type) {
	case nil:
	case *ModelError:
		*err = r
	case splitPanic:
		*err = ErrSurrogateSplit
	case *replaceError:
		*err = &ModelError{Msg: r.msg}
	default:
		panic(r)
	}
}

// String describes the node as prosemirror-model's toString does.
func (n *Node) String() string {
	var b strings.Builder
	n.writeString(&b)
	return b.String()
}

func (n *Node) writeString(b *strings.Builder) {
	for i := len(n.marks) - 1; i >= 0; i-- {
		b.WriteString(n.marks[i].typ.name)
		b.WriteByte('(')
	}
	if n.typ.isText {
		b.Write(appendString(nil, n.text))
	} else {
		b.WriteString(n.typ.name)
		if n.content.size > 0 {
			b.WriteByte('(')
			n.content.writeInner(b)
			b.WriteByte(')')
		}
	}
	for range n.marks {
		b.WriteByte(')')
	}
}

// appendJSON writes the node as JSON.stringify(node.toJSON()) does.
func (n *Node) appendJSON(b []byte) []byte {
	b = append(b, `{"type":`...)
	b = appendString(b, n.typ.name)
	if len(n.attrs) > 0 {
		b = append(b, `,"attrs":`...)
		b = appendAttrs(b, n.typ.attrs, n.attrs)
	}
	if n.content.size > 0 {
		b = append(b, `,"content":`...)
		b = n.content.appendJSON(b)
	}
	if len(n.marks) > 0 {
		b = append(b, `,"marks":[`...)
		for i, m := range n.marks {
			if i > 0 {
				b = append(b, ',')
			}
			b = m.appendJSON(b)
		}
		b = append(b, ']')
	}
	if n.typ.isText {
		b = append(b, `,"text":`...)
		b = appendString(b, n.text)
	}
	return append(b, '}')
}

// MarshalJSON writes the node as prosemirror-model's toJSON and
// JSON.stringify do, byte for byte.
func (n *Node) MarshalJSON() ([]byte, error) { return n.appendJSON(nil), nil }

// JSONLen is the length of the node's JSON, cached in the node.
func (n *Node) JSONLen() int {
	if v := n.jsonLen.Load(); v > 0 {
		return int(v - 1)
	}
	l := 0
	if n.content.size > 0 && len(n.marks) == 0 {
		// The children's cached lengths, plus the node's own markup.
		head := len(appendAttrsPrefix(nil, n))
		l = head + len(`,"content":[]}`) + len(n.content.nodes) - 1
		for _, c := range n.content.nodes {
			l += c.JSONLen()
		}
	} else {
		l = len(n.appendJSON(nil))
	}
	n.jsonLen.Store(int64(l) + 1)
	return l
}

func appendAttrsPrefix(b []byte, n *Node) []byte {
	b = append(b, `{"type":`...)
	b = appendString(b, n.typ.name)
	if len(n.attrs) > 0 {
		b = append(b, `,"attrs":`...)
		b = appendAttrs(b, n.typ.attrs, n.attrs)
	}
	return b
}

// Blocks counts the block nodes inside the node, cached in the node.
func (n *Node) Blocks() int {
	if v := n.blocks.Load(); v > 0 {
		return int(v - 1)
	}
	c := 0
	if n.typ.inlineContent {
		c = 0
	} else {
		for _, ch := range n.content.nodes {
			if ch.typ.isBlock {
				c++
			}
			c += ch.Blocks()
		}
	}
	n.blocks.Store(int32(c) + 1)
	return c
}

// TextBetween returns the text between from and to, with blockSep between
// blocks and leafText for leaves that are not text.
func (n *Node) TextBetween(from, to int, blockSep string, leafText func(*Node) string) string {
	var b strings.Builder
	first := true
	n.content.NodesBetween(from, to, func(node *Node, pos int, _ *Node, _ int) bool {
		var t string
		switch {
		case node.typ.isText:
			a := max(from, pos) - pos
			e := min(node.size, to-pos)
			t = node.text[byteOffset(node.text, node.size, a):byteOffset(node.text, node.size, e)]
		case !node.IsLeaf():
			t = ""
		case leafText != nil:
			t = leafText(node)
		}
		if node.typ.isBlock && ((node.IsLeaf() && t != "") || node.IsTextblock()) && blockSep != "" {
			if first {
				first = false
			} else {
				b.WriteString(blockSep)
			}
		}
		b.WriteString(t)
		return true
	}, 0, n)
	return b.String()
}

// Create makes a node of the type with prosemirror-model's attribute rules
// (attrs may be nil when every attribute has a default). The content is
// not checked; Check does that.
func (t *NodeType) Create(attrs map[string]any, content []*Node, marks ...*Mark) (*Node, error) {
	if t.isText {
		return nil, &ModelError{Msg: "NodeType.create can't construct text nodes"}
	}
	var v any
	if attrs != nil {
		o := NewObject()
		for _, a := range t.attrs {
			if x, ok := attrs[a.name]; ok {
				o.Set(a.name, x)
			}
		}
		v = o
	}
	a, err := t.computeAttrs(v)
	if err != nil {
		return nil, err
	}
	return newNode(t, a, fragmentFromArray(content), setFrom(marks)), nil
}

func (t *NodeType) computeAttrs(value any) ([]any, error) {
	return computeAttrs(t.name, t.attrs, t.defaultAttrs, value)
}

// Text makes a text node. The text must be valid UTF-8 and not empty.
func (s *Schema) Text(text string, marks ...*Mark) (*Node, error) {
	if text == "" {
		return nil, &ModelError{Msg: "Empty text nodes are not allowed"}
	}
	if !utf8.ValidString(text) {
		return nil, ErrLoneSurrogate
	}
	return newText(s.text, text, utf16Len(text), setFrom(marks)), nil
}
