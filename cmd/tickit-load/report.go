package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// Dist is a distribution of durations.
type Dist struct {
	N                  int
	P50, P95, P99, Max time.Duration
}

func dist(ds []time.Duration) Dist {
	if len(ds) == 0 {
		return Dist{}
	}
	slices.Sort(ds)
	at := func(q float64) time.Duration { return ds[min(len(ds)-1, int(q*float64(len(ds))))] }
	return Dist{N: len(ds), P50: at(0.5), P95: at(0.95), P99: at(0.99), Max: ds[len(ds)-1]}
}

func (d Dist) String() string {
	r := func(x time.Duration) string { return fmt.Sprintf("%.1f ms", float64(x.Microseconds())/1000) }
	return fmt.Sprintf("n %d, p50 %s, p95 %s, p99 %s, max %s", d.N, r(d.P50), r(d.P95), r(d.P99), r(d.Max))
}

// Growth is one sample of the database during the sustained run.
type Growth struct {
	Minute       float64
	Bytes        int64
	FreePages    int64
	Events       map[string]int64
	Folded       float64 // events folded
	Folds        float64 // fold commands that folded something
	FoldsOver5ms float64 // of them, those that held the writer more than 5 ms
	FoldMean     float64 // seconds
	RSS          int64
}

// RegionStats are the frames of one patched element on one kind of tab.
type RegionStats struct {
	Frames    int64
	MaxBytes  int
	AvgBytes  int64
	MinGap    time.Duration
	PerSecond float64 // frames a second on the busiest tab
}

// Results are a run's numbers.
type Results struct {
	Date      string
	Seconds   int
	Manifest  Manifest
	DBBytes   int64
	StartTook time.Duration

	OpenTook    time.Duration
	StreamsOpen int
	Stream201   int

	Times map[string]Dist
	Codes map[string]int64

	ServerCores, ClientCores      float64
	RSS, RSSPeak, RSSMaxSampled   int64
	AnonMax, FileMax              int64
	RSSAfterRestart               int64
	RSSPeakSustain                int64
	Ticks, Overrun                float64
	Renders, Deferred             float64
	FramesSent, FramesDropped     float64
	Commands, Batches, WriterBusy float64
	CommitP99                     float64
	RateLimited                   float64
	BudgetRefused                 float64
	BudgetTiers                   map[string]float64
	BudgetReserved, GoSys, Heap   float64
	TickHist                      map[string]float64
	Overruns                      []string
	Unseen                        map[string]string
	Regions                       map[string]RegionStats

	Typed, DocChars int
	DBBytesAfter    int64

	StopTook, RestartTook time.Duration
	Reconnect             Dist
	NotBack               int
	BoardSame             bool
	IssuesSame            string
	EditorStates          map[string]int

	Growth []Growth
}

func (r *Results) delta(m0, m1 metrics) {
	d := func(name string) float64 { return m1.sum(name) - m0.sum(name) }
	r.Ticks, r.Overrun = d("tickit_live_ticks_total"), d("tickit_live_tick_overruns_total")
	r.Renders, r.Deferred = d("tickit_live_renders_total"), d("tickit_live_renders_deferred_total")
	r.FramesSent, r.FramesDropped = d("tickit_live_frames_sent_total"), d("tickit_live_frames_dropped_total")
	r.Commands, r.Batches, r.WriterBusy = d("tickit_writer_commands_total"), d("tickit_writer_batches_total"), d("tickit_writer_busy_seconds_total")
	r.CommitP99 = m1["tickit_writer_commit_p99_seconds"]
	r.RateLimited = d("tickit_rate_limited_total")
	r.BudgetRefused = d("tickit_sse_budget_refused_total")
	r.BudgetTiers = map[string]float64{}
	r.BudgetReserved, r.GoSys, r.Heap = m1["tickit_sse_budget_reserved_bytes"], m1["go_memstats_sys_bytes"], m1["go_memstats_heap_inuse_bytes"]
	r.TickHist = map[string]float64{}
	for k, v := range m1 {
		if strings.HasPrefix(k, "tickit_sse_budget_streams{") {
			r.BudgetTiers[strings.TrimPrefix(k, "tickit_sse_budget_streams")] = v
		}
		if strings.HasPrefix(k, "tickit_tick_seconds_bucket{") {
			r.TickHist[strings.TrimPrefix(k, "tickit_tick_seconds_bucket")] = v - m0[k]
		}
	}
	r.TickHist["count"] = d("tickit_tick_seconds_count")
	r.TickHist["sum"] = d("tickit_tick_seconds_sum")
}

