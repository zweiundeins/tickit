package docnodes

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/zweiundeins/prosemirror-go"
)

// The reader view's render (internal/web, readerHTML) as it is, a
// BlockCache kept through the resolver's changes, which the atoms' keys
// follow (keyed), and as it was before DynamicKey: issue lists rendered
// every time, the whole cache reset when the resolver's answers changed
// (reset).
type readerRender interface {
	render(d prosemirror.Doc, r StaticResolver) string
}

type keyed struct{ c BlockCache }

func (k *keyed) render(d prosemirror.Doc, r StaticResolver) string { return k.c.Render(d, r) }

type reset struct {
	c   *prosemirror.BlockCache[Resolver]
	key string
}

func newReset() *reset {
	old := *html
	old.Dynamic, old.DynamicKey = []string{"issue_list"}, nil
	return &reset{c: prosemirror.NewBlockCache(&old)}
}

func (rr *reset) render(d prosemirror.Doc, r StaticResolver) string {
	if k, _ := json.Marshal(r); string(k) != rr.key {
		rr.c.Reset()
		rr.key = string(k)
	}
	return rr.c.Render(d, r)
}

// benchDoc is a page of 160 blocks with chips and mentions, and one
// paragraph with a chip of WEB-13.
func benchDoc(b *testing.B) prosemirror.Doc {
	d := bigDoc(b)
	st, err := schema.ParseStep([]byte(`{"stepType":"replace","from":0,"to":0,"slice":{"content":[{"type":"paragraph","content":[{"type":"issue_chip","attrs":{"key":"WEB-13"}}]}]}}`))
	if err != nil {
		b.Fatal(err)
	}
	if d, err = st.Apply(d); err != nil {
		b.Fatal(err)
	}
	return d
}

// A frame while people type: one block changed, the resolver the same.
func benchTyping(b *testing.B, rr readerRender) {
	d := benchDoc(b)
	docs := []prosemirror.Doc{d}
	for i := 0; i < 50; i++ {
		st, err := schema.ParseStep([]byte(`{"stepType":"replace","from":` + strconv.Itoa(30+i) + `,"to":` + strconv.Itoa(30+i) + `,"slice":{"content":[{"type":"text","text":"x"}]}}`))
		if err != nil {
			b.Fatal(err)
		}
		if d, err = st.Apply(d); err != nil {
			b.Fatal(err)
		}
		docs = append(docs, d)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.render(docs[i%len(docs)], testResolver)
	}
}

// A frame after an issue changed that one block shows.
func benchChip(b *testing.B, rr readerRender) {
	d := benchDoc(b)
	res := [2]StaticResolver{testResolver, testResolver}
	res[1].Issues = map[string]Chip{"WEB-12": testResolver.Issues["WEB-12"], "WEB-13": {Summary: "Back", Status: "Open", Type: "task"}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.render(d, res[i%2])
	}
}

func BenchmarkReaderTypingReset(b *testing.B) { benchTyping(b, newReset()) }
func BenchmarkReaderTypingKeyed(b *testing.B) { benchTyping(b, &keyed{}) }
func BenchmarkReaderChipReset(b *testing.B)   { benchChip(b, newReset()) }
func BenchmarkReaderChipKeyed(b *testing.B)   { benchChip(b, &keyed{}) }
