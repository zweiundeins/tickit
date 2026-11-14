package web_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/docnodes"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// An issue page shows its description and its comments: the same heading
// in both gets an id of each document's own, and their tables of contents
// link to their own. A comment holds a heading when an import brought it.
func TestHeadingIDsPerDocument(t *testing.T) {
	e := newList(t)
	doc := `{"type":"doc","content":[{"type":"toc"},{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"Steps"}]}]}`
	e.issue("WEB-4", tracker.CreateIssue{Project: e.web, Summary: "Headings twice", Description: []byte(doc)})
	e.Do(t, tracker.AddComment{By: e.by(e.ada), Issue: e.ids["WEB-4"], Body: tracker.PlainDoc("placeholder")})
	var comment string
	if err := e.DB.R.QueryRow(`SELECT id FROM comments WHERE subject_id = ?`, e.ids["WEB-4"]).Scan(&comment); err != nil {
		t.Fatal(err)
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE comments SET body = ? WHERE id = ?`, doc, comment)
		return err
	}))
	_, page := webtest.Get(t, webtest.Client(t), e.URL+"/WEB-4")
	d, c := "h-"+docnodes.DocName("d", e.ids["WEB-4"])+"-steps", "h-"+docnodes.DocName("c", comment)+"-steps"
	for _, want := range []string{`<h2 id="` + d + `">Steps</h2>`, `<h2 id="` + c + `">Steps</h2>`, `href="#` + d + `"`, `href="#` + c + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the issue page has no %s", want)
		}
	}
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(` id="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		if seen[m[1]] {
			i := strings.Index(page, `id="`+m[1])
			j := strings.LastIndex(page, `id="`+m[1])
			t.Errorf("the id %s twice:\n%s\n...\n%s", m[1], page[max(0, i-600):i+50], page[max(0, j-600):j+50])
		}
		seen[m[1]] = true
	}
}
