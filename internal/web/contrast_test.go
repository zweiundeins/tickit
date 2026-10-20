package web

import (
	"fmt"
	"io/fs"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestContrast checks that the semantic tokens of tokens.css keep WCAG 2.2
// AA in both themes (NFR-50): 4.5:1 for text on every surface it sits on,
// 3:1 for marks that are not text (focus rings, field edges, icon tiles).
// Washes with alpha are composited over each surface first. The per-hue
// tokens are checked for every hue the app uses: the categories of issue
// types, priorities and statuses, and the sixteen hues of faces.
func TestContrast(t *testing.T) {
	b, err := fs.ReadFile(staticFiles, "static/css/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	vars := map[string]string{}
	for _, m := range declaration.FindAllStringSubmatch(css, -1) {
		if _, seen := vars[m[1]]; !seen { // the :root blocks come first
			vars[m[1]] = strings.TrimSpace(m[2])
		}
	}
	surfaces := []string{"bg", "surface-1", "surface-2", "surface-inset", "surface-hover"}
	type pair struct {
		fg  string
		bgs []string
		min float64
	}
	pairs := []pair{
		{"text-1", surfaces, 4.5},
		{"text-2", surfaces, 4.5},
		{"text-muted", surfaces, 4.5},
		{"accent-text", surfaces, 4.5},
		{"danger", surfaces, 4.5},
		{"warn", surfaces, 4.5},
		{"ok", surfaces, 4.5},
		{"info", surfaces, 4.5},
		{"focus", surfaces, 3},
		{"control-border", []string{"bg", "surface-1", "surface-inset"}, 3},
		{"on-accent", []string{"accent", "accent-strong"}, 4.5},
		{"on-danger", []string{"danger"}, 4.5},
		{"tip-text", []string{"tip-bg"}, 4.5},
		{"code-text", []string{"code-bg"}, 4.5},
		{"code-keyword", []string{"code-bg"}, 4.5},
		{"code-function", []string{"code-bg"}, 4.5},
		{"code-string", []string{"code-bg"}, 4.5},
		{"code-number", []string{"code-bg"}, 4.5},
		{"code-comment", []string{"code-bg"}, 4.5},
	}
	// Text on a wash, the wash over a surface: the current nav row, a
	// selected row, notices.
	washes := []struct{ fg, wash string }{
		{"accent-text", "accent-subtle"},
		{"text-1", "accent-subtle"},
		{"text-1", "surface-selected"},
		{"danger", "danger-subtle"},
		{"warn", "warn-subtle"},
		{"ok", "ok-subtle"},
		{"info", "info-subtle"},
	}
	// The hues: categories (tokens.css) and the sixteen of faces.
	hues := map[string]float64{}
	for _, name := range []string{"progress", "done", "epic", "story", "task", "bug", "subtask", "low", "medium", "high", "critical"} {
		v, err := strconv.ParseFloat(vars["--tk-hue-"+name], 64)
		if err != nil {
			t.Fatalf("--tk-hue-%s: %v", name, err)
		}
		hues[name] = v
	}
	for i := range 16 {
		hues[fmt.Sprintf("face %d", i)] = float64(i) * 22.5
	}

	for theme, side := range map[string]int{"light": 0, "dark": 1} {
		r := resolver{vars: vars, side: side}
		check := func(what string, fg, bg rgba, min float64) {
			t.Helper()
			if c := contrast(fg, bg); c < min {
				t.Errorf("%s theme: %s is %.2f:1, want %.1f:1", theme, what, c, min)
			}
		}
		for _, p := range pairs {
			for _, b := range p.bgs {
				check(p.fg+" on "+b, r.color(t, "--tk-"+p.fg), r.color(t, "--tk-"+b), p.min)
			}
		}
		for _, w := range washes {
			for _, s := range surfaces[:4] {
				bg := over(r.color(t, "--tk-"+w.wash), r.color(t, "--tk-"+s))
				check(fmt.Sprintf("%s on %s over %s", w.fg, w.wash, s), r.color(t, "--tk-"+w.fg), bg, 4.5)
			}
		}
		for name, h := range hues {
			r.h = h
			for _, s := range surfaces[:4] {
				surface := r.color(t, "--tk-"+s)
				// A lozenge: the hue's text on its wash.
				check(fmt.Sprintf("%s: hue-text on hue-wash over %s", name, s), r.color(t, "--tk-hue-text"), over(r.color(t, "--tk-hue-wash"), surface), 4.5)
				// Text in the hue: a mention, a name tag's neighbour.
				check(fmt.Sprintf("%s: hue-text on %s", name, s), r.color(t, "--tk-hue-text"), surface, 4.5)
				// A type icon's tile against the page.
				check(fmt.Sprintf("%s: hue-fill on %s", name, s), r.color(t, "--tk-hue-fill"), surface, 3)
			}
			// The glyph knocked out of the tile, a caret's name tag.
			check(name+": hue-on on hue-fill", r.color(t, "--tk-hue-on"), r.color(t, "--tk-hue-fill"), 4.5)
			// A face's pixels on its tile.
			check(name+": hue-text on hue-tile", r.color(t, "--tk-hue-text"), r.color(t, "--tk-hue-tile"), 3)
		}
	}
}

var (
	declaration = regexp.MustCompile(`(--tk-[a-z0-9_-]+):\s*([^;{}]+);`)
	lightDark   = regexp.MustCompile(`^light-dark\((.+)\)$`)
	varRef      = regexp.MustCompile(`^var\((--[a-z0-9_-]+)\)$`)
	oklchRe     = regexp.MustCompile(`^oklch\(([\d.]+)%\s+([\d.]+)\s+([\d.]+|var\(--h\))(?:\s*/\s*([\d.]+))?\)$`)
)

// rgba is a colour in gamma-encoded sRGB, each part 0 to 1, with alpha.
type rgba [4]float64

type resolver struct {
	vars map[string]string
	side int     // 0 light, 1 dark
	h    float64 // the hue that var(--h) stands for
}

// color follows a token to its colour.
func (r resolver) color(t *testing.T, name string) rgba {
	t.Helper()
	v, ok := r.vars[name]
	if !ok {
		t.Fatalf("no token %s", name)
	}
	for range 12 {
		if m := lightDark.FindStringSubmatch(v); m != nil {
			v = strings.TrimSpace(splitTop(m[1])[r.side])
			continue
		}
		if m := varRef.FindStringSubmatch(v); m != nil {
			if v, ok = r.vars[m[1]]; !ok {
				t.Fatalf("%s refers to %s, which is not defined", name, m[1])
			}
			continue
		}
		break
	}
	m := oklchRe.FindStringSubmatch(v)
	if m == nil {
		t.Fatalf("%s is %q: want oklch(), light-dark() or var()", name, v)
	}
	l, _ := strconv.ParseFloat(m[1], 64)
	c, _ := strconv.ParseFloat(m[2], 64)
	h := r.h
	if m[3] != "var(--h)" {
		h, _ = strconv.ParseFloat(m[3], 64)
	}
	a := 1.0
	if m[4] != "" {
		a, _ = strconv.ParseFloat(m[4], 64)
	}
	rgb := srgb(l/100, c, h)
	return rgba{rgb[0], rgb[1], rgb[2], a}
}

// splitTop splits light-dark's two arguments at the comma outside
// parentheses.
func splitTop(s string) []string {
	depth := 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return []string{s[:i], s[i+1:]}
			}
		}
	}
	return []string{s, s}
}

