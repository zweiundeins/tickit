package main

import (
	"fmt"
)

// browser paints a desktop browser window with the shop's header and
// returns the y where the page content starts.
func browser(d *doc, url string, shopHeader bool) float64 {
	w, h := float64(d.w), float64(d.h)
	d.rect(0, 0, w, h, 10, slate200, "", 0)
	d.rect(0, 0, w, 40, 10, slate200, "", 0)
	for i, c := range []string{rose300, amber300, teal300} {
		d.circle(20+float64(i)*16, 20, 5, c, "", 0)
	}
	d.rect(80, 10, w-160, 20, 10, white, "", 0)
	d.text(96, 24, url, 12, slate500, mono, "start", 400, 0)
	d.rect(0, 40, w, h-40, 0, white, "", 0)
	if !shopHeader {
		return 40
	}
	d.rect(0, 40, w, 56, 0, white, "", 0)
	d.line(0, 96, w, 96, slate200, 1, "")
	logo(d, 28, 56)
	x := 210.0
	for _, n := range []string{"Hike", "Climb", "Camp", "Snow", "Sale"} {
		col := slate700
		if n == "Sale" {
			col = rose700
		}
		d.text(x, 74, n, 14, col, sans, "start", 600, 0)
		x += 70
	}
	d.rect(w-330, 56, 200, 26, 13, slate100, slate200, 1)
	icon(d, "search", w-320, 61, 2, map[rune]string{'#': slate500, 'o': slate300})
	d.text(w-296, 74, "Search gear", 12, slate400, sans, "start", 400, 0)
	icon(d, "person", w-110, 61, 2, map[rune]string{'#': slate600, 'o': slate600})
	icon(d, "box", w-70, 61, 2, map[rune]string{'#': slate600, 'o': teal500})
	d.circle(w-50, 60, 7, teal600, "", 0)
	d.text(w-50, 64, "2", 10, white, sans, "middle", 700, 0)
	return 96
}

func logo(d *doc, x, y float64) {
	icon(d, "mark", x, y, 3, map[rune]string{'#': teal700, 'o': amber500})
	d.text(x+32, y+18, "alpenglow", 20, slate900, sans, "start", 700, -0.5)
}

// bar is a grey line standing for text.
func bar(d *doc, x, y, w float64) { d.rect(x, y, w, 8, 4, slate200, "", 0) }

func button(d *doc, x, y, w, h float64, t string, primary bool) {
	if primary {
		d.rect(x, y, w, h, 6, teal600, "", 0)
		d.rect(x, y+h-3, w, 3, 0, teal800, "", 0)
		d.text(x+w/2, y+h/2+5, t, 14, white, sans, "middle", 600, 0)
		return
	}
	d.rect(x, y, w, h, 6, white, slate300, 1)
	d.text(x+w/2, y+h/2+5, t, 14, slate700, sans, "middle", 600, 0)
}

func field(d *doc, x, y, w float64, label, value string) {
	d.text(x, y, label, 12, slate600, sans, "start", 600, 0)
	d.rect(x, y+6, w, 34, 6, white, slate300, 1)
	col := slate900
	if value == "" {
		value, col = "Required", slate400
	}
	d.text(x+12, y+28, value, 13, col, sans, "start", 400, 0)
}

// productImage embeds a small pixel scene as the picture of a product.
func productImage(d *doc, c *canvas, x, y, w float64) {
	h := w * float64(c.h) / float64(c.w)
	d.raw(`<svg x="%s" y="%s" width="%s" height="%s" viewBox="0 0 %d %d" shape-rendering="crispEdges">%s</svg>`, num(x), num(y), num(w), num(h), c.w, c.h, c.paths(1, 0, 0))
}

// place draws a scene into a box, cropped to viewBox; a scene drawn
// twice in one image is stored once.
func place(d *doc, c *canvas, key string, x, y, w, h float64, viewBox string) {
	if d.defs == nil {
		d.defs = map[string]bool{}
	}
	if !d.defs[key] {
		d.defs[key] = true
		d.raw(`<defs><g id="%s" shape-rendering="crispEdges">%s</g></defs>`, key, c.paths(1, 0, 0))
	}
	d.raw(`<svg x="%s" y="%s" width="%s" height="%s" viewBox="%s" preserveAspectRatio="xMidYMid slice"><use href="#%s"/></svg>`, num(x), num(y), num(w), num(h), viewBox, key)
}