// unseen counts, per kind of change, the (change, tab) pairs whose frame
// never came.
func (r *Results) unseen(l *load) {
	r.Unseen = map[string]string{}
	count := func(name string, list *pendingList, tabs int) {
		var want, got int
		list.mu.Lock()
		for _, p := range list.all {
			p.mu.Lock()
			want += tabs
			got += len(p.seen)
			p.mu.Unlock()
		}
		list.mu.Unlock()
		r.Unseen[name] = fmt.Sprintf("%d of %d frames came", got, want)
	}
	count("move", &l.moves, nBoard)
	count("field", &l.changes, nIssue/len(l.man.Hot))
	count("commit", &l.commits, nEditors-1)
}

// regions sums the frames of each element by kind of tab, for NFR-37.
func (r *Results) regions(l *load) {
	r.Regions = map[string]RegionStats{}
	for _, t := range l.all {
		t.mu.Lock()
		for id, g := range t.regions {
			if g.frames == 0 {
				continue
			}
			slow := ""
			if t.slow {
				slow = " (slow)"
			}
			key := t.kind + slow + " #" + id
			s := r.Regions[key]
			s.Frames += g.frames
			s.MaxBytes = max(s.MaxBytes, g.maxSize)
			s.AvgBytes = (s.AvgBytes*(s.Frames-g.frames) + g.bytes) / s.Frames
			if s.MinGap == 0 || (g.minGap > 0 && g.minGap < s.MinGap) {
				s.MinGap = g.minGap
			}
			s.PerSecond = max(s.PerSecond, float64(g.frames)/float64(r.Seconds))
			r.Regions[key] = s
		}
		t.mu.Unlock()
	}
}

func mib(b int64) string { return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20)) }

