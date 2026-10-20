package doc

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

func docOf(blocks ...string) string {
	return `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
}

const para = `{"type":"paragraph","content":[{"type":"text","text":"hi"}]}`

// check() agrees with the oracle on hand-made documents, valid and not,
// with prosemirror-model's messages.
func TestCheckAgreesWithOracle(t *testing.T) {
	o := theOracle(t)
	cases := []string{
		docOf(para),
		`{"type":"doc"}`,
		docOf(`{"type":"paragraph","content":[{"type":"heading","attrs":{"level":1}}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"issue_chip"}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"issue_chip","attrs":{}}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"issue_chip","attrs":""}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"issue_chip","attrs":{"key":7}}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"issue_chip","attrs":{"key":"WEB-1","extra":1}}]}`),
		docOf(`{"type":"video"}`),
		docOf(`{"type":"heading","attrs":{"level":"2"},"content":[{"type":"text","text":"x"}]}`),
		docOf(`{"type":"heading","attrs":{"level":null}}`),
		docOf(`{"type":"heading","attrs":null}`),
		docOf(`{"type":"heading","attrs":0}`),
		docOf(`{"type":"heading","attrs":{"level":2.5}}`),
		docOf(`{"type":"task_list","content":[{"type":"task_item","attrs":{"checked":"yes"},"content":[` + para + `]}]}`),
		docOf(`{"type":"task_list","content":[{"type":"task_item","content":[{"type":"heading"}]}]}`),
		docOf(`{"type":"image","attrs":{"imageId":5}}`),
		docOf(`{"type":"image"}`),
		docOf(`{"type":"image","attrs":{"imageId":"img-1"}}`),
		docOf(`{"type":"code_block","content":[{"type":"text","text":"x","marks":[{"type":"strong"}]}]}`),
		docOf(`{"type":"code_block","content":[{"type":"hard_break"}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"strong"},{"type":"strong"}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"em"},{"type":"strong"}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"blink"}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link"}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":1}}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":{"type":"em"}}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":""}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text"}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}`),
		docOf(`{"type":"paragraph","content":[null]}`),
		docOf(`{"type":"paragraph","content":{"type":"text","text":"a"}}`),
		docOf(`{"type":"paragraph","marks":[{"type":"strong"}]}`),
		docOf(`{"type":"table","content":[{"type":"table_row","content":[` + para + `]}]}`),
		docOf(`{"type":"table","content":[{"type":"table_row","content":[{"type":"table_cell","attrs":{"colspan":"2"},"content":[` + para + `]}]}]}`),
		docOf(`{"type":"table","content":[{"type":"table_row","content":[{"type":"table_cell","attrs":{"colwidth":[100,{"b":1,"2":0,"a":[]}]},"content":[` + para + `]}]}]}`),
		docOf(`{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"bullet_list","content":[{"type":"list_item","content":[` + para + `]}]}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"tab\tquote\" back\\ ctl\u0001 é 😀 \u2028"}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"mention","attrs":{"personId":"s-priya"},"marks":[{"type":"link","attrs":{"href":"https://x.y"}}]}]}`),
		`{"type":"paragraph"}`,
	}
	for i, c := range cases {
		want := o.must(t, "check", c)
		_, err := ParseDoc([]byte(c))
		got := ""
		if err != nil {
			got = err.Error()
		}
		if strings.HasPrefix(got, "A document is") {
			// The oracle checks the node it gets; ParseDoc wants a doc.
			continue
		}
		if got != want {
			t.Errorf("case %d %s:\n go: %q\n js: %q", i, c, got, want)
		}
		if err == nil {
			rt := o.must(t, "roundtrip", c)
			d, _ := ParseDoc([]byte(c))
			if out, _ := MarshalDoc(d); string(out) != rt {
				t.Errorf("case %d: writes %s, oracle %s", i, out, rt)
			}
		}
	}
}

// A lone surrogate is kept by ProseMirror but cannot be held in Go's
// UTF-8 strings: Go refuses the document.
func TestLoneSurrogateRefused(t *testing.T) {
	for _, s := range []string{`\ud83d`, `\ude00`, `a\ud83dx`, `\ud83d\u0041`} {
		_, err := ParseDoc([]byte(docOf(`{"type":"paragraph","content":[{"type":"text","text":"` + s + `"}]}`)))
		if !errors.Is(err, ErrLoneSurrogate) {
			t.Errorf("%s: %v", s, err)
		}
	}
	d, err := ParseDoc([]byte(docOf(`{"type":"paragraph","content":[{"type":"text","text":"\ud83d\ude00"}]}`)))
	if err != nil || d.Child(0).Child(0).Text() != "😀" || d.Child(0).Child(0).NodeSize() != 2 {
		t.Fatalf("pair: %v", err)
	}
	// A step that cuts the pair in half is refused.
	for _, st := range []string{
		`{"stepType":"replace","from":2,"to":2,"slice":{"content":[{"type":"text","text":"x"}]}}`,
		`{"stepType":"replace","from":1,"to":2}`,
		`{"stepType":"addMark","mark":{"type":"em"},"from":2,"to":3}`,
	} {
		s, err := tickit.ParseStep([]byte(st))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(d); !errors.Is(err, ErrSurrogateSplit) {
			t.Errorf("%s: %v", st, err)
		}
	}
}