func card(d *doc, x, y float64, c *canvas, name, price string, sale bool) {
	d.rect(x, y, 200, 260, 8, white, slate200, 1)
	productImage(d, c, x+1, y+1, 198)
	d.text(x+14, y+150, name, 14, slate900, sans, "start", 600, 0)
	d.text(x+14, y+170, "Alpenglow", 12, slate500, sans, "start", 400, 0)
	col := slate900
	if sale {
		col = rose700
		d.rect(x+10, y+10, 52, 20, 4, rose500, "", 0)
		d.text(x+36, y+24, "-20 %", 11, white, sans, "middle", 700, 0)
	}
	d.text(x+14, y+200, price, 15, col, sans, "start", 700, 0)
	for i := 0; i < 5; i++ {
		c := amber500
		if i == 4 {
			c = slate300
		}
		d.rect(x+14+float64(i)*12, y+214, 8, 8, 1, c, "", 0)
	}
	d.text(x+80, y+222, "(48)", 11, slate500, sans, "start", 400, 0)
}

func shotShopHome(title string) string {
	d := newDoc(1000, 680, title)
	y := browser(d, "shop.example.com", true)
	productImage(d, miniScene(), 0, y, 1000)
	d.raw(`<rect x="40" y="%s" width="400" height="150" rx="10" fill="%s" opacity="0.88"/>`, num(y+60), slate900)
	d.text(64, y+104, "First Snow Sale", 30, white, sans, "start", 700, 0)
	d.text(64, y+132, "Up to 30 % off winter hiking gear, this week only.", 14, slate200, sans, "start", 400, 0)
	button(d, 64, y+150, 150, 40, "Shop the sale", true)
	y += 334
	d.text(40, y+30, "New this week", 18, slate900, sans, "start", 700, 0)
	cards := []struct {
		c       *canvas
		k, n, p string
		sale    bool
	}{
		{sceneBackpack(), "pack", "Firn 30 backpack", "CHF 149.00", false},
		{sceneBoots(), "boots", "Gratwander boots", "CHF 183.20", true},
		{sceneJacket(), "jacket", "Nebel rain shell", "CHF 229.00", false},
		{sceneTent(), "tent", "Biwak 2 tent", "CHF 399.00", true},
	}
	for i, c := range cards {
		cardSmall(d, 40+float64(i)*235, y+50, c.c, c.k, c.n, c.p, c.sale)
	}
	return d.String()
}

func cardSmall(d *doc, x, y float64, c *canvas, key, name, price string, sale bool) {
	d.rect(x, y, 215, 230, 8, white, slate200, 1)
	place(d, c, key, x+1, y+1, 213, 134, "0 0 160 100")
	d.text(x+14, y+160, name, 14, slate900, sans, "start", 600, 0)
	col := slate900
	if sale {
		col = rose700
		d.rect(x+10, y+10, 52, 20, 4, rose500, "", 0)
		d.text(x+36, y+24, "-20 %", 11, white, sans, "middle", 700, 0)
	}
	d.text(x+14, y+184, price, 15, col, sans, "start", 700, 0)
	for i := 0; i < 5; i++ {
		cc := amber500
		if i == 4 {
			cc = slate300
		}
		d.rect(x+14+float64(i)*12, y+200, 8, 8, 1, cc, "", 0)
	}
	d.text(x+80, y+208, "(48)", 11, slate500, sans, "start", 400, 0)
}

func shotProduct(title string) string {
	d := newDoc(1000, 640, title)
	y := browser(d, "shop.example.com/p/firn-30-backpack", true)
	d.text(40, y+30, "Hike  /  Backpacks  /  Firn 30", 12, slate500, sans, "start", 400, 0)
	d.raw(`<svg x="40" y="%s" width="460" height="460" viewBox="30 0 100 100"><g shape-rendering="crispEdges">%s</g></svg>`, num(y+50), sceneBackpack().paths(1, 0, 0))
	x := 540.0
	d.text(x, y+80, "Firn 30 backpack", 28, slate900, sans, "start", 700, 0)
	d.text(x, y+108, "30 litres, 1.1 kg, made for long day hikes", 14, slate500, sans, "start", 400, 0)
	d.text(x, y+150, "CHF 149.00", 24, slate900, sans, "start", 700, 0)
	d.text(x, y+190, "Colour: Glacier teal", 13, slate600, sans, "start", 600, 0)
	for i, c := range []string{teal600, slate600, "#d9653b"} {
		d.circle(x+14+float64(i)*34, y+214, 12, c, white, 3)
		if i == 0 {
			d.circle(x+14, y+214, 15, "none", teal700, 1.5)
		}
	}
	d.text(x, y+256, "Back length", 13, slate600, sans, "start", 600, 0)
	for i, s := range []string{"S", "M", "L"} {
		st := false
		if s == "M" {
			st = true
		}
		bx := x + float64(i)*64
		if st {
			d.rect(bx, y+266, 54, 38, 6, teal50, teal600, 2)
			d.text(bx+27, y+290, s, 14, teal800, sans, "middle", 700, 0)
		} else {
			d.rect(bx, y+266, 54, 38, 6, white, slate300, 1)
			d.text(bx+27, y+290, s, 14, slate700, sans, "middle", 600, 0)
		}
	}
	d.text(x+200, y+290, "Size guide", 13, teal700, sans, "start", 600, 0)
	button(d, x, y+330, 300, 46, "Add to cart", true)
	icon(d, "check", x, y+400, 2, map[rune]string{'#': teal600})
	d.text(x+24, y+413, "In stock, ships tomorrow from the warehouse", 13, slate600, sans, "start", 400, 0)
	bar(d, x, y+440, 380)
	bar(d, x, y+458, 340)
	bar(d, x, y+476, 360)
	return d.String()
}

