package docnodes

import (
	"strconv"
	"strings"
	"testing"

	"github.com/zweiundeins/prosemirror-go"
)

// sizedDoc is a page of paragraphs and short bullet lists, with a heading
// every 50 blocks, filling blocks block nodes and about bytes stored.
func sizedDoc(t testing.TB, blocks, bytes int) prosemirror.Doc {
	// A section: a heading, 24 paragraphs, a list of 12 items (25 blocks).
	sections := blocks / 62
	words := max(1, (bytes/sections-1200)/(24*30))
	var out []string
	for s := 0; s < sections; s++ {
		out = append(out, heading(2, "Section "+strconv.Itoa(s)))
		for p := 0; p < 24; p++ {
			out = append(out, `{"type":"paragraph","content":[`+text(strings.Repeat("a few words of text, ", words))+`]}`)
		}
		var items []string
		for i := 0; i < 18; i++ {
			items = append(items, `{"type":"list_item","content":[`+para+`]}`)
		}
		out = append(out, `{"type":"bullet_list","content":[`+strings.Join(items, ",")+`]}`)
	}
	d, err := schema.ParseDoc([]byte(docOf(out...)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// PI-18, DM-05.1: what a page at the demo's limits (2,000 blocks, 200 KB)
// and at a private instance's (10,000 blocks, 1 MB) costs: the reader's
// render and its frame, the block cache after a keystroke, the writer's
// check of a commit, the JSON of a snapshot, a checkpoint or a version,
// and reading it back (benchmarks/RESULTS.md, "Page limits").
func BenchmarkPageLimits(b *testing.B) {
	for _, size := range []struct {
		name          string
		blocks, bytes int
	}{{"demo-2000-200KB", 2000, 200 << 10}, {"private-10000-1MB", 10000, 1 << 20}} {
		d := sizedDoc(b, size.blocks, size.bytes)
		html := RenderHTML(d, nil)
		b.Run("render/"+size.name, func(b *testing.B) {
			for b.Loop() {
				RenderHTML(d, nil)
			}
			b.ReportMetric(float64(d.Blocks()), "blocks")
			b.ReportMetric(float64(d.JSONLen()), "stored-bytes")
			b.ReportMetric(float64(len(html)), "html-bytes")
			b.ReportMetric(float64(gzipLen(html)), "gzip-bytes")
		})
		st, err := schema.ParseStep([]byte(`{"stepType":"replace","from":12,"to":12,"slice":{"content":[{"type":"text","text":"x"}]}}`))
		if err != nil {
			b.Fatal(err)
		}
		next, err := st.Apply(d)
		if err != nil {
			b.Fatal(err)
		}
		b.Run("cache-after-a-keystroke/"+size.name, func(b *testing.B) {
			var c BlockCache
			for b.Loop() {
				b.StopTimer()
				c.Render(d, nil)
				b.StartTimer()
				c.Render(next, nil)
			}
		})
		r := Rules{}.Rules()
		r.MaxBlocks, r.MaxBytes = 0, 0
		b.Run("commit/"+size.name, func(b *testing.B) {
			for b.Loop() {
				after, err := st.Apply(d)
				if err != nil {
					b.Fatal(err)
				}
				if err := r.Check(d, after); err != nil {
					b.Fatal(err)
				}
				_, _ = after.JSONLen(), after.Blocks()
			}
		})
		raw := Marshal(d)
		b.Run("marshal/"+size.name, func(b *testing.B) {
			for b.Loop() {
				Marshal(d)
			}
		})
		b.Run("parse/"+size.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := ParseDoc(raw); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
