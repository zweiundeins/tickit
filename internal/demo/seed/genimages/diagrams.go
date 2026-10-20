package main

import (
	"fmt"
	"strings"
)

func diagramArchitecture(title string) string {
	d := newDoc(960, 560, title)
	d.frame("Architecture overview")
	d.node(40, 130, 170, 60, boxPlain, "browser", "Web shop", "shop.example.com")
	d.node(40, 250, 170, 60, boxPlain, "phone", "Mobile app", "both platforms")
	d.node(270, 130, 170, 180, boxPlain, "cloud", "Edge", "")
	d.text(310, 245, "CDN for images", 12, slate500, sans, "start", 400, 0)
	d.text(310, 263, "and assets, load", 12, slate500, sans, "start", 400, 0)
	d.text(310, 281, "balancer for the API", 12, slate500, sans, "start", 400, 0)
	d.node(500, 180, 200, 80, boxStrong, "server", "summit", "the Go API, one service")
	d.node(500, 70, 200, 60, boxOurs, "search", "cairn", "search and suggestions")
	d.node(500, 340, 200, 60, boxOurs, "gear", "porter", "jobs: mail, stock, images")
	d.cylinder(770, 180, 160, 84, boxOurs, "PostgreSQL", "orders, catalog, outbox")
	d.node(770, 70, 160, 60, boxPlain, "bucket", "Object storage", "product photos")
	d.node(270, 460, 170, 56, boxOutside, "card", "Paygrove", "payments")
	d.node(500, 460, 200, 56, boxOutside, "box", "Warehouse", "stock files, shipments")
	d.node(770, 460, 160, 56, boxOutside, "mail", "Mail relay", "order emails")
	d.arrow(false, 210, 160, 268, 160)
	d.arrow(false, 210, 280, 268, 280)
	d.arrow(false, 440, 220, 498, 220)
	d.label(470, 205, "HTTPS")
	d.arrow(false, 600, 180, 600, 132)
	d.arrow(false, 700, 222, 768, 222)
	d.label(735, 208, "SQL")
	d.arrow(false, 700, 100, 790, 178)
	d.label(752, 150, "outbox")
	d.arrow(false, 680, 340, 790, 266)
	d.arrow(false, 600, 400, 600, 458)
	d.label(600, 432, "SFTP")
	d.arrow(false, 700, 390, 830, 458)
	d.label(778, 420, "SMTP")
	d.arrow(false, 520, 260, 380, 458)
	d.label(430, 380, "webhooks")
	d.arrow(false, 355, 130, 355, 50, 850, 50, 850, 68)
	d.legend(40, 535, []style{boxStrong, boxOurs, boxOutside}, []string{"core service", "ours", "outside partners"})
	return d.String()
}

func (d *doc) legend(x, y float64, styles []style, names []string) {
	for i, s := range styles {
		d.raw(`<rect x="%s" y="%s" width="14" height="10" rx="2" fill="%s" stroke="%s"`, num(x), num(y-9), s.fill, s.stroke)
		if s.dash {
			d.raw(` stroke-dasharray="3 2"`)
		}
		d.raw(`/>`)
		d.text(x+20, y, names[i], 12, slate600, sans, "start", 400, 0)
		x += 30 + float64(len(names[i]))*7
	}
}