// over composites a colour with alpha over an opaque one, as browsers do,
// in gamma-encoded sRGB.
func over(fg, bg rgba) rgba {
	a := fg[3]
	return rgba{fg[0]*a + bg[0]*(1-a), fg[1]*a + bg[1]*(1-a), fg[2]*a + bg[2]*(1-a), 1}
}

// srgb converts OKLCH to gamma-encoded sRGB, clipped to the gamut.
func srgb(l, c, h float64) [3]float64 {
	a, b := c*math.Cos(h*math.Pi/180), c*math.Sin(h*math.Pi/180)
	l_ := l + 0.3963377774*a + 0.2158037573*b
	m_ := l - 0.1055613458*a - 0.0638541728*b
	s_ := l - 0.0894841775*a - 1.2914855480*b
	L, M, S := l_*l_*l_, m_*m_*m_, s_*s_*s_
	lin := [3]float64{
		4.0767416621*L - 3.3077115913*M + 0.2309699292*S,
		-1.2684380046*L + 2.6097574011*M - 0.3413193965*S,
		-0.0041960863*L - 0.7034186147*M + 1.7076147010*S,
	}
	var out [3]float64
	for i, x := range lin {
		x = math.Min(1, math.Max(0, x))
		if x <= 0.0031308 {
			out[i] = 12.92 * x
		} else {
			out[i] = 1.055*math.Pow(x, 1/2.4) - 0.055
		}
	}
	return out
}

// contrast is WCAG's contrast ratio of two opaque colours.
func contrast(a, b rgba) float64 {
	lin := func(x float64) float64 {
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	lum := func(c rgba) float64 { return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2]) }
	x, y := lum(a), lum(b)
	return (math.Max(x, y) + 0.05) / (math.Min(x, y) + 0.05)
}
