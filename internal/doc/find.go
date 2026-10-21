package doc

import (
	"strings"
	"unicode"
)

// FindInBlock finds the nth occurrence (from 0) of text in the top-level
// block at index block, within one textblock and outside atoms, as a
// reader's selection in the rendered page names it (FR-105): any run of
// white space matches any run of white space. It returns the positions
// before and after the text.
func FindInBlock(d Doc, block int, text string, nth int) (from, to int, ok bool) {
	want := []rune(strings.Join(strings.Fields(text), " "))
	if block < 0 || block >= len(d.content.nodes) || len(want) == 0 || nth < 0 {
		return 0, 0, false
	}
	start := 0
	for _, b := range d.content.nodes[:block] {
		start += b.size
	}
	seen := 0
	var walk func(n *Node, pos int) bool
	walk = func(n *Node, pos int) bool {
		if n.typ.inlineContent {
			var runes []rune
			var at []int // the position before each rune, and after the last
			p := pos + 1
			for _, c := range n.content.nodes {
				if !c.typ.isText {
					runes, at = append(runes, '￼'), append(at, p)
					p += c.size
					continue
				}
				for _, r := range c.text {
					runes, at = append(runes, r), append(at, p)
					p += runeUnits(r)
				}
			}
			at = append(at, p)
			for i := range runes {
				if end, hit := matchAt(runes, i, want); hit {
					if seen == nth {
						from, to, ok = at[i], at[end], true
						return true
					}
					seen++
				}
			}
			return false
		}
		p := pos + 1
		for _, c := range n.content.nodes {
			if walk(c, p) {
				return true
			}
			p += c.size
		}
		return false
	}
	walk(d.content.nodes[block], start)
	return from, to, ok
}

// matchAt matches want, whose white space is single spaces, at runes[i:];
// it returns the index after the match.
func matchAt(runes []rune, i int, want []rune) (int, bool) {
	if unicode.IsSpace(runes[i]) {
		return 0, false
	}
	j := i
	for k := 0; k < len(want); k++ {
		if j >= len(runes) {
			return 0, false
		}
		if want[k] == ' ' {
			if !unicode.IsSpace(runes[j]) {
				return 0, false
			}
			for j < len(runes) && unicode.IsSpace(runes[j]) {
				j++
			}
			continue
		}
		if runes[j] != want[k] {
			return 0, false
		}
		j++
	}
	return j, true
}

// ChipStep is the step that puts an issue's chip in place of the range
// from, to (FR-105).
func ChipStep(from, to int, key string) (Step, error) {
	b := []byte(`{"stepType":"replace","from":`)
	b = appendNumber(b, float64(from))
	b = append(b, `,"to":`...)
	b = appendNumber(b, float64(to))
	b = append(b, `,"slice":{"content":[{"type":"issue_chip","attrs":{"key":`...)
	b = appendString(b, key)
	b = append(b, `}}]}}`...)
	return tickit.ParseStep(b)
}
