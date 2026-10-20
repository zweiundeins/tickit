package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/web"
)

// loadEditor is an editor without a browser: it keeps its tab's stream,
// posts its keystrokes as commits on the newest version it saw, one
// commit at a time as sb-rich-editor does, and notes when each other
// editor's commit first shows in one of its frames.
type loadEditor struct {
	id     int
	c      *http.Client
	tab    string
	page   string
	epoch  string
	url    string
	client string

	mu      sync.Mutex
	head    int64
	seen    map[string]time.Time // ref to when a frame first carried it
	confirm chan string
}

var batchesRe = regexp.MustCompile(` batches="([^"]*)"`)

func newLoadEditor(t *testing.T, s *realServer, page string, i int) *loadEditor {
	jar, _ := cookiejar.New(nil)
	e := &loadEditor{id: i, c: &http.Client{Jar: jar}, tab: fmt.Sprintf("load%02d", i), page: page, url: s.URL,
		client: fmt.Sprintf("%012x", i+1), seen: map[string]time.Time{}, confirm: make(chan string, 64)}
	res, err := e.c.Get(s.URL + "/pages/" + page + "/edit")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	m := regexp.MustCompile(`epoch="([^"]+)"[^>]*base="(\d+)"`).FindSubmatch(body)
	if m == nil {
		t.Fatalf("no editor in the page")
	}
	e.epoch = string(m[1])
	e.head, _ = strconv.ParseInt(string(m[2]), 10, 64)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", s.URL+"/stream?tab="+e.tab+"&v=carets:"+page+",pagepeople:"+page, nil)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("X-Real-IP", e.addr())
	sres, err := e.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	go e.read(sres.Body)
	if code := e.post(t, "/cmd/pages/"+page+"/open", fmt.Sprintf(`{"tab":%q,"op":"open%d","base":%d,"client":%q}`, e.tab, i, e.head, e.client)); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	return e
}

// addr is the editor's client address: each person comes from an address
// of their own, as the proxy reports it (X-Real-IP from loopback).
func (e *loadEditor) addr() string { return fmt.Sprintf("10.0.%d.%d", e.id/200, e.id%200+1) }

