package doc

import (
	"testing"
)

// FR-91.1: the text an inline comment is on renders marked, inside the
// marks of the text and around no atom.
func TestRenderHighlighted(t *testing.T) {
	two := docOf(`{"type":"paragraph","content":[`+text("Hello world")+`]}`, `{"type":"paragraph","content":[`+text("second")+`]}`)
	cases := []struct {
		doc  string
		hs   []Highlight
		html string
	}{
		{inPara(text("Hello world")), []Highlight{{"c1", 7, 12}}, `<p>Hello <mark class="tk-ic" data-c="c1">world</mark></p>`},
		{inPara(text("Hello world")), nil, `<p>Hello world</p>`},
		// Overlapping ranges: the overlap names both.
		{inPara(text("abcdef")), []Highlight{{"b", 3, 6}, {"a", 1, 4}}, `<p><mark class="tk-ic" data-c="a">ab</mark><mark class="tk-ic" data-c="a b">c</mark><mark class="tk-ic" data-c="b">de</mark>f</p>`},
		// Inside other marks, and across text nodes.
		{inPara(text("bold", strong) + `,` + text(" plain")), []Highlight{{"c1", 3, 8}}, `<p><strong>bo<mark class="tk-ic" data-c="c1">ld</mark></strong><mark class="tk-ic" data-c="c1"> pl</mark>ain</p>`},
		// Across two blocks; an id is escaped.
		{two, []Highlight{{`x"y`, 7, 17}}, `<p>Hello <mark class="tk-ic" data-c="x&#34;y">world</mark></p><p><mark class="tk-ic" data-c="x&#34;y">sec</mark>ond</p>`},
		// An atom in the range stays as it is.
		{inPara(text("a") + `,{"type":"issue_chip","attrs":{"key":"WEB-99"}},` + text("b")), []Highlight{{"c", 1, 4}}, `<p><mark class="tk-ic" data-c="c">a</mark><a class="issue-chip" data-key="WEB-99" href="/WEB-99" data-state="unknown"><span class="issue-chip__key">WEB-99</span><span class="issue-chip__summary">unknown issue</span></a><mark class="tk-ic" data-c="c">b</mark></p>`},
		// Code blocks too.
		{docOf(`{"type":"code_block","content":[` + text("x := 1") + `]}`), []Highlight{{"c", 1, 2}}, `<pre><code><mark class="tk-ic" data-c="c">x</mark> := 1</code></pre>`},
		// A cut inside a surrogate pair moves before it; a range beyond the
		// document, empty or reversed breaks nothing.
		{inPara(text("a😀b")), []Highlight{{"c", 2, 3}}, `<p>a😀b</p>`},
		{inPara(text("a😀b")), []Highlight{{"c", 2, 4}}, `<p>a<mark class="tk-ic" data-c="c">😀</mark>b</p>`},
		{inPara(text("ab")), []Highlight{{"c", 2, 99}, {"d", 2, 2}, {"e", 3, 1}}, `<p>a<mark class="tk-ic" data-c="c">b</mark></p>`},
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

// The block cache renders a block again when its highlights change, and
// keeps it when only another block's do.
func TestBlockCacheHighlights(t *testing.T) {
	d := mustDoc(t, docOf(`{"type":"paragraph","content":[`+text("one")+`]}`, `{"type":"paragraph","content":[`+text("two")+`]}`))
	var bc BlockCache
	if got := bc.RenderHighlighted(d, nil, []Highlight{{"c", 1, 2}}); got != `<p><mark class="tk-ic" data-c="c">o</mark>ne</p><p>two</p>` {
		t.Errorf("first %s", got)
	}
	second := d.Child(1)
	before := bc.m[second]
	if got := bc.RenderHighlighted(d, nil, []Highlight{{"c", 6, 8}}); got != `<p>one</p><p><mark class="tk-ic" data-c="c">tw</mark>o</p>` {
		t.Errorf("moved %s", got)
	}
	if bc.m[second] == before {
		t.Error("the second block kept its old HTML")
	}
	if got := bc.Render(d, nil); got != `<p>one</p><p>two</p>` {
		t.Errorf("none %s", got)
	}
}

// FR-91.2: a range moves with its text; typing at its edges stays
// outside; deleting its text leaves it gone.
func TestMapRange(t *testing.T) {
	d := mustDoc(t, inPara(text("say hello now")))
	// "hello" is 5 to 10.
	apply := func(t *testing.T, raw string) (Doc, []Step) {
		t.Helper()
		steps, err := ParseSteps([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		next, err := Apply(d, steps)
		if err != nil {
			t.Fatal(err)
		}
		return next, steps
	}
	ins := func(at int, s string) string {
		return `[{"stepType":"replace","from":` + itoa(at) + `,"to":` + itoa(at) + `,"slice":{"content":[` + text(s) + `]}}]`
	}
	del := func(from, to int) string {
		return `[{"stepType":"replace","from":` + itoa(from) + `,"to":` + itoa(to) + `}]`
	}
	cases := []struct {
		name     string
		steps    string
		from, to int
		gone     bool
		quote    string
	}{
		{"before", ins(1, "I "), 7, 12, false, "hello"},
		{"at the start", ins(5, "oh "), 8, 13, false, "hello"},
		{"at the end", ins(10, "!"), 5, 10, false, "hello"},
		{"inside", ins(7, "LL"), 5, 12, false, "heLLllo"},
		{"after", ins(13, " please"), 5, 10, false, "hello"},
		{"part of it", del(8, 12), 5, 8, false, "hel"},
		{"all of it", del(5, 10), 5, 5, true, ""},
		{"with its neighbours", del(2, 13), 2, 2, true, ""},
		// Typed over: the edges stay at the edges of what replaced it.
		{"typed over", `[{"stepType":"replace","from":5,"to":10,"slice":{"content":[` + text("bye") + `]}}]`, 5, 8, false, "bye"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			next, steps := apply(t, c.steps)
			f, to, gone := MapRange(next, steps, 5, 10)
			if f != c.from || to != c.to || gone != c.gone {
				t.Errorf("got %d %d %v, want %d %d %v", f, to, gone, c.from, c.to, c.gone)
			}
			if !gone {
				if q, _ := InlineText(next, f, to); q != c.quote {
					t.Errorf("text %q, want %q", q, c.quote)
				}
			}
		})
	}
	// A range over two blocks whose text went, the blocks staying.
	two := mustDoc(t, docOf(`{"type":"paragraph","content":[`+text("ab")+`]}`, `{"type":"paragraph","content":[`+text("cd")+`]}`))
	steps, _ := ParseSteps([]byte(`[{"stepType":"replace","from":2,"to":3},{"stepType":"replace","from":4,"to":5}]`))
	next, err := Apply(two, steps)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, gone := MapRange(next, steps, 2, 6); !gone {
		t.Errorf("no text left in %s, not gone", next)
	}
}

// InlineText refuses ranges the document does not have.
func TestInlineText(t *testing.T) {
	d := mustDoc(t, docOf(`{"type":"paragraph","content":[`+text("a😀")+`,{"type":"hard_break"},`+text("b")+`]}`, `{"type":"paragraph","content":[`+text("c")+`]}`))
	for _, c := range []struct {
		from, to int
		want     string
		ok       bool
	}{{1, 7, "a😀b", true}, {1, 10, "a😀bc", true}, {0, 99, "", false}, {3, 2, "", false}, {1, 3, "", false}, {4, 5, "", true}} {
		got, ok := InlineText(d, c.from, c.to)
		if got != c.want || ok != c.ok {
			t.Errorf("%d %d: %q %v, want %q %v", c.from, c.to, got, ok, c.want, c.ok)
		}
	}
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// Find puts a seeded inline comment on its text.
func TestFind(t *testing.T) {
	d := mustDoc(t, docOf(`{"type":"paragraph","content":[`+text("a😀")+`]}`, `{"type":"paragraph","content":[`+text("say hello")+`]}`))
	if f, to, ok := Find(d, "hello"); !ok || f != 10 || to != 15 {
		t.Errorf("hello at %d %d %v", f, to, ok)
	}
	if _, _, ok := Find(d, "absent"); ok {
		t.Error("found what is not there")
	}
}