func TestRefusedSteps(t *testing.T) {
	for _, st := range []string{
		`{"stepType":"docAttr","attr":"x","value":1}`,
		`{"stepType":"addNodeMark","pos":0,"mark":{"type":"em"}}`,
		`{"stepType":"replace","from":1.5,"to":2}`,
		`{"stepType":"replace","from":-1,"to":2}`,
		`{"stepType":"replace","from":3,"to":2}`,
		`{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"x"}],"openStart":1}}`,
		`{"stepType":"replaceAround","from":1,"to":2,"gapFrom":3,"gapTo":2,"insert":0}`,
		`{"stepType":"bogus"}`,
		`{}`,
	} {
		if _, err := tickit.ParseStep([]byte(st)); err == nil {
			t.Errorf("%s: accepted", st)
		}
	}
}

func randomMapping(r *rand.Rand) (string, *Mapping) {
	n := 1 + r.IntN(5)
	m := &Mapping{ownData: true}
	var js struct {
		Maps   [][2]any `json:"maps"`
		Mirror []int    `json:"mirror"`
	}
	for i := 0; i < n; i++ {
		var ranges []int
		pos := r.IntN(5)
		for k := r.IntN(3); k >= 0; k-- {
			ranges = append(ranges, pos, r.IntN(4), r.IntN(4))
			pos += 1 + r.IntN(6)
		}
		inv := r.IntN(3) == 0
		m.AppendMap(&StepMap{ranges: ranges, inverted: inv}, -1)
		js.Maps = append(js.Maps, [2]any{ranges, inv})
	}
	if r.IntN(2) == 0 {
		// A mirror pairs a map with its inverse, as rebasing does.
		a := r.IntN(n)
		inv := m.maps[a].Invert()
		m.AppendMap(inv, a)
		js.Maps = append(js.Maps, [2]any{inv.ranges, inv.inverted})
		js.Mirror = []int{len(m.maps) - 1, a}
	}
	b, _ := json.Marshal(js)
	return string(b), m
}

// StepMap and Mapping with mirrors map positions and steps as
// prosemirror-transform does.
func TestMappingAgreesWithOracle(t *testing.T) {
	o := theOracle(t)
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 3000; i++ {
		js, m := randomMapping(r)
		pos, assoc := r.IntN(30), []int{-1, 1}[r.IntN(2)]
		res := m.MapResult(pos, assoc)
		got := fmt.Sprintf(`{"pos":%d,"deleted":%t,"before":%t,"after":%t,"across":%t}`, res.Pos, res.Deleted(), res.DeletedBefore(), res.DeletedAfter(), res.DeletedAcross())
		if want := o.must(t, "mapPos", js, pos, assoc); got != want {
			t.Fatalf("mapping %s pos %d assoc %d:\n go %s\n js %s", js, pos, assoc, got, want)
		}
		if p := m.Map(pos, assoc); p != res.Pos && m.mirror == nil {
			t.Fatalf("Map %d, MapResult %d", p, res.Pos)
		}
		var st string
		a, b := r.IntN(20), r.IntN(20)
		if a > b {
			a, b = b, a
		}
		switch r.IntN(4) {
		case 0:
			st = fmt.Sprintf(`{"stepType":"replace","from":%d,"to":%d}`, a, b)
		case 1:
			st = fmt.Sprintf(`{"stepType":"addMark","mark":{"type":"em"},"from":%d,"to":%d}`, a, b)
		case 2:
			st = fmt.Sprintf(`{"stepType":"attr","pos":%d,"attr":"level","value":2}`, a)
		default:
			g := a + r.IntN(b-a+1)
			st = fmt.Sprintf(`{"stepType":"replaceAround","from":%d,"to":%d,"gapFrom":%d,"gapTo":%d,"insert":0}`, a, b, g, g+r.IntN(b-g+1))
		}
		s, err := tickit.ParseStep([]byte(st))
		if err != nil {
			t.Fatal(err)
		}
		gotStep := "null"
		if ms := s.Map(m); ms != nil {
			gotStep = string(MarshalStep(ms))
		}
		if want := o.must(t, "mapStep", st, js); gotStep != want {
			t.Fatalf("mapping %s step %s:\n go %s\n js %s", js, st, gotStep, want)
		}
		inv := m.Invert()
		_ = inv.Map(pos, assoc)
	}
}

func TestJSONNumbersAndStrings(t *testing.T) {
	for _, c := range []struct {
		in  float64
		out string
	}{{0, "0"}, {1, "1"}, {-3, "-3"}, {2.5, "2.5"}, {0.1, "0.1"}, {1e21, "1e+21"}, {1e-7, "1e-7"}, {123456789012345680000, "123456789012345680000"}, {1 << 60, "1152921504606847000"}, {0.000001, "0.000001"}, {-1.5e-10, "-1.5e-10"}} {
		if got := string(appendNumber(nil, c.in)); got != c.out {
			t.Errorf("%v: %s, want %s", c.in, got, c.out)
		}
	}
	v, err := ParseValue([]byte(`{"b":1,"2":2,"a":3,"1":4,"b":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(appendValue(nil, v)); got != `{"1":4,"2":2,"b":5,"a":3}` {
		t.Errorf("object order: %s", got)
	}
	if got := string(appendString(nil, "a\"\\\n\x01<>&\u2028é")); got != `"a\"\\\n\u0001<>&`+"\u2028é\"" {
		t.Errorf("string: %s", got)
	}
}
