package doc

import (
	"encoding/json"
	"fmt"
	"testing"
)

type batchBench struct {
	docs    []Doc
	batches [][]Step
}

func loadBatches(b *testing.B, prefix string) batchBench {
	d := mustDoc(b, string(readFixture(b, prefix+"-doc.json")))
	var bb batchBench
	for _, raw := range rawList(b, readFixture(b, prefix+"-batches.json")) {
		steps, err := ParseSteps(raw)
		if err != nil {
			b.Fatal(err)
		}
		bb.docs = append(bb.docs, d)
		bb.batches = append(bb.batches, steps)
		if d, err = Apply(d, steps); err != nil {
			b.Fatal(err)
		}
	}
	return bb
}

func benchBatch(b *testing.B, prefix string) {
	bb := loadBatches(b, prefix)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := i % len(bb.docs)
		if _, err := Apply(bb.docs[k], bb.batches[k]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkApplyBatch5_20k(b *testing.B)  { benchBatch(b, "f20k") }
func BenchmarkApplyBatch5_200k(b *testing.B) { benchBatch(b, "f200k") }

func benchRebase(b *testing.B, prefix string, k int) {
	d := mustDoc(b, string(readFixture(b, prefix+"-doc.json")))
	var commits []commitJSON
	if err := json.Unmarshal(readFixture(b, fmt.Sprintf("%s-k%d-commits.json", prefix, k)), &commits); err != nil {
		b.Fatal(err)
	}
	type c struct {
		doc          Doc
		since, steps []Step
	}
	var cs []c
	var history [][]Step
	for _, cm := range commits {
		steps, _ := ParseSteps(cm.Steps)
		var since []Step
		for v := cm.Base; v < len(history); v++ {
			since = append(since, history[v]...)
		}
		cs = append(cs, c{d, since, steps})
		next, applied, err := Rebase(d, since, steps)
		if err != nil {
			b.Fatal(err)
		}
		d = next
		history = append(history, applied)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := cs[i%len(cs)]
		if _, _, err := Rebase(x.doc, x.since, x.steps); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRebaseK5_20k(b *testing.B)  { benchRebase(b, "f20k", 5) }
func BenchmarkRebaseK20_20k(b *testing.B) { benchRebase(b, "f20k", 20) }
func BenchmarkRebaseK5_200k(b *testing.B) { benchRebase(b, "f200k", 5) }

func benchParse(b *testing.B, name string) {
	data := readFixture(b, name+".json")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ParseDoc(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseDoc20k(b *testing.B)  { benchParse(b, "f20k-doc") }
func BenchmarkParseDoc200k(b *testing.B) { benchParse(b, "f200k-doc") }

func benchMarshal(b *testing.B, name string) {
	d := mustDoc(b, string(readFixture(b, name+".json")))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MarshalDoc(d)
	}
}

func BenchmarkMarshalDoc20k(b *testing.B)  { benchMarshal(b, "f20k-doc") }
func BenchmarkMarshalDoc200k(b *testing.B) { benchMarshal(b, "f200k-doc") }

func benchRender(b *testing.B, name string) {
	d := mustDoc(b, string(readFixture(b, name+".json")))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RenderHTML(d, testResolver)
	}
}

func BenchmarkRenderHTML20k(b *testing.B)  { benchRender(b, "f20k-doc") }
func BenchmarkRenderHTML200k(b *testing.B) { benchRender(b, "f200k-doc") }

func benchText(b *testing.B, name string) {
	d := mustDoc(b, string(readFixture(b, name+".json")))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Text(d)
	}
}

func BenchmarkText20k(b *testing.B)  { benchText(b, "f20k-doc") }
func BenchmarkText200k(b *testing.B) { benchText(b, "f200k-doc") }

// The rules on a typing commit of a 200 KB page: the changed block only,
// with the size and block counts cached in the unchanged blocks.
func BenchmarkRulesCheck200k(b *testing.B) {
	bb := loadBatches(b, "f200k")
	for i := range bb.docs {
		bb.docs[i].JSONLen()
		bb.docs[i].Blocks()
	}
	var afters []Doc
	for i, d := range bb.docs {
		a, _ := Apply(d, bb.batches[i])
		afters = append(afters, a)
	}
	r := Rules{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := i % len(bb.docs)
		r.Check(bb.docs[k], afters[k])
	}
}

func BenchmarkDiff20k(b *testing.B) {
	a := mustDoc(b, string(readFixture(b, "f20k-doc.json")))
	c := mustDoc(b, string(readFixture(b, "f20k-final.json")))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Diff(a, c).HTML(testResolver)
	}
}
