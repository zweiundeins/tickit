package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
)

// canvas is a small grid of coloured pixels, written to SVG as one path per
// colour of horizontal runs.
type canvas struct {
	w, h int
	px   []string
	rng  *rand.Rand
}

func newCanvas(w, h int, seed uint64) *canvas {
	return &canvas{w: w, h: h, px: make([]string, w*h), rng: rand.New(rand.NewPCG(seed, 7))}
}

func (c *canvas) set(x, y int, col string) {
	if x >= 0 && y >= 0 && x < c.w && y < c.h && col != "" {
		c.px[y*c.w+x] = col
	}
}

func (c *canvas) get(x, y int) string {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return ""
	}
	return c.px[y*c.w+x]
}

func (c *canvas) rect(x, y, w, h int, col string) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			c.set(i, j, col)
		}
	}
}

var bayer = [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}

// bands paints rows y0 to y1 as a vertical gradient through cols, with a
// short dithered seam between neighbouring colours.
func (c *canvas) bands(y0, y1 int, cols ...string) {
	for y := y0; y < y1; y++ {
		t := float64(y-y0) / float64(y1-y0) * float64(len(cols)-1)
		i := int(t)
		if i >= len(cols)-1 {
			i = len(cols) - 2
		}
		f := (t - float64(i) - 0.4) / 0.2
		for x := 0; x < c.w; x++ {
			col := cols[i]
			if f > (bayer[y%4][x%4]+0.5)/16 {
				col = cols[i+1]
			}
			c.set(x, y, col)
		}
	}
}

// ridge returns a skyline through the control points (x, y), roughened.
func (c *canvas) ridge(rough float64, pts ...[2]int) []int {
	out := make([]int, c.w)
	walk := 0.0
	for x := 0; x < c.w; x++ {
		k := 0
		for k < len(pts)-2 && x > pts[k+1][0] {
			k++
		}
		a, b := pts[k], pts[k+1]
		t := 0.0
		if b[0] != a[0] {
			t = float64(x-a[0]) / float64(b[0]-a[0])
		}
		t = math.Max(0, math.Min(1, t))
		walk += (c.rng.Float64() - 0.5) * rough
		walk *= 0.85
		out[x] = int(math.Round(float64(a[1]) + (float64(b[1])-float64(a[1]))*t + walk))
	}
	return out
}

type rock struct{ lit, shade, snowLit, snowShade string }

// mountain fills below a ridge down to bottom, lit from the left, with snow
// on what rises above snowLine.
func (c *canvas) mountain(r []int, bottom, snowLine, depth int, m rock) {
	face := 1
	for x := 0; x < c.w; x++ {
		prev, next := r[max(x-4, 0)], r[min(x+4, c.w-1)]
		if next < prev-1 {
			face = 1
		} else if next > prev+1 {
			face = -1
		}
		d := depth + int(c.rng.IntN(3)) - 1
		for y := r[x]; y < bottom; y++ {
			snow := r[x] < snowLine && y < r[x]+d+(snowLine-r[x])/3
			var col string
			switch {
			case snow && face > 0:
				col = m.snowLit
			case snow:
				col = m.snowShade
			case face > 0:
				col = m.lit
			default:
				col = m.shade
			}
			c.set(x, y, col)
		}
	}
}

func (c *canvas) fillBelow(r []int, bottom int, col string) {
	for x := 0; x < c.w; x++ {
		for y := r[x]; y < bottom; y++ {
			c.set(x, y, col)
		}
	}
}

// pine draws a fir tree with its foot at x, y.
func (c *canvas) pine(x, y, h int, col, dark string) {
	c.rect(x, y-2, 1, 2, "#3f2a1d")
	for i := 0; i < h; i++ {
		half := (i%4 + i/2) / 2
		if half < 1 {
			half = 0
		}
		yy := y - 2 - (h - 1 - i)
		for dx := -half; dx <= half; dx++ {
			col2 := col
			if dx > 0 {
				col2 = dark
			}
			c.set(x+dx, yy, col2)
		}
	}
}

