package web_test

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/web/webtest"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// DM-07.1: a moderator hides a page's version, which leaves its history
// and compare for everyone but staff, and an image, which every open page
// that shows it replaces within two ticks.
func TestModerateVersionsAndImages(t *testing.T) {
	e := seededEnv(t)
	e.Account(t, "mo@example.com", "correct horse battery", people.Moderator)
	mo, g := webtest.Client(t), webtest.Client(t)
	e.Login(t, mo, "mo@example.com", "correct horse battery")
	webtest.Get(t, g, e.URL+"/")

	// A seeded page whose newest version shows an image of the library.
	var page, key, title, body string
	if err := e.DB.R.QueryRow(`SELECT p.id, s.key, p.title, v.doc FROM pages p JOIN spaces s ON s.id = p.space_id
		JOIN page_versions v ON v.page_id = p.id AND v.number = (SELECT max(number) FROM page_versions WHERE page_id = p.id)
		WHERE v.doc LIKE '%"imageId"%' ORDER BY p.id LIMIT 1`).Scan(&page, &key, &title, &body); err != nil {
		t.Fatalf("no seeded page with an image: %v", err)
	}
	img := regexp.MustCompile(`"imageId":"([^"]+)"`).FindStringSubmatch(body)[1]
	reader := webtest.Listen(t, g, e.URL+"/stream?tab=gtab&v=pagedoc:"+page, nil)
	webtest.Next(t, reader, "/img/lib/"+img)
	start := time.Now()
	if code, b := webtest.Post(t, mo, e.URL+"/cmd/moderation/act", `{"tab":"tab1","op":"m1","action":"hide","kind":"image","id":"`+img+`","reason":"not here"}`); code != http.StatusNoContent {
		t.Fatalf("hide the image: %d %s", code, b)
	}
	frame := webtest.Next(t, reader, "Image hidden by a moderator")
	if took := time.Since(start); took > time.Second {
		t.Errorf("the hidden image left the open page after %s", took)
	}
	if strings.Contains(frame, "/img/lib/"+img+`"`) {
		t.Error("the frame still shows the hidden image")
	}
	if code, _ := webtest.Get(t, g, e.URL+"/img/lib/"+img); code != http.StatusNotFound {
		t.Errorf("the hidden image is served: %d", code)
	}
	if code, page := webtest.Get(t, mo, e.URL+"/moderation"); code != http.StatusOK || !strings.Contains(page, `data-id="`+img+`"`) {
		t.Errorf("the moderation page lists no hidden image: %d", code)
	}
	webtest.Post(t, mo, e.URL+"/cmd/moderation/act", `{"tab":"tab1","op":"m2","action":"restore","kind":"image","id":"`+img+`"}`)
	webtest.Next(t, reader, "/img/lib/"+img)

	// A version: the newest of a seeded page with at least three.
	var vpage, vkey, vtitle string
	var n int
	if err := e.DB.R.QueryRow(`SELECT v.page_id, s.key, p.title, max(v.number) FROM page_versions v JOIN pages p ON p.id = v.page_id
		JOIN spaces s ON s.id = p.space_id GROUP BY v.page_id HAVING max(v.number) >= 3 ORDER BY v.page_id LIMIT 1`).Scan(&vpage, &vkey, &vtitle, &n); err != nil {
		t.Fatalf("no seeded page with three versions: %v", err)
	}
	history := wiki.PageURL(vkey, vpage, vtitle)
	history = strings.TrimSuffix(history, "/"+wiki.Slug(vtitle)) + "/history"
	v2 := wiki.VersionURL(vkey, vpage, 2)
	if _, h := webtest.Get(t, g, e.URL+history); !strings.Contains(h, `href="`+v2+`"`) {
		t.Fatalf("the history does not list version 2")
	}
	hist := webtest.Listen(t, g, e.URL+"/stream?tab=htab&v=history:"+vpage, nil)
	webtest.Next(t, hist, `href="`+v2+`"`)
	if code, b := webtest.Post(t, mo, e.URL+"/cmd/moderation/act", `{"tab":"tab1","op":"m3","action":"hide","kind":"version","id":"`+vpage+`.2","reason":"spam"}`); code != http.StatusNoContent {
		t.Fatalf("hide the version: %d %s", code, b)
	}
	if o := outcome(t, e, people.Person{}, "m3"); !o.OK {
		t.Fatalf("hide the version: %+v", o)
	}
	if ev := webtest.Next(t, hist, `id="`+"page-history"); strings.Contains(ev, `href="`+v2+`"`) {
		t.Error("the open history still lists the hidden version")
	}
	if _, h := webtest.Get(t, g, e.URL+history); strings.Contains(h, `href="`+v2+`"`) {
		t.Error("the history lists the hidden version")
	}
	if code, _ := webtest.Get(t, g, e.URL+v2); code != http.StatusNotFound {
		t.Errorf("a guest opens the hidden version: %d", code)
	}
	compare := strings.TrimSuffix(v2, "/versions/2") + "/compare?from=1&to=2"
	if code, _ := webtest.Get(t, g, e.URL+compare); code != http.StatusNotFound {
		t.Errorf("a guest compares the hidden version: %d", code)
	}
	if code, h := webtest.Get(t, mo, e.URL+history); code != http.StatusOK || !strings.Contains(h, `href="`+v2+`"`) || !strings.Contains(h, "hidden by a moderator") {
		t.Errorf("the moderator's history: %d", code)
	}
	if code, _ := webtest.Get(t, mo, e.URL+v2); code != http.StatusOK {
		t.Errorf("the moderator opens the hidden version: %d", code)
	}
	if code, _ := webtest.Post(t, mo, e.URL+"/cmd/moderation/act", `{"tab":"tab1","op":"m4","action":"hide","kind":"version","id":"`+vpage+`.`+strconv.Itoa(n)+`"}`); code != http.StatusNoContent {
		t.Errorf("hide the newest version: %d", code)
	}
	if o := outcome(t, e, people.Person{}, "m4"); o.OK {
		t.Error("the newest version was hidden")
	}
}