func diagramCheckoutSequence(title string) string {
	d := newDoc(960, 600, title)
	d.frame("Checkout: pay and confirm")
	cols := []float64{120, 350, 590, 830}
	names := []string{"Customer", "Web shop", "summit", "Paygrove"}
	icons := []string{"person", "browser", "server", "card"}
	for i, x := range cols {
		s := boxPlain
		if i == 2 {
			s = boxOurs
		}
		if i == 3 {
			s = boxOutside
		}
		d.node(x-80, 60, 160, 46, s, icons[i], names[i], "")
		d.line(x, 106, x, 570, slate300, 1.5, "4 4")
	}
	type msg struct {
		from, to int
		text     string
		reply    bool
		teal     bool
	}
	msgs := []msg{
		{0, 1, "Pay CHF 214.00", false, false},
		{1, 2, "POST /checkout", false, false},
		{2, 2, "reserve stock, open order", false, false},
		{2, 3, "create payment", false, false},
		{3, 2, "payment URL", true, false},
		{2, 1, "303 to Paygrove", true, false},
		{0, 3, "approve the payment", false, false},
		{3, 2, "webhook: payment.paid", false, true},
		{2, 2, "mark paid, queue email", false, true},
		{3, 0, "back to the shop", true, false},
		{0, 1, "GET /orders/A-10482", false, false},
	}
	y := 140.0
	for _, m := range msgs {
		x1, x2 := cols[m.from], cols[m.to]
		if m.from == m.to {
			d.arrowIn(slate600, "a", false, x1+4, y-6, x1+50, y-6, x1+50, y+10, x1+8, y+10)
			if m.teal {
				d.arrowIn(teal700, "t", false, x1+4, y-6, x1+50, y-6, x1+50, y+10, x1+8, y+10)
			}
			d.text(x1+58, y+6, m.text, 12, slate700, sans, "start", 400, 0)
			d.rect(x1-4, y-12, 8, 28, 2, teal100, teal600, 1)
			y += 44
			continue
		}
		dir := 1.0
		if x2 < x1 {
			dir = -1
		}
		if m.teal {
			d.arrowTeal(false, x1+dir*6, y, x2-dir*6, y)
		} else {
			d.arrow(m.reply, x1+dir*6, y, x2-dir*6, y)
		}
		d.text((x1+x2)/2, y-8, m.text, 12, slate700, sans, "middle", 400, 0)
		y += 40
	}
	d.text(40, 585, "The order is paid when the webhook arrives, not when the customer comes back. Some never come back.", 12, slate500, sans, "start", 400, 0)
	return d.String()
}

func diagramStockSync(title string) string {
	d := newDoc(960, 440, title)
	d.frame("Stock sync, with the guard added after the incident")
	d.node(30, 120, 150, 64, boxOutside, "box", "Warehouse", "every 15 minutes")
	d.node(215, 120, 150, 64, boxPlain, "server", "SFTP drop", "stock.csv")
	d.node(400, 120, 150, 64, boxOurs, "gear", "porter", "stock-sync job")
	d.node(585, 120, 160, 64, boxWarn, "flag", "Guard", "rows and sold-out ratio")
	d.cylinder(785, 112, 150, 80, boxOurs, "PostgreSQL", "stock_levels")
	d.arrow(false, 180, 152, 213, 152)
	d.arrow(false, 365, 152, 398, 152)
	d.arrow(false, 550, 152, 583, 152)
	d.arrow(false, 745, 152, 783, 152)
	d.label(764, 138, "ok")
	d.node(585, 270, 160, 64, boxAlert, "bell", "Stop and page", "keep the last good stock")
	d.arrow(false, 665, 184, 665, 268)
	d.label(665, 228, "fails")
	d.node(785, 270, 150, 64, boxOurs, "search", "cairn", "reindex changed rows")
	d.arrow(false, 860, 192, 860, 268)
	d.text(30, 250, "The guard refuses a file with fewer than", 13, slate700, sans, "start", 400, 0)
	d.text(30, 270, "90 % of yesterday's rows, or one that would", 13, slate700, sans, "start", 400, 0)
	d.text(30, 290, "mark more than 5 % of products sold out.", 13, slate700, sans, "start", 400, 0)
	d.text(30, 320, "See the postmortem of the sold-out socks.", 13, teal700, sans, "start", 600, 0)
	d.legend(30, 410, []style{boxOurs, boxWarn, boxAlert, boxOutside}, []string{"ours", "new check", "on failure", "partner"})
	return d.String()
}

