package doc

import (
	"slices"
	"strconv"
	"strings"
)

// Highlight is a range of a document that the page shows marked: the text
// an inline comment is on (FR-91). Its positions are the document's; the
// marks live outside the document, so the schema knows nothing of them.
type Highlight struct {
	ID       string
	From, To int
}

// hlState is where the renderer is in the document, and the ranges it
// marks, sorted.
type hlState struct {
	pos int
	hs  []Highlight
}

// RenderHighlighted renders the document as RenderHTML does, with the text
// inside each highlight in <mark class="tk-ic" data-c="<ids>">, the ids of
// every highlight over it separated by spaces. Atoms stay unmarked; a
// range outside the document marks what it covers of it.
func RenderHighlighted(d Doc, r Resolver, hs []Highlight) string {
	hs = sortHighlights(hs)
	var b strings.Builder
	b.Grow(d.content.size * 2)
	pos := 0
	for _, n := range d.content.nodes {
		b.WriteString(renderBlockAt(n, r, pos, hs))
		pos += n.size
	}
	return b.String()
}

// renderBlockAt renders a block that starts at pos.
func renderBlockAt(n *Node, r Resolver, pos int, hs []Highlight) string {
	var b strings.Builder
	w := htmlWriter{b: &b, r: resolverOrNone(r)}
	if highlightsIn(hs, pos, pos+n.size) != "" {
		w.hl = &hlState{pos: pos, hs: hs}
	}
	w.block(n)
	return b.String()
}

func sortHighlights(hs []Highlight) []Highlight {
	out := slices.DeleteFunc(slices.Clone(hs), func(h Highlight) bool { return h.From >= h.To || h.ID == "" })
	slices.SortFunc(out, func(a, b Highlight) int {
		if a.From != b.From {
			return a.From - b.From
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// highlightsIn names the highlights over from to to, relative to from: a
// block's key in the BlockCache.
func highlightsIn(hs []Highlight, from, to int) string {
	var b strings.Builder
	for _, h := range hs {
		if h.To > from && h.From < to {
			b.WriteString(h.ID)
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(max(h.From, from) - from))
			b.WriteByte('-')
			b.WriteString(strconv.Itoa(min(h.To, to) - from))
			b.WriteByte(';')
		}
	}
	return b.String()
}

// text writes a text node, its highlighted parts marked.
func (w htmlWriter) text(n *Node) {
	if w.hl == nil {
		w.esc(n.text)
		return
	}
	start, end := w.hl.pos, w.hl.pos+n.size
	cuts := []int{0, n.size}
	for _, h := range w.hl.hs {
		if h.To > start && h.From < end {
			cuts = append(cuts, max(h.From, start)-start, min(h.To, end)-start)
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	for i := 0; i+1 < len(cuts); i++ {
		a, b := safeByteOffset(n.text, n.size, cuts[i]), safeByteOffset(n.text, n.size, cuts[i+1])
		if a >= b {
			continue
		}
		var ids []string
		for _, h := range w.hl.hs {
			if h.From <= start+cuts[i] && h.To >= start+cuts[i+1] {
				ids = append(ids, h.ID)
			}
		}
		if len(ids) == 0 {
			w.esc(n.text[a:b])
			continue
		}
		w.s(`<mark class="tk-ic" data-c="`)
		w.esc(strings.Join(ids, " "))
		w.s(`">`)
		w.esc(n.text[a:b])
		w.s(`</mark>`)
	}
}

// advance moves the renderer's position past an inline node.
func (w htmlWriter) advance(n *Node) {
	if w.hl != nil {
		w.hl.pos += n.size
	}
}

// safeByteOffset is byteOffset that moves a cut inside a surrogate pair
// to before it.
func safeByteOffset(s string, units, off int) (b int) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(splitPanic); !ok {
				panic(r)
			}
			b = safeByteOffset(s, units, off-1)
		}
	}()
	return byteOffset(s, units, off)
}

// InlineText is the text between from and to without separators: the
// characters of its text nodes alone, as the page compares what a person
// selected (FR-91). ok is false for a range outside the document, a
// reversed one, and one that cuts a character in half.
func InlineText(d Doc, from, to int) (s string, ok bool) {
	if from < 0 || to > d.content.size || from > to {
		return "", false
	}
	defer func() {
		if r := recover(); r != nil {
			if _, split := r.(splitPanic); !split {
				panic(r)
			}
			s, ok = "", false
		}
	}()
	return d.TextBetween(from, to, "", func(*Node) string { return "" }), true
}

// Quote is the text between from and to as a list of comments shows it:
// blocks separated by a space, at most n characters with an ellipsis.
func Quote(d Doc, from, to, n int) string {
	if _, ok := InlineText(d, from, to); !ok {
		return ""
	}
	q := []rune(strings.Join(strings.Fields(d.TextBetween(from, to, " ", func(*Node) string { return "" })), " "))
	if len(q) > n {
		return string(q[:n-1]) + "…"
	}
	return string(q)
}

// MapRange maps a highlight's range through steps: its start moves with
// text inserted at it, its end does not, so typing at either edge stays
// outside. gone reports a range that holds no text any more.
func MapRange(next Doc, steps []Step, from, to int) (nf, nt int, gone bool) {
	nf, nt = from, to
	for _, s := range steps {
		m := s.StepMap()
		nf, nt = m.Map(nf, 1), m.Map(nt, -1)
	}
	if nf >= nt {
		return nf, nf, true
	}
	if nt-nf < to-from {
		if t, ok := InlineText(next, nf, nt); !ok || t == "" {
			return nf, nt, true
		}
	}
	return nf, nt, false
}
