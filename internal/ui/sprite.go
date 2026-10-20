package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// A board of a hundred cards must stay under 60 KB a frame (NFR-37.1),
// and an icon drawn inline costs up to 800 bytes. BoardCard draws its
// glyphs from the stylesheet instead: Glyphs writes, once in a page, a
// mask for every issue type, priority and status category and for the
// card's icons, from the same pixel art the inline icons draw;
// FaceStyles writes, in the frame that shows them, a mask for each face.
// An <i> names its glyph with data attributes: <i data-type="bug"> draws
// the type's tile and glyph in its hue, data-prio a priority, data-cat a
// category; .tk-icon-glyph[data-icon] an icon in the text's colour;
// .tk-av[data-face] a face in its tile.

// glyphIcons are the icons the board's markup names by data-icon.
var glyphIcons = []string{"more", "calendar", "warning", "plus"}

// mask is a CSS url() of an SVG of one path, as a mask image.
func mask(viewBox, path string) string {
	svg := `<svg xmlns='http://www.w3.org/2000/svg' viewBox='` + viewBox + `' shape-rendering='crispEdges'><path d='` + path + `'/></svg>`
	return `url("data:image/svg+xml,` + strings.NewReplacer("<", "%3C", ">", "%3E", "#", "%23", `"`, "'").Replace(svg) + `")`
}

// Glyphs is the stylesheet of BoardCard's glyphs: once in a page that
// shows board cards or backlog rows, outside the regions the stream
// patches.
func Glyphs() templ.Component { return raw(`<style>` + glyphSheet() + `</style>`) }

func glyphSheet() string {
	var b strings.Builder
	b.WriteString(`i:is([data-type],[data-prio],[data-cat]),.tk-icon-glyph{position:relative;display:inline-block;flex:none;inline-size:1rem;block-size:1rem}` +
		`i:is([data-type],[data-prio],[data-cat])::before,i[data-type]::after,.tk-icon-glyph::before,.tk-av[data-face]::before{content:"";position:absolute;inset:0;mask:var(--_m) center/100% 100% no-repeat}` +
		`i[data-type]::before{--_m:` + mask("0 0 16 16", typeTile) + `;background:var(--tk-hue-fill)}` +
		`i[data-type]::after{background:var(--tk-hue-on)}` +
		`i[data-prio]::before,i[data-cat]::before{background:var(--tk-hue-text)}` +
		`i[data-cat]{inline-size:0.625rem;block-size:0.625rem}` +
		`.tk-icon-glyph::before{background:currentColor}` +
		`i[data-cat]::before{background:currentColor}` +
		`.tk-av[data-face]::before{inset:calc(var(--_s) / 8);background:currentColor}.tk-av--xs[data-face]::before{inset:0}` +
		`.tk-board--flat .tk-card>button::before{content:"";inline-size:1rem;block-size:1rem;background:currentColor;mask:` + mask("0 0 16 16", icons["more"].main) + ` center/100% 100% no-repeat}` +
		`:is(.tk-board--flat,.tk-blrow) .tk-due::before{content:"";flex:none;inline-size:0.875rem;block-size:0.875rem;background:currentColor;mask:` + mask("0 0 16 16", icons["calendar"].main) + ` center/100% 100% no-repeat}` +
		`:is(.tk-board--flat,.tk-blrow) .tk-due[data-overdue]::before{mask-image:` + mask("0 0 16 16", icons["warning"].main) + `}`)
	for _, t := range IssueTypes {
		b.WriteString(`i[data-type="` + string(t) + `"]::after{--_m:` + mask("0 0 16 16", typePaths[string(t)]) + `}`)
	}
	for _, p := range append([]Priority{""}, Priorities...) {
		glyph := prioPaths[string(p)]
		if glyph == "" {
			glyph = "M4 7h8v2H4z"
		}
		b.WriteString(`i[data-prio="` + string(p) + `"]::before{--_m:` + mask("0 0 16 16", glyph) + `}`)
	}
	for _, c := range []StatusCategory{ToDo, InProgress, Done} {
		b.WriteString(`i[data-cat="` + c.attr() + `"]::before{--_m:` + mask("0 0 10 10", catPaths[c.attr()]) + `}`)
	}
	for _, name := range glyphIcons {
		b.WriteString(`[data-icon="` + name + `"]::before{--_m:` + mask("0 0 16 16", icons[name].main) + `}`)
	}
	return b.String()
}

// FaceRef is the name of a face in FaceStyles, from a person's id.
func FaceRef(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:3])
}

// FaceStyles are the masks and hues of the faces of people, each once: in the
// region that shows their cards, so a frame brings the faces it needs.
func FaceStyles(people []Person) templ.Component {
	var b strings.Builder
	b.WriteString(`<style>`)
	seen := map[string]bool{}
	for _, p := range people {
		ref := FaceRef(p.Seed)
		if seen[ref] {
			continue
		}
		seen[ref] = true
		b.WriteString(`[data-face="` + ref + `"]{--h:` + strconv.FormatFloat(float64(p.HueIndex())*22.5, 'f', -1, 64) + `}[data-face="` + ref + `"]::before{--_m:` + mask("0 0 7 7", FaceOf(p.Seed).Path) + `}`)
	}
	b.WriteString(`</style>`)
	return raw(b.String())
}