func diagramDataModel(title string) string {
	d := newDoc(960, 560, title)
	d.frame("Core data model")
	table := func(x, y float64, name string, cols []string, key bool) {
		h := float64(34 + 20*len(cols))
		s := boxPlain
		if key {
			s = boxOurs
		}
		d.box(x, y, 200, h, s)
		d.rect(x+1, y+1, 198, 28, 5, s.fill, "", 0)
		d.text(x+16, y+20, name, 14, slate900, mono, "start", 700, 0)
		d.line(x, y+30, x+200, y+30, s.stroke, 1, "")
		for i, c := range cols {
			fg := slate700
			if strings.HasSuffix(c, " PK") || strings.HasSuffix(c, " FK") {
				fg = teal700
			}
			d.text(x+16, y+48+float64(i)*20, c, 12, fg, mono, "start", 400, 0)
		}
	}
	table(40, 70, "products", []string{"id PK", "slug", "name", "category", "season"}, true)
	table(380, 70, "variants", []string{"id PK", "product_id FK", "sku", "size, colour", "price_chf"}, true)
	table(720, 70, "stock_levels", []string{"variant_id FK", "on_hand", "reserved", "synced_at"}, false)
	table(40, 330, "customers", []string{"id PK", "email_hash", "created_at"}, false)
	table(380, 330, "orders", []string{"id PK", "customer_id FK", "status", "total_chf", "placed_at"}, true)
	table(720, 330, "order_lines", []string{"order_id FK", "variant_id FK", "quantity", "price_chf"}, false)
	rel := func(x1, y1, x2, y2 float64, a, b string, lx, ly float64) {
		d.line(x1, y1, x2, y2, slate500, 1.5, "")
		d.text(x1+lx, y1+ly, a, 12, slate500, mono, "middle", 600, 0)
		d.text(x2-lx, y2+ly, b, 12, slate500, mono, "middle", 600, 0)
	}
	rel(240, 120, 380, 120, "1", "n", 10, -6)
	rel(580, 120, 720, 120, "1", "1", 10, -6)
	rel(240, 380, 380, 380, "1", "n", 10, -6)
	rel(580, 380, 720, 380, "1", "n", 10, -6)
	d.line(820, 330, 820, 230, slate500, 1.5, "")
	d.line(820, 230, 520, 230, slate500, 1.5, "4 3")
	d.line(520, 230, 520, 194, slate500, 1.5, "4 3")
	d.text(670, 222, "order_lines.variant_id", 11, slate500, mono, "middle", 400, 0)
	d.text(40, 535, "Prices are stored in rappen as integers; price_chf is the column's name for historical reasons.", 12, slate500, sans, "start", 400, 0)
	return d.String()
}

func diagramDeployPipeline(title string) string {
	d := newDoc(960, 360, title)
	d.frame("From a merge to production")
	steps := []struct{ t, s string }{
		{"Merge", "main"}, {"Test", "go test, lint"}, {"Build", "one image"}, {"Staging", "auto deploy"},
		{"Smoke", "12 checks"}, {"Canary", "10 % for 15 min"}, {"Production", "100 %"},
	}
	x := 30.0
	for i, s := range steps {
		st := boxPlain
		if i == len(steps)-1 {
			st = boxStrong
		} else if i == 5 {
			st = boxOurs
		}
		d.node(x, 110, 118, 64, st, "", s.t, s.s)
		if i < len(steps)-1 {
			d.arrow(false, x+118, 142, x+132, 142)
		}
		d.circle(x+104, 124, 4, teal500, "", 0)
		x += 132
	}
	d.arrowIn(rose500, "a", true, 820, 180, 820, 250, 700, 250)
	d.text(690, 255, "alp deploy rollback summit", 13, rose700, mono, "end", 600, 0)
	d.text(30, 230, "A deploy takes about 14 minutes.", 13, slate700, sans, "start", 400, 0)
	d.text(30, 250, "Canary errors above 1 % stop it by themselves.", 13, slate700, sans, "start", 400, 0)
	d.text(30, 320, "No deploys on Friday after 15:00, and none during a sale's first hour.", 12, slate500, sans, "start", 400, 0)
	return d.String()
}

