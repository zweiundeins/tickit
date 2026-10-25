package wiki

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/docnodes"
)

// DM-07.1: a moderator hides a page's version, never its newest, and shows
// it again; nobody else does, and nobody but staff restores a hidden one.
func TestHideVersion(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Notes", Doc: paras("One")})
	e.must(SetDocument{By: e.by(rainy), Page: p, Doc: paras("Spam spam")})
	e.must(SetDocument{By: e.by(sunny), Page: p, Doc: paras("One again")})
	var denial *access.Denial
	if err := e.do(HideVersion{By: e.by(member), Page: p, Number: 2, Hidden: true}); !errors.As(err, &denial) {
		t.Errorf("a member hides a version: %v", err)
	}
	if msg := refusal(t, e.do(HideVersion{By: e.by(moderator), Page: p, Number: 3, Hidden: true})); msg == "" {
		t.Error("the newest version was hidden")
	}
	e.must(HideVersion{By: e.by(moderator), Page: p, Number: 2, Hidden: true, Reason: "spam"})
	hidden := func() map[int]bool {
		var vs []VersionInfo
		e.View(t, func(tx *sql.Tx) (err error) { vs, err = Versions(context.Background(), tx, p); return err })
		m := map[int]bool{}
		for _, v := range vs {
			m[v.Number] = v.Hidden
		}
		return m
	}
	if h := hidden(); !h[2] || h[1] || h[3] {
		t.Errorf("hidden versions %v", h)
	}
	if r := e.row(`SELECT hidden_reason FROM page_versions WHERE page_id = ? AND number = 2`, p); r != "spam" {
		t.Errorf("reason %q", r)
	}
	if msg := refusal(t, e.do(RestoreVersion{By: e.by(member), Page: p, Number: 2})); msg != "A moderator hid this version." {
		t.Errorf("a member restores a hidden version: %q", msg)
	}
	e.must(HideVersion{By: e.by(moderator), Page: p, Number: 2, Hidden: false})
	if h := hidden(); h[2] {
		t.Errorf("shown again: %v", h)
	}
}

// DM-07.1: a hidden image renders as its place without it, wherever a
// document shows it.
func TestHideImage(t *testing.T) {
	e := newEnv(t)
	img := Images()[0].ID
	if err := e.do(HideImage{By: e.by(member), Image: img, Hidden: true}); err == nil {
		t.Error("a member hid an image")
	}
	if err := e.do(HideImage{By: e.by(moderator), Image: "no-such-image", Hidden: true}); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("an unknown image: %v", err)
	}
	d, err := docnodes.ParseDoc([]byte(`{"type":"doc","content":[{"type":"image","attrs":{"imageId":"` + img + `","alt":"A diagram"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	render := func() string {
		var html string
		e.View(t, func(tx *sql.Tx) error {
			res, err := Resolve(context.Background(), tx, docnodes.References(d))
			html = docnodes.RenderHTML(d, res)
			return err
		})
		return html
	}
	e.must(HideImage{By: e.by(moderator), Image: img, Hidden: true, Reason: "not for here"})
	if html := render(); html != `<figure class="doc-image doc-image--hidden"><p>Image hidden by a moderator</p></figure>` {
		t.Errorf("hidden: %s", html)
	}
	e.must(HideImage{By: e.by(moderator), Image: img, Hidden: false})
	if html := render(); html == "" || !strings.Contains(html, `/img/lib/`+img) {
		t.Errorf("shown again: %s", html)
	}
}