func shotCheckout(title string) string {
	d := newDoc(1000, 700, title)
	y := browser(d, "shop.example.com/checkout", false)
	logo(d, 40, y+20)
	d.text(960, y+42, "Secure checkout", 12, slate500, sans, "end", 400, 0)
	d.line(0, y+70, 1000, y+70, slate200, 1, "")
	y += 100
	section := func(n, t string, yy float64) {
		d.circle(52, yy-5, 12, teal600, "", 0)
		d.text(52, yy, n, 12, white, sans, "middle", 700, 0)
		d.text(74, yy, t, 16, slate900, sans, "start", 700, 0)
	}
	section("1", "Contact and address", y)
	field(d, 40, y+30, 260, "Email", "you@example.com")
	field(d, 320, y+30, 260, "Phone (optional)", "")
	field(d, 40, y+100, 540, "Street and number", "Bergweg 12")
	field(d, 40, y+170, 160, "Postcode", "3800")
	field(d, 220, y+170, 360, "Town", "Interlaken")
	section("2", "Delivery", y+250)
	d.rect(40, y+272, 540, 44, 6, teal50, teal600, 2)
	d.circle(62, y+294, 6, teal600, "", 0)
	d.text(80, y+299, "Standard, 1 to 2 working days", 13, slate900, sans, "start", 600, 0)
	d.text(564, y+299, "free", 13, teal700, sans, "end", 600, 0)
	d.rect(40, y+324, 540, 44, 6, white, slate300, 1)
	d.circle(62, y+346, 6, "none", slate400, 1.5)
	d.text(80, y+351, "Pick up at a partner shop", 13, slate700, sans, "start", 400, 0)
	section("3", "Payment", y+410)
	for i, t := range []string{"Card", "Bank app", "Invoice"} {
		x := 40 + float64(i)*184
		if i == 0 {
			d.rect(x, y+430, 172, 44, 6, teal50, teal600, 2)
		} else {
			d.rect(x, y+430, 172, 44, 6, white, slate300, 1)
		}
		icon(d, "card", x+14, y+444, 2, map[rune]string{'#': slate600, 'o': teal500})
		d.text(x+40, y+457, t, 13, slate800, sans, "start", 600, 0)
	}
	// Summary.
	d.rect(630, y-20, 330, 470, 10, slate50, slate200, 1)
	d.text(654, y+10, "Your order", 16, slate900, sans, "start", 700, 0)
	items := [][2]string{{"Firn 30 backpack, teal, M", "149.00"}, {"Steinbock socks, 2 pairs", "38.00"}, {"Arve fleece, S", "89.00"}}
	for i, it := range items {
		yy := y + 50 + float64(i)*56
		d.rect(654, yy-20, 40, 40, 6, slate200, "", 0)
		d.text(706, yy-2, it[0], 12, slate800, sans, "start", 400, 0)
		d.text(936, yy-2, it[1], 12, slate800, mono, "end", 400, 0)
	}
	d.line(654, y+230, 936, y+230, slate200, 1, "")
	d.text(654, y+256, "Voucher FIRSTSNOW", 12, teal700, sans, "start", 600, 0)
	d.text(936, y+256, "-62.00", 12, teal700, mono, "end", 600, 0)
	d.text(654, y+282, "Delivery", 12, slate600, sans, "start", 400, 0)
	d.text(936, y+282, "0.00", 12, slate600, mono, "end", 400, 0)
	d.text(654, y+320, "Total CHF", 16, slate900, sans, "start", 700, 0)
	d.text(936, y+320, "214.00", 16, slate900, mono, "end", 700, 0)
	button(d, 654, y+350, 282, 48, "Pay CHF 214.00", true)
	d.text(795, y+422, "You can still change everything above.", 11, slate500, sans, "middle", 400, 0)
	return d.String()
}