// Print writes the numbers per requirement.
func (r *Results) Print(w io.Writer) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format+"\n", args...) }
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	pass := func(ok bool) string {
		if ok {
			return "pass"
		}
		return "FAIL"
	}
	p("tickit-load %s, %d s measured, database %s (%d issues, %d pages, %d events), server started in %v",
		r.Date, r.Seconds, mib(r.DBBytes), r.Manifest.Issues, r.Manifest.Pages, r.Manifest.Events, r.StartTook.Round(time.Millisecond))
	p("")
	p("NFR-38: %d streams open (opened in %v); actions answered: %v", r.StreamsOpen, r.OpenTook.Round(time.Millisecond), r.Codes)
	p("NFR-35 commit to frame, from the request to the frame read by each watcher (an upper bound of commit to frame write):")
	for _, k := range []string{"frame:move", "frame:field", "frame:commit"} {
		d := r.Times[k]
		p("  %-13s %v  [%s: p50 <= 50 ms, p95 <= 100 ms]", k, d, pass(d.N > 0 && d.P50 <= 50*time.Millisecond && d.P95 <= 100*time.Millisecond))
	}
	p("  frame:move:slow (the 10 board tabs with Save-Data, held to a frame every 250 ms by design): %v", r.Times["frame:move:slow"])
	p("  frames that never came: %v", r.Unseen)
	ratio := 0.0
	if r.Ticks > 0 {
		ratio = r.Overrun / r.Ticks
	}
	p("  ticks %.0f, overruns %.0f (%.4f%%) [%s: under 0.1%%]; renders %.0f, deferred %.0f; frames sent %.0f, dropped %.0f", r.Ticks, r.Overrun, 100*ratio, pass(ratio < 0.001), r.Renders, r.Deferred, r.FramesSent, r.FramesDropped)
	if len(r.TickHist) > 2 {
		p("  tick durations (tickit_tick_seconds): %v", r.TickHist)
	}
	for _, l := range r.Overruns {
		p("  log: %s", l)
	}
	d := r.Times["action"]
	p("NFR-36 actions answered 204: %v [%s: p95 <= 50 ms]", d, pass(d.N > 0 && d.P95 <= 50*time.Millisecond))
	for _, k := range []string{"action:move", "action:field", "action:commit", "action:presence"} {
		p("  %-16s %v", k, r.Times[k])
	}
	p("  writer: %.0f commands in %.0f batches, busy %.1f%%, commit p99 %.2f ms; rate limited %.0f", r.Commands, r.Batches, 100*r.WriterBusy/float64(r.Seconds+2), 1000*r.CommitP99, r.RateLimited)
	p("NFR-37 frames (bytes of the SSE event before compression; per element patched):")
	keys := make([]string, 0, len(r.Regions))
	for k := range r.Regions {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		s := r.Regions[k]
		if s.Frames < 2 && !strings.Contains(k, "(slow)") {
			continue
		}
		p("  %-44s frames %6d, max %6d B, avg %6d B, fastest tab %.1f/s, shortest gap %.0f ms", k, s.Frames, s.MaxBytes, s.AvgBytes, s.PerSecond, ms(s.MinGap))
	}
	p("NFR-39 documents under the load, to the last byte:")
	for _, k := range []string{"doc:home", "doc:board", "doc:issue", "doc:list", "doc:page", "doc"} {
		d := r.Times[k]
		p("  %-10s %v [%s: p95 <= 200 ms]", k, d, pass(d.N > 0 && d.P95 <= 200*time.Millisecond))
	}
	for _, k := range []string{"cold:board", "cold:editor", "cold:reader", "cold:issue", "cold:list", "cold:home"} {
		p("  %-12s %v (the tabs' first loads, 20 at a time, before the load)", k, r.Times[k])
	}
	d = r.Times["first"]
	p("  first frame of a stream, from its request: %v [%s: within one tick, 50 ms, at p95]", d, pass(d.N > 0 && d.P95 <= 50*time.Millisecond))
	p("NFR-41: the 201st stream answered %d [%s]; RSS now %s, peak (VmHWM) %s, after the restart %s; peaks sampled every 100 ms: anonymous %s [%s: under 640M], file-backed (the readers' mmap of the database) %s; budget refused %.0f, tiers %v; server %.2f cores, load tool %.2f cores",
		r.Stream201, pass(r.Stream201 == 503), mib(r.RSS), mib(r.RSSPeak), mib(r.RSSAfterRestart), mib(r.AnonMax), pass(r.AnonMax < 640e6), mib(r.FileMax), r.BudgetRefused, r.BudgetTiers, r.ServerCores, r.ClientCores)
	p("  memory at the end: Go heap in use %s, Go's memory from the OS %s, the stream budget's reservations %s", mib(int64(r.Heap)), mib(int64(r.GoSys)), mib(int64(r.BudgetReserved)))
	p("MP-05.1: %d keystrokes taken, %d in the page [%s]", r.Typed, r.DocChars, pass(r.Typed == r.DocChars))
	p("NFR-42: stop took %v, back to /healthz in %v; streams back, from /healthz to their first frame: %v, %d not back [%s: within 10 s]; board as before: %v, issues as before: %s, editors: %v",
		r.StopTook.Round(time.Millisecond), r.RestartTook.Round(time.Millisecond), r.Reconnect, r.NotBack, pass(r.NotBack == 0 && r.Reconnect.Max <= 10*time.Second), r.BoardSame, r.IssuesSame, r.EditorStates)
	p("database: %s before, %s after the measured run", mib(r.DBBytes), mib(r.DBBytesAfter))
	if len(r.Growth) > 0 {
		p("NFR-43 sustained run (minute, size, free pages, events by kind, events folded, fold commands, of them over 5 ms, mean):")
		for _, g := range r.Growth {
			p("  %5.1f  %s  free %d  %v  folded %.0f  folds %.0f  over 5 ms %.0f  mean %.2f ms  rss %s", g.Minute, mib(g.Bytes), g.FreePages, g.Events, g.Folded, g.Folds, g.FoldsOver5ms, 1000*g.FoldMean, mib(g.RSS))
		}
		p("  peak RSS over the whole run %s", mib(r.RSSPeakSustain))
	}
}
