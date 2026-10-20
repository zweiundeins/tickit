package doc

import (
	"strings"
	"testing"
)

var testResolver = StaticResolver{
	Issues: map[string]Chip{
		"WEB-12": {Summary: "Fix the <login>", Status: "In Review", Category: "progress", Type: "bug"},
		"WEB-13": {Deleted: true},
	},
	People: map[string]string{"s-ada": "Ada \"Countess\""},
	Pages:  map[string]string{"0192f0c4-7b1e-7cc3-9d2a-5f6e7a8b9c0d": "Release <plan>"},
}

func text(s string, marks ...string) string {
	if len(marks) == 0 {
		return `{"type":"text","text":` + string(appendString(nil, s)) + `}`
	}
	return `{"type":"text","text":` + string(appendString(nil, s)) + `,"marks":[` + strings.Join(marks, ",") + `]}`
}

const (
	strong = `{"type":"strong"}`
	em     = `{"type":"em"}`
)

func link(href string) string {
	return `{"type":"link","attrs":{"href":` + string(appendString(nil, href)) + `}}`
}

// The HTML table of docs/editor.md, node by node (DM-06).
func TestRenderHTML(t *testing.T) {
	cell := func(typ, attrs, s string) string {
		return `{"type":"` + typ + `","attrs":` + attrs + `,"content":[{"type":"paragraph","content":[` + text(s) + `]}]}`
	}
	cases := []struct{ doc, html string }{
		{docOf(`{"type":"paragraph"}`), `<p></p>`},
		{docOf(`{"type":"heading","attrs":{"level":1},"content":[`+text("A")+`]}`, `{"type":"heading","attrs":{"level":3},"content":[`+text("C")+`]}`), `<h2>A</h2><h4>C</h4>`},
		{docOf(`{"type":"blockquote","content":[` + para + `]}`), `<blockquote><p>hi</p></blockquote>`},
		{docOf(`{"type":"code_block","attrs":{"language":"go"},"content":[`+text("a < b\n")+`]}`, `{"type":"code_block"}`), `<pre data-language="go"><code>a &lt; b` + "\n" + `</code></pre><pre><code></code></pre>`},
		{docOf(`{"type":"horizontal_rule"}`), `<hr>`},
		{docOf(`{"type":"bullet_list","content":[{"type":"list_item","content":[` + para + `]}]}`), `<ul><li><p>hi</p></li></ul>`},
		{docOf(`{"type":"ordered_list","attrs":{"order":3},"content":[{"type":"list_item","content":[`+para+`]}]}`, `{"type":"ordered_list","content":[{"type":"list_item","content":[`+para+`]}]}`), `<ol start="3"><li><p>hi</p></li></ol><ol><li><p>hi</p></li></ol>`},
		{docOf(`{"type":"task_list","content":[{"type":"task_item","attrs":{"checked":true},"content":[` + para + `]},{"type":"task_item","content":[` + para + `]}]}`), `<ul class="task-list"><li class="task-item" data-checked="true"><input type="checkbox" disabled checked><div><p>hi</p></div></li><li class="task-item" data-checked="false"><input type="checkbox" disabled><div><p>hi</p></div></li></ul>`},
		{docOf(`{"type":"table","content":[{"type":"table_row","content":[` + cell("table_header", `{"colspan":2}`, "h") + `]},{"type":"table_row","content":[` + cell("table_cell", `{"rowspan":2}`, "a") + `,` + cell("table_cell", `{}`, "b") + `]}]}`), `<table><tbody><tr><th colspan="2"><p>h</p></th></tr><tr><td rowspan="2"><p>a</p></td><td><p>b</p></td></tr></tbody></table>`},
		{docOf(`{"type":"image","attrs":{"imageId":"img-1","src":"https://evil.example/x.png","alt":"A \"diagram\""}}`), `<figure class="doc-image"><img src="/img/lib/img-1" alt="A &#34;diagram&#34;" loading="lazy"></figure>`},
		{inPara(text("a") + `,{"type":"hard_break"},` + text("b")), `<p>a<br>b</p>`},
		{inPara(`{"type":"issue_chip","attrs":{"key":"WEB-12"}}`), `<p><a class="issue-chip" data-key="WEB-12" href="/WEB-12" data-type="bug" data-category="progress"><span class="issue-chip-type" aria-hidden="true"></span><span class="issue-chip-key">WEB-12</span> <span class="issue-chip-summary">Fix the &lt;login&gt;</span> <span class="issue-chip-status">In Review</span></a></p>`},
		{inPara(`{"type":"issue_chip","attrs":{"key":"WEB-13"}},{"type":"issue_chip","attrs":{"key":"WEB-99"}}`), `<p><a class="issue-chip" data-key="WEB-13" href="/WEB-13" data-state="deleted"><span class="issue-chip-key">WEB-13</span> <span class="issue-chip-status">deleted issue</span></a><a class="issue-chip" data-key="WEB-99" href="/WEB-99" data-state="unknown"><span class="issue-chip-key">WEB-99</span> <span class="issue-chip-status">unknown issue</span></a></p>`},
		{inPara(`{"type":"mention","attrs":{"personId":"s-ada","label":"Old"}},{"type":"mention","attrs":{"personId":"s-bob","label":"Bob"}}`), `<p><a class="mention" data-person="s-ada" href="/people/s-ada">@Ada &#34;Countess&#34;</a><a class="mention" data-person="s-bob" href="/people/s-bob">@Bob</a></p>`},
		{inPara(`{"type":"page_link","attrs":{"pageId":"0192f0c4-7b1e-7cc3-9d2a-5f6e7a8b9c0d"}},{"type":"page_link","attrs":{"pageId":"gone"}}`), `<p><a class="page-link" data-page="0192f0c4-7b1e-7cc3-9d2a-5f6e7a8b9c0d" href="/pages/0192f0c4-7b1e-7cc3-9d2a-5f6e7a8b9c0d">Release &lt;plan&gt;</a><span class="page-link" data-state="gone">a removed page</span></p>`},
		{inPara(text("b", strong) + `,` + text("i", em) + `,` + text("s", `{"type":"strike"}`) + `,` + text("c", `{"type":"code"}`)), `<p><strong>b</strong><em>i</em><s>s</s><code>c</code></p>`},
		// Marks shared with the previous node stay open.
		{inPara(text("a", strong) + `,` + text("b", strong, em) + `,` + text("c", strong)), `<p><strong>a<em>b</em>c</strong></p>`},
		{inPara(text("go", link("https://example.com/x?a=1&b=2"), strong) + `,` + text("!", link("https://example.com/x?a=1&b=2"))), `<p><a href="https://example.com/x?a=1&amp;b=2" rel="nofollow ugc noopener" title="example.com"><strong>go</strong>!</a></p>`},
		// No <a> around an atom inside a link mark.
		{inPara(text("see ", link("https://x.y")) + `,{"type":"issue_chip","attrs":{"key":"WEB-99"},"marks":[` + link("https://x.y") + `,` + strong + `]},` + text(" now", link("https://x.y"))), `<p><a href="https://x.y" rel="nofollow ugc noopener" title="x.y">see </a><strong><a class="issue-chip" data-key="WEB-99" href="/WEB-99" data-state="unknown"><span class="issue-chip-key">WEB-99</span> <span class="issue-chip-status">unknown issue</span></a></strong><a href="https://x.y" rel="nofollow ugc noopener" title="x.y"> now</a></p>`},
		{inPara(text("mail", link("mailto:ada@example.com"))), `<p><a href="mailto:ada@example.com" rel="nofollow ugc noopener" title="ada@example.com">mail</a></p>`},
	}
	for i, c := range cases {
		if got := RenderHTML(mustDoc(t, c.doc), testResolver); got != c.html {
			t.Errorf("case %d:\n got %s\nwant %s", i, got, c.html)
		}
	}
}