func shotCartDrawer(title string) string {
	d := newDoc(1000, 640, title)
	y := browser(d, "shop.example.com/c/hike", true)
	for i := 0; i < 4; i++ {
		d.rect(40+float64(i)*235, y+40, 215, 230, 8, slate100, slate200, 1)
	}
	d.raw(`<rect x="0" y="%s" width="1000" height="%s" fill="%s" opacity="0.45"/>`, num(y), num(640-y), slate900)
	d.rect(600, 40, 400, 600, 0, white, "", 0)
	d.text(628, 84, "Cart (2)", 20, slate900, sans, "start", 700, 0)
	d.text(970, 84, "Close", 13, slate500, sans, "end", 400, 0)
	d.line(600, 104, 1000, 104, slate200, 1, "")
	items := []struct {
		c    *canvas
		n, p string
	}{{sceneBoots(), "Gratwander boots, 42", "CHF 183.20"}, {sceneJacket(), "Nebel rain shell, M", "CHF 229.00"}}
	for i, it := range items {
		yy := 124 + float64(i)*110
		place(d, it.c, fmt.Sprintf("s%d", i), 628, yy, 90, 90, "30 0 100 100")
		d.text(734, yy+22, it.n, 14, slate900, sans, "start", 600, 0)
		d.text(734, yy+44, it.p, 13, slate700, sans, "start", 400, 0)
		d.rect(734, yy+56, 90, 28, 6, white, slate300, 1)
		d.text(779, yy+75, "-   1   +", 13, slate700, mono, "middle", 400, 0)
		d.text(970, yy+75, "Remove", 12, slate500, sans, "end", 400, 0)
	}
	d.rect(628, 360, 344, 52, 8, amber100, amber500, 1)
	d.text(644, 382, "Add CHF 37.80 more for free delivery", 13, amber700, sans, "start", 600, 0)
	d.rect(644, 394, 312, 6, 3, white, "", 0)
	d.rect(644, 394, 260, 6, 3, amber500, "", 0)
	d.text(628, 460, "Subtotal", 15, slate700, sans, "start", 400, 0)
	d.text(972, 460, "CHF 412.20", 15, slate900, sans, "end", 700, 0)
	button(d, 628, 484, 344, 48, "Go to checkout", true)
	button(d, 628, 544, 344, 44, "Keep shopping", false)
	return d.String()
}

func shotSearch(title string) string {
	d := newDoc(1000, 680, title)
	y := browser(d, "shop.example.com/search?q=rain+jacket", true)
	d.text(40, y+40, "38 results for \"rain jacket\"", 20, slate900, sans, "start", 700, 0)
	d.text(960, y+40, "Sort: Relevance", 13, slate600, sans, "end", 400, 0)
	groups := []struct {
		t    string
		opts []string
	}{{"Category", []string{"Jackets (24)", "Ponchos (6)", "Trousers (8)"}}, {"Size", []string{"S", "M", "L", "XL"}}, {"Colour", []string{"Teal", "Slate", "Rust"}}}
	yy := y + 80
	for _, g := range groups {
		d.text(40, yy, g.t, 13, slate900, sans, "start", 700, 0)
		yy += 12
		for i, o := range g.opts {
			yy += 26
			fill, st := white, slate300
			if i == 0 {
				fill, st = teal600, teal600
			}
			d.rect(40, yy-12, 14, 14, 3, fill, st, 1)
			d.text(64, yy, o, 13, slate700, sans, "start", 400, 0)
		}
		yy += 36
	}
	jacket, pack, boots := sceneJacket(), sceneBackpack(), sceneBoots()
	scenes := []*canvas{jacket, jacket, pack, jacket, boots, jacket}
	keys := []string{"jacket", "jacket", "pack", "jacket", "boots", "jacket"}
	names := []string{"Nebel rain shell", "Nebel rain shell W", "Firn 30 rain cover", "Föhn light jacket", "Gaiters, waterproof", "Nebel kids shell"}
	for i, c := range scenes {
		x := 240 + float64(i%3)*245
		cy := y + 70 + float64(i/3)*250
		cardSmall(d, x, cy, c, keys[i], names[i], fmt.Sprintf("CHF %d.00", 229-i*31), i == 3)
	}
	return d.String()
}

