package docnodes

import (
	"errors"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/zweiundeins/prosemirror-go"
	"github.com/zweiundeins/prosemirror-go/oracle"

	"github.com/zweiundeins/tickit/internal/demo/seed"
)

// tickit's schema.json in the real prosemirror-model and
// prosemirror-transform (prosemirror-go's package oracle): the seed's
// documents and hand-made ones parse, check and write back as V8 does, and
// edits of them, with tickit's atoms, apply alike.
func newOracle(t *testing.T) *oracle.Oracle {
	o, err := oracle.New(SchemaJSON())
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// seedDocs are the documents of the seed: pages, their versions,
// descriptions, comments and templates.
func seedDocs(t *testing.T) []string {
	c, err := seed.Load()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	add := func(markup, space string) {
		d, err := seed.Parse(markup, c.Resolver(space, nil))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(d))
	}
	for _, is := range c.Issues {
		add(string(is.Description), "")
		for _, cm := range is.Comments {
			add(string(cm.Body), "")
		}
	}
	for _, p := range c.Pages {
		for _, v := range p.Versions {
			add(v.Body, p.Space)
		}
	}
	for _, tp := range c.Templates {
		add(tp.Body, "")
	}
	return out
}

func TestSchemaAgreesWithOracle(t *testing.T) {
	o := newOracle(t)
	cases := append(seedDocs(t),
		docOf(`{"type":"issue_list"}`),
		docOf(issueList(`{"filter":"status=todo&project=WEB","limit":5}`)),
		docOf(issueList(`{"limit":"5"}`)),
		docOf(issueList(`{"filter":7}`)),
		docOf(`{"type":"paragraph","content":[{"type":"issue_list"}]}`),
		docOf(`{"type":"issue_list","content":[`+para+`]}`),
		inPara(`{"type":"issue_chip"}`),
		inPara(`{"type":"issue_chip","attrs":{"key":7}}`),
		inPara(`{"type":"issue_chip","attrs":{"key":"WEB-1"},"marks":[{"type":"link","attrs":{"href":"https://x.y"}},{"type":"strong"}]}`),
		inPara(`{"type":"mention","attrs":{"personId":"s-ada"}},{"type":"mention","attrs":{"label":"x"}}`),
		inPara(`{"type":"page_link","attrs":{"pageId":"p"}},{"type":"hard_break"}`),
		docOf(`{"type":"code_block","content":[{"type":"text","text":"x","marks":[{"type":"strong"}]}]}`),
		docOf(`{"type":"table","content":[{"type":"table_row","content":[{"type":"table_cell","attrs":{"colwidth":[100]},"content":[`+para+`]}]}]}`),
	)
	n := 0
	for i, c := range cases {
		want, err := o.Check(c)
		if err != nil {
			t.Fatal(err)
		}
		d, err := ParseDoc([]byte(c))
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != want {
			t.Errorf("case %d %.200s:\n go: %q\n js: %q", i, c, got, want)
			continue
		}
		if err != nil {
			continue
		}
		n++
		out, _ := prosemirror.MarshalDoc(d)
		if js, _ := o.Roundtrip(c); string(out) != js {
			t.Errorf("case %d writes %.200s\n js %.200s", i, out, js)
		}
	}
	t.Logf("%d documents, %d valid", len(cases), n)
}

// Edits with tickit's atoms and attributes, at random places of the seed's
// pages: each applies or fails in Go as in prosemirror-transform, and the
// documents stay equal.
func TestEditsAgreeWithOracle(t *testing.T) {
	o := newOracle(t)
	rnd := rand.New(rand.NewPCG(7, 11))
	steps, failed := 0, 0
	for _, c := range seedDocs(t)[:40] {
		d, err := ParseDoc([]byte(c))
		if err != nil {
			t.Fatal(err)
		}
		id, err := o.Load(c)
		if err != nil {
			t.Fatal(err)
		}
		for k := 0; k < 25; k++ {
			size := d.Content().Size()
			a := rnd.IntN(size + 1)
			b := min(size, a+rnd.IntN(12))
			var raw string
			switch rnd.IntN(6) {
			case 0:
				raw = `{"stepType":"replace","from":` + strconv.Itoa(a) + `,"to":` + strconv.Itoa(b) + `}`
			case 1:
				raw = `{"stepType":"replace","from":` + strconv.Itoa(a) + `,"to":` + strconv.Itoa(a) + `,"slice":{"content":[{"type":"issue_chip","attrs":{"key":"WEB-` + strconv.Itoa(1+rnd.IntN(99)) + `"}}]}}`
			case 2:
				raw = `{"stepType":"replace","from":` + strconv.Itoa(a) + `,"to":` + strconv.Itoa(b) + `,"slice":{"content":[{"type":"mention","attrs":{"personId":"s-ada","label":"Ada"}},{"type":"text","text":"é😀"}]}}`
			case 3:
				raw = `{"stepType":"addMark","from":` + strconv.Itoa(a) + `,"to":` + strconv.Itoa(b) + `,"mark":{"type":"link","attrs":{"href":"https://x.y"}}}`
			case 4:
				raw = `{"stepType":"replace","from":` + strconv.Itoa(a) + `,"to":` + strconv.Itoa(a) + `,"slice":{"content":[{"type":"issue_list","attrs":{"filter":"type=bug","savedFilter":"","limit":3}}]}}`
			default:
				raw = `{"stepType":"removeMark","from":` + strconv.Itoa(a) + `,"to":` + strconv.Itoa(b) + `,"mark":{"type":"strong"}}`
			}
			st, err := schema.ParseStep([]byte(raw))
			if err != nil {
				t.Fatalf("%s: %v", raw, err)
			}
			next, goErr := st.Apply(d)
			if errors.Is(goErr, prosemirror.ErrSurrogateSplit) {
				continue // Go refuses a cut through a surrogate pair, which no browser makes
			}
			js, jsErr := o.Apply(id, raw)
			var sf *prosemirror.StepFailed
			switch {
			case jsErr != nil || goErr != nil && !errors.As(goErr, &sf):
				if jsErr == nil || goErr == nil || errors.As(goErr, &sf) {
					t.Fatalf("%s: js %v, go %v", raw, jsErr, goErr)
				}
				failed++
				continue
			case js != "" || goErr != nil:
				if js == "" || goErr == nil {
					t.Fatalf("%s: js %q, go %v", raw, js, goErr)
				}
				failed++
				continue
			}
			d = next
			steps++
			got, _ := prosemirror.MarshalDoc(d)
			if want, _ := o.JSON(id); string(got) != want {
				t.Fatalf("%s:\n go %.300s\n js %.300s", raw, got, want)
			}
		}
		o.Free(id)
	}
	t.Logf("%d steps applied, %d failed or refused alike", steps, failed)
}
