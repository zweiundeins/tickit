package views

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// The outline nests deeper levels in the entry before them, escapes the
// headings' text, and costs the page's frame a few dozen bytes a heading.
func TestPageOutline(t *testing.T) {
	got := outlineList([]OutlineEntry{{1, "A & B", "h-p-1-a-b"}, {3, "C", "h-p-1-c"}, {2, "D", "h-p-1-d"}, {1, "E", "h-p-1-e"}})
	want := `<ol><li><a href="#h-p-1-a-b">A &amp; B</a><ol><li><a href="#h-p-1-c">C</a></li></ol><ol><li><a href="#h-p-1-d">D</a></li></ol></li><li><a href="#h-p-1-e">E</a></li></ol>`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	var hs []OutlineEntry
	for i := range 20 {
		hs = append(hs, OutlineEntry{Level: 1 + i%2, Text: "A section heading " + strconv.Itoa(i), ID: "h-p-7a8b9c0d-a-section-heading-" + strconv.Itoa(i)})
	}
	var b strings.Builder
	if err := PageOutlineRegion(PageBody{Outline: hs}).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	if n := b.Len(); n > 2500 {
		t.Errorf("the outline of 20 headings takes %d bytes", n)
	} else {
		t.Logf("the outline of 20 headings: %d bytes", n)
	}
	b.Reset()
	PageOutlineRegion(PageBody{}).Render(context.Background(), &b)
	if b.String() != `<div id="page-outline" class="wk-outline"></div>` {
		t.Errorf("no outline: %s", b.String())
	}
}