func shotSizeGuide(title string) string {
	d := newDoc(1000, 640, title)
	y := browser(d, "shop.example.com/p/gratwander-boots", true)
	d.raw(`<rect x="0" y="%s" width="1000" height="%s" fill="%s" opacity="0.45"/>`, num(y), num(640-y), slate900)
	d.rect(180, 130, 640, 460, 12, white, "", 0)
	d.text(212, 176, "Size guide: Gratwander boots", 20, slate900, sans, "start", 700, 0)
	d.text(790, 176, "Close", 13, slate500, sans, "end", 400, 0)
	d.text(212, 204, "Measure your foot in the evening, with the socks you hike in.", 13, slate600, sans, "start", 400, 0)
	head := []string{"EU", "UK", "US", "Foot length"}
	rows := [][]string{{"39", "6", "7", "24.6 cm"}, {"40", "6.5", "7.5", "25.3 cm"}, {"41", "7.5", "8.5", "25.9 cm"}, {"42", "8", "9", "26.6 cm"}, {"43", "9", "10", "27.3 cm"}, {"44", "9.5", "10.5", "27.9 cm"}}
	x0 := []float64{212, 330, 448, 566}
	d.rect(212, 228, 576, 36, 0, slate100, "", 0)
	for i, h := range head {
		d.text(x0[i]+12, 251, h, 13, slate700, sans, "start", 700, 0)
	}
	for r, row := range rows {
		yy := 264 + float64(r)*38
		if r == 3 {
			d.rect(212, yy, 576, 38, 0, teal50, "", 0)
			d.rect(212, yy, 4, 38, 0, teal600, "", 0)
		}
		d.line(212, yy+38, 788, yy+38, slate200, 1, "")
		for i, v := range row {
			d.text(x0[i]+12, yy+25, v, 13, slate800, sans, "start", 400, 0)
		}
	}
	d.text(212, 520, "Between two sizes? Take the larger one: feet swell on long descents.", 13, slate600, sans, "start", 400, 0)
	button(d, 212, 540, 180, 40, "Choose 42", true)
	return d.String()
}

func shotErrorPage(title string) string {
	d := newDoc(1000, 600, title)
	y := browser(d, "shop.example.com/checkout", true)
	c := newCanvas(100, 40, 31)
	c.bands(0, 40, "#e8edf2", "#dfe5ec", "#d3dbe4")
	m := c.ridge(1.3, [2]int{0, 26}, [2]int{30, 12}, [2]int{60, 22}, [2]int{99, 14})
	c.mountain(m, 40, 20, 3, rock{"#b6c0cc", "#a3aebb", "#eef2f6", "#d6dde6"})
	for yy := 18; yy < 40; yy += 3 {
		for x := (yy * 7) % 9; x < 100; x += 4 + yy%5 {
			c.rect(x, yy, 3, 1, "#f1f4f7")
		}
	}
	d.raw(`<svg x="250" y="%s" width="500" height="200" viewBox="0 0 100 40"><g shape-rendering="crispEdges">%s</g></svg>`, num(y+40), c.paths(1, 0, 0))
	d.text(500, y+300, "Lost in the fog", 32, slate900, sans, "middle", 700, 0)
	d.text(500, y+334, "Something went wrong on our side. Your cart is safe, and nobody was charged.", 15, slate600, sans, "middle", 400, 0)
	button(d, 380, y+364, 240, 46, "Back to the cart", true)
	d.text(500, y+446, "Error 500, reference 7f3a-c2", 12, slate400, mono, "middle", 400, 0)
	return d.String()
}

// phone paints a phone with the app's top bar and returns the content's
// top.
func phone(d *doc, dark bool, screen string) float64 {
	bg, fg := white, slate900
	if dark {
		bg, fg = slate900, slate100
	}
	d.rect(0, 0, float64(d.w), float64(d.h), 0, slate100, "", 0)
	d.rect(20, 20, 360, 740, 40, slate800, "", 0)
	d.rect(32, 32, 336, 716, 30, bg, "", 0)
	d.rect(160, 42, 80, 16, 8, slate800, "", 0)
	d.text(60, 54, "08:15", 12, fg, sans, "start", 600, 0)
	d.rect(318, 45, 22, 10, 2, "none", fg, 1.5)
	d.rect(320, 47, 14, 6, 0, fg, "", 0)
	d.text(56, 102, screen, 22, fg, sans, "start", 700, 0)
	return 120
}

func tabBar(d *doc, active int) {
	d.line(32, 690, 368, 690, slate200, 1, "")
	names := []string{"Home", "Search", "Pack", "Orders", "Me"}
	ic := []string{"mark", "search", "box", "card", "person"}
	for i, n := range names {
		x := 66 + float64(i)*67
		col := slate400
		if i == active {
			col = teal600
		}
		icon(d, ic[i], x-8, 700, 2, map[rune]string{'#': col, 'o': col})
		d.text(x, 732, n, 10, col, sans, "middle", 600, 0)
	}
}