func diagramIncident(title string) string {
	d := newDoc(960, 580, title)
	d.frame("Incident response")
	d.node(380, 60, 200, 52, boxAlert, "bell", "An alert fires", "")
	d.node(380, 145, 200, 52, boxPlain, "person", "On-call acknowledges", "")
	d.arrow(false, 480, 112, 480, 143)
	d.arrow(false, 480, 197, 480, 233)
	d.path("M480 235L590 285L480 335L370 285z", amber100, amber500, 1.5)
	d.text(480, 282, "Customers", 13, slate900, sans, "middle", 600, 0)
	d.text(480, 299, "affected?", 13, slate900, sans, "middle", 600, 0)
	d.arrow(false, 590, 285, 668, 285)
	d.label(628, 271, "yes")
	d.node(670, 255, 250, 60, boxOurs, "flag", "Declare an incident", "a lead, a channel, a status page")
	d.node(670, 350, 250, 52, boxOurs, "", "Update every 30 minutes", "")
	d.node(670, 435, 250, 52, boxStrong, "", "Fix or roll back", "")
	d.arrow(false, 795, 315, 795, 348)
	d.arrow(false, 795, 402, 795, 433)
	d.arrow(false, 370, 285, 292, 285)
	d.label(332, 271, "no")
	d.node(40, 255, 250, 60, boxPlain, "", "File an OPS issue", "fix it in working hours")
	d.node(380, 500, 200, 52, boxPlain, "check", "Postmortem", "within 3 days")
	d.arrow(false, 670, 461, 480, 461, 480, 498)
	d.text(40, 360, "Acknowledge within 5 minutes.", 13, slate700, sans, "start", 400, 0)
	d.text(40, 380, "No blame. Write down what you knew,", 13, slate700, sans, "start", 400, 0)
	d.text(40, 400, "when you knew it, and what you did.", 13, slate700, sans, "start", 400, 0)
	return d.String()
}

func diagramSearchIndexing(title string) string {
	d := newDoc(960, 420, title)
	d.frame("Search indexing on cairn")
	d.node(30, 110, 180, 64, boxStrong, "server", "summit", "writes a product")
	d.cylinder(260, 102, 170, 80, boxOurs, "outbox", "same transaction")
	d.node(480, 110, 190, 64, boxOurs, "gear", "cairn indexer", "polls every 2 s")
	d.node(720, 110, 210, 64, boxOurs, "search", "Index v14", "swapped by alias")
	d.arrow(false, 210, 142, 258, 142)
	d.arrow(false, 430, 142, 478, 142)
	d.arrow(false, 670, 142, 718, 142)
	d.node(720, 260, 210, 64, boxPlain, "browser", "Shop and app", "query and suggest")
	d.arrow(false, 825, 258, 825, 176)
	d.label(825, 218, "GET /search")
	d.node(480, 260, 190, 64, boxWarn, "bolt", "Full rebuild", "nightly, 03:10")
	d.arrow(true, 670, 292, 718, 176)
	d.text(30, 250, "An edit reaches search in under 5 seconds.", 13, slate700, sans, "start", 400, 0)
	d.text(30, 270, "A rebuild writes a new index and moves the", 13, slate700, sans, "start", 400, 0)
	d.text(30, 290, "alias when it is complete, so search never", 13, slate700, sans, "start", 400, 0)
	d.text(30, 310, "shows half an index.", 13, slate700, sans, "start", 400, 0)
	return d.String()
}

