package doc

import "strings"

// Fragment is the sequence of a node's children. Like nodes, fragments are
// immutable values; the zero Fragment is empty.
type Fragment struct {
	nodes []*Node
	size  int
}

func newFragment(nodes []*Node) Fragment {
	size := 0
	for _, n := range nodes {
		size += n.size
	}
	return Fragment{nodes: nodes, size: size}
}

// FragmentOf builds a fragment from nodes, joining adjacent text nodes with
// the same marks as prosemirror-model's Fragment.fromArray does.
func FragmentOf(nodes ...*Node) Fragment { return fragmentFromArray(nodes) }

func fragmentFromArray(array []*Node) Fragment {
	if len(array) == 0 {
		return Fragment{}
	}
	var joined []*Node
	size := 0
	for i, n := range array {
		size += n.size
		if i > 0 && n.typ.isText && array[i-1].sameMarkup(n) {
			if joined == nil {
				joined = append([]*Node(nil), array[:i]...)
			}
			last := joined[len(joined)-1]
			joined[len(joined)-1] = n.withText(last.text+n.text, last.size+n.size)
		} else if joined != nil {
			joined = append(joined, n)
		}
	}
	if joined == nil {
		joined = array
	}
	return Fragment{nodes: joined, size: size}
}

// Size is the fragment's size in positions.
func (f Fragment) Size() int { return f.size }

// ChildCount is the number of children.
func (f Fragment) ChildCount() int { return len(f.nodes) }

// Child returns the i-th child.
func (f Fragment) Child(i int) *Node { return f.nodes[i] }

// Nodes returns the children; the slice must not be changed.
func (f Fragment) Nodes() []*Node { return f.nodes }

func (f Fragment) firstChild() *Node {
	if len(f.nodes) == 0 {
		return nil
	}
	return f.nodes[0]
}

func (f Fragment) lastChild() *Node {
	if len(f.nodes) == 0 {
		return nil
	}
	return f.nodes[len(f.nodes)-1]
}

func (f Fragment) maybeChild(i int) *Node {
	if i < 0 || i >= len(f.nodes) {
		return nil
	}
	return f.nodes[i]
}

func (f Fragment) child(i int) *Node {
	if i < 0 || i >= len(f.nodes) {
		panic(&ModelError{Msg: "Index out of range for fragment"})
	}
	return f.nodes[i]
}

// same reports whether two fragments are the same value (identity, as
// prosemirror-model compares fragments in Node.copy).
func (f Fragment) same(o Fragment) bool {
	if len(f.nodes) != len(o.nodes) {
		return false
	}
	return len(f.nodes) == 0 || &f.nodes[0] == &o.nodes[0]
}

// NodesBetween calls fn for every node overlapping from to to (relative to
// the fragment's start), descending unless fn returns false.
func (f Fragment) NodesBetween(from, to int, fn func(n *Node, pos int, parent *Node, index int) bool, nodeStart int, parent *Node) {
	pos := 0
	for i := 0; pos < to && i < len(f.nodes); i++ {
		child := f.nodes[i]
		end := pos + child.size
		if end > from && fn(child, nodeStart+pos, parent, i) && child.content.size > 0 {
			start := pos + 1
			child.content.NodesBetween(max(0, from-start), min(child.content.size, to-start), fn, nodeStart+start, child)
		}
		pos = end
	}
}

func (f Fragment) append(o Fragment) Fragment {
	if o.size == 0 {
		return f
	}
	if f.size == 0 {
		return o
	}
	last, first := f.lastChild(), o.firstChild()
	content := make([]*Node, len(f.nodes), len(f.nodes)+len(o.nodes))
	copy(content, f.nodes)
	i := 0
	if last.typ.isText && last.sameMarkup(first) {
		content[len(content)-1] = last.withText(last.text+first.text, last.size+first.size)
		i = 1
	}
	content = append(content, o.nodes[i:]...)
	return Fragment{nodes: content, size: f.size + o.size}
}

func (f Fragment) cut(from, to int) Fragment {
	if from == 0 && to == f.size {
		return f
	}
	var result []*Node
	size := 0
	if to > from {
		pos := 0
		for i := 0; pos < to; i++ {
			child := f.nodes[i]
			end := pos + child.size
			if end > from {
				if pos < from || end > to {
					if child.typ.isText {
						child = child.cutText(max(0, from-pos), min(child.size, to-pos))
					} else {
						child = child.cut(max(0, from-pos-1), min(child.content.size, to-pos-1))
					}
				}
				result = append(result, child)
				size += child.size
			}
			pos = end
		}
	}
	return Fragment{nodes: result, size: size}
}

func (f Fragment) cutByIndex(from, to int) Fragment {
	if from == to {
		return Fragment{}
	}
	if from == 0 && to == len(f.nodes) {
		return f
	}
	return newFragment(f.nodes[from:to:to])
}

func (f Fragment) replaceChild(index int, n *Node) Fragment {
	cur := f.nodes[index]
	if cur == n {
		return f
	}
	cp := make([]*Node, len(f.nodes))
	copy(cp, f.nodes)
	cp[index] = n
	return Fragment{nodes: cp, size: f.size + n.size - cur.size}
}

func (f Fragment) addToStart(n *Node) Fragment {
	cp := make([]*Node, 0, len(f.nodes)+1)
	cp = append(cp, n)
	return Fragment{nodes: append(cp, f.nodes...), size: f.size + n.size}
}

func (f Fragment) addToEnd(n *Node) Fragment {
	cp := make([]*Node, len(f.nodes), len(f.nodes)+1)
	copy(cp, f.nodes)
	return Fragment{nodes: append(cp, n), size: f.size + n.size}
}

// Eq reports whether two fragments hold equal nodes.
func (f Fragment) Eq(o Fragment) bool {
	if len(f.nodes) != len(o.nodes) {
		return false
	}
	for i := range f.nodes {
		if !f.nodes[i].Eq(o.nodes[i]) {
			return false
		}
	}
	return true
}

// findIndex returns the index of the child at pos and its offset.
func (f Fragment) findIndex(pos int) (index, offset int) {
	if pos == 0 {
		return 0, 0
	}
	if pos == f.size {
		return len(f.nodes), pos
	}
	if pos > f.size || pos < 0 {
		panic(modelErrorf("Position %d outside of fragment (%s)", pos, f.String()))
	}
	cur := 0
	for i := 0; ; i++ {
		end := cur + f.nodes[i].size
		if end >= pos {
			if end == pos {
				return i + 1, end
			}
			return i, cur
		}
		cur = end
	}
}

// String describes the fragment as prosemirror-model's toString does.
func (f Fragment) String() string {
	var b strings.Builder
	b.WriteByte('<')
	f.writeInner(&b)
	b.WriteByte('>')
	return b.String()
}

func (f Fragment) writeInner(b *strings.Builder) {
	for i, n := range f.nodes {
		if i > 0 {
			b.WriteString(", ")
		}
		n.writeString(b)
	}
}

func (f Fragment) appendJSON(b []byte) []byte {
	b = append(b, '[')
	for i, n := range f.nodes {
		if i > 0 {
			b = append(b, ',')
		}
		b = n.appendJSON(b)
	}
	return append(b, ']')
}