// poly fills a polygon by scanlines.
func (c *canvas) poly(col string, pts ...[2]float64) {
	minY, maxY := c.h, 0
	for _, p := range pts {
		minY = min(minY, int(math.Floor(p[1])))
		maxY = max(maxY, int(math.Ceil(p[1])))
	}
	for y := minY; y <= maxY; y++ {
		fy := float64(y) + 0.5
		var xs []float64
		for i := range pts {
			a, b := pts[i], pts[(i+1)%len(pts)]
			if (a[1] <= fy) != (b[1] <= fy) {
				xs = append(xs, a[0]+(fy-a[1])/(b[1]-a[1])*(b[0]-a[0]))
			}
		}
		sort.Float64s(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			for x := int(math.Round(xs[i])); x < int(math.Round(xs[i+1])); x++ {
				c.set(x, y, col)
			}
		}
	}
}

func (c *canvas) ellipse(cx, cy, rx, ry float64, col string) {
	for y := int(cy - ry); y <= int(cy+ry); y++ {
		for x := int(cx - rx); x <= int(cx+rx); x++ {
			dx, dy := (float64(x)+0.5-cx)/rx, (float64(y)+0.5-cy)/ry
			if dx*dx+dy*dy <= 1 {
				c.set(x, y, col)
			}
		}
	}
}

// sprite draws rows of characters, each mapped to a colour; '.' is clear.
func (c *canvas) sprite(x, y int, rows []string, cols map[rune]string) {
	for j, row := range rows {
		for i, ch := range row {
			if col, ok := cols[ch]; ok {
				c.set(x+i, y+j, col)
			}
		}
	}
}

// paths returns the canvas as SVG paths at the given scale and offset. A
// run of pixels repeated on the rows below becomes one taller rectangle.
func (c *canvas) paths(scale, ox, oy float64) string {
	type run struct{ x, y, n, h int }
	byCol := map[string][]run{}
	var order []string
	type span struct {
		x, n int
		col  string
	}
	open := map[span]int{} // a run of the previous row to its index in byCol[col]
	for y := 0; y < c.h; y++ {
		next := map[span]int{}
		for x := 0; x < c.w; {
			col := c.px[y*c.w+x]
			n := 1
			for x+n < c.w && c.px[y*c.w+x+n] == col {
				n++
			}
			if col != "" {
				if _, ok := byCol[col]; !ok {
					order = append(order, col)
				}
				key := span{x, n, col}
				if i, ok := open[key]; ok && byCol[col][i].y+byCol[col][i].h == y {
					byCol[col][i].h++
					next[key] = i
				} else {
					byCol[col] = append(byCol[col], run{x, y, n, 1})
					next[key] = len(byCol[col]) - 1
				}
			}
			x += n
		}
		open = next
	}
	var out strings.Builder
	for _, col := range order {
		var b strings.Builder
		for _, r := range byCol[col] {
			fmt.Fprintf(&b, "M%s %sh%sv%sh-%sz", num(ox+float64(r.x)*scale), num(oy+float64(r.y)*scale), num(float64(r.n)*scale), num(float64(r.h)*scale), num(float64(r.n)*scale))
		}
		fmt.Fprintf(&out, `<path fill="%s" d="%s"/>`, col, b.String())
	}
	return out.String()
}

