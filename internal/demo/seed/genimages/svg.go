package main

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

// Palette: slate neutrals, a teal accent and a few warm tones, to sit well
// beside tickit's interface.
const (
	slate50  = "#f8fafc"
	slate100 = "#f1f5f9"
	slate200 = "#e2e8f0"
	slate300 = "#cbd5e1"
	slate400 = "#94a3b8"
	slate500 = "#64748b"
	slate600 = "#475569"
	slate700 = "#334155"
	slate800 = "#1e293b"
	slate900 = "#0f172a"
	teal50   = "#f0fdfa"
	teal100  = "#ccfbf1"
	teal200  = "#99f6e4"
	teal300  = "#5eead4"
	teal400  = "#2dd4bf"
	teal500  = "#14b8a6"
	teal600  = "#0d9488"
	teal700  = "#0f766e"
	teal800  = "#115e59"
	teal900  = "#134e4a"
	amber100 = "#fef3c7"
	amber300 = "#fcd34d"
	amber500 = "#f59e0b"
	amber700 = "#b45309"
	rose100  = "#ffe4e6"
	rose300  = "#fda4af"
	rose500  = "#f43f5e"
	rose700  = "#be123c"
	white    = "#ffffff"
)

const (
	sans = `ui-sans-serif, system-ui, 'DejaVu Sans', sans-serif`
	mono = `ui-monospace, 'DejaVu Sans Mono', monospace`
)

type doc struct {
	w, h int
	b    strings.Builder
	defs map[string]bool
}

func num(v float64) string {
	return strconv.FormatFloat(float64(int(v*10+copysign(v)))/10, 'f', -1, 64)
}

func copysign(v float64) float64 {
	if v < 0 {
		return -0.5
	}
	return 0.5
}

func esc(s string) string { return html.EscapeString(s) }

