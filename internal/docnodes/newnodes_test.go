package docnodes

import (
	"slices"
	"testing"

	"github.com/zweiundeins/prosemirror-go/adf"
)

func heading(level int, s string) string {
	return `{"type":"heading","attrs":{"level":` + string(rune('0'+level)) + `},"content":[` + text(s) + `]}`
}

// FR-82.1: headings of six levels, panels, statuses, a table of contents
// and text colours, as docs/editor.md's table writes them.
func TestRenderNewNodes(t *testing.T) {
	d := mustDoc(t, docOf(
		`{"type":"toc"}`,
		heading(1, "Intro"),
		heading(4, "Deep & <low>"),
		heading(6, "Sixth"),
		heading(2, "Intro"),
		`{"type":"panel","attrs":{"kind":"warning"},"content":[`+heading(3, "In a panel")+`,`+para+`]}`,
		`{"type":"paragraph","content":[{"type":"status","attrs":{"text":"In <review>","color":"blue"}},`+text("red", `{"type":"text_color","attrs":{"color":"red"}}`, strong)+`]}`,
	))
	want := `<nav class="doc-toc" aria-label="Contents"><ol><li><a href="#h-intro">Intro</a><ol><li><a href="#h-deep-low">Deep &amp; &lt;low&gt;</a><ol><li><a href="#h-sixth">Sixth</a></li></ol></li></ol><ol><li><a href="#h-intro-2">Intro</a><ol><li><a href="#h-in-a-panel">In a panel</a></li></ol></li></ol></li></ol></nav>` +
		`<h2 id="h-intro">Intro</h2><h5 id="h-deep-low">Deep &amp; &lt;low&gt;</h5><p role="heading" aria-level="7" id="h-sixth">Sixth</p><h3 id="h-intro-2">Intro</h3>` +
		`<div class="doc-panel" data-panel="warning" role="note" aria-label="Warning"><h4 id="h-in-a-panel">In a panel</h4><p>hi</p></div>` +
		`<p><span class="tk-lozenge doc-status" data-color="blue">In &lt;review&gt;</span><strong><span class="doc-color" data-color="red">red</span></strong></p>`
	if got := RenderHTML(d, nil); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got := RenderHTML(mustDoc(t, docOf(`{"type":"toc"}`, para)), nil); got != `<nav class="doc-toc" aria-label="Contents"><p class="doc-toc__empty">No headings yet</p></nav><p>hi</p>` {
		t.Errorf("empty toc: %s", got)
	}
	o := Outline(d)
	if len(o) != 5 || o[3] != (Heading{Level: 2, Text: "Intro", ID: "h-intro-2"}) {
		t.Errorf("outline %+v", o)
	}
}

// A toc renders again when a heading changes, and a heading when the
// one before it takes its id; other blocks stay cached.
func TestBlockCacheOutline(t *testing.T) {
	var c BlockCache
	render := func(blocks ...string) {
		t.Helper()
		d := mustDoc(t, docOf(blocks...))
		if got, want := c.Render(d, testResolver), RenderHTML(d, testResolver); got != want {
			t.Errorf("cache:\n got %s\nwant %s", got, want)
		}
	}
	render(`{"type":"toc"}`, heading(1, "A"), para, heading(1, "B"))
	render(`{"type":"toc"}`, heading(1, "A"), para, heading(1, "C"))
	render(`{"type":"toc"}`, heading(1, "C"), para, heading(1, "C"))
	render(`{"type":"toc"}`, heading(1, "D"), para, heading(1, "C"))
}

func TestMarkdownNewNodes(t *testing.T) {
	d := mustDoc(t, docOf(
		`{"type":"toc"}`,
		heading(1, "Plan"),
		heading(2, "Plan"),
		heading(6, "Six, *really*"),
		`{"type":"panel","attrs":{"kind":"success"},"content":[`+para+`,{"type":"bullet_list","content":[{"type":"list_item","content":[`+para+`]}]}]}`,
		`{"type":"paragraph","content":[{"type":"status","attrs":{"text":"DONE","color":"green"}},`+text(" red", `{"type":"text_color","attrs":{"color":"red"}}`)+`]}`,
	))
	want := "- [Plan](#plan)\n  - [Plan](#plan-1)\n    - [Six, \\*really\\*](#six-really)\n\n## Plan\n\n### Plan\n\n###### Six, \\*really\\*\n\n> [!TIP]\n> hi\n>\n> - hi\n\n\\[DONE\\] red\n"
	if got := Markdown(d, nil); got != want {
		t.Errorf("markdown:\n%s\nwant\n%s", got, want)
	}
	if got, want := Text(d), "Plan\nPlan\nSix, *really*\nhi\nhi\nDONE red"; got != want {
		t.Errorf("Text: %q, want %q", got, want)
	}
}

// The lists of schema.json are the converter's: what an import gives
// passes the rules.
func TestLimitsMatchADF(t *testing.T) {
	lim := SchemaLimits()
	if !slices.Equal(lim.TextColors, adf.Colors) || !slices.Equal(lim.PanelKinds, adf.PanelKinds) || !slices.Equal(lim.StatusColors, adf.StatusColors) {
		t.Errorf("schema.json %v %v %v, adf %v %v %v", lim.TextColors, lim.PanelKinds, lim.StatusColors, adf.Colors, adf.PanelKinds, adf.StatusColors)
	}
}