// DM-06.1 and DM-06.2: what people type or paste never becomes markup.
func TestEscapes(t *testing.T) {
	cases := []struct{ doc, html string }{
		{inPara(text(`<script>alert("x")</script>`)), `<p>&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;</p>`},
		{inPara(text("x", link("javascript:alert(1)"))), `<p><span class="link-unsafe">x</span></p>`},
		{inPara(text("x", link(" javascript:alert(1)"))), `<p><span class="link-unsafe">x</span></p>`},
		{inPara(text("x", link("data:text/html;base64,PHNjcmlwdD4="))), `<p><span class="link-unsafe">x</span></p>`},
		{inPara(text("x", link(`https://x.y/" onmouseover="alert(1)`))), `<p><span class="link-unsafe">x</span></p>`},
		{inPara(text("x", link(`https://x.y/'><img/src=x/onerror=alert(1)>`))), `<p><a href="https://x.y/&#39;&gt;&lt;img/src=x/onerror=alert(1)&gt;" rel="nofollow ugc noopener" title="x.y">x</a></p>`},
		{docOf(`{"type":"code_block","attrs":{"language":"\"><script>"}}`), `<pre data-language="&#34;&gt;&lt;script&gt;"><code></code></pre>`},
		{docOf(`{"type":"image","attrs":{"imageId":"../../x\"onerror","alt":"<b>"}}`), `<figure class="doc-image"><img src="/img/lib/..%2F..%2Fx%22onerror" alt="&lt;b&gt;" loading="lazy"></figure>`},
		{inPara(`{"type":"issue_chip","attrs":{"key":"\"><b>"}}`), `<p><a class="issue-chip" data-key="&#34;&gt;&lt;b&gt;" href="/%22%3E%3Cb%3E" data-state="unknown"><span class="issue-chip-key">&#34;&gt;&lt;b&gt;</span> <span class="issue-chip-status">unknown issue</span></a></p>`},
		{inPara(`{"type":"mention","attrs":{"personId":"\" x=\"","label":"<i>"}}`), `<p><a class="mention" data-person="&#34; x=&#34;" href="/people/%22%20x=%22">@&lt;i&gt;</a></p>`},
	}
	for i, c := range cases {
		if got := RenderHTML(mustDoc(t, c.doc), testResolver); got != c.html {
			t.Errorf("case %d:\n got %s\nwant %s", i, got, c.html)
		}
	}
}