func shotAppHome(title string) string {
	d := newDoc(400, 780, title)
	y := phone(d, false, "Good morning")
	d.raw(`<svg x="48" y="%s" width="304" height="150" viewBox="0 0 120 40" preserveAspectRatio="xMidYMid slice"><g shape-rendering="crispEdges">%s</g></svg>`, num(y), miniScene().paths(1, 0, 0))
	d.raw(`<rect x="60" y="%s" width="200" height="56" rx="8" fill="%s" opacity="0.85"/>`, num(y+82), slate900)
	d.text(74, y+106, "First Snow Sale", 16, white, sans, "start", 700, 0)
	d.text(74, y+126, "Starts in 9 days", 12, slate200, sans, "start", 400, 0)
	d.text(48, y+186, "Your next trip", 15, slate900, sans, "start", 700, 0)
	d.rect(48, y+198, 304, 76, 10, teal50, teal600, 1.5)
	d.text(64, y+226, "Hut to hut, 3 days", 14, slate900, sans, "start", 600, 0)
	d.text(64, y+248, "Pack list: 18 of 23 packed", 12, slate600, sans, "start", 400, 0)
	d.rect(64, y+258, 270, 6, 3, white, "", 0)
	d.rect(64, y+258, 211, 6, 3, teal600, "", 0)
	d.text(48, y+310, "Back in stock", 15, slate900, sans, "start", 700, 0)
	for i, c := range []*canvas{sceneBoots(), sceneBackpack()} {
		x := 48 + float64(i)*158
		place(d, c, fmt.Sprintf("s%d", i), x, y+322, 146, 110, "20 0 120 100")
		d.rect(x, y+322, 146, 110, 8, "none", slate200, 1)
		d.text(x, y+452, []string{"Gratwander boots", "Firn 30 backpack"}[i], 12, slate900, sans, "start", 600, 0)
		d.text(x, y+470, []string{"CHF 229.00", "CHF 149.00"}[i], 12, slate600, sans, "start", 400, 0)
	}
	tabBar(d, 0)
	return d.String()
}

func shotAppPackList(title string) string {
	d := newDoc(400, 780, title)
	y := phone(d, false, "Hut to hut, 3 days")
	d.text(56, y+8, "18 of 23 packed  ·  6.8 kg", 13, slate500, sans, "start", 400, 0)
	items := []struct {
		t    string
		done bool
		miss bool
	}{{"Rain shell", true, false}, {"Fleece, warm layer", true, false}, {"Hut sleeping bag liner", false, true}, {"Head torch", true, false}, {"Blister plasters", false, false}, {"Water bottles, 2 x 0.75 l", true, false}, {"Sun hat and cream", false, false}, {"Trekking poles", false, true}, {"Snacks for 3 days", true, false}}
	group := []struct {
		at int
		t  string
	}{{0, "CLOTHING"}, {3, "GEAR"}, {6, "EXTRAS"}}
	yy := y + 40
	for i, it := range items {
		for _, g := range group {
			if g.at == i {
				d.text(56, yy+6, g.t, 11, slate500, mono, "start", 600, 1.5)
				yy += 22
			}
		}
		fill, st := white, slate300
		if it.done {
			fill, st = teal600, teal600
		}
		d.rect(56, yy, 20, 20, 5, fill, st, 1.5)
		if it.done {
			icon(d, "check", 58, yy+2, 2, map[rune]string{'#': white})
		}
		col := slate900
		if it.done {
			col = slate500
		}
		d.text(88, yy+15, it.t, 14, col, sans, "start", 400, 0)
		if it.miss {
			d.rect(274, yy-1, 66, 22, 11, amber100, amber500, 1)
			d.text(307, yy+14, "missing", 11, amber700, sans, "middle", 600, 0)
		}
		yy += 38
	}
	button(d, 56, 620, 288, 48, "Add 2 missing items to cart", true)
	tabBar(d, 2)
	return d.String()
}

