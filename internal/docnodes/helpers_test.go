package docnodes

import (
	"strings"
	"testing"

	"github.com/zweiundeins/prosemirror-go"
)

func mustDoc(t testing.TB, s string) prosemirror.Doc {
	t.Helper()
	d, err := ParseDoc([]byte(s))
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return d
}

func docOf(blocks ...string) string {
	return `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
}

const para = `{"type":"paragraph","content":[{"type":"text","text":"hi"}]}`

func inPara(inline string) string { return docOf(`{"type":"paragraph","content":[` + inline + `]}`) }

func jsString(s string) string { return string(prosemirror.AppendValue(nil, s)) }

func text(s string, marks ...string) string {
	if len(marks) == 0 {
		return `{"type":"text","text":` + jsString(s) + `}`
	}
	return `{"type":"text","text":` + jsString(s) + `,"marks":[` + strings.Join(marks, ",") + `]}`
}

const (
	strong = `{"type":"strong"}`
	em     = `{"type":"em"}`
)

func link(href string) string {
	return `{"type":"link","attrs":{"href":` + jsString(href) + `}}`
}

var testResolver = StaticResolver{
	Issues: map[string]Chip{
		"WEB-12": {Summary: "Fix the <login>", Status: "In Review", Category: "progress", Type: "bug"},
		"WEB-13": {Deleted: true},
	},
	People: map[string]string{"s-priya": "Ada \"Countess\""},
	Pages:  map[string]string{"0192f0c4-7b1e-7cc3-9d2a-5f6e7a8b9c0d": "Release <plan>"},
}

var library = Rules{Image: func(id string) bool { return strings.HasPrefix(id, "img-") }}

// bigDoc is a document of many kinds of blocks, repeated.
func bigDoc(t testing.TB) prosemirror.Doc {
	var blocks []string
	for i := 0; i < 40; i++ {
		blocks = append(blocks,
			`{"type":"heading","attrs":{"level":2},"content":[`+text("Section")+`]}`,
			`{"type":"paragraph","content":[`+text("see ")+`,{"type":"issue_chip","attrs":{"key":"WEB-12"}},`+text(" and ", strong)+`,{"type":"mention","attrs":{"personId":"s-priya","label":"Ada"}}]}`,
			`{"type":"bullet_list","content":[{"type":"list_item","content":[`+para+`]}]}`,
			`{"type":"code_block","attrs":{"language":"go"},"content":[`+text("x := 1")+`]}`,
		)
	}
	return mustDoc(t, docOf(blocks...))
}