func diagramReleaseTrain(title string) string {
	d := newDoc(960, 400, title)
	d.frame("The app's two-week release train")
	x0, x1 := 60.0, 900.0
	day := (x1 - x0) / 14
	d.line(x0, 300, x1, 300, slate400, 1.5, "")
	for i := 0; i <= 14; i++ {
		x := x0 + float64(i)*day
		d.line(x, 296, x, 304, slate400, 1.5, "")
		if i < 14 {
			d.text(x+day/2, 322, fmt.Sprintf("%d", i+1), 11, slate500, mono, "middle", 400, 0)
		}
	}
	d.text(x0, 350, "day of the train", 12, slate500, sans, "start", 400, 0)
	seg := func(from, to float64, row int, s style, t string) {
		y := 100 + float64(row)*48
		d.box(x0+from*day, y, (to-from)*day, 34, s)
		fg := slate900
		if s == boxStrong {
			fg = white
		}
		d.text(x0+from*day+16, y+22, t, 13, fg, sans, "start", 600, 0)
	}
	seg(0, 8, 0, boxPlain, "Build features on main")
	seg(8, 10, 1, boxOurs, "Branch cut, beta")
	seg(9.5, 11.5, 2, boxWarn, "Store review")
	seg(11, 14, 3, boxStrong, "Staged rollout")
	d.text(x0+14*day, 296-8, "10 %, 50 %, 100 %", 12, slate600, sans, "end", 400, 0)
	d.text(x0+8*day, 92, "Monday of week 2: branch cut", 12, slate600, sans, "start", 400, 0)
	return d.String()
}

func hueColor(h int) string {
	cols := []string{"#e05a5a", "#e0795a", "#e0985a", "#d9b44a", "#a8b84a", "#6fb85a", "#4ab878", "#4ab8a0", "#4aa8b8", "#4a8fd0", "#5a74d9", "#7a64d9", "#9a5ad0", "#b85ab8", "#d05a9a", "#d95a78"}
	return cols[h%16]
}

func diagramTeam(title string) string {
	d := newDoc(960, 500, title)
	d.frame("Who does what at Alpenglow")
	people := []struct {
		name, role string
		hue        int
		x, y       float64
	}{
		{"Priya Raman", "Product manager", 11, 60, 110}, {"Amara Okafor", "Product designer", 14, 60, 290},
		{"Tomasz Nowak", "Engineering lead", 3, 380, 110}, {"Mateo Silva", "Backend developer", 6, 380, 290},
		{"Yuki Tanaka", "Mobile and frontend", 9, 560, 290}, {"Lena Brunner", "QA and support", 1, 766, 110},
	}
	d.rect(40, 80, 250, 380, 10, "none", slate300, 1)
	d.text(56, 100, "PRODUCT", 11, slate500, mono, "start", 600, 1.5)
	d.rect(360, 80, 380, 380, 10, "none", slate300, 1)
	d.text(376, 100, "ENGINEERING", 11, slate500, mono, "start", 600, 1.5)
	d.rect(752, 80, 196, 200, 10, "none", slate300, 1)
	d.text(766, 100, "QA AND SUPPORT", 11, slate500, mono, "start", 600, 1.5)
	for _, p := range people {
		d.box(p.x, p.y, 170, 150, boxPlain)
		face(d, p.x+57, p.y+18, 7, hueColor(p.hue))
		d.text(p.x+85, p.y+100, p.name, 14, slate900, sans, "middle", 600, 0)
		d.text(p.x+85, p.y+120, p.role, 12, slate500, sans, "middle", 400, 0)
	}
	return d.String()
}

// face is a pixel avatar of 8 by 8 dots.
func face(d *doc, x, y, s float64, col string) {
	rows := []string{
		"..####..",
		".######.",
		"##.##.##",
		"########",
		"##o##o##",
		"###oo###",
		".######.",
		"..####..",
	}
	var main, eyes strings.Builder
	for r, row := range rows {
		for c, ch := range row {
			p := fmt.Sprintf("M%s %sh%sv%sh-%sz", num(x+float64(c)*s), num(y+float64(r)*s), num(s), num(s), num(s))
			switch ch {
			case '#':
				main.WriteString(p)
			case '.':
			default:
				eyes.WriteString(p)
			}
		}
	}
	d.raw(`<path d="%s" fill="%s"/>`, main.String(), col)
	d.raw(`<path d="%s" fill="%s" opacity="0.35"/>`, eyes.String(), white)
	// Eyes.
	d.rect(x+2*s, y+2*s, s, s, 0, slate900, "", 0)
	d.rect(x+5*s, y+2*s, s, s, 0, slate900, "", 0)
}