func (e *loadEditor) post(t *testing.T, path, body string) int {
	req, _ := http.NewRequest("POST", e.url+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Real-IP", e.addr())
	res, err := e.c.Do(req)
	if err != nil {
		return 0
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res.StatusCode
}

// read takes the frames of the tab's editor view: the batches since its
// ack, which it acknowledges as the editor does with a presence post.
func (e *loadEditor) read(r io.ReadCloser) {
	defer r.Close()
	var seq int64
	last := time.Time{}
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 4<<20)
	for sc.Scan() {
		m := batchesRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		var list []struct {
			V   int64  `json:"v"`
			Ref string `json:"ref"`
		}
		if json.Unmarshal([]byte(html.UnescapeString(m[1])), &list) != nil {
			continue
		}
		now := time.Now()
		e.mu.Lock()
		for _, b := range list {
			if _, ok := e.seen[b.Ref]; !ok {
				e.seen[b.Ref] = now
				if strings.HasPrefix(b.Ref, fmt.Sprintf("e%02d", e.id)) {
					select {
					case e.confirm <- b.Ref:
					default:
					}
				}
			}
			e.head = max(e.head, b.V)
		}
		head := e.head
		e.mu.Unlock()
		if time.Since(last) >= 100*time.Millisecond {
			last, seq = time.Now(), seq+1
			go e.post(nil, "/cmd/pages/"+e.page+"/presence", fmt.Sprintf(`{"tab":%q,"epoch":%q,"ack":%d,"v":%d,"anchor":1,"head":1,"seq":%d}`, e.tab, e.epoch, head, head, seq))
		}
	}
}

// TestCoEditingLoad measures MP-05.3 without browsers: LOAD_EDITORS
// editors (20) type 5 characters a second each for LOAD_SECONDS (20)
// into one paragraph through the real server's commit route and streams.
func TestCoEditingLoad(t *testing.T) {
	n, secs := 20, 20
	if v, err := strconv.Atoi(os.Getenv("LOAD_EDITORS")); err == nil {
		n = v
	}
	if v, err := strconv.Atoi(os.Getenv("LOAD_SECONDS")); err == nil {
		secs = v
	} else if testing.Short() || os.Getenv("LOAD_EDITORS") == "" {
		t.Skip("set LOAD_EDITORS to run the load")
	}
	s := newRealServer(t)
	page, _ := s.wikiPage(t)
	eds := make([]*loadEditor, n)
	for i := range eds {
		eds[i] = newLoadEditor(t, s, page, i)
	}
	type sent struct {
		ref   string
		chars []time.Time
	}
	commits := make([][]sent, n)
	web.CommitTimes.Reset()
	var wg sync.WaitGroup
	d := time.Duration(secs) * time.Second
	start := time.Now()
	for i, e := range eds {
		wg.Go(func() {
			var typed []time.Time // keystrokes not yet sent
			tick := time.NewTicker(200 * time.Millisecond)
			defer tick.Stop()
			time.Sleep(time.Duration(i) * 200 * time.Millisecond / time.Duration(n))
			inflight := ""
			k := 0
			for time.Since(start) < d || len(typed) > 0 || inflight != "" {
				select {
				case <-tick.C:
					if time.Since(start) < d {
						typed = append(typed, time.Now())
					}
				case ref := <-e.confirm:
					if ref == inflight {
						inflight = ""
					}
				case <-time.After(3 * time.Second):
					inflight = "" // resend after a lost answer would go here
				}
				if inflight != "" || len(typed) == 0 {
					continue
				}
				k++
				ref := fmt.Sprintf("e%02d-%d", i, k)
				e.mu.Lock()
				base := e.head
				e.mu.Unlock()
				text := strings.Repeat(string(rune('a'+i%26)), len(typed))
				body := fmt.Sprintf(`{"tab":%q,"epoch":%q,"base":%d,"ref":%q,"client":%q,"steps":[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":%q}]}}]}`,
					e.tab, e.epoch, base, ref, e.client, text)
				commits[i] = append(commits[i], sent{ref, typed})
				typed, inflight = nil, ref
				if code := e.post(t, "/cmd/pages/"+page+"/commit", body); code != http.StatusNoContent {
					t.Errorf("commit %s: %d", ref, code)
					inflight = ""
				}
			}
		})
	}
	wg.Wait()
	time.Sleep(time.Second)
	var lat []time.Duration
	total := 0
	for i := range eds {
		for _, c := range commits[i] {
			total += len(c.chars)
			for j, other := range eds {
				if j == i {
					continue
				}
				other.mu.Lock()
				at, ok := other.seen[c.ref]
				other.mu.Unlock()
				if !ok {
					t.Errorf("editor %d never saw %s", j, c.ref)
					continue
				}
				for _, k := range c.chars {
					lat = append(lat, at.Sub(k))
				}
			}
		}
	}
	slices.Sort(lat)
	if len(lat) == 0 {
		t.Fatal("no samples")
	}
	got := headText(t, s, page)
	if len([]rune(got)) != total {
		t.Errorf("MP-05.1: the document holds %d characters, %d were typed", len([]rune(got)), total)
	}
	cn, w50, w95, wmax := web.CommitTimes.Percentiles()
	r := coeditResult{Editors: n, Typed: total, Seconds: d.Seconds(), Samples: len(lat), P50: lat[len(lat)/2], P95: lat[len(lat)*95/100], Max: lat[len(lat)-1],
		WriterCommits: cn, WriterP50: w50, WriterP95: w95, WriterMax: wmax}
	t.Logf("Go clients, %d editors, %v, %d keystrokes: keystroke to the others p50 %v p95 %v max %v (%d samples); writer per commit (%d) p50 %v p95 %v max %v",
		n, d, total, r.P50, r.P95, r.Max, r.Samples, cn, w50, w95, wmax)
	writeResult(t, r)
}
