package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/a-h/templ"
)

// A board of a hundred cards must stay under 60 KB a frame (NFR-37.1),
// and an icon drawn inline costs up to 800 bytes. BoardCard refers to
// symbols instead: Sprites draws the types, priorities and the card's
// icons once in the page, FaceSprites the faces of the people a frame
// shows, once in that frame. A <use> instance takes its colours from the
// classes of its paths and the custom properties of its <svg>, as the
// inline icon does.

// spriteIcons are the icons BoardCard refers to.
var spriteIcons = []string{"more", "calendar", "warning"}

// Sprites are the symbols of the types, priorities and BoardCard's icons:
// once in a page that shows BoardCard, outside the regions the stream
// patches.
func Sprites() templ.Component { return raw(spriteSheet()) }

func spriteSheet() string {
	var b strings.Builder
	b.WriteString(`<svg class="tk-sprites" aria-hidden="true" focusable="false" shape-rendering="crispEdges">`)
	for _, t := range IssueTypes {
		b.WriteString(`<symbol id="tk-t-` + string(t) + `" viewBox="0 0 16 16"><path class="tk-type__tile" d="` + typeTile + `"/><path class="tk-type__glyph" d="` + typePaths[string(t)] + `"/></symbol>`)
	}
	for _, p := range append([]Priority{""}, Priorities...) {
		glyph := prioPaths[string(p)]
		if glyph == "" {
			glyph = "M4 7h8v2H4z"
		}
		b.WriteString(`<symbol id="tk-p-` + string(p) + `" viewBox="0 0 16 16"><path d="` + glyph + `"/></symbol>`)
	}
	for _, name := range spriteIcons {
		g := icons[name]
		b.WriteString(`<symbol id="tk-i-` + name + `" viewBox="0 0 16 16"><path d="` + g.main + `"/>`)
		if g.second != "" {
			b.WriteString(`<path class="tk-icon__2" d="` + g.second + `"/>`)
		}
		b.WriteString(`</symbol>`)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// FaceRef is the id of a face's symbol in FaceSprites.
func FaceRef(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return "tk-f-" + hex.EncodeToString(sum[:5])
}

// FaceSprites are the symbols of the faces of people, each once: in the
// region that shows their BoardCards, so a frame brings the faces it
// needs.
func FaceSprites(people []Person) templ.Component {
	var b strings.Builder
	b.WriteString(`<svg class="tk-sprites" aria-hidden="true" focusable="false" shape-rendering="crispEdges">`)
	seen := map[string]bool{}
	for _, p := range people {
		if seen[p.Seed] {
			continue
		}
		seen[p.Seed] = true
		b.WriteString(`<symbol id="` + FaceRef(p.Seed) + `" viewBox="0 0 7 7"><path d="` + FaceOf(p.Seed).Path + `"/></symbol>`)
	}
	b.WriteString(`</svg>`)
	return raw(b.String())
}

func useRef(id string) string { return "#" + id }
