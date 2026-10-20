package doc

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// FindDiffStart returns the first position at which two fragments differ,
// counted from pos, as prosemirror-model's findDiffStart; false when they
// are equal.
func FindDiffStart(a, b Fragment, pos int) (int, bool) {
	for i := 0; ; i++ {
		if i == len(a.nodes) || i == len(b.nodes) {
			return pos, len(a.nodes) != len(b.nodes)
		}
		ca, cb := a.nodes[i], b.nodes[i]
		if ca == cb {
			pos += ca.size
			continue
		}
		if !ca.sameMarkup(cb) {
			return pos, true
		}
		if ca.typ.isText && ca.text != cb.text {
			ta, tb := ca.text, cb.text
			for len(ta) > 0 && len(tb) > 0 {
				ra, na := utf8.DecodeRuneInString(ta)
				rb, nb := utf8.DecodeRuneInString(tb)
				if ra != rb {
					break
				}
				pos += runeUnits(ra)
				ta, tb = ta[na:], tb[nb:]
			}
			return pos, true
		}
		if ca.content.size > 0 || cb.content.size > 0 {
			if inner, ok := FindDiffStart(ca.content, cb.content, pos+1); ok {
				return inner, true
			}
		}
		pos += ca.size
	}
}

func runeUnits(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// FindDiffEnd returns the first positions, searching from the ends, at
// which two fragments differ, as prosemirror-model's findDiffEnd; false
// when they are equal.
func FindDiffEnd(a, b Fragment, posA, posB int) (int, int, bool) {
	ia, ib := len(a.nodes), len(b.nodes)
	for {
		if ia == 0 || ib == 0 {
			return posA, posB, ia != ib
		}
		ia--
		ib--
		ca, cb := a.nodes[ia], b.nodes[ib]
		size := ca.size
		if ca == cb {
			posA -= size
			posB -= size
			continue
		}
		if !ca.sameMarkup(cb) {
			return posA, posB, true
		}
		if ca.typ.isText && ca.text != cb.text {
			ta, tb := ca.text, cb.text
			for len(ta) > 0 && len(tb) > 0 {
				ra, na := utf8.DecodeLastRuneInString(ta)
				rb, nb := utf8.DecodeLastRuneInString(tb)
				if ra != rb {
					break
				}
				posA -= runeUnits(ra)
				posB -= runeUnits(ra)
				ta, tb = ta[:len(ta)-na], tb[:len(tb)-nb]
			}
			return posA, posB, true
		}
		if ca.content.size > 0 || cb.content.size > 0 {
			if ea, eb, ok := FindDiffEnd(ca.content, cb.content, posA-1, posB-1); ok {
				return ea, eb, true
			}
		}
		posA -= size
		posB -= size
	}
}

// RestoreStep returns one replace step that turns current into old
// (FR-89): it replaces only the range where the two differ, so the
// positions of open editors outside it keep their place. Nil when the
// documents are equal.
func RestoreStep(current, old Doc) Step {
	start, ok := FindDiffStart(current.content, old.content, 0)
	if !ok {
		return nil
	}
	endA, endB, _ := FindDiffEnd(current.content, old.content, current.content.size, old.content.size)
	if overlap := start - min(endA, endB); overlap > 0 {
		endA += overlap
		endB += overlap
	}
	if s := tryRestore(current, old, start, endA, endB); s != nil {
		return s
	}
	return &ReplaceStep{From: 0, To: current.content.size, Slice: Slice{Content: old.content}}
}

func tryRestore(current, old Doc, start, endA, endB int) (step Step) {
	defer func() {
		if recover() != nil {
			step = nil
		}
	}()
	s := &ReplaceStep{From: start, To: endA, Slice: old.slice(start, endB)}
	got, err := s.Apply(current)
	if err != nil || !got.Eq(old) {
		return nil
	}
	return s
}

// DocDiff compares two versions of a page (FR-88): top-level blocks are
// matched by content, paired blocks of one type are compared word by
// word, atom by atom and by marks.
type DocDiff struct {
	changes []change
	// Inserted and Deleted count the words, atoms and blocks added and
	// removed.
	Inserted, Deleted int
}

type changeKind int

const (
	same changeKind = iota
	deleted
	inserted
	paired
)

type change struct {
	kind     changeKind
	a, b     *Node
	children []change // paired containers
	tokens   []tokenOp
}

type token struct {
	text  string
	atom  *Node
	marks []*Mark
}

func (t token) eq(o token) bool {
	if t.atom != nil || o.atom != nil {
		return t.atom != nil && o.atom != nil && t.atom.Eq(o.atom)
	}
	return t.text == o.text && sameMarkSet(t.marks, o.marks)
}

type tokenOp struct {
	kind changeKind
	tok  token
}

// Diff compares version a with version b.
func Diff(a, b Doc) *DocDiff {
	d := &DocDiff{}
	d.changes = d.blocks(a.content.nodes, b.content.nodes)
	return d
}

// Equal reports whether the versions are the same.
func (d *DocDiff) Equal() bool { return d.Inserted == 0 && d.Deleted == 0 }

func blockKey(n *Node) string { return string(n.appendJSON(nil)) }

func (d *DocDiff) blocks(a, b []*Node) []change {
	ka, kb := make([]string, len(a)), make([]string, len(b))
	for i, n := range a {
		ka[i] = blockKey(n)
	}
	for i, n := range b {
		kb[i] = blockKey(n)
	}
	ops := myers(len(a), len(b), func(i, j int) bool { return ka[i] == kb[j] })
	var out []change
	for i := 0; i < len(ops); {
		if ops[i].kind == same {
			out = append(out, change{kind: same, a: a[ops[i].i], b: b[ops[i].j]})
			i++
			continue
		}
		// A hunk of deletions and insertions: pair blocks of one type.
		var dels, ins []*Node
		for ; i < len(ops) && ops[i].kind != same; i++ {
			if ops[i].kind == deleted {
				dels = append(dels, a[ops[i].i])
			} else {
				ins = append(ins, b[ops[i].j])
			}
		}
		pairs := myers(len(dels), len(ins), func(x, y int) bool { return dels[x].typ == ins[y].typ })
		for _, p := range pairs {
			switch p.kind {
			case deleted:
				d.Deleted++
				out = append(out, change{kind: deleted, a: dels[p.i]})
			case inserted:
				d.Inserted++
				out = append(out, change{kind: inserted, b: ins[p.j]})
			default:
				out = append(out, d.pair(dels[p.i], ins[p.j]))
			}
		}
	}
	return out
}

func (d *DocDiff) pair(a, b *Node) change {
	switch {
	case a.Eq(b):
		return change{kind: same, a: a, b: b}
	case b.typ.inlineContent || b.typ.code:
		ta, tb := tokenize2(a), tokenize2(b)
		ops := myers(len(ta), len(tb), func(i, j int) bool { return ta[i].eq(tb[j]) })
		c := change{kind: paired, a: a, b: b}
		for _, o := range ops {
			switch o.kind {
			case deleted:
				d.Deleted++
				c.tokens = append(c.tokens, tokenOp{deleted, ta[o.i]})
			case inserted:
				d.Inserted++
				c.tokens = append(c.tokens, tokenOp{inserted, tb[o.j]})
			default:
				c.tokens = append(c.tokens, tokenOp{same, tb[o.j]})
			}
		}
		return c
	case b.IsLeaf():
		d.Deleted++
		d.Inserted++
		return change{kind: paired, a: a, b: b} // shown as the old block, then the new
	default:
		return change{kind: paired, a: a, b: b, children: d.blocks(a.content.nodes, b.content.nodes)}
	}
}

// tokenize2 splits a block's inline content into words, runs of white
// space, single other characters and atoms, each with its marks.
func tokenize2(n *Node) []token {
	var out []token
	for _, c := range n.content.nodes {
		if !c.typ.isText {
			out = append(out, token{atom: c, marks: c.marks})
			continue
		}
		s := c.text
		for len(s) > 0 {
			r, size := utf8.DecodeRuneInString(s)
			end := size
			switch {
			case unicode.IsLetter(r) || unicode.IsNumber(r):
				for end < len(s) {
					r2, n2 := utf8.DecodeRuneInString(s[end:])
					if !(unicode.IsLetter(r2) || unicode.IsNumber(r2) || r2 == '_') {
						break
					}
					end += n2
				}
			case unicode.IsSpace(r):
				for end < len(s) {
					r2, n2 := utf8.DecodeRuneInString(s[end:])
					if !unicode.IsSpace(r2) {
						break
					}
					end += n2
				}
			}
			out = append(out, token{text: s[:end], marks: c.marks})
			s = s[end:]
		}
	}
	return out
}

type editOp struct {
	kind changeKind
	i, j int
}

// myers returns the shortest edit script of two sequences (Myers' O(ND)
// algorithm after trimming the common ends). Past 512 edits it gives up on
// the middle and replaces it whole.
func myers(n, m int, eq func(i, j int) bool) []editOp {
	pre := 0
	for pre < n && pre < m && eq(pre, pre) {
		pre++
	}
	suf := 0
	for suf < n-pre && suf < m-pre && eq(n-1-suf, m-1-suf) {
		suf++
	}
	var out []editOp
	for k := 0; k < pre; k++ {
		out = append(out, editOp{same, k, k})
	}
	out = append(out, myersMiddle(pre, n-suf, pre, m-suf, eq)...)
	for k := 0; k < suf; k++ {
		out = append(out, editOp{same, n - suf + k, m - suf + k})
	}
	return out
}

func myersMiddle(a0, a1, b0, b1 int, eq func(i, j int) bool) []editOp {
	n, m := a1-a0, b1-b0
	whole := func() []editOp {
		var out []editOp
		for i := a0; i < a1; i++ {
			out = append(out, editOp{deleted, i, -1})
		}
		for j := b0; j < b1; j++ {
			out = append(out, editOp{inserted, -1, j})
		}
		return out
	}
	if n == 0 || m == 0 {
		return whole()
	}
	maxD := min(n+m, 512)
	off := maxD + 1
	v := make([]int, 2*maxD+3)
	var trace [][]int
	for d := 0; d <= maxD; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && eq(a0+x, b0+y) {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				return backtrack(trace, d, off, n, m, a0, b0)
			}
		}
	}
	return whole()
}