func TestBlockCache(t *testing.T) {
	d := mustDoc(t, string(readFixture(t, "f20k-doc.json")))
	var c BlockCache
	if got, want := c.Render(d, testResolver), RenderHTML(d, testResolver); got != want {
		t.Fatal("cache renders differently")
	}
	st, _ := tickit.ParseStep([]byte(`{"stepType":"replace","from":3,"to":3,"slice":{"content":[{"type":"text","text":"<x>"}]}}`))
	d2, err := st.Apply(d)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.Render(d2, testResolver), RenderHTML(d2, testResolver); got != want {
		t.Fatal("cache renders the changed document differently")
	}
}

func TestTextAndReferences(t *testing.T) {
	d := mustDoc(t, docOf(
		`{"type":"heading","attrs":{"level":1},"content":[`+text("Title")+`]}`,
		`{"type":"paragraph","content":[`+text("see ")+`,{"type":"issue_chip","attrs":{"key":"WEB-12"}},`+text(" and ")+`,{"type":"mention","attrs":{"personId":"s-ada","label":"Ada"}},{"type":"hard_break"},`+text("x")+`,{"type":"page_link","attrs":{"pageId":"p2"}},{"type":"issue_chip","attrs":{"key":"WEB-12"}}]}`,
		`{"type":"bullet_list","content":[{"type":"list_item","content":[`+para+`]}]}`,
		`{"type":"image","attrs":{"imageId":"img-1","alt":"diagram"}}`,
		`{"type":"horizontal_rule"}`,
		`{"type":"code_block","content":[`+text("a\nb")+`]}`,
	))
	if got, want := Text(d), "Title\nsee WEB-12 and @Ada\nxWEB-12\nhi\ndiagram\na\nb"; got != want {
		t.Errorf("Text: %q, want %q", got, want)
	}
	r := References(d)
	if strings.Join(r.Issues, ",") != "WEB-12" || strings.Join(r.People, ",") != "s-ada" || strings.Join(r.Pages, ",") != "p2" {
		t.Errorf("References: %+v", r)
	}
}
