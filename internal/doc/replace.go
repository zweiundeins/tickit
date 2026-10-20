package doc

// Slice is a piece of a document with the depth of its open sides, as
// prosemirror-model's Slice.
type Slice struct {
	Content   Fragment
	OpenStart int
	OpenEnd   int
}

// Size is the slice's size in positions.
func (s Slice) Size() int { return s.Content.size - s.OpenStart - s.OpenEnd }

// Eq reports whether two slices are equal.
func (s Slice) Eq(o Slice) bool {
	return s.Content.Eq(o.Content) && s.OpenStart == o.OpenStart && s.OpenEnd == o.OpenEnd
}

func (s Slice) appendJSON(b []byte) []byte {
	b = append(b, `{"content":`...)
	b = s.Content.appendJSON(b)
	if s.OpenStart > 0 {
		b = append(b, `,"openStart":`...)
		b = appendNumber(b, float64(s.OpenStart))
	}
	if s.OpenEnd > 0 {
		b = append(b, `,"openEnd":`...)
		b = appendNumber(b, float64(s.OpenEnd))
	}
	return append(b, '}')
}

// maxOpen returns the deepest the fragment can be open at its start and end.
func maxOpen(f Fragment) (int, int) {
	start, end := 0, 0
	for n := f.firstChild(); n != nil && !n.IsLeaf(); n = n.content.firstChild() {
		start++
	}
	for n := f.lastChild(); n != nil && !n.IsLeaf(); n = n.content.lastChild() {
		end++
	}
	return start, end
}

func (s Slice) insertAt(pos int, f Fragment) (Slice, bool) {
	content, ok := insertInto(s.Content, pos+s.OpenStart, f, s.OpenStart+1, s.OpenEnd+1, nil)
	if !ok {
		return Slice{}, false
	}
	return Slice{content, s.OpenStart, s.OpenEnd}, true
}

func insertInto(content Fragment, dist int, insert Fragment, openStart, openEnd int, parent *Node) (Fragment, bool) {
	index, offset := content.findIndex(dist)
	child := content.maybeChild(index)
	if offset == dist || child.typ.isText {
		if parent != nil && openStart <= 0 && openEnd <= 0 && !parent.canReplace(index, index, insert, 0, len(insert.nodes)) {
			return Fragment{}, false
		}
		return content.cut(0, dist).append(insert).append(content.cut(dist, content.size)), true
	}
	os, oe := 0, 0
	if index == 0 {
		os = openStart - 1
	}
	if index == len(content.nodes)-1 {
		oe = openEnd - 1
	}
	inner, ok := insertInto(child.content, dist-offset-1, insert, os, oe, child)
	if !ok {
		return Fragment{}, false
	}
	return content.replaceChild(index, child.copy(inner)), true
}

// replaceError is prosemirror-model's ReplaceError: a step that raises it
// fails, and the document stays as it was.
type replaceError struct{ msg string }

func (n *Node) slice(from, to int) Slice {
	if from == to {
		return Slice{}
	}
	rf, rt := resolve(n, from), resolve(n, to)
	depth := rf.sharedDepth(to)
	start, node := rf.Start(depth), rf.Node(depth)
	content := node.content.cut(rf.Pos-start, rt.Pos-start)
	return Slice{content, rf.Depth - depth, rt.Depth - depth}
}

// Slice cuts out the part of the document between from and to.
func (n *Node) Slice(from, to int) (s Slice, err error) {
	defer recoverModel(&err)
	return n.slice(from, to), nil
}

func (n *Node) replace(from, to int, s Slice) *Node {
	return replaceResolved(resolve(n, from), resolve(n, to), s)
}

func replaceResolved(rf, rt *ResolvedPos, s Slice) *Node {
	if s.OpenStart > rf.Depth {
		panic(&replaceError{"Inserted content deeper than insertion position"})
	}
	if rf.Depth-s.OpenStart != rt.Depth-s.OpenEnd {
		panic(&replaceError{"Inconsistent open depths"})
	}
	return replaceOuter(rf, rt, s, 0)
}

