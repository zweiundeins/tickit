package docnodes

import (
	"strings"
	"testing"
)

// DM-05, DM-06, FR-83, FR-84, FR-100, FR-103: refused documents, with the
// sentence a person reads.
func TestRulesRefuse(t *testing.T) {
	cell := func(attrs string) string {
		return `{"type":"table_cell","attrs":` + attrs + `,"content":[` + para + `]}`
	}
	row := func(cells ...string) string {
		return `{"type":"table_row","content":[` + strings.Join(cells, ",") + `]}`
	}
	cases := []struct{ doc, msg string }{
		{docOf(`{"type":"heading","attrs":{"level":4}}`), "A heading has level 1, 2 or 3."},
		{docOf(`{"type":"heading","attrs":{"level":1.5}}`), "A heading has level 1, 2 or 3."},
		{docOf(`{"type":"code_block","attrs":{"language":"cobol"}}`), "Code blocks take one of the listed languages."},
		{docOf(`{"type":"ordered_list","attrs":{"order":0},"content":[{"type":"list_item","content":[` + para + `]}]}`), "A numbered list starts at a number from 1 to 9,999."},
		{docOf(`{"type":"image","attrs":{"imageId":"evil"}}`), "Images come from the image library."},
		{docOf(`{"type":"image","attrs":{"imageId":"img-1","src":"https://evil.example/x.png"}}`), "Images come from the image library."},
		{inPara(`{"type":"issue_chip","attrs":{"key":"WEB-0"}}`), "An issue chip needs an issue key such as WEB-12."},
		{inPara(`{"type":"issue_chip","attrs":{"key":"web-1"}}`), "An issue chip needs an issue key such as WEB-12."},
		{inPara(`{"type":"mention","attrs":{"personId":"p1"}}`), "A mention names a person of this site."},
		{inPara(`{"type":"mention","attrs":{"personId":"s-priya","label":"A"}}`), "A mention's name has 2 to 30 characters."},
		{inPara(`{"type":"page_link","attrs":{"pageId":"pg1"}}`), "A page link names a page of this site."},
		{inPara(`{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}`), "A link must start with http://, https:// or mailto: and be at most 2,048 characters."},
		{inPara(`{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"https://` + strings.Repeat("a", 2050) + `"}}]}`), "An attribute holds at most 2,048 characters."},
		{inPara(`{"type":"text","text":"a\u0007b"}`), "The text holds a control character."},
		{docOf(`{"type":"table","content":[` + row(cell(`{"colspan":21}`)) + `]}`), "A table cell spans 1 to 20 columns and 1 to 50 rows."},
		{docOf(`{"type":"table","content":[` + row(cell(`{"colspan":12}`), cell(`{"colspan":9}`)) + `]}`), "A table holds at most 20 columns."},
		{docOf(`{"type":"table","content":[` + row(cell(`{"colspan":12,"rowspan":2}`)) + `,` + row(cell(`{"colspan":9}`)) + `]}`), "A table holds at most 20 columns."},
		{docOf(`{"type":"table","content":[` + strings.Repeat(row(cell(`{}`))+",", 50) + row(cell(`{}`)) + `]}`), "A table holds at most 50 rows."},
		{docOf(`{"type":"table","content":[` + row(cell(`{"colspan":2,"colwidth":[100]}`)) + `]}`), "A table cell's column widths do not match its columns."},
		{docOf(strings.Repeat(`{"type":"horizontal_rule"},`, 2000) + `{"type":"horizontal_rule"}`), "The page is at its limit of 2,000 blocks."},
		{docOf(strings.Repeat(`{"type":"code_block","content":[{"type":"text","text":"`+strings.Repeat("x", 1000)+`"}]},`, 205) + para), "The page is at its limit of 200 KB."},
	}
	for i, c := range cases {
		err := library.Check(nil, mustDoc(t, c.doc))
		if err == nil || err.Error() != c.msg {
			t.Errorf("case %d: %v, want %q", i, err, c.msg)
		}
	}
	ok := []string{
		docOf(`{"type":"code_block","attrs":{"language":"go"},"content":[{"type":"text","text":"a\tb\nc"}]}`),
		inPara(`{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"mailto:ada@example.com"}}]}`),
		inPara(`{"type":"mention","attrs":{"personId":"g-abcdefghijklmnopqrst"}},{"type":"mention","attrs":{"personId":"a-0192f0c4-7b1e-7cc3-9d2a-5f6e7a8b9c0d","label":"Ada"}}`),
		docOf(`{"type":"table","content":[` + row(cell(`{"colspan":2,"colwidth":[100,0]}`), cell(`{"colspan":18}`)) + `]}`),
		docOf(`{"type":"image","attrs":{"imageId":"img-1","src":"/img/lib/img-1"}}`),
	}
	for i, c := range ok {
		if err := library.Check(nil, mustDoc(t, c)); err != nil {
			t.Errorf("ok %d: %v", i, err)
		}
	}
}

// Documents are shared between the writer, views and jobs: concurrent
// reads, caches included, are safe (run with -race).
func TestConcurrentReads(t *testing.T) {
	d := bigDoc(t)
	st, _ := schema.ParseStep([]byte(`{"stepType":"replace","from":3,"to":3,"slice":{"content":[{"type":"text","text":"x"}]}}`))
	var c BlockCache
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			after, err := st.Apply(d)
			if err != nil {
				t.Error(err)
				return
			}
			_ = library.Check(d, after)
			_ = RenderHTML(after, testResolver)
			_ = c.Render(after, testResolver)
			_ = Text(d)
			_ = d.JSONLen() + d.Blocks()
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
