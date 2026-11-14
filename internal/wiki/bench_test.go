package wiki

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith-coedit/collab"

	"github.com/zweiundeins/tickit/internal/docnodes"
)

// NFR-43.3: one fold command, FoldBatch commits of a page of about
// 100 KB, and Commit on that page: how long the writer spends.
// FOLD_PARAS=4000 makes the page about 1 MB, under a private instance's
// limits (PI-18).
func TestFoldTime(t *testing.T) {
	if testing.Short() {
		t.Skip("a measurement")
	}
	count := 400
	if n, err := strconv.Atoi(os.Getenv("FOLD_PARAS")); err == nil && n > 0 {
		count = n
		docnodes.UsePrivateLimits(true)
		defer docnodes.UsePrivateLimits(false)
	}
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	var texts []string
	for i := range count {
		texts = append(texts, fmt.Sprintf("Paragraph %d: %s", i, strings.Repeat("words of the page ", 12)))
	}
	p := e.page(sunny, CreatePage{Space: sp, Title: "Big", Doc: paras(texts...)})
	var took []time.Duration
	for i := range 500 {
		c := &Commit{By: e.by(sunny), Session: "s", Tab: "t", Page: p, Epoch: e.docState(p).Epoch, Base: int64(i + 1), Ref: fmt.Sprintf("r%d", i), Steps: []byte(steps(insert(1, "x")))}
		if err := e.W.Do(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		took = append(took, c.Took)
	}
	e.must(CutVersion{By: e.by(sunny), Page: p})
	collab.ResetCaches()
	for range 3 {
		f := &Fold{At: e.now.Add(time.Hour)}
		start := time.Now()
		e.must(f)
		t.Logf("document %s bytes; commit p50 %v; one fold of %d events: %v (round trip through the writer)",
			e.row(`SELECT bytes FROM page_doc WHERE page_id = ?`, p), took[len(took)/2], f.Deleted, time.Since(start))
	}
}
