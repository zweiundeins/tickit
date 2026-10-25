package ui

import (
	"crypto/sha256"
	"html"
	"strconv"
	"strings"
)

// Face is a person's pixel face, drawn from a seed alone, so every page
// draws the same one: lith's identicon (site/internal/demos/board/avatar.go).
type Face struct {
	// Path draws the face on a grid of 7 (a pixel of margin around 5 by 5),
	// mirrored left to right.
	Path string
	// Hue is one of sixteen, 0 to 15: data-hue on the face's tile, and the
	// colour of the person's caret, selection and name tag.
	Hue int
}

// FaceOf draws the face of a seed, a person's id: the first four bytes of
// its SHA-256 pick the face (15 bits: three columns of five, the third
// mirrored onto the first two) and the hue (4 bits). The seed never shows.
func FaceOf(seed string) Face {
	sum := sha256.Sum256([]byte(seed))
	n := uint32(sum[0])<<24 | uint32(sum[1])<<16 | uint32(sum[2])<<8 | uint32(sum[3])
	face := n & 0x7fff
	// A face needs enough pixels to read as one: sparse ones turn over.
	if ones(face) < 6 {
		face ^= 0x7fff
	}
	var px [5][5]bool
	for col := range 3 {
		for row := range 5 {
			if face&(1<<(col*5+row)) != 0 {
				px[row][col], px[row][4-col] = true, true
			}
		}
	}
	// Each row as runs of pixels: a rectangle per run.
	var p strings.Builder
	for row := range px {
		for x := 0; x < 5; {
			if !px[row][x] {
				x++
				continue
			}
			w := 1
			for x+w < 5 && px[row][x+w] {
				w++
			}
			s := strconv.Itoa(w)
			p.WriteString("M" + strconv.Itoa(x+1) + " " + strconv.Itoa(row+1) + "h" + s + "v1h-" + s + "z")
			x += w
		}
	}
	return Face{Path: p.String(), Hue: int(n >> 15 & 15)}
}

func ones(n uint32) int {
	c := 0
	for ; n != 0; n &= n - 1 {
		c++
	}
	return c
}

// HueOf is a stable hue, 0 to 15, for a name such as a label's or an
// epic's, so that the same name has the same colour everywhere.
func HueOf(name string) int {
	sum := sha256.Sum256([]byte(name))
	return int(sum[0] & 15)
}

// FaceHTML is a decorative face of 1em as HTML, for what renders outside
// templ: a mention in a document (FR-67.2, docnodes.FaceResolver). The name is
// written beside it.
func FaceHTML(p Person) string {
	return `<span class="tk-av tk-av--xs" data-hue="` + strconv.Itoa(p.HueIndex()) + `" aria-hidden="true"><svg class="tk-face" viewBox="0 0 7 7" shape-rendering="crispEdges" focusable="false"><path d="` +
		html.EscapeString(FaceOf(p.Seed).Path) + `"></path></svg></span>`
}