func diagramRoadmap(title string) string {
	d := newDoc(960, 540, title)
	d.frame("Roadmap: now, next, later")
	cols := []struct {
		t     string
		s     style
		cards [][2]string
	}{
		{"Now", boxOurs, [][2]string{{"One-page checkout", "Web shop"}, {"Pack lists", "Mobile app"}, {"PostgreSQL 18", "Platform"}, {"First Snow Sale", "everyone"}}},
		{"Next", boxPlain, [][2]string{{"Offline pack lists", "Mobile app"}, {"Back-in-stock alerts", "Mobile app"}, {"Search on cairn for all", "Platform"}}},
		{"Later", boxQuiet, [][2]string{{"Boot fit finder", "Web shop"}, {"Rental gear", "everyone"}, {"Trail journal 2.0", "Web shop"}}},
	}
	for i, c := range cols {
		x := 30 + float64(i)*305
		d.rect(x, 70, 290, 440, 10, slate100, slate200, 1)
		d.text(x+18, 98, strings.ToUpper(c.t), 12, slate600, mono, "start", 700, 1.5)
		for j, card := range c.cards {
			y := 115 + float64(j)*92
			d.node(x+14, y, 262, 76, c.s, "", card[0], card[1])
		}
	}
	return d.String()
}

// Charts.

type series struct {
	name  string
	color string
	vals  []float64
}