// newDoc starts an image of w by h with an accessible title.
func newDoc(w, h int, title string) *doc {
	d := &doc{w: w, h: h}
	fmt.Fprintf(&d.b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`, w, h, w, h, esc(title))
	fmt.Fprintf(&d.b, `<title>%s</title>`, esc(title))
	d.raw(`<defs><marker id="a" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="8" markerHeight="8" orient="auto-start-reverse"><path d="M0 0L8 4L0 8z" fill="%s"/></marker>`+
		`<marker id="t" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="8" markerHeight="8" orient="auto-start-reverse"><path d="M0 0L8 4L0 8z" fill="%s"/></marker></defs>`, slate600, teal700)
	return d
}

func (d *doc) raw(format string, args ...any) { fmt.Fprintf(&d.b, format, args...) }

func (d *doc) String() string { return d.b.String() + "</svg>\n" }

// frame paints the background and a small title with a pixel mark, the
// look every diagram and chart shares.
func (d *doc) frame(title string) {
	d.rect(0, 0, float64(d.w), float64(d.h), 0, slate50, "", 0)
	d.rect(0.5, 0.5, float64(d.w)-1, float64(d.h)-1, 10, "none", slate200, 1)
	d.pixmark(24, 22)
	d.text(44, 33, strings.ToUpper(title), 12, slate500, mono, "start", 600, 1.5)
}

// pixmark is a tiny pixel mountain, the house mark of the library.
func (d *doc) pixmark(x, y float64) {
	icon(d, "mark", x, y, 2, map[rune]string{'#': teal600, 'o': teal300})
}

type style struct {
	fill, stroke string
	dash         bool
	width        float64
}

var (
	boxPlain    = style{white, slate400, false, 1.5}
	boxOurs     = style{teal50, teal600, false, 1.5}
	boxOutside  = style{slate100, slate400, true, 1.5}
	boxWarn     = style{amber100, amber500, false, 1.5}
	boxAlert    = style{rose100, rose500, false, 1.5}
	boxStrong   = style{teal600, teal700, false, 1.5}
	boxQuiet    = style{slate100, slate300, false, 1}
	boxDarkCard = style{slate800, slate700, false, 1}
)

func (d *doc) rect(x, y, w, h, rx float64, fill, stroke string, sw float64) {
	d.raw(`<rect x="%s" y="%s" width="%s" height="%s"`, num(x), num(y), num(w), num(h))
	if rx > 0 {
		d.raw(` rx="%s"`, num(rx))
	}
	d.raw(` fill="%s"`, fill)
	if stroke != "" {
		d.raw(` stroke="%s" stroke-width="%s"`, stroke, num(sw))
	}
	d.raw(`/>`)
}

func (d *doc) box(x, y, w, h float64, s style) {
	d.raw(`<rect x="%s" y="%s" width="%s" height="%s" rx="6" fill="%s" stroke="%s" stroke-width="%s"`, num(x), num(y), num(w), num(h), s.fill, s.stroke, num(s.width))
	if s.dash {
		d.raw(` stroke-dasharray="6 4"`)
	}
	d.raw(`/>`)
	// Pixel detail: a notch of two squares in the top left corner.
	if !s.dash {
		d.rect(x+5, y+5, 3, 3, 0, s.stroke, "", 0)
		d.rect(x+9, y+5, 3, 3, 0, s.stroke, "", 0)
	}
}

// node is a box with an icon, a title and an optional second line.
func (d *doc) node(x, y, w, h float64, s style, iconName, title, sub string) {
	d.box(x, y, w, h, s)
	fg, muted := slate900, slate500
	if s == boxStrong || s == boxDarkCard {
		fg, muted = white, teal100
	}
	tx := x + 14
	if iconName != "" {
		ic := s.stroke
		if s == boxStrong {
			ic = white
		}
		icon(d, iconName, x+14, y+h/2-8, 2, map[rune]string{'#': ic, 'o': teal400})
		tx = x + 40
	}
	if sub == "" {
		d.text(tx, y+h/2+5, title, 14, fg, sans, "start", 600, 0)
		return
	}
	d.text(tx, y+h/2-3, title, 14, fg, sans, "start", 600, 0)
	d.text(tx, y+h/2+15, sub, 12, muted, sans, "start", 400, 0)
}

func (d *doc) text(x, y float64, s string, size float64, fill, font, anchor string, weight int, spacing float64) {
	d.raw(`<text x="%s" y="%s" font-family="%s" font-size="%s" fill="%s"`, num(x), num(y), font, num(size), fill)
	if anchor != "start" {
		d.raw(` text-anchor="%s"`, anchor)
	}
	if weight != 400 {
		d.raw(` font-weight="%d"`, weight)
	}
	if spacing != 0 {
		d.raw(` letter-spacing="%s"`, num(spacing))
	}
	d.raw(`>%s</text>`, esc(s))
}

// label is small text on a pale background, for arrows.
func (d *doc) label(x, y float64, s string) {
	w := float64(len(s))*6.4 + 10
	d.rect(x-w/2, y-10, w, 17, 3, slate50, "", 0)
	d.text(x, y+3, s, 11, slate500, mono, "middle", 400, 0)
}

func (d *doc) line(x1, y1, x2, y2 float64, stroke string, sw float64, dash string) {
	d.raw(`<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"`, num(x1), num(y1), num(x2), num(y2), stroke, num(sw))
	if dash != "" {
		d.raw(` stroke-dasharray="%s"`, dash)
	}
	d.raw(`/>`)
}

// arrow draws a polyline through the points with a head at the end.
func (d *doc) arrow(dash bool, pts ...float64) { d.arrowIn(slate600, "a", dash, pts...) }

func (d *doc) arrowTeal(dash bool, pts ...float64) { d.arrowIn(teal700, "t", dash, pts...) }

func (d *doc) arrowIn(color, marker string, dash bool, pts ...float64) {
	var p strings.Builder
	for i := 0; i < len(pts); i += 2 {
		if i == 0 {
			p.WriteString("M")
		} else {
			p.WriteString("L")
		}
		p.WriteString(num(pts[i]) + " " + num(pts[i+1]))
	}
	d.raw(`<path d="%s" fill="none" stroke="%s" stroke-width="1.5" stroke-linejoin="round" marker-end="url(#%s)"`, p.String(), color, marker)
	if dash {
		d.raw(` stroke-dasharray="5 4"`)
	}
	d.raw(`/>`)
}

func (d *doc) circle(cx, cy, r float64, fill, stroke string, sw float64) {
	d.raw(`<circle cx="%s" cy="%s" r="%s" fill="%s"`, num(cx), num(cy), num(r), fill)
	if stroke != "" {
		d.raw(` stroke="%s" stroke-width="%s"`, stroke, num(sw))
	}
	d.raw(`/>`)
}

func (d *doc) path(dpath, fill, stroke string, sw float64) {
	d.raw(`<path d="%s" fill="%s"`, dpath, fill)
	if stroke != "" {
		d.raw(` stroke="%s" stroke-width="%s" stroke-linejoin="round" stroke-linecap="round"`, stroke, num(sw))
	}
	d.raw(`/>`)
}

// cylinder is a database.
func (d *doc) cylinder(x, y, w, h float64, s style, title, sub string) {
	ry := 9.0
	d.path(fmt.Sprintf("M%s %sv%sa%s %s 0 0 0 %s 0v-%sa%s %s 0 0 0 -%s 0z", num(x), num(y+ry), num(h-2*ry), num(w/2), num(ry), num(w), num(h-2*ry), num(w/2), num(ry), num(w)), s.fill, s.stroke, s.width)
	d.path(fmt.Sprintf("M%s %sa%s %s 0 0 0 %s 0", num(x), num(y+ry), num(w/2), num(ry), num(w)), "none", s.stroke, s.width)
	d.text(x+w/2, y+h/2+8, title, 14, slate900, sans, "middle", 600, 0)
	if sub != "" {
		d.text(x+w/2, y+h/2+25, sub, 12, slate500, sans, "middle", 400, 0)
	}
}

// Icons of 8 by 8 pixels: # is the main colour, o the accent.
var icons = map[string][]string{
	"mark": {
		"...o....",
		"..###...",
		".#####..",
		".######.",
		"###..###",
		"##....##",
		"........",
		"........",
	},
	"browser": {
		"########",
		"#o#o#o.#",
		"########",
		"#......#",
		"#.###..#",
		"#......#",
		"#.####.#",
		"########",
	},
	"phone": {
		".######.",
		".#oooo#.",
		".#oooo#.",
		".#oooo#.",
		".#oooo#.",
		".#oooo#.",
		".######.",
		".##..##.",
	},
	"server": {
		"########",
		"#o.....#",
		"########",
		"#o.....#",
		"########",
		"#o.....#",
		"########",
		"........",
	},
	"search": {
		"..####..",
		".#....#.",
		"#..oo..#",
		"#..oo..#",
		".#....#.",
		"..#####.",
		"......##",
		".......#",
	},
	"gear": {
		"...##...",
		".#.##.#.",
		"..####..",
		"###oo###",
		"###oo###",
		"..####..",
		".#.##.#.",
		"...##...",
	},
	"box": {
		"..####..",
		".#oooo#.",
		"########",
		"#..##..#",
		"#..##..#",
		"#......#",
		"#......#",
		"########",
	},
	"card": {
		"........",
		"########",
		"#......#",
		"########",
		"#......#",
		"#.oo...#",
		"########",
		"........",
	},
	"mail": {
		"........",
		"########",
		"##....##",
		"#.#..#.#",
		"#..##..#",
		"#......#",
		"########",
		"........",
	},
	"cloud": {
		"........",
		"...##...",
		"..#oo#..",
		".##oo##.",
		"#oooooo#",
		"#oooooo#",
		".######.",
		"........",
	},
	"bucket": {
		"########",
		"#oooooo#",
		".#....#.",
		".#....#.",
		".#....#.",
		".#....#.",
		"..####..",
		"........",
	},
	"bolt": {
		"....##..",
		"...##...",
		"..##....",
		".######.",
		"....##..",
		"...##...",
		"..##....",
		".#......",
	},
	"bell": {
		"...##...",
		"..####..",
		".######.",
		".######.",
		".######.",
		"########",
		"...oo...",
		"........",
	},
	"flag": {
		"#.......",
		"#ooo....",
		"#ooooo..",
		"#ooo....",
		"#.......",
		"#.......",
		"#.......",
		"#.......",
	},
	"check": {
		"........",
		".......#",
		"......##",
		"#....##.",
		"##..##..",
		".####...",
		"..##....",
		"........",
	},
	"person": {
		"..####..",
		"..####..",
		"..####..",
		"........",
		".######.",
		"########",
		"########",
		"........",
	},
}

// icon draws a named icon with its top left corner at x, y, scale pixels a
// dot.
func icon(d *doc, name string, x, y, scale float64, colors map[rune]string) {
	rows := icons[name]
	paths := map[rune]*strings.Builder{}
	for r, row := range rows {
		for c, ch := range row {
			if ch == '.' {
				continue
			}
			p := paths[ch]
			if p == nil {
				p = &strings.Builder{}
				paths[ch] = p
			}
			fmt.Fprintf(p, "M%s %sh%sv%sh-%sz", num(x+float64(c)*scale), num(y+float64(r)*scale), num(scale), num(scale), num(scale))
		}
	}
	for _, ch := range []rune{'#', 'o'} {
		if p := paths[ch]; p != nil {
			d.raw(`<path d="%s" fill="%s"/>`, p.String(), colors[ch])
		}
	}
}