func backtrack(trace [][]int, d, off, n, m, a0, b0 int) []editOp {
	var rev []editOp
	x, y := n, m
	for ; d > 0; d-- {
		v := trace[d]
		k := x - y
		var pk int
		if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := v[off+pk]
		py := px - pk
		for x > px && y > py {
			x--
			y--
			rev = append(rev, editOp{same, a0 + x, b0 + y})
		}
		if x == px {
			y--
			rev = append(rev, editOp{inserted, -1, b0 + y})
		} else {
			x--
			rev = append(rev, editOp{deleted, a0 + x, -1})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		rev = append(rev, editOp{same, a0 + x, b0 + y})
	}
	out := make([]editOp, len(rev))
	for i, o := range rev {
		out[len(rev)-1-i] = o
	}
	return out
}

// HTML renders the comparison: the newer version, with added text in
// <ins class="diff-ins"> and removed text in <del class="diff-del">; whole
// blocks added or removed carry data-diff="ins" or "del" (FR-88.1).
func (d *DocDiff) HTML(r Resolver) string {
	var b strings.Builder
	w := htmlWriter{&b, resolverOrNone(r)}
	w.changes(d.changes)
	return b.String()
}

func (w htmlWriter) changes(cs []change) {
	for _, c := range cs {
		switch c.kind {
		case same:
			w.block(c.b)
		case deleted:
			w.whole(c.a, "del")
		case inserted:
			w.whole(c.b, "ins")
		default:
			switch {
			case c.tokens != nil || (c.b.typ.inlineContent || c.b.typ.code):
				w.wrap(c.b, "", func() { w.tokenOps(c.b, c.tokens) })
			case c.children != nil:
				w.wrap(c.b, "", func() { w.changes(c.children) })
			default:
				w.whole(c.a, "del")
				w.whole(c.b, "ins")
			}
		}
	}
}

// whole renders a block added or removed as a whole.
func (w htmlWriter) whole(n *Node, kind string) {
	open, close := `<del class="diff-del">`, `</del>`
	if kind == "ins" {
		open, close = `<ins class="diff-ins">`, `</ins>`
	}
	extra := ` data-diff="` + kind + `"`
	if n.IsLeaf() {
		w.s(open)
		w.wrap(n, extra, nil)
		w.s(close)
		return
	}
	var inner func(n *Node, extra string)
	inner = func(n *Node, extra string) {
		w.wrap(n, extra, func() {
			if n.typ.inlineContent || n.typ.code {
				if n.content.size > 0 {
					w.s(open)
					w.body(n)
					w.s(close)
				}
				return
			}
			for _, c := range n.content.nodes {
				if c.IsLeaf() {
					w.s(open)
					w.block(c)
					w.s(close)
				} else {
					inner(c, "")
				}
			}
		})
	}
	inner(n, extra)
}

func (w htmlWriter) tokenOps(n *Node, ops []tokenOp) {
	code := n.typ.code
	for i := 0; i < len(ops); {
		j := i
		for j < len(ops) && ops[j].kind == ops[i].kind {
			j++
		}
		switch ops[i].kind {
		case deleted:
			w.s(`<del class="diff-del">`)
		case inserted:
			w.s(`<ins class="diff-ins">`)
		}
		if code {
			for _, o := range ops[i:j] {
				w.esc(o.tok.text)
			}
		} else {
			var nodes []*Node
			for _, o := range ops[i:j] {
				if o.tok.atom != nil {
					nodes = append(nodes, o.tok.atom)
				} else {
					nodes = append(nodes, newText(tickitText(n), o.tok.text, utf16Len(o.tok.text), o.tok.marks))
				}
			}
			w.inline(fragmentFromArray(nodes).nodes)
		}
		switch ops[i].kind {
		case deleted:
			w.s(`</del>`)
		case inserted:
			w.s(`</ins>`)
		}
		i = j
	}
}

func tickitText(n *Node) *NodeType { return n.typ.schema.text }
