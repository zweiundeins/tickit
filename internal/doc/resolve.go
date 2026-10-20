package doc

// ResolvedPos is a position in a document with its context, as
// prosemirror-model's ResolvedPos.
type ResolvedPos struct {
	Pos          int
	ParentOffset int
	Depth        int
	path         []pathEntry
}

type pathEntry struct {
	node   *Node
	index  int
	offset int // absolute position of the child at index
}

func resolve(doc *Node, pos int) *ResolvedPos {
	if !(pos >= 0 && pos <= doc.content.size) {
		panic(modelErrorf("Position %d out of range", pos))
	}
	path := make([]pathEntry, 0, 6)
	start, parentOffset := 0, pos
	for node := doc; ; {
		index, offset := node.content.findIndex(parentOffset)
		rem := parentOffset - offset
		path = append(path, pathEntry{node, index, start + offset})
		if rem == 0 {
			break
		}
		node = node.content.child(index)
		if node.typ.isText {
			break
		}
		parentOffset = rem - 1
		start += offset + 1
	}
	return &ResolvedPos{Pos: pos, ParentOffset: parentOffset, Depth: len(path) - 1, path: path}
}

// Resolve resolves pos in the document.
func (n *Node) Resolve(pos int) (r *ResolvedPos, err error) {
	defer recoverModel(&err)
	return resolve(n, pos), nil
}

func (r *ResolvedPos) depth(d int) int {
	if d < 0 {
		return r.Depth + d
	}
	return d
}

// Node returns the ancestor at depth d.
func (r *ResolvedPos) Node(d int) *Node { return r.path[r.depth(d)].node }

// Parent returns the innermost node holding the position.
func (r *ResolvedPos) Parent() *Node { return r.path[r.Depth].node }

// Index returns the child index in the ancestor at depth d.
func (r *ResolvedPos) Index(d int) int { return r.path[r.depth(d)].index }

func (r *ResolvedPos) indexAfter(d int) int {
	d = r.depth(d)
	if d == r.Depth && r.textOffset() == 0 {
		return r.Index(d)
	}
	return r.Index(d) + 1
}

// Start returns the position at the start of the ancestor at depth d.
func (r *ResolvedPos) Start(d int) int {
	d = r.depth(d)
	if d == 0 {
		return 0
	}
	return r.path[d-1].offset + 1
}

// End returns the position at the end of the ancestor at depth d.
func (r *ResolvedPos) End(d int) int {
	d = r.depth(d)
	return r.Start(d) + r.path[d].node.content.size
}

// Before returns the position before the ancestor at depth d.
func (r *ResolvedPos) Before(d int) int {
	d = r.depth(d)
	if d == 0 {
		panic(&ModelError{Msg: "There is no position before the top-level node"})
	}
	if d == r.Depth+1 {
		return r.Pos
	}
	return r.path[d-1].offset
}

// After returns the position after the ancestor at depth d.
func (r *ResolvedPos) After(d int) int {
	d = r.depth(d)
	if d == 0 {
		panic(&ModelError{Msg: "There is no position after the top-level node"})
	}
	if d == r.Depth+1 {
		return r.Pos
	}
	return r.path[d-1].offset + r.path[d].node.size
}

func (r *ResolvedPos) textOffset() int { return r.Pos - r.path[len(r.path)-1].offset }

// TextOffset is the offset into a text node when the position is inside one.
func (r *ResolvedPos) TextOffset() int { return r.textOffset() }

// NodeAfter returns the node after the position, or nil.
func (r *ResolvedPos) NodeAfter() *Node {
	parent, index := r.Parent(), r.Index(r.Depth)
	if index == len(parent.content.nodes) {
		return nil
	}
	dOff, child := r.textOffset(), parent.content.nodes[index]
	if dOff > 0 {
		return child.cut(dOff, child.contentSizeOrText())
	}
	return child
}

// NodeBefore returns the node before the position, or nil.
func (r *ResolvedPos) NodeBefore() *Node {
	index := r.Index(r.Depth)
	dOff := r.textOffset()
	if dOff > 0 {
		return r.Parent().content.nodes[index].cut(0, dOff)
	}
	if index == 0 {
		return nil
	}
	return r.Parent().content.nodes[index-1]
}

func (n *Node) contentSizeOrText() int {
	if n.typ.isText {
		return n.size
	}
	return n.content.size
}

// Marks returns the marks at the position as prosemirror-model does
// (non-inclusive marks end at their edge).
func (r *ResolvedPos) Marks() []*Mark {
	parent, index := r.Parent(), r.Index(r.Depth)
	if parent.content.size == 0 {
		return nil
	}
	if r.textOffset() > 0 {
		return parent.content.nodes[index].marks
	}
	main, other := parent.content.maybeChild(index-1), parent.content.maybeChild(index)
	if main == nil {
		main, other = other, main
	}
	marks := main.marks
	for i := 0; i < len(marks); i++ {
		if !marks[i].typ.inclusive && (other == nil || !marks[i].isInSet(other.marks)) {
			marks = marks[i].removeFromSet(marks)
			i--
		}
	}
	return marks
}

func (r *ResolvedPos) sharedDepth(pos int) int {
	for d := r.Depth; d > 0; d-- {
		if r.Start(d) <= pos && r.End(d) >= pos {
			return d
		}
	}
	return 0
}
