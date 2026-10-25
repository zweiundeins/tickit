package docnodes

import (
	"testing"

	"github.com/zweiundeins/prosemirror-go"
)

// FR-91.1: the text an inline comment is on renders marked, inside the
// marks of the text and around no atom.
func TestRenderHighlighted(t *testing.T) {
	two := docOf(`{"type":"paragraph","content":[`+text("Hello world")+`]}`, `{"type":"paragraph","content":[`+text("second")+`]}`)
	cases := []struct {
		doc  string
		hs   []prosemirror.Highlight
		html string
	}{
		{inPara(text("Hello world")), []prosemirror.Highlight{{ID: "c1", From: 7, To: 12}}, `<p>Hello <mark class="tk-ic" data-c="c1">world</mark></p>`},
		{inPara(text("Hello world")), nil, `<p>Hello world</p>`},
		// Overlapping ranges: the overlap names both.
		{inPara(text("abcdef")), []prosemirror.Highlight{{ID: "b", From: 3, To: 6}, {ID: "a", From: 1, To: 4}}, `<p><mark class="tk-ic" data-c="a">ab</mark><mark class="tk-ic" data-c="a b">c</mark><mark class="tk-ic" data-c="b">de</mark>f</p>`},
		// Inside other marks, and across text nodes.
		{inPara(text("bold", strong) + `,` + text(" plain")), []prosemirror.Highlight{{ID: "c1", From: 3, To: 8}}, `<p><strong>bo<mark class="tk-ic" data-c="c1">ld</mark></strong><mark class="tk-ic" data-c="c1"> pl</mark>ain</p>`},
		// Across two blocks; an id is escaped.
		{two, []prosemirror.Highlight{{ID: `x"y`, From: 7, To: 17}}, `<p>Hello <mark class="tk-ic" data-c="x&#34;y">world</mark></p><p><mark class="tk-ic" data-c="x&#34;y">sec</mark>ond</p>`},
		// An atom in the range stays as it is.
		{inPara(text("a") + `,{"type":"issue_chip","attrs":{"key":"WEB-99"}},` + text("b")), []prosemirror.Highlight{{ID: "c", From: 1, To: 4}}, `<p><mark class="tk-ic" data-c="c">a</mark><a class="issue-chip" data-key="WEB-99" href="/WEB-99" data-state="unknown"><span class="issue-chip__key">WEB-99</span><span class="issue-chip__summary">unknown issue</span></a><mark class="tk-ic" data-c="c">b</mark></p>`},
		// Code blocks too.
		{docOf(`{"type":"code_block","content":[` + text("x := 1") + `]}`), []prosemirror.Highlight{{ID: "c", From: 1, To: 2}}, `<pre><code><mark class="tk-ic" data-c="c">x</mark> := 1</code></pre>`},
		// A cut inside a surrogate pair moves before it; a range beyond the
		// document, empty or reversed breaks nothing.
		{inPara(text("a😀b")), []prosemirror.Highlight{{ID: "c", From: 2, To: 3}}, `<p>a😀b</p>`},
		{inPara(text("a😀b")), []prosemirror.Highlight{{ID: "c", From: 2, To: 4}}, `<p>a<mark class="tk-ic" data-c="c">😀</mark>b</p>`},
		{inPara(text("ab")), []prosemirror.Highlight{{ID: "c", From: 2, To: 99}, {ID: "d", From: 2, To: 2}, {ID: "e", From: 3, To: 1}}, `<p>a<mark class="tk-ic" data-c="c">b</mark></p>`},
	}
	for _, c := range cases {
		d := mustDoc(t, c.doc)
		if got := RenderHighlighted(d, testResolver, c.hs); got != c.html {
			t.Errorf("%s %v:\n got %s\nwant %s", c.doc, c.hs, got, c.html)
		}
		var bc BlockCache
		if got := bc.RenderHighlighted(d, testResolver, c.hs); got != c.html {
			t.Errorf("cache %s %v:\n got %s\nwant %s", c.doc, c.hs, got, c.html)
		}
	}
	// Without highlights it is RenderHTML.
	d := mustDoc(t, two)
	if RenderHighlighted(d, nil, nil) != RenderHTML(d, nil) {
		t.Error("no highlights differ from RenderHTML")
	}
}
