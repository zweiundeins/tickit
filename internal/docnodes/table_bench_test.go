package docnodes

import (
	"bytes"
	"compress/gzip"
	"strconv"
	"strings"
	"testing"

	"github.com/zweiundeins/prosemirror-go"
)

// tableDoc is a page of a heading, a paragraph and a table of rows by cols
// cells of a few words, as a price list or an inventory holds.
func tableDoc(t testing.TB, rows, cols int) prosemirror.Doc {
	cell := func(typ, s string) string {
		return `{"type":"` + typ + `","content":[{"type":"paragraph","content":[` + text(s) + `]}]}`
	}
	var rs []string
	for r := 0; r < rows; r++ {
		var cs []string
		for c := 0; c < cols; c++ {
			typ, s := "table_cell", "Item "+strconv.Itoa(r)+" value "+strconv.Itoa(c)
			if r == 0 {
				typ, s = "table_header", "Column "+strconv.Itoa(c)
			}
			cs = append(cs, cell(typ, s))
		}
		rs = append(rs, `{"type":"table_row","content":[`+strings.Join(cs, ",")+`]}`)
	}
	d, err := schema.ParseDoc([]byte(docOf(heading(1, "Stock"), para, `{"type":"table","content":[`+strings.Join(rs, ",")+`]}`)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// cellStep types a letter into the last row's first cell.
func cellStep(t testing.TB, d prosemirror.Doc) prosemirror.Step {
	pos := d.Content().Size() - 1 // in the table, before its end
	tbl := d.Child(d.ChildCount() - 1)
	last := tbl.Child(tbl.ChildCount() - 1)
	pos -= last.NodeSize() - 1 - 3 // the start of the last row's first cell's text
	st, err := schema.ParseStep([]byte(`{"stepType":"replace","from":` + strconv.Itoa(pos) + `,"to":` + strconv.Itoa(pos) + `,"slice":{"content":[{"type":"text","text":"x"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func gzipLen(s string) int {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.Len()
}

// DM-05.1: what a table of 50, 200 and 500 rows costs the reader's render,
// its frame, and a commit into it (benchmarks/RESULTS.md, "Table rows").
// The block limit is left out (Rules{} without MaxBlocks), so that the
// cost of the rows alone shows.
func BenchmarkTable(b *testing.B) {
	for _, rows := range []int{50, 200, 500} {
		for _, cols := range []int{1, 4} {
			d := tableDoc(b, rows, cols)
			name := strconv.Itoa(rows) + "x" + strconv.Itoa(cols)
			html := RenderHTML(d, nil)
			b.Run("render/"+name, func(b *testing.B) {
				for b.Loop() {
					RenderHTML(d, nil)
				}
				b.ReportMetric(float64(len(html)), "html-bytes")
				b.ReportMetric(float64(gzipLen(html)), "gzip-bytes")
				b.ReportMetric(float64(d.JSONLen()), "stored-bytes")
				b.ReportMetric(float64(d.Blocks()), "blocks")
			})
			st := cellStep(b, d)
			next, err := st.Apply(d)
			if err != nil {
				b.Fatal(err)
			}
			b.Run("cache-after-a-keystroke/"+name, func(b *testing.B) {
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
			b.Run("commit/"+name, func(b *testing.B) {
				for b.Loop() {
					after, err := st.Apply(d)
					if err != nil {
						b.Fatal(err)
					}
					if err := r.Check(d, after); err != nil {
						b.Fatal(err)
					}
					_ = after.JSONLen()
				}
			})
		}
	}
}
