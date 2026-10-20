package web_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// NFR-38, MP-11: 100 streams on one board while a card moves a second.
// It reports the renders of the board's view per key, the frames and
// bytes each stream got, the time from a move's request to the frame on
// the slowest stream, and the process's CPU time. TICKIT_LOAD_SECONDS
// sets how long (10 by default). Skipped with -short and under -race.
func TestBoardLoad(t *testing.T) {
	if testing.Short() || raceDetector {
		t.Skip("a load test")
	}
	seconds := 10
	if s, err := strconv.Atoi(os.Getenv("TICKIT_LOAD_SECONDS")); err == nil && s > 0 {
		seconds = s
	}
	const tabs = 100
	e, pr := boardEnv(t, 100)
	var pid string
	e.DB.View(context.Background(), func(tx *sql.Tx) error {
		p, err := tracker.ProjectByKey(context.Background(), tx, "WEB")
		pid = p.ID
		return err
	})
	key := views.BoardKey(pid, tracker.Filter{})

	type tab struct {
		events <-chan string
		frames atomic.Int64
		bytes  atomic.Int64
		last   atomic.Int64 // unix ns of the last board frame
	}
	all := make([]*tab, tabs)
	var mover *http.Client
	for i := range all {
		c, _, ev := onBoard(t, e, fmt.Sprintf("load%d", i), "")
		if i == 0 {
			mover = c
		}
		all[i] = &tab{events: ev}
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	for _, tb := range all {
		wg.Go(func() {
			for {
				select {
				case ev, ok := <-tb.events:
					if !ok {
						return
					}
					if strings.Contains(ev, `id="board-lanes"`) {
						tb.frames.Add(1)
						tb.bytes.Add(int64(len(ev)))
						tb.last.Store(time.Now().UnixNano())
					}
				case <-done:
					return
				}
			}
		})
	}
	statuses := []string{"todo", "inprogress", "inreview", "done"}
	version := regexp.MustCompile(`id="c-(WEB-\d+)"[^>]*? data-v="(\d+)"`)
	var before syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &before)
	renders0 := e.Hub.Stats().RendersByKey[key]
	var lags []time.Duration
	start := time.Now()
	for i := range seconds {
		next := start.Add(time.Duration(i) * time.Second)
		time.Sleep(time.Until(next))
		_, page := webtest.Get(t, mover, e.URL+"/projects/WEB/board")
		seen := map[string]string{}
		for _, m := range version.FindAllStringSubmatch(page, -1) {
			seen[m[1]] = m[2]
		}
		card := pr.Keys[1+i%(len(pr.Keys)-1)]
		sent := time.Now()
		code, _ := webtest.Post(t, mover, e.URL+"/cmd/board/move", fmt.Sprintf(`{"tab":"load0","op":"lop%d","issue":%q,"status":%q,"seen":%s}`, i, card, statuses[i%len(statuses)], seen[card]))
		if code != http.StatusNoContent {
			t.Fatalf("move %d: %d", i, code)
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			slowest := int64(1<<63 - 1)
			for _, tb := range all {
				slowest = min(slowest, tb.last.Load())
			}
			if slowest >= sent.UnixNano() {
				lags = append(lags, time.Duration(slowest-sent.UnixNano()))
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	time.Sleep(200 * time.Millisecond)
	var after syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &after)
	close(done)
	wall := time.Since(start)
	cpu := time.Duration(after.Utime.Nano()+after.Stime.Nano()-before.Utime.Nano()-before.Stime.Nano()) * time.Nanosecond
	st := e.Hub.Stats()
	var frames, bytes int64
	for _, tb := range all {
		frames += tb.frames.Load()
		bytes += tb.bytes.Load()
	}
	slices.Sort(lags)
	p := func(q float64) time.Duration {
		if len(lags) == 0 {
			return 0
		}
		return lags[min(len(lags)-1, int(q*float64(len(lags))))]
	}
	t.Logf("%d streams on one board of 100 cards, %d moves in %v", tabs, seconds, wall.Round(time.Millisecond))
	t.Logf("renders of %s: %d for %d moves; %d board views watched", key, st.RendersByKey[key]-renders0, seconds, boardKeys(st.RendersByKey, pid))
	t.Logf("board frames received: %d (%.1f per stream), %d bytes, %d bytes a frame, uncompressed", frames, float64(frames)/tabs, bytes, bytes/max(frames, 1))
	t.Logf("from a move's request to its frame on the slowest of %d streams: p50 %v, p95 %v, max %v (%d moves)", tabs, p(0.5), p(0.95), p(1), len(lags))
	t.Logf("process CPU (server, %d clients and the test): %v, %.1f%% of one core", tabs, cpu.Round(time.Millisecond), 100*cpu.Seconds()/wall.Seconds())
	if got := st.RendersByKey[key] - renders0; got > int64(seconds)+1 {
		t.Errorf("%d renders for %d moves", got, seconds)
	}
}

func boardKeys(m map[string]int64, pid string) int {
	n := 0
	for k := range m {
		if strings.HasPrefix(k, "board:"+pid+":") {
			n++
		}
	}
	return n
}