// photo writes a canvas as a stand-alone image, each pixel scale wide. Its
// coordinates stay in pixels, so the paths stay short.
func photo(c *canvas, scale int, title string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="%s"><title>%s</title>%s</svg>`+"\n",
		c.w*scale, c.h*scale, c.w, c.h, esc(title), esc(title), c.paths(1, 0, 0))
}

// Photo palette.
const (
	night0  = "#0b1220"
	night1  = "#111c33"
	night2  = "#1b2a4a"
	dawn0   = "#27325a"
	dawn1   = "#4a4a7d"
	dawn2   = "#8a6a9a"
	dawn3   = "#d48a8f"
	dawn4   = "#f2b08a"
	dawn5   = "#fbd7a8"
	sky0    = "#7cc4e8"
	sky1    = "#a6d8f0"
	sky2    = "#cfeaf7"
	sky3    = "#e8f5fb"
	cloud   = "#f8fafc"
	cloud2  = "#dbe7ef"
	glowLit = "#f9c6c9"
	glowSh  = "#a9a3cf"
	snowLit = "#f4f7fb"
	snowSh  = "#b9c7da"
	rockLit = "#7d8aa3"
	rockSh  = "#55617a"
	far0    = "#8f9bb8"
	far1    = "#6f7c9b"
	grass0  = "#7fae3f"
	grass1  = "#5e8f2e"
	grass2  = "#477526"
	grass3  = "#a7c957"
	forest0 = "#2f5d3a"
	forest1 = "#1f4630"
	wood0   = "#9a5b32"
	wood1   = "#74401f"
	wood2   = "#c07a43"
	stone0  = "#9aa3ae"
	stone1  = "#6f7884"
	water0  = "#2c6f86"
	water1  = "#3f8ea6"
	water2  = "#7cc0d2"
	fire0   = "#f97316"
	fire1   = "#fbbf24"
	fire2   = "#fde68a"
	red0    = "#c2413b"
	red1    = "#e0605a"
)

func clouds(c *canvas, n, yMax int) {
	for i := 0; i < n; i++ {
		x, y := float64(c.rng.IntN(c.w)), float64(6+c.rng.IntN(yMax))
		w := float64(10 + c.rng.IntN(14))
		c.ellipse(x, y+1, w, 2.5, cloud2)
		c.ellipse(x-w/4, y, w*0.45, 3, cloud)
		c.ellipse(x+w/5, y-1, w*0.4, 3.5, cloud)
	}
}

func meadowDots(c *canvas, y0 int, cols ...string) {
	for i := 0; i < c.w*(c.h-y0)/12; i++ {
		x, y := c.rng.IntN(c.w), y0+c.rng.IntN(c.h-y0)
		if c.get(x, y) == grass0 || c.get(x, y) == grass1 {
			c.set(x, y, cols[c.rng.IntN(len(cols))])
		}
	}
}

func sceneAlpenglow() *canvas {
	c := newCanvas(160, 100, 1)
	c.bands(0, 62, dawn0, dawn1, dawn2, dawn3, dawn4, dawn5)
	far := c.ridge(1.2, [2]int{0, 50}, [2]int{30, 40}, [2]int{60, 46}, [2]int{95, 36}, [2]int{130, 44}, [2]int{159, 38})
	c.mountain(far, 100, 46, 3, rock{far0, far1, "#e7c6d6", "#a7a2c9"})
	main := c.ridge(1.6, [2]int{0, 62}, [2]int{28, 44}, [2]int{52, 22}, [2]int{70, 34}, [2]int{92, 18}, [2]int{118, 40}, [2]int{140, 32}, [2]int{159, 52})
	c.mountain(main, 100, 42, 5, rock{"#8a6f8c", "#5a4f72", glowLit, glowSh})
	fg := c.ridge(1.0, [2]int{0, 78}, [2]int{40, 72}, [2]int{80, 80}, [2]int{120, 74}, [2]int{159, 79})
	c.fillBelow(fg, 100, "#2a3550")
	for x := 4; x < 160; x += 7 + c.rng.IntN(6) {
		c.pine(x, fg[x]+3, 6+c.rng.IntN(5), "#1d2a40", "#16203a")
	}
	c.rect(118, 8, 1, 1, "#fff4d6")
	c.rect(20, 12, 1, 1, "#e8dcf0")
	c.rect(64, 6, 1, 1, "#e8dcf0")
	return c
}

func sceneHut() *canvas {
	c := newCanvas(160, 100, 2)
	c.bands(0, 55, sky0, sky1, sky2, sky3)
	clouds(c, 3, 18)
	m := c.ridge(1.4, [2]int{0, 42}, [2]int{35, 22}, [2]int{60, 34}, [2]int{100, 16}, [2]int{140, 34}, [2]int{159, 30})
	c.mountain(m, 100, 34, 4, rock{rockLit, rockSh, snowLit, snowSh})
	hill := c.ridge(0.8, [2]int{0, 60}, [2]int{60, 56}, [2]int{110, 62}, [2]int{159, 58})
	c.fillBelow(hill, 100, grass1)
	near := c.ridge(0.6, [2]int{0, 74}, [2]int{80, 70}, [2]int{159, 76})
	c.fillBelow(near, 100, grass0)
	meadowDots(c, 72, grass3, "#f8fafc", "#fcd34d", grass2)
	// The hut: stone base, wooden walls, a roof with snow on its ridge.
	c.rect(56, 62, 44, 8, stone0)
	for i := 0; i < 14; i++ {
		c.set(56+c.rng.IntN(44), 62+c.rng.IntN(8), stone1)
	}
	c.rect(58, 50, 40, 12, wood0)
	for y := 52; y < 62; y += 3 {
		c.rect(58, y, 40, 1, wood1)
	}
	c.poly("#4b5563", [2]float64{52, 51}, [2]float64{78, 36}, [2]float64{104, 51})
	c.poly("#6b7280", [2]float64{78, 36}, [2]float64{104, 51}, [2]float64{96, 51}, [2]float64{78, 40})
	c.poly(snowLit, [2]float64{70, 41}, [2]float64{78, 36}, [2]float64{86, 41}, [2]float64{78, 39})
	c.rect(64, 54, 6, 5, "#fef3c7")
	c.rect(63, 54, 1, 5, red0)
	c.rect(70, 54, 1, 5, red0)
	c.rect(86, 54, 6, 5, "#fef3c7")
	c.rect(85, 54, 1, 5, red0)
	c.rect(92, 54, 1, 5, red0)
	c.rect(76, 55, 6, 15, wood1)
	c.rect(90, 32, 3, 9, stone1)
	for i, p := range [][2]int{{91, 29}, {93, 26}, {92, 22}, {95, 19}} {
		c.ellipse(float64(p[0]), float64(p[1]), 2+float64(i)*0.5, 1.5+float64(i)*0.4, "#e5e7eb")
	}
	// A path down the meadow.
	for y := 70; y < 100; y++ {
		w := 2 + (y-70)/5
		x := 79 + int(6*math.Sin(float64(y)/7))
		c.rect(x-w/2, y, w, 1, "#c9b48a")
	}
	return c
}

func sceneSignpost() *canvas {
	c := newCanvas(160, 100, 3)
	c.bands(0, 60, "#5fb3df", sky0, sky1, sky2)
	clouds(c, 4, 22)
	h := c.ridge(1.0, [2]int{0, 50}, [2]int{50, 40}, [2]int{100, 48}, [2]int{159, 36})
	c.mountain(h, 100, 40, 3, rock{"#7f9bb5", "#64809b", snowLit, snowSh})
	g := c.ridge(0.7, [2]int{0, 66}, [2]int{70, 60}, [2]int{159, 68})
	c.fillBelow(g, 100, grass1)
	g2 := c.ridge(0.5, [2]int{0, 80}, [2]int{159, 76})
	c.fillBelow(g2, 100, grass0)
	meadowDots(c, 70, grass3, grass2, "#fde68a", "#f8fafc")
	// Post with three yellow arrow plates.
	c.rect(76, 30, 3, 60, "#8a8f98")
	c.rect(78, 30, 1, 60, "#6b7079")
	plates := []struct{ y, dir int }{{32, 1}, {41, -1}, {50, 1}}
	for _, p := range plates {
		x0 := 79
		if p.dir < 0 {
			x0 = 46
		}
		c.rect(x0, p.y, 30, 7, "#f2c230")
		c.rect(x0, p.y+6, 30, 1, "#c99a12")
		tip := x0 + 30
		if p.dir < 0 {
			tip = x0 - 1
		}
		for k := 0; k < 4; k++ {
			c.rect(tip+p.dir*k, p.y+k, 1, 7-2*k, "#f2c230")
		}
		c.rect(x0+4, p.y+2, 14, 1, "#3f3f46")
		c.rect(x0+4, p.y+4, 9, 1, "#3f3f46")
		c.rect(x0+22, p.y+2, 4, 1, "#3f3f46")
	}
	c.rect(74, 26, 7, 4, "#f2c230")
	c.rect(75, 27, 5, 1, "#3f3f46")
	return c
}

func sceneLake() *canvas {
	c := newCanvas(160, 100, 4)
	c.bands(0, 56, "#9ccbe6", sky1, sky2, sky3)
	clouds(c, 2, 14)
	m := c.ridge(1.5, [2]int{0, 40}, [2]int{30, 20}, [2]int{55, 32}, [2]int{82, 12}, [2]int{112, 30}, [2]int{140, 22}, [2]int{159, 38})
	c.mountain(m, 56, 30, 4, rock{rockLit, rockSh, snowLit, snowSh})
	f := c.ridge(0.9, [2]int{0, 50}, [2]int{159, 52})
	c.fillBelow(f, 56, forest0)
	for x := 2; x < 160; x += 3 + c.rng.IntN(3) {
		c.pine(x, f[x]+3, 5+c.rng.IntN(5), forest0, forest1)
	}
	c.rect(0, 56, 160, 2, "#355a3a")
	// The lake mirrors the scene above its shore, darker, with ripples.
	for y := 58; y < 100; y++ {
		src := 56 - (y - 58) - 1
		for x := 0; x < 160; x++ {
			col := c.get(x, max(src, 0))
			if src < 0 {
				col = sky3
			}
			c.set(x, y, mirror(col))
		}
	}
	for i := 0; i < 70; i++ {
		x, y := c.rng.IntN(150), 60+c.rng.IntN(40)
		c.rect(x, y, 3+c.rng.IntN(8), 1, water2)
	}
	return c
}

// mirror darkens and cools a colour for its reflection in water.
func mirror(col string) string {
	var r, g, b int
	if _, err := fmt.Sscanf(col, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return water0
	}
	r, g, b = r*62/100, g*72/100+18, b*78/100+30
	return fmt.Sprintf("#%02x%02x%02x", min(r, 255), min(g, 255), min(b, 255))
}

func sceneTent() *canvas {
	c := newCanvas(160, 100, 5)
	c.bands(0, 70, night0, night1, night2, "#2a3a5e")
	for i := 0; i < 90; i++ {
		col := "#94a3b8"
		if i%7 == 0 {
			col = "#fde68a"
		} else if i%3 == 0 {
			col = "#e2e8f0"
		}
		c.set(c.rng.IntN(160), c.rng.IntN(60), col)
	}
	c.ellipse(128, 16, 6, 6, "#f1f5f9")
	c.ellipse(131, 14, 6, 6, night1)
	m := c.ridge(1.3, [2]int{0, 58}, [2]int{40, 44}, [2]int{70, 52}, [2]int{110, 40}, [2]int{159, 56})
	c.mountain(m, 100, 46, 3, rock{"#3a4766", "#2c3754", "#c7d2e6", "#8e9cbc"})
	g := c.ridge(0.5, [2]int{0, 76}, [2]int{159, 74})
	c.fillBelow(g, 100, "#1c2a24")
	// A tent with light inside.
	c.poly(teal700, [2]float64{58, 84}, [2]float64{80, 60}, [2]float64{102, 84})
	c.poly(teal500, [2]float64{80, 60}, [2]float64{102, 84}, [2]float64{92, 84})
	c.poly("#fcd58a", [2]float64{73, 84}, [2]float64{80, 70}, [2]float64{87, 84})
	c.poly("#f9b25a", [2]float64{80, 70}, [2]float64{87, 84}, [2]float64{83, 84})
	c.rect(56, 84, 48, 1, "#0f1a16")
	for i := 0; i < 30; i++ {
		x := 66 + c.rng.IntN(30)
		c.set(x, 85+c.rng.IntN(4), "#3b3226")
	}
	for x := 2; x < 50; x += 6 + c.rng.IntN(4) {
		c.pine(x, g[x]+2, 8+c.rng.IntN(6), "#14211c", "#0e1814")
	}
	for x := 118; x < 160; x += 6 + c.rng.IntN(4) {
		c.pine(x, g[x]+2, 8+c.rng.IntN(6), "#14211c", "#0e1814")
	}
	return c
}

// studio is the backdrop of a product photo: a pale wall, a floor, a
// shadow.
func studio(seed uint64, cx, shadowW float64) *canvas {
	c := newCanvas(160, 100, seed)
	c.rect(0, 0, 160, 72, "#e9eef3")
	c.rect(0, 72, 160, 28, "#cfd8e2")
	c.rect(0, 72, 160, 1, "#c3cdd9")
	c.ellipse(cx, 86, shadowW, 4, "#b3bfcc")
	return c
}

func sceneBoots() *canvas {
	c := studio(6, 80, 52)
	boot := func(x float64, body, dark string) {
		c.poly(body, [2]float64{x, 40}, [2]float64{x + 16, 40}, [2]float64{x + 18, 62}, [2]float64{x + 34, 70}, [2]float64{x + 36, 80}, [2]float64{x - 2, 80}, [2]float64{x - 2, 60})
		c.poly(dark, [2]float64{x + 18, 62}, [2]float64{x + 34, 70}, [2]float64{x + 36, 80}, [2]float64{x + 20, 80})
		c.rect(int(x)-3, 80, 40, 4, "#2b2f36")
		for k := 0; k < 40; k += 3 {
			c.rect(int(x)-3+k, 83, 2, 1, "#14171c")
		}
		c.rect(int(x)-1, 38, 18, 3, "#4b5563")
		for k := 0; k < 5; k++ {
			c.rect(int(x)+14, 44+k*4, 6, 1, amber300)
			c.set(int(x)+13, 44+k*4, "#e5e7eb")
		}
		c.rect(int(x)+2, 66, 14, 2, teal500)
	}
	boot(40, "#8a5a3c", "#6d4329")
	boot(86, "#946446", "#734a30")
	return c
}

func sceneBackpack() *canvas {
	c := studio(7, 80, 34)
	c.poly(teal700, [2]float64{58, 24}, [2]float64{102, 24}, [2]float64{108, 84}, [2]float64{52, 84})
	c.ellipse(80, 26, 22, 8, teal700)
	c.poly(teal800, [2]float64{96, 24}, [2]float64{102, 24}, [2]float64{108, 84}, [2]float64{100, 84})
	c.ellipse(80, 22, 18, 6, teal600)
	c.rect(64, 20, 32, 2, teal500)
	c.poly(teal600, [2]float64{62, 50}, [2]float64{98, 50}, [2]float64{100, 78}, [2]float64{60, 78})
	c.rect(62, 50, 36, 2, teal800)
	c.rect(66, 60, 28, 1, "#0b3b37")
	c.rect(78, 52, 4, 6, slate300)
	c.rect(70, 30, 2, 52, "#1f2937")
	c.rect(88, 30, 2, 52, "#1f2937")
	c.rect(70, 40, 20, 2, "#1f2937")
	c.rect(74, 39, 12, 4, slate500)
	c.rect(76, 12, 8, 2, "#1f2937")
	for k := 0; k < 4; k++ {
		c.rect(54+k, 66+k*4, 2, 2, amber300)
	}
	return c
}

func sceneGlacier() *canvas {
	c := newCanvas(160, 100, 8)
	c.bands(0, 44, "#6ea8d6", "#9ccbe6", "#cfe6f4")
	m := c.ridge(1.6, [2]int{0, 30}, [2]int{40, 14}, [2]int{80, 26}, [2]int{120, 10}, [2]int{159, 28})
	c.mountain(m, 100, 40, 9, rock{rockLit, rockSh, snowLit, snowSh})
	ice := c.ridge(0.8, [2]int{0, 50}, [2]int{80, 46}, [2]int{159, 52})
	c.fillBelow(ice, 100, "#d6ecf5")
	for y := 50; y < 100; y += 1 {
		if y%6 == 0 {
			x := c.rng.IntN(150)
			w := 6 + c.rng.IntN(18)
			c.rect(x, y, w, 1, "#7fb6cf")
			c.rect(x+1, y+1, w-2, 1, "#4f8fb0")
		}
	}
	c.bands(88, 100, "#d6ecf5", "#bfe0ee")
	// A rope team of three crossing the ice.
	for i, x := range []int{60, 72, 84} {
		y := 70 - i*2
		c.rect(x, y-4, 2, 4, []string{red0, teal600, amber500}[i])
		c.rect(x, y-6, 2, 2, "#f1d3b5")
		c.rect(x, y, 1, 2, "#334155")
		c.rect(x+1, y, 1, 2, "#334155")
		if i < 2 {
			for k := 2; k < 12; k++ {
				c.set(x+k, y-2-(k*2)/12, "#fbbf24")
			}
		}
	}
	return c
}

func sceneCableCar() *canvas {
	c := newCanvas(160, 100, 9)
	c.bands(0, 60, sky0, sky1, sky2)
	clouds(c, 3, 20)
	m := c.ridge(1.4, [2]int{0, 30}, [2]int{50, 46}, [2]int{100, 24}, [2]int{159, 12})
	c.mountain(m, 100, 36, 4, rock{rockLit, rockSh, snowLit, snowSh})
	v := c.ridge(0.7, [2]int{0, 70}, [2]int{80, 84}, [2]int{159, 66})
	c.fillBelow(v, 100, forest0)
	for x := 1; x < 160; x += 3 + c.rng.IntN(3) {
		c.pine(x, v[x]+4, 6+c.rng.IntN(6), forest0, forest1)
	}
	// Cable from the valley to the top, and a cabin hanging on it.
	for x := 0; x < 160; x++ {
		y := 60 - x*44/160
		c.set(x, y, "#334155")
		c.set(x, y+3, "#475569")
	}
	cx, cy := 92, 60-92*44/160
	c.rect(cx, cy, 1, 6, "#334155")
	c.rect(cx-5, cy+6, 11, 2, "#334155")
	c.rect(cx-7, cy+8, 15, 11, red0)
	c.rect(cx-7, cy+8, 15, 1, red1)
	c.rect(cx-5, cy+10, 4, 4, "#dbeafe")
	c.rect(cx+1, cy+10, 4, 4, "#dbeafe")
	c.rect(cx-7, cy+17, 15, 2, "#7f1d1d")
	return c
}

func sceneCows() *canvas {
	c := newCanvas(160, 100, 10)
	c.bands(0, 50, "#7ec0e6", sky1, sky2)
	clouds(c, 3, 16)
	m := c.ridge(1.2, [2]int{0, 34}, [2]int{60, 18}, [2]int{100, 30}, [2]int{159, 22})
	c.mountain(m, 100, 32, 4, rock{rockLit, rockSh, snowLit, snowSh})
	g := c.ridge(0.6, [2]int{0, 54}, [2]int{159, 50})
	c.fillBelow(g, 100, grass1)
	g2 := c.ridge(0.6, [2]int{0, 66}, [2]int{159, 70})
	c.fillBelow(g2, 100, grass0)
	meadowDots(c, 56, grass3, grass2, "#f8fafc", "#fde68a", "#c4b5fd")
	cow := []string{
		"....ww........",
		"...wbbwwwwbb..",
		"bb.wwwbbwwwwbw",
		"bwwwwwbbbwwwwt",
		".wwwbwwwwwbww.",
		"..ppwwwwwwww..",
		"...y.w.w.ww.w.",
		".....w.w..w.w.",
	}
	cols := map[rune]string{'w': "#f8f5ee", 'b': "#6b4a32", 'p': "#e8b4a0", 'y': amber300, 't': "#6b4a32"}
	c.sprite(36, 70, cow, cols)
	c.sprite(98, 76, cow, cols)
	small := []string{"..ww.....", "bwwwbbwwt", "wwbwwwww.", ".y.w.w.w."}
	c.sprite(70, 58, small, cols)
	// A fence along the meadow.
	for x := 0; x < 160; x += 12 {
		c.rect(x, 86, 1, 8, wood1)
	}
	c.rect(0, 88, 160, 1, wood0)
	c.rect(0, 91, 160, 1, wood0)
	return c
}

func sceneJacket() *canvas {
	c := studio(11, 80, 30)
	c.rect(79, 8, 2, 6, slate500)
	c.poly(slate500, [2]float64{62, 18}, [2]float64{80, 13}, [2]float64{98, 18}, [2]float64{97, 19}, [2]float64{80, 15}, [2]float64{63, 19})
	body := "#d9653b"
	dark := "#b54d28"
	c.poly(body, [2]float64{64, 18}, [2]float64{96, 18}, [2]float64{112, 30}, [2]float64{118, 66}, [2]float64{108, 68}, [2]float64{104, 40}, [2]float64{104, 82}, [2]float64{56, 82}, [2]float64{56, 40}, [2]float64{52, 68}, [2]float64{42, 66}, [2]float64{48, 30})
	c.poly(dark, [2]float64{104, 40}, [2]float64{104, 82}, [2]float64{96, 82}, [2]float64{98, 40})
	c.poly(dark, [2]float64{112, 30}, [2]float64{118, 66}, [2]float64{108, 68}, [2]float64{106, 34})
	c.poly(dark, [2]float64{70, 16}, [2]float64{90, 16}, [2]float64{88, 24}, [2]float64{72, 24})
	c.rect(79, 22, 2, 60, "#3f3f46")
	c.rect(62, 52, 12, 1, dark)
	c.rect(86, 52, 12, 1, dark)
	c.rect(66, 30, 4, 4, teal600)
	c.rect(42, 64, 10, 3, slate600)
	c.rect(108, 66, 10, 3, slate600)
	return c
}

func sceneCampfire() *canvas {
	c := newCanvas(160, 100, 12)
	c.bands(0, 64, dawn0, dawn1, dawn2, dawn3)
	m := c.ridge(1.2, [2]int{0, 50}, [2]int{60, 38}, [2]int{110, 48}, [2]int{159, 40})
	c.mountain(m, 100, 40, 3, rock{"#4a4a6a", "#3a3a58", "#d6c3d6", "#9890b8"})
	g := c.ridge(0.4, [2]int{0, 70}, [2]int{159, 68})
	c.fillBelow(g, 100, "#2a2a2e")
	c.ellipse(80, 84, 40, 10, "#3a3330")
	c.ellipse(80, 84, 24, 6, "#5a4636")
	// Logs, flames and sparks.
	c.poly(wood1, [2]float64{60, 88}, [2]float64{64, 84}, [2]float64{98, 80}, [2]float64{96, 84})
	c.poly(wood0, [2]float64{62, 80}, [2]float64{66, 78}, [2]float64{100, 86}, [2]float64{96, 89})
	c.poly(fire0, [2]float64{68, 82}, [2]float64{74, 66}, [2]float64{78, 72}, [2]float64{82, 58}, [2]float64{87, 70}, [2]float64{90, 64}, [2]float64{93, 82})
	c.poly(fire1, [2]float64{72, 82}, [2]float64{77, 72}, [2]float64{81, 66}, [2]float64{85, 74}, [2]float64{89, 82})
	c.poly(fire2, [2]float64{77, 82}, [2]float64{81, 72}, [2]float64{85, 82})
	for i := 0; i < 16; i++ {
		c.set(70+c.rng.IntN(22), 36+c.rng.IntN(24), []string{fire1, fire2, fire0}[i%3])
	}
	for x := 4; x < 160; x += 8 + c.rng.IntN(6) {
		if x > 40 && x < 120 {
			continue
		}
		c.pine(x, g[x]+2, 10+c.rng.IntN(8), "#1d1d24", "#15151b")
	}
	// A mug and a stone to sit on.
	c.ellipse(118, 86, 8, 4, stone1)
	c.rect(40, 82, 4, 5, teal600)
	c.rect(44, 83, 1, 2, teal600)
	return c
}

func sceneWarehouse() *canvas {
	c := newCanvas(160, 100, 13)
	c.bands(0, 100, "#e5e9ee", "#d7dde4", "#c9d1da")
	c.rect(0, 84, 160, 16, "#b8c2cc")
	for x := 0; x < 160; x += 16 {
		c.rect(x, 84, 1, 16, "#aab4bf")
	}
	box := []string{"#c8955c", "#b88349", "#d6a46d"}
	for s, sx := range []int{8, 60, 112} {
		c.rect(sx, 14, 3, 72, "#3b82a0")
		c.rect(sx+39, 14, 3, 72, "#3b82a0")
		for _, sy := range []int{30, 52, 74} {
			c.rect(sx, sy, 42, 3, amber500)
			for bx := sx + 4; bx < sx+38; {
				w := 6 + c.rng.IntN(8)
				h := 6 + c.rng.IntN(10)
				if bx+w > sx+39 {
					break
				}
				col := box[(bx+s)%3]
				c.rect(bx, sy-h, w, h, col)
				c.rect(bx, sy-h, w, 1, "#e3bb88")
				c.rect(bx+w/2, sy-h, 1, h, "#a46f3a")
				if c.rng.IntN(3) == 0 {
					c.rect(bx+1, sy-h+2, 3, 2, slate50)
				}
				bx += w + 1
			}
		}
	}
	// A pallet truck and a teal crate in the aisle.
	c.rect(52, 86, 3, 8, slate700)
	c.rect(52, 92, 18, 2, slate700)
	c.rect(56, 82, 14, 10, teal600)
	c.rect(56, 82, 14, 1, teal400)
	for _, l := range []int{6, 30, 54, 78, 102, 126, 150} {
		c.rect(l, 0, 6, 2, "#fff7d6")
		c.rect(l+1, 2, 4, 1, "#fde68a")
	}
	return c
}

// miniScene is the landscape banner of the shop's home page.
func miniScene() *canvas {
	c := newCanvas(120, 40, 21)
	c.bands(0, 30, dawn1, dawn2, dawn3, dawn4, dawn5)
	m := c.ridge(1.2, [2]int{0, 26}, [2]int{30, 12}, [2]int{55, 20}, [2]int{80, 6}, [2]int{119, 22})
	c.mountain(m, 40, 20, 3, rock{"#8a6f8c", "#5a4f72", glowLit, glowSh})
	f := c.ridge(0.6, [2]int{0, 34}, [2]int{119, 32})
	c.fillBelow(f, 40, "#2a3550")
	return c
}