func shotAppOrder(title string) string {
	d := newDoc(400, 780, title)
	y := phone(d, false, "Order A-10482")
	d.text(56, y+8, "Placed 3 days ago  ·  CHF 214.00", 13, slate500, sans, "start", 400, 0)
	steps := []struct{ t, s string }{{"Order placed", "Mon 09:12"}, {"Packed at the warehouse", "Mon 16:40"}, {"Handed to Parcelwise", "Tue 07:05"}, {"Out for delivery", "today, 08:02"}, {"Delivered", "expected by 13:00"}}
	for i, s := range steps {
		yy := y + 60 + float64(i)*70
		done := i < 4
		if i < len(steps)-1 {
			col := teal600
			if i >= 3 {
				col = slate300
			}
			d.line(70, yy+12, 70, yy+70, col, 3, "")
		}
		if done {
			d.circle(70, yy, 12, teal600, "", 0)
			icon(d, "check", 62, yy-8, 2, map[rune]string{'#': white})
		} else {
			d.circle(70, yy, 12, white, slate300, 2)
		}
		col := slate900
		if !done {
			col = slate500
		}
		d.text(96, yy+2, s.t, 14, col, sans, "start", 600, 0)
		d.text(96, yy+22, s.s, 12, slate500, sans, "start", 400, 0)
	}
	d.rect(48, y+420, 304, 70, 10, slate50, slate200, 1)
	d.text(64, y+448, "3 items", 13, slate900, sans, "start", 600, 0)
	d.text(64, y+470, "Backpack, socks, fleece", 12, slate500, sans, "start", 400, 0)
	button(d, 48, y+508, 304, 46, "Start a return", false)
	tabBar(d, 3)
	return d.String()
}

func shotBackOffice(title string) string {
	d := newDoc(1000, 620, title)
	y := browser(d, "admin.example.com/orders", false)
	d.rect(0, y, 200, 620-y, 0, slate900, "", 0)
	logo(d, 20, y+18)
	d.raw(`<rect x="0" y="%s" width="200" height="40" fill="%s"/>`, num(y), slate900)
	icon(d, "mark", 20, y+12, 2, map[rune]string{'#': teal400, 'o': amber300})
	d.text(44, y+26, "back office", 14, white, sans, "start", 700, 0)
	for i, n := range []string{"Orders", "Returns", "Products", "Stock", "Vouchers", "Customers"} {
		yy := y + 70 + float64(i)*38
		if i == 0 {
			d.rect(10, yy-22, 180, 32, 6, slate700, "", 0)
		}
		d.text(28, yy, n, 14, slate200, sans, "start", 400, 0)
	}
	d.text(230, y+44, "Orders", 22, slate900, sans, "start", 700, 0)
	d.rect(700, y+22, 270, 32, 6, white, slate300, 1)
	d.text(714, y+43, "Search order, email, SKU", 12, slate400, sans, "start", 400, 0)
	head := []string{"Order", "Placed", "Customer", "Items", "Total", "Status"}
	xs := []float64{230, 340, 460, 640, 720, 830}
	d.rect(220, y+76, 760, 34, 0, slate100, "", 0)
	for i, h := range head {
		d.text(xs[i], y+98, h, 12, slate600, sans, "start", 700, 0)
	}
	rows := [][]string{
		{"A-10491", "09:58", "customer 4471", "2", "412.20", "Paid"},
		{"A-10490", "09:41", "customer 1180", "1", "38.00", "Paid"},
		{"A-10489", "09:12", "customer 3302", "4", "618.50", "Packing"},
		{"A-10488", "08:57", "customer 0921", "1", "229.00", "Payment pending"},
		{"A-10487", "08:30", "customer 5520", "3", "183.20", "Shipped"},
		{"A-10486", "yesterday", "customer 2203", "2", "96.00", "Shipped"},
		{"A-10485", "yesterday", "customer 7712", "1", "149.00", "Return requested"},
		{"A-10484", "yesterday", "customer 6610", "5", "702.00", "Shipped"},
	}
	for r, row := range rows {
		yy := y + 110 + float64(r)*46
		d.line(220, yy+46, 980, yy+46, slate200, 1, "")
		for i, v := range row {
			if i == 5 {
				fill, fg := teal50, teal800
				switch v {
				case "Payment pending":
					fill, fg = amber100, amber700
				case "Return requested":
					fill, fg = rose100, rose700
				case "Shipped":
					fill, fg = slate100, slate700
				}
				w := float64(len(v))*7 + 18
				d.rect(xs[i], yy+12, w, 22, 11, fill, "", 0)
				d.text(xs[i]+w/2, yy+27, v, 11, fg, sans, "middle", 600, 0)
				continue
			}
			font := sans
			if i == 0 || i == 4 {
				font = mono
			}
			d.text(xs[i], yy+28, v, 13, slate800, font, "start", 400, 0)
		}
	}
	return d.String()
}

