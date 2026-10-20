package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// wikiPage makes a space with a blank page on the server and returns the
// page's id and its editor's address.
func (s *realServer) wikiPage(t testing.TB) (page, edit string) {
	t.Helper()
	ada := people.Seeded[0]
	if err := s.do(people.SeedPeople{At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	b, err := s.w.DoValue(context.Background(), func(tx *writerTx) ([]byte, error) {
		return wiki.CreateSpace{By: tabs.By{Actor: ada, Op: "sp1", At: time.Now()}, Key: "CO", Name: "Co-editing", Open: true}.ApplyResult(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	space := jsonField(b, "id")
	b, err = s.w.DoValue(context.Background(), func(tx *writerTx) ([]byte, error) {
		return wiki.CreatePage{By: tabs.By{Actor: ada, Op: "pg1", At: time.Now()}, Space: space, Title: "Typing together"}.ApplyResult(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	page = jsonField(b, "id")
	return page, s.URL + wiki.EditURL("CO", page)
}

// observe records, in the page, when the count of each letter in the
// editor's text first reached each number: window.__seen[letter][n-1] is
// the time in ms the n-th letter showed.
const observe = `(() => {
	window.__seen = {};
	const mount = document.querySelector('.pm-mount'), ed = document.getElementById('editor');
	const look = () => {
		const t = Date.now(), c = {};
		for (const ch of ed.text()) c[ch] = (c[ch] || 0) + 1;
		for (const [ch, n] of Object.entries(c)) {
			const a = window.__seen[ch] ||= [];
			while (a.length < n) a.push(t);
		}
	};
	new MutationObserver(look).observe(mount, {subtree: true, childList: true, characterData: true});
	// For the breakdown: when each own commit went out, and when a frame
	// first brought each commit to Datastar.
	window.__sent = [];
	ed.addEventListener('sb-commit', (e) => window.__sent.push([Date.now(), e.detail.ref]), true);
	window.__frames = {};
	const dec = document.createElement('textarea');
	window.addEventListener('datastar-fetch', (e) => {
		const d = e.detail, el = d?.argsRaw?.elements;
		if (d?.type !== 'datastar-patch-elements' || typeof el !== 'string' || !el.includes('<sb-rich-editor')) return;
		const m = / batches="([^"]*)"/.exec(el);
		if (!m) return;
		const t = Date.now();
		dec.innerHTML = m[1];
		try { for (const b of JSON.parse(dec.value)) if (!(b.ref in window.__frames)) window.__frames[b.ref] = t } catch {}
	}, true);
	return true;
})()`

// editorReady waits for a mounted editor that registered its tab, and
// puts the caret in the first paragraph.
func editorReady(t testing.TB, p *browsertest.Page) {
	t.Helper()
	p.Call(t, "Emulation.setFocusEmulationEnabled", map[string]any{"enabled": true})
	p.Wait(t, `document.querySelector('.pm-mount .ProseMirror, .pm-mount[contenteditable]') && document.getElementById('editor').view`, 15*time.Second)
	p.Wait(t, `performance.getEntriesByType('resource').some(r => r.name.endsWith('/open'))`, 10*time.Second)
	p.Eval(t, `(document.getElementById('editor').focus(), true)`, nil)
}

type coeditResult struct {
	Editors, Typed int
	P50, P95, Max  time.Duration
	Settled        time.Duration
	WriterCommits  int64
	WriterP50      time.Duration
	WriterP95      time.Duration
	WriterMax      time.Duration
	Seconds        float64
	Samples        int
}

// coedit has n editors type their own letter into one paragraph at rate
// characters a second for d, then checks that every editor and the
// server hold the same document with every typed character (MP-05.1),
// and measures how long each keystroke took to show in the other editors
// (MP-05.3).
func coedit(t *testing.T, n int, d time.Duration, rate float64) coeditResult {
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	// MP-05.3's link: COEDIT_DELAY each way (25ms for the 50 ms round trip).
	if delay, err := time.ParseDuration(os.Getenv("COEDIT_DELAY")); err == nil && delay > 0 {
		proxy := newDelayProxy(t, s.URL, delay)
		edit = proxy.URL + strings.TrimPrefix(edit, s.URL)
		t.Logf("through a delaying proxy: %v each way", delay)
	}
	b := browsertest.Start(t)
	pages := make([]*browsertest.Page, n)
	for i := range pages {
		pages[i] = b.NewPage(t, 1000, 700)
		// Each person from an address of their own, as the proxy reports it.
		pages[i].Call(t, "Network.enable", nil)
		pages[i].Call(t, "Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"X-Real-IP": fmt.Sprintf("10.1.0.%d", i+1)}})
		pages[i].Navigate(t, edit)
		editorReady(t, pages[i])
		pages[i].Eval(t, observe, nil)
	}
	letters := "abcdefghijklmnopqrstuvwxyz"
	typed := make([][]int64, n)
	web.CommitTimes.Reset()
	var wg sync.WaitGroup
	start := time.Now()
	every := time.Duration(float64(time.Second) / rate)
	for i := range pages {
		wg.Go(func() {
			time.Sleep(time.Duration(i) * every / time.Duration(n)) // spread the editors over the interval
			tick := time.NewTicker(every)
			defer tick.Stop()
			for time.Since(start) < d {
				<-tick.C
				typed[i] = append(typed[i], time.Now().UnixMilli())
				pages[i].Call(t, "Input.insertText", map[string]any{"text": string(letters[i])})
			}
		})
	}
	wg.Wait()
	last := time.Now()
	total := 0
	for _, ty := range typed {
		total += len(ty)
	}
	// MP-05.4: every editor saved and holding the same document.
	var docs []string
	deadline := time.Now().Add(30 * time.Second)
	for {
		docs = docs[:0]
		for _, p := range pages {
			var j string
			p.Eval(t, `JSON.stringify(document.getElementById('editor').json())`, &j)
			docs = append(docs, j)
		}
		same := true
		for _, j := range docs {
			same = same && j == docs[0]
		}
		if same && countAll(docs[0], letters[:n]) == total {
			break
		}
		if time.Now().After(deadline) {
			for i, j := range docs {
				t.Logf("editor %d: %d letters", i, countAll(j, letters[:n]))
			}
			t.Fatalf("MP-05.1: the editors differ or lost keystrokes after 30 s (typed %d)", total)
		}
		time.Sleep(20 * time.Millisecond)
	}
	settled := time.Since(last)
	// The server holds the same document, and the reader sees it.
	var head wiki.Head
	s.db.View(context.Background(), func(tx *sql.Tx) (err error) { head, err = wiki.HeadOf(context.Background(), tx, page); return err })
	serverDoc, _ := doc.MarshalDoc(head.Doc)
	if !sameDoc(string(serverDoc), docs[0]) {
		t.Errorf("the server's document differs from the editors'\n%s\n%s", serverDoc, docs[0])
	}
	for i := range n {
		want := len(typed[i])
		if got := strings.Count(docText(docs[0]), string(letters[i])); got != want {
			t.Errorf("editor %d typed %d %c, the document holds %d", i, want, letters[i], got)
		}
	}
	// Keystroke to the other editors' screens.
	var lat []time.Duration
	for j, p := range pages {
		var seen map[string][]int64
		p.Eval(t, `window.__seen`, &seen)
		for i := range n {
			if i == j {
				continue
			}
			got := seen[string(letters[i])]
			for k, at := range typed[i] {
				if k < len(got) {
					lat = append(lat, time.Duration(got[k]-at)*time.Millisecond)
				}
			}
		}
	}
	slices.Sort(lat)
	breakdown(t, pages, typed, letters)
	r := coeditResult{Editors: n, Typed: total, Settled: settled, Seconds: d.Seconds(), Samples: len(lat)}
	if len(lat) > 0 {
		r.P50, r.P95, r.Max = lat[len(lat)/2], lat[len(lat)*95/100], lat[len(lat)-1]
	}
	r.WriterCommits, r.WriterP50, r.WriterP95, r.WriterMax = web.CommitTimes.Percentiles()
	hs := s.hub.Stats()
	t.Logf("hub: %d ticks, %d renders, %d deferred, %d frames sent, %d dropped, %d overruns; pages behind %d (%d streams), paced %d, released %d, acks %d",
		hs.Ticks, hs.Renders, hs.RendersDeferred, hs.FramesSent, hs.FramesDropped, hs.Overruns, hs.Behind, hs.BehindStreams, hs.Paces, hs.Releases, hs.Acks)
	t.Logf("%d editors, %v, %d keystrokes: keystroke to the others p50 %v p95 %v max %v (%d samples); settled %v after the last; writer per commit (%d) p50 %v p95 %v max %v",
		n, d, total, r.P50, r.P95, r.Max, r.Samples, settled, r.WriterCommits, r.WriterP50, r.WriterP95, r.WriterMax)
	// The page, loaded again, shows the same document.
	reader := b.NewPage(t, 1000, 700)
	reader.Navigate(t, s.URL+"/pages/"+page)
	var body string
	reader.Eval(t, `document.getElementById('page-body').textContent`, &body)
	if body != docText(docs[0]) {
		t.Errorf("the page loaded again shows other text:\n%q\n%q", body, docText(docs[0]))
	}
	writeResult(t, r)
	return r
}

func countAll(j, letters string) int {
	text := docText(j)
	n := 0
	for _, l := range letters {
		n += strings.Count(text, string(l))
	}
	return n
}

// docText is the text of a document's JSON, blocks joined without
// separators, as textContent reads it.
func docText(j string) string {
	d, err := doc.ParseDoc([]byte(j))
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(doc.Text(d), "\n", "")
}

func sameDoc(a, b string) bool {
	da, err1 := doc.ParseDoc([]byte(a))
	db, err2 := doc.ParseDoc([]byte(b))
	return err1 == nil && err2 == nil && da.Eq(db)
}

// writeResult keeps a run's numbers for benchmarks/RESULTS.md.
func writeResult(t *testing.T, r coeditResult) {
	path := os.Getenv("COEDIT_OUT")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Log(err)
		return
	}
	defer f.Close()
	b, _ := json.Marshal(r)
	fmt.Fprintln(f, string(b))
}

// Section 12 item 4, Appendix B done 1, MP-05.1, MP-05.4: five editors
// type into one paragraph for 60 s against the real server and end with
// identical documents holding every typed character; the page loaded
// again shows the same.
func TestCoEditing(t *testing.T) {
	n, secs := 5, 60
	if v, err := strconv.Atoi(os.Getenv("COEDIT_EDITORS")); err == nil {
		n = v
	}
	if v, err := strconv.Atoi(os.Getenv("COEDIT_SECONDS")); err == nil {
		secs = v
	}
	coedit(t, n, time.Duration(secs)*time.Second, 5)
}

// breakdown splits each keystroke's way to the other editors (MP-05.3):
// from the keystroke to its commit going out (the editor sends one commit
// at a time, so a keystroke waits for the one in flight), from there to
// the frame that brings it to each other editor's Datastar (the request,
// the writer, the tick, the render, the stream), and from there to the
// text in the other editor (the morph, the editor's apply).
func breakdown(t *testing.T, pages []*browsertest.Page, typed [][]int64, letters string) {
	n := len(pages)
	sent := make([][][2]any, n)
	frames := make([]map[string]int64, n)
	seen := make([]map[string][]int64, n)
	for i, p := range pages {
		p.Eval(t, `window.__sent`, &sent[i])
		p.Eval(t, `window.__frames`, &frames[i])
		p.Eval(t, `window.__seen`, &seen[i])
	}
	var wait, wire, apply []time.Duration
	for i := range n {
		k := 0
		for c, at := range typed[i] {
			for k < len(sent[i]) && int64(sent[i][k][0].(float64)) < at {
				k++
			}
			if k == len(sent[i]) {
				break
			}
			out, ref := int64(sent[i][k][0].(float64)), sent[i][k][1].(string)
			wait = append(wait, time.Duration(out-at)*time.Millisecond)
			for j := range n {
				if j == i {
					continue
				}
				f, ok := frames[j][ref]
				got := seen[j][string(letters[i])]
				if !ok || c >= len(got) {
					continue
				}
				wire = append(wire, time.Duration(f-out)*time.Millisecond)
				apply = append(apply, time.Duration(got[c]-f)*time.Millisecond)
			}
		}
	}
	pct := func(l []time.Duration) string {
		if len(l) == 0 {
			return "none"
		}
		slices.Sort(l)
		return fmt.Sprintf("p50 %v p95 %v", l[len(l)/2], l[len(l)*95/100])
	}
	t.Logf("breakdown: keystroke to its commit %s; commit to the frame at the others %s; frame to the text %s", pct(wait), pct(wire), pct(apply))
}