func replaceOuter(rf, rt *ResolvedPos, s Slice, depth int) *Node {
	index, node := rf.Index(depth), rf.Node(depth)
	switch {
	case index == rt.Index(depth) && depth < rf.Depth-s.OpenStart:
		inner := replaceOuter(rf, rt, s, depth+1)
		return node.copy(node.content.replaceChild(index, inner))
	case s.Content.size == 0:
		return closeNode(node, replaceTwoWay(rf, rt, depth))
	case s.OpenStart == 0 && s.OpenEnd == 0 && rf.Depth == depth && rt.Depth == depth:
		parent := rf.Parent()
		c := parent.content
		return closeNode(parent, c.cut(0, rf.ParentOffset).append(s.Content).append(c.cut(rt.ParentOffset, c.size)))
	default:
		start, end := prepareSliceForReplace(s, rf)
		return closeNode(node, replaceThreeWay(rf, start, end, rt, depth))
	}
}

func checkJoin(main, sub *Node) {
	if !sub.typ.compatibleContent(main.typ) {
		panic(&replaceError{"Cannot join " + sub.typ.name + " onto " + main.typ.name})
	}
}

func joinable(before, after *ResolvedPos, depth int) *Node {
	node := before.Node(depth)
	checkJoin(node, after.Node(depth))
	return node
}

func addNode(child *Node, target []*Node) []*Node {
	last := len(target) - 1
	if last >= 0 && child.typ.isText && child.sameMarkup(target[last]) {
		target[last] = child.withText(target[last].text+child.text, target[last].size+child.size)
		return target
	}
	return append(target, child)
}

func addRange(start, end *ResolvedPos, depth int, target []*Node) []*Node {
	r := end
	if r == nil {
		r = start
	}
	node := r.Node(depth)
	startIndex, endIndex := 0, len(node.content.nodes)
	if end != nil {
		endIndex = end.Index(depth)
	}
	if start != nil {
		startIndex = start.Index(depth)
		if start.Depth > depth {
			startIndex++
		} else if start.textOffset() > 0 {
			target = addNode(start.NodeAfter(), target)
			startIndex++
		}
	}
	for i := startIndex; i < endIndex; i++ {
		target = addNode(node.content.nodes[i], target)
	}
	if end != nil && end.Depth == depth && end.textOffset() > 0 {
		target = addNode(end.NodeBefore(), target)
	}
	return target
}

func closeNode(node *Node, content Fragment) *Node {
	if !node.typ.validContent(content) {
		panic(&replaceError{"Invalid content for node " + node.typ.name})
	}
	return node.copy(content)
}

func replaceThreeWay(rf, start, end, rt *ResolvedPos, depth int) Fragment {
	var openStart, openEnd *Node
	if rf.Depth > depth {
		openStart = joinable(rf, start, depth+1)
	}
	if rt.Depth > depth {
		openEnd = joinable(end, rt, depth+1)
	}
	var content []*Node
	content = addRange(nil, rf, depth, content)
	if openStart != nil && openEnd != nil && start.Index(depth) == end.Index(depth) {
		checkJoin(openStart, openEnd)
		content = addNode(closeNode(openStart, replaceThreeWay(rf, start, end, rt, depth+1)), content)
	} else {
		if openStart != nil {
			content = addNode(closeNode(openStart, replaceTwoWay(rf, start, depth+1)), content)
		}
		content = addRange(start, end, depth, content)
		if openEnd != nil {
			content = addNode(closeNode(openEnd, replaceTwoWay(end, rt, depth+1)), content)
		}
	}
	content = addRange(rt, nil, depth, content)
	return newFragment(content)
}

func replaceTwoWay(rf, rt *ResolvedPos, depth int) Fragment {
	var content []*Node
	content = addRange(nil, rf, depth, content)
	if rf.Depth > depth {
		t := joinable(rf, rt, depth+1)
		content = addNode(closeNode(t, replaceTwoWay(rf, rt, depth+1)), content)
	}
	content = addRange(rt, nil, depth, content)
	return newFragment(content)
}

func prepareSliceForReplace(s Slice, along *ResolvedPos) (*ResolvedPos, *ResolvedPos) {
	extra := along.Depth - s.OpenStart
	parent := along.Node(extra)
	node := parent.copy(s.Content)
	for i := extra - 1; i >= 0; i-- {
		node = along.Node(i).copy(Fragment{nodes: []*Node{node}, size: node.size})
	}
	return resolve(node, s.OpenStart+extra), resolve(node, node.content.size-s.OpenEnd-extra)
}