func shotOpsDashboard(title string) string {
	d := newDoc(1000, 620, title)
	d.rect(0, 0, 1000, 620, 10, slate900, "", 0)
	icon(d, "mark", 24, 20, 2, map[rune]string{'#': teal400, 'o': amber300})
	d.text(48, 34, "summit, production, last 6 hours", 14, slate200, mono, "start", 600, 0)
	tiles := []struct {
		t, v, s string
		col     string
	}{{"Requests", "412 /s", "+8 % on last week", teal400}, {"p95 latency", "131 ms", "budget 250 ms", teal400}, {"Errors", "0.08 %", "budget 0.5 %", teal400}, {"Stock sync", "14 min ago", "guard passed", amber300}}
	for i, t := range tiles {
		x := 24 + float64(i)*240
		d.rect(x, 60, 224, 110, 10, slate800, slate700, 1)
		d.text(x+16, 88, t.t, 12, slate400, sans, "start", 600, 0)
		d.text(x+16, 128, t.v, 28, white, sans, "start", 700, 0)
		d.text(x+16, 154, t.s, 12, t.col, sans, "start", 400, 0)
	}
	spark := func(x, y, w, h float64, vals []float64, col, name string) {
		d.rect(x, y, w, h, 10, slate800, slate700, 1)
		d.text(x+16, y+28, name, 12, slate400, sans, "start", 600, 0)
		max := 0.0
		for _, v := range vals {
			if v > max {
				max = v
			}
		}
		bw := (w - 32) / float64(len(vals))
		for i, v := range vals {
			bh := (h - 60) * v / max
			d.rect(x+16+float64(i)*bw, y+h-16-bh, bw-2, bh, 0, col, "", 0)
		}
	}
	reqs := []float64{30, 32, 31, 35, 40, 44, 52, 61, 66, 70, 72, 74, 71, 69, 72, 75, 78, 80, 77, 79, 81, 84, 86, 82, 80, 83, 85, 88, 90, 87, 85, 84, 86, 89, 92, 94}
	spark(24, 190, 468, 200, reqs, teal500, "Requests per second")
	lat := []float64{40, 42, 41, 39, 44, 43, 45, 40, 38, 41, 43, 47, 52, 46, 42, 40, 39, 41, 44, 42, 40, 38, 37, 39, 41, 40, 38, 37, 36, 38, 39, 37, 36, 35, 37, 36}
	spark(508, 190, 468, 200, lat, slate400, "p95 latency")
	d.rect(24, 410, 952, 186, 10, slate800, slate700, 1)
	d.text(40, 438, "Recent alerts", 12, slate400, sans, "start", 600, 0)
	alerts := [][3]string{{"resolved", "StockSyncGuardFailed", "4 days ago, 21 minutes"}, {"resolved", "CheckoutErrorRateHigh", "19 days ago, 2 h 52 min"}, {"silenced", "CertificateExpiresSoon", "staging only, until Friday"}}
	for i, a := range alerts {
		yy := 470 + float64(i)*38
		col := teal400
		if a[0] == "silenced" {
			col = amber300
		}
		d.rect(40, yy-14, 76, 22, 11, slate700, "", 0)
		d.text(78, yy+1, a[0], 11, col, sans, "middle", 600, 0)
		d.text(130, yy+1, a[1], 13, slate100, mono, "start", 400, 0)
		d.text(960, yy+1, a[2], 12, slate400, sans, "end", 400, 0)
	}
	return d.String()
}

func shotOrderEmail(title string) string {
	d := newDoc(800, 720, title)
	d.rect(0, 0, 800, 720, 10, slate100, "", 0)
	d.rect(100, 30, 600, 660, 10, white, slate200, 1)
	logo(d, 130, 54)
	productImage(d, miniScene(), 101, 100, 598)
	d.text(130, 340, "Thank you, your gear is on its way", 22, slate900, sans, "start", 700, 0)
	d.text(130, 368, "Order A-10482 was handed to Parcelwise this morning.", 14, slate600, sans, "start", 400, 0)
	button(d, 130, 390, 200, 44, "Track your parcel", true)
	items := [][2]string{{"Firn 30 backpack, teal, M", "CHF 149.00"}, {"Steinbock socks, 2 pairs", "CHF 38.00"}, {"Arve fleece, S", "CHF 89.00"}, {"Voucher FIRSTSNOW", "CHF -62.00"}}
	for i, it := range items {
		yy := 480 + float64(i)*34
		d.line(130, yy-22, 670, yy-22, slate200, 1, "")
		d.text(130, yy, it[0], 13, slate800, sans, "start", 400, 0)
		d.text(670, yy, it[1], 13, slate800, mono, "end", 400, 0)
	}
	d.text(130, 630, "Total", 15, slate900, sans, "start", 700, 0)
	d.text(670, 630, "CHF 214.00", 15, slate900, mono, "end", 700, 0)
	d.text(400, 670, "Alpenglow, outdoor gear from the mountains. Reply to this email if anything is wrong.", 11, slate500, sans, "middle", 400, 0)
	return d.String()
}