func chartFrame(d *doc, x0, y0, x1, y1, max float64, ticks int, unit string) func(v float64) float64 {
	sy := func(v float64) float64 { return y1 - (y1-y0)*v/max }
	for i := 0; i <= ticks; i++ {
		v := max * float64(i) / float64(ticks)
		d.line(x0, sy(v), x1, sy(v), slate200, 1, "")
		d.text(x0-8, sy(v)+4, strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0")+unit, 11, slate500, mono, "end", 400, 0)
	}
	d.line(x0, y1, x1, y1, slate400, 1.5, "")
	return sy
}

func chartBars(title, heading string, labels []string, ss []series, max float64, unit, note string) string {
	d := newDoc(800, 450, title)
	d.frame(heading)
	x0, y0, x1, y1 := 70.0, 80.0, 770.0, 370.0
	sy := chartFrame(d, x0, y0, x1, y1, max, 5, unit)
	gw := (x1 - x0) / float64(len(labels))
	bw := (gw - 24) / float64(len(ss))
	for i, l := range labels {
		gx := x0 + float64(i)*gw + 12
		for j, s := range ss {
			v := s.vals[i]
			if v <= 0 {
				continue
			}
			x := gx + float64(j)*bw
			d.rect(x+2, sy(v), bw-4, y1-sy(v), 2, s.color, "", 0)
			d.rect(x+2, sy(v), bw-4, 3, 0, slate900, "", 0)
			d.raw(`<rect x="%s" y="%s" width="%s" height="3" fill="%s" opacity="0.25"/>`, num(x+2), num(sy(v)), num(bw-4), white)
			d.text(x+bw/2, sy(v)-6, strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0"), 11, slate600, mono, "middle", 400, 0)
		}
		d.text(gx+(gw-24)/2, y1+20, l, 12, slate600, sans, "middle", 400, 0)
	}
	lx := x0
	for _, s := range ss {
		d.rect(lx, 404, 12, 12, 2, s.color, "", 0)
		d.text(lx+18, 414, s.name, 12, slate600, sans, "start", 400, 0)
		lx += 40 + float64(len(s.name))*7
	}
	if note != "" {
		d.text(770, 414, note, 11, slate500, sans, "end", 400, 0)
	}
	return d.String()
}

func chartConversion(title string) string {
	return chartBars(title, "Checkout conversion by week", []string{"W36", "W37", "W38", "W39", "W40", "W41"},
		[]series{
			{"Three-step checkout", slate400, []float64{2.8, 2.9, 2.7, 2.8, 2.9, 2.8}},
			{"One-page checkout", teal600, []float64{0, 0, 3.1, 3.3, 3.4, 3.6}},
		}, 4, " %", "one-page checkout behind a flag from W38, at 50 % of sessions")
}

func chartCrashFree(title string) string {
	return chartBars(title, "Crash-free sessions by app version", []string{"2.6", "2.7", "2.8", "3.0 beta 1", "3.0 beta 2"},
		[]series{{"crash-free sessions", teal600, []float64{99.1, 99.4, 99.6, 98.2, 99.3}}}, 100, " %", "target 99.5 %; beta 1 crashed on resume")
}

func chartReturns(title string) string {
	d := newDoc(800, 450, title)
	d.frame("Why customers send things back")
	rows := []struct {
		l string
		v float64
	}{{"Too small", 34}, {"Too big", 21}, {"Changed my mind", 17}, {"Colour not as shown", 11}, {"Damaged", 9}, {"Arrived late", 8}}
	for i, r := range rows {
		y := 80 + float64(i)*52
		d.text(200, y+22, r.l, 13, slate700, sans, "end", 400, 0)
		w := r.v / 40 * 520
		col := teal600
		if i > 1 {
			col = slate400
		}
		d.rect(216, y+6, w, 24, 3, col, "", 0)
		for k := 0.0; k < w-6; k += 12 {
			d.rect(216+k+4, y+10, 3, 3, 0, white, "", 0)
		}
		d.text(216+w+10, y+23, fmt.Sprintf("%.0f %%", r.v), 12, slate600, mono, "start", 600, 0)
	}
	d.text(216, 420, "Share of returns in the last 90 days. Sizing causes more than half.", 12, slate500, sans, "start", 400, 0)
	return d.String()
}

func chartLatency(title string) string {
	d := newDoc(800, 450, title)
	d.frame("API latency, last 30 days")
	x0, y0, x1, y1 := 70.0, 80.0, 770.0, 360.0
	sy := chartFrame(d, x0, y0, x1, y1, 400, 4, "")
	p95 := []float64{182, 176, 190, 185, 178, 181, 195, 188, 180, 176, 384, 352, 210, 190, 186, 179, 175, 183, 172, 168, 150, 142, 138, 141, 136, 133, 139, 131, 128, 130}
	p50 := []float64{61, 60, 63, 62, 60, 61, 64, 62, 60, 59, 118, 104, 70, 64, 62, 60, 59, 61, 58, 56, 49, 47, 46, 47, 45, 44, 46, 43, 42, 43}
	sx := func(i int) float64 { return x0 + (x1-x0)*float64(i)/float64(len(p95)-1) }
	line := func(vals []float64, col string, w float64) {
		var b strings.Builder
		for i, v := range vals {
			if i == 0 {
				b.WriteString("M")
			} else {
				b.WriteString("L")
			}
			b.WriteString(num(sx(i)) + " " + num(sy(v)))
		}
		d.path(b.String(), "none", col, w)
		for i, v := range vals {
			d.rect(sx(i)-2, sy(v)-2, 4, 4, 0, col, "", 0)
		}
	}
	d.rect(sx(10)-6, y0, sx(11)-sx(10)+12, y1-y0, 0, rose100, "", 0)
	d.text(sx(11)+10, y0+16, "stock sync incident", 11, rose700, sans, "start", 600, 0)
	d.line(sx(20), y0, sx(20), y1, teal600, 1.5, "4 3")
	d.text(sx(20)+6, y0+34, "search moved to cairn", 11, teal700, sans, "start", 600, 0)
	line(p95, teal700, 2)
	line(p50, slate400, 2)
	d.text(x0-8, 70, "ms", 11, slate500, mono, "end", 400, 0)
	for _, i := range []int{0, 7, 14, 21, 29} {
		d.text(sx(i), y1+20, fmt.Sprintf("day %d", i-29), 11, slate500, mono, "middle", 400, 0)
	}
	d.rect(x0, 404, 12, 12, 2, teal700, "", 0)
	d.text(x0+18, 414, "p95", 12, slate600, sans, "start", 400, 0)
	d.rect(x0+70, 404, 12, 12, 2, slate400, "", 0)
	d.text(x0+88, 414, "p50", 12, slate600, sans, "start", 400, 0)
	return d.String()
}
