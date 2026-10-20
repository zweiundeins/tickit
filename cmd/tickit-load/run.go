package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zweiundeins/lith/sqlite"
)

// The load of NFR-38.1: 200 streams.
const (
	nBoard   = 100 // on the LOAD board
	nEditors = 20  // co-editing one page
	nReaders = 20  // reading that page
	nIssue   = 30  // three on each of the 10 issues whose fields change
	nList    = 20  // the issue list of 25 rows
	nHome    = 10  // the home page
)

// samples collects durations by name.
type samples struct {
	mu sync.Mutex
	m  map[string][]time.Duration
}

func (s *samples) add(name string, d time.Duration) {
	s.mu.Lock()
	if s.m == nil {
		s.m = map[string][]time.Duration{}
	}
	s.m[name] = append(s.m[name], d)
	s.mu.Unlock()
}

func (s *samples) names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.m {
		out = append(out, k)
	}
	return out
}

func (s *samples) get(name string) []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.m[name])
}

// pending is a change sent, waiting for the frames that show it.
type pending struct {
	key   string // the card's key, the issue's id or the commit's ref
	after int64  // the version the change was made on
	sent  time.Time
	mu    sync.Mutex
	seen  map[int]bool // tabs whose frame showed it
}

type pendingList struct {
	mu   sync.Mutex
	list []*pending
	all  []*pending // every change of the measured window
}

func (l *pendingList) add(p *pending, measured bool) {
	p.seen = map[int]bool{}
	l.mu.Lock()
	l.list = append(l.list, p)
	if len(l.list) > 400 {
		l.list = l.list[len(l.list)-400:]
	}
	if measured {
		l.all = append(l.all, p)
	}
	l.mu.Unlock()
}

func (l *pendingList) each(f func(p *pending)) {
	l.mu.Lock()
	list := slices.Clone(l.list)
	l.mu.Unlock()
	for _, p := range list {
		f(p)
	}
}

var (
	cardRe    = regexp.MustCompile(`id="c-([A-Z]+-\d+)" data-v="(\d+)"`)
	lanesRe   = regexp.MustCompile(`id="board-lanes"`)
	dataRevRe = regexp.MustCompile(`data-rev="([0-9a-f]+)"`)
	maxRe     = regexp.MustCompile(`"_is":\{"max":(\d+)\}`)
	batchesRe = regexp.MustCompile(` batches="([^"]*)"`)
	hostRe    = regexp.MustCompile(`epoch="([^"]+)" client="([^"]+)" base="(\d+)"`)
	stateRe   = regexp.MustCompile(`<sb-rich-editor [^>]*state="([a-z]+)"`)
)

type load struct {
	base string
	man  Manifest
	srv  *server
	ctx  context.Context

	all     []*tab
	boards  []*tab
	editors []*editor
	readers []*tab
	issues  []*tab
	lists   []*tab
	homes   []*tab

	times samples // durations by name
	codes sync.Map

	moves, changes, commits pendingList
	measuring               atomic.Bool

	cardV   sync.Map // card key -> version, from the mover's frames
	issueV  sync.Map // issue id -> version, from the issue tabs' frames
	lastRev atomic.Value
	profile string
}

// editor is a tab that co-edits: one commit in flight at a time, on the
// newest version it saw, as sb-rich-editor sends them.
type editor struct {
	*tab
	i             int
	epoch, client string

	mu      sync.Mutex
	head    int64
	confirm chan string
	typed   int // keystrokes the server took
	commits int // refs used: each commit's ref is new, or writer.Once answers the old one
	seq     int64
	lastAck time.Time
	state   string
}

func run(args []string) error {
	f := flag.NewFlagSet("run", flag.ExitOnError)
	bin := f.String("bin", "./tickit", "the tickit binary")
	base := f.String("base", "", "the database from gen; the run works on a copy")
	dir := f.String("dir", "", "where the copy and the server's log go (default: beside -base)")
	seconds := f.Int("seconds", 60, "how long the load is measured")
	sustain := f.Duration("sustain", 0, "after the restart, keep the load up this long and sample the database every minute (NFR-43; the fold starts after 15 minutes)")
	addr := f.String("addr", "127.0.0.1:7351", "the server's address")
	metricsAddr := f.String("metrics", "127.0.0.1:7352", "the server's metrics address")
	cpus := f.String("server-cpus", "", "taskset -c list for the server (e.g. 0-5); run the load tool on others")
	out := f.String("out", "", "write the results as JSON to this file")
	profile := f.String("cpuprofile", "", "write the server's CPU profile of the measured time here (a binary built with -tags pprof)")
	f.Parse(args)
	if *base == "" {
		return errors.New("run: -base is required")
	}
	b, err := os.ReadFile(*base + ".json")
	if err != nil {
		return err
	}
	var man Manifest
	if err := json.Unmarshal(b, &man); err != nil {
		return err
	}
	if *dir == "" {
		*dir = filepath.Dir(*base)
	}
	db := filepath.Join(*dir, "run.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(db + suffix)
	}
	if err := copyFile(*base, db); err != nil {
		return err
	}
	logPath := filepath.Join(*dir, "server.log")
	os.Remove(logPath)
	srv := &server{bin: *bin, db: db, addr: *addr, metrics: *metricsAddr, cpus: *cpus, logPath: logPath}
	started := time.Now()
	if _, err := srv.start(); err != nil {
		return err
	}
	defer srv.stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := &load{base: srv.url(), man: man, srv: srv, ctx: ctx, profile: *profile}
	r := &Results{Date: time.Now().Format(time.RFC3339), Seconds: *seconds, Manifest: man, StartTook: time.Since(started)}
	r.DBBytes = fileSize(db)
	if err := l.open(r); err != nil {
		return err
	}
	if err := l.measure(r, time.Duration(*seconds)*time.Second); err != nil {
		return err
	}
	if err := l.restart(r); err != nil {
		return err
	}
	if *sustain > 0 {
		if err := l.sustain(r, db, *sustain); err != nil {
			return err
		}
	}
	for _, t := range l.all {
		t.stop.Store(true)
	}
	r.Print(os.Stdout)
	if *out != "" {
		b, _ := json.MarshalIndent(r, "", "  ")
		return os.WriteFile(*out, b, 0o644)
	}
	return nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	o, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(o, in); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}

func fileSize(db string) int64 {
	var n int64
	for _, s := range []string{"", "-wal"} {
		if fi, err := os.Stat(db + s); err == nil {
			n += fi.Size()
		}
	}
	return n
}

// open loads every tab's document and opens its stream, then tries one
// stream more.
func (l *load) open(r *Results) error {
	man := l.man
	n := 0
	mk := func(kind, path string) *tab {
		t := newTab(kind, n, l.base)
		t.path = path
		if n%2 == 1 {
			t.coding = "zstd"
		}
		n++
		l.all = append(l.all, t)
		return t
	}
	for i := range nBoard {
		t := mk("board", "/projects/"+man.Board+"/board")
		t.slow = i%10 == 9
		t.onEvent = l.onBoard
		l.boards = append(l.boards, t)
	}
	for i := range nEditors {
		t := mk("editor", "/spaces/"+man.SpaceKey+"/pages/"+man.Page+"/edit")
		e := &editor{tab: t, i: i, confirm: make(chan string, 64)}
		t.onEvent = func(_ *tab, ev *event) { l.onEditor(e, ev) }
		l.editors = append(l.editors, e)
	}
	for i := range nReaders {
		t := mk("reader", "/spaces/"+man.SpaceKey+"/pages/"+man.Page)
		t.slow = i%4 == 3
		l.readers = append(l.readers, t)
	}
	for i := range nIssue {
		t := mk("issue", "/"+man.Hot[i%len(man.Hot)])
		id := man.HotIDs[i%len(man.HotIDs)]
		t.onEvent = func(t *tab, ev *event) { l.onIssue(t, id, ev) }
		l.issues = append(l.issues, t)
	}
	for range nList {
		l.lists = append(l.lists, mk("list", "/issues?"+man.List))
	}
	for range nHome {
		l.homes = append(l.homes, mk("home", "/"))
	}

	// The documents, 20 at a time, then the streams, 20 at a time: each
	// stream's first frame is timed from its request (NFR-39).
	err := parallel(l.all, 20, func(t *tab) error {
		took, body, err := t.load(l.ctx, t.path)
		if err != nil {
			return err
		}
		l.times.add("cold:"+t.kind, took)
		if t.kind == "editor" {
			m := hostRe.FindSubmatch(body)
			if m == nil {
				return fmt.Errorf("no editor in %s", t.path)
			}
			e := l.editorOf(t)
			e.epoch, e.client = string(m[1]), string(m[2])
			e.head, _ = strconv.ParseInt(string(m[3]), 10, 64)
		}
		return nil
	})
	if err != nil {
		return err
	}
	opened := time.Now()
	err = parallel(l.all, 20, func(t *tab) error {
		ch := t.expect()
		go t.run(l.ctx)
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			return fmt.Errorf("%s tab %d: no frame within 10 s (status %d)", t.kind, t.n, t.status)
		}
		t.mu.Lock()
		l.times.add("first", t.firstAt.Sub(t.opened))
		t.mu.Unlock()
		return nil
	})
	if err != nil {
		return err
	}
	r.OpenTook = time.Since(opened)
	for _, e := range l.editors {
		code, d := e.post("/cmd/pages/"+l.man.Page+"/open", fmt.Sprintf(`{"tab":%q,"op":"open%d","base":%d,"client":%q}`, e.id, e.i, e.head, e.client))
		if code != 204 {
			return fmt.Errorf("editor %d: open answered %d", e.i, code)
		}
		l.times.add("open", d)
	}

	// NFR-41.1: the 201st stream.
	extra := newTab("extra", 9999, l.base)
	if _, _, err := extra.load(l.ctx, "/"); err != nil {
		return err
	}
	extra.once(l.ctx)
	r.Stream201 = extra.status
	if m, err := l.srv.scrape(l.ctx); err == nil {
		r.StreamsOpen = int(m.sum("tickit_live_streams"))
	}
	slog.Info("streams open", "streams", r.StreamsOpen, "201st", r.Stream201, "took", r.OpenTook.Round(time.Millisecond))
	return nil
}

func (l *load) editorOf(t *tab) *editor {
	for _, e := range l.editors {
		if e.tab == t {
			return e
		}
	}
	return nil
}

func parallel(list []*tab, n int, f func(*tab) error) error {
	sem := make(chan struct{}, n)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for _, t := range list {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			if err := f(t); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// onBoard reads a board frame: the mover's card versions, and the moves
// it shows.
func (l *load) onBoard(t *tab, ev *event) {
	if ev.kind != "datastar-patch-elements" || !lanesRe.MatchString(ev.data) {
		return
	}
	if t == l.boards[0] {
		for _, m := range cardRe.FindAllStringSubmatch(ev.data, -1) {
			v, _ := strconv.ParseInt(m[2], 10, 64)
			l.cardV.Store(m[1], v)
		}
		if m := dataRevRe.FindStringSubmatch(ev.data); m != nil {
			l.lastRev.Store(m[1])
		}
	}
	l.moves.each(func(p *pending) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.seen[t.n] {
			return
		}
		mark := `id="c-` + p.key + `" data-v="`
		i := strings.Index(ev.data, mark)
		if i < 0 {
			return
		}
		rest := ev.data[i+len(mark):]
		v, _ := strconv.ParseInt(rest[:strings.IndexByte(rest, '"')], 10, 64)
		if v > p.after {
			p.seen[t.n] = true
			name := "frame:move"
			if t.slow {
				name = "frame:move:slow" // Spec.Slow: a frame at most every 250 ms (NFR-37.2)
			}
			l.times.add(name, ev.at.Sub(p.sent))
		}
	})
}

// onIssue reads an issue frame's version.
func (l *load) onIssue(t *tab, id string, ev *event) {
	m := maxRe.FindStringSubmatch(ev.data)
	if m == nil {
		return
	}
	v, _ := strconv.ParseInt(m[1], 10, 64)
	if old, ok := l.issueV.Load(id); !ok || old.(int64) < v {
		l.issueV.Store(id, v)
	}
	l.changes.each(func(p *pending) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.key != id || p.seen[t.n] || v <= p.after {
			return
		}
		p.seen[t.n] = true
		l.times.add("frame:field", ev.at.Sub(p.sent))
	})
}

// onEditor reads the batches of an editor frame, confirms its own
// commits, and acknowledges as the editor does, at most every 100 ms.
func (l *load) onEditor(e *editor, ev *event) {
	if m := stateRe.FindStringSubmatch(ev.data); m != nil {
		e.mu.Lock()
		e.state = m[1]
		e.mu.Unlock()
	}
	m := batchesRe.FindStringSubmatch(ev.data)
	if m == nil {
		return
	}
	var list []struct {
		V   int64  `json:"v"`
		Ref string `json:"ref"`
	}
	if json.Unmarshal([]byte(html.UnescapeString(m[1])), &list) != nil {
		return
	}
	mine := fmt.Sprintf("e%02d-", e.i)
	refs := map[string]bool{}
	for _, b := range list {
		if strings.HasPrefix(b.Ref, mine) {
			select {
			case e.confirm <- b.Ref:
			default:
			}
		} else {
			refs[b.Ref] = true
		}
	}
	l.commits.each(func(p *pending) {
		if !refs[p.key] {
			return
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.seen[e.n] {
			p.seen[e.n] = true
			l.times.add("frame:commit", ev.at.Sub(p.sent))
		}
	})
	e.mu.Lock()
	for _, b := range list {
		e.head = max(e.head, b.V)
	}
	head := e.head
	send := time.Since(e.lastAck) >= 100*time.Millisecond
	if send {
		e.lastAck = time.Now()
		e.seq++
	}
	seq := e.seq
	e.mu.Unlock()
	if send {
		go func() {
			code, d := e.post("/cmd/pages/"+l.man.Page+"/presence", fmt.Sprintf(`{"tab":%q,"epoch":%q,"ack":%d,"v":%d,"anchor":1,"head":1,"seq":%d}`, e.id, e.epoch, head, head, seq))
			l.action("presence", code, d)
		}()
	}
}

// action counts an action's answer and, while measuring, its time.
func (l *load) action(name string, code int, d time.Duration) {
	if !l.measuring.Load() {
		return
	}
	c, _ := l.codes.LoadOrStore(name+" "+strconv.Itoa(code), new(atomic.Int64))
	c.(*atomic.Int64).Add(1)
	if code == 204 {
		l.times.add("action:"+name, d)
		l.times.add("action", d)
	}
}

// drive sends the actions of NFR-38.1 until ctx ends: a card moves a
// second, 10 issue fields change a second, 20 editors type 5 characters
// a second; and a document loads every 200 ms (NFR-39).
func (l *load) drive(ctx context.Context) *sync.WaitGroup {
	var wg sync.WaitGroup
	hot := map[string]bool{}
	for _, k := range l.man.Hot {
		hot[k] = true
	}
	var cards []string
	for _, k := range l.man.Cards {
		if !hot[k] {
			cards = append(cards, k)
		}
	}
	statuses := []string{"todo", "inprogress", "inreview", "done"}
	mover := l.boards[0]
	wg.Go(func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			key := cards[i%len(cards)]
			v, _ := l.cardV.Load(key)
			seen, _ := v.(int64)
			l.moves.add(&pending{key: key, after: seen, sent: time.Now()}, l.measuring.Load())
			code, d := mover.post("/cmd/board/move", fmt.Sprintf(`{"tab":%q,"op":"mv%d","issue":%q,"status":%q,"seen":%d}`, mover.id, i, key, statuses[(i/len(cards)+i)%len(statuses)], seen))
			l.action("move", code, d)
		}
	})
	// The first tab of each hot issue changes a field of it every second,
	// with the version its page showed.
	for i := range len(l.man.Hot) {
		t, id := l.issues[i], l.man.HotIDs[i]
		r := rand.New(rand.NewPCG(uint64(i), 7))
		wg.Go(func() {
			select {
			case <-time.After(time.Duration(i) * 100 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			for k := 0; ; k++ {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
				v, _ := l.issueV.Load(id)
				seen, _ := v.(int64)
				var field, value string
				switch k % 3 {
				case 0:
					field, value = "priority", []string{"low", "medium", "high", "critical"}[(k/3+i)%4]
				case 1:
					field, value = "points", strconv.Itoa(1+(k/3+i)%13) // never the value it holds: that stores nothing
				default:
					field, value = "summary", sentence(r, 3+r.IntN(5))
				}
				l.changes.add(&pending{key: id, after: seen, sent: time.Now()}, l.measuring.Load())
				code, d := t.post("/cmd/issues/"+id+"/"+field, fmt.Sprintf(`{"tab":%q,"op":"f%d-%d","seen":%d,"value":%q}`, t.id, i, k, seen, value))
				l.action("field", code, d)
			}
		})
	}
	for _, e := range l.editors {
		wg.Go(func() { l.type5(ctx, e) })
	}
	// NFR-39: a document every 200 ms, in turn, as a person who opens it.
	paths := []string{"/", "/projects/" + l.man.Board + "/board", "/" + l.man.Hot[0], "/issues?" + l.man.List, "/spaces/" + l.man.SpaceKey + "/pages/" + l.man.Page}
	names := []string{"home", "board", "issue", "list", "page"}
	wg.Go(func() {
		probe := newTab("probe", 9000, l.base)
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			took, _, err := probe.load(ctx, paths[i%len(paths)])
			if err == nil && l.measuring.Load() {
				l.times.add("doc:"+names[i%len(names)], took)
				l.times.add("doc", took)
			}
		}
	})
	return &wg
}

// type5 types 5 characters a second into the page's first paragraph, one
// commit in flight, the keystrokes typed meanwhile in the next commit.
func (l *load) type5(ctx context.Context, e *editor) {
	time.Sleep(time.Duration(e.i) * 200 * time.Millisecond / nEditors)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	typed, inflight := 0, ""
	letter := string(rune('a' + e.i%26))
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			typed++
		case ref := <-e.confirm:
			if ref == inflight {
				inflight = ""
			}
		case <-time.After(3 * time.Second):
			inflight = "" // a lost answer: the editor would resend
		}
		if inflight != "" || typed == 0 {
			continue
		}
		e.mu.Lock()
		e.commits++
		ref := fmt.Sprintf("e%02d-%d", e.i, e.commits)
		base := e.head
		e.mu.Unlock()
		body := fmt.Sprintf(`{"tab":%q,"epoch":%q,"base":%d,"ref":%q,"client":%q,"steps":[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":%q}]}}]}`,
			e.id, e.epoch, base, ref, e.client, strings.Repeat(letter, typed))
		l.commits.add(&pending{key: ref, after: base, sent: time.Now()}, l.measuring.Load())
		code, d := e.post("/cmd/pages/"+l.man.Page+"/commit", body)
		l.action("commit", code, d)
		if code == 204 {
			e.mu.Lock()
			e.typed += typed
			e.mu.Unlock()
			typed, inflight = 0, ref
		}
	}
}

// measure runs the load for d and takes the numbers.
func (l *load) measure(r *Results, d time.Duration) error {
	ctx, stop := context.WithCancel(l.ctx)
	defer stop()
	wg := l.drive(ctx)
	time.Sleep(5 * time.Second) // warm-up
	m0, err := l.srv.scrape(l.ctx)
	if err != nil {
		return err
	}
	for _, t := range l.all {
		t.mu.Lock()
		t.regions = map[string]*region{}
		t.mu.Unlock()
	}
	cpu0, at0, self0 := l.srv.cpu(), time.Now(), selfCPU()
	logLines0 := len(overrunLines(l.srv.logPath))
	l.measuring.Store(true)
	slog.Info("measuring", "for", d)
	var prof sync.WaitGroup
	if l.profile != "" {
		prof.Go(func() {
			if err := l.srv.cpuProfile(l.ctx, d, l.profile); err != nil {
				slog.Warn("profile", "err", err)
			}
		})
	}
	time.Sleep(d)
	prof.Wait()
	l.measuring.Store(false)
	cpu1, at1, self1 := l.srv.cpu(), time.Now(), selfCPU()
	stop()
	wg.Wait()
	time.Sleep(2 * time.Second) // the last frames
	m1, err := l.srv.scrape(l.ctx)
	if err != nil {
		return err
	}
	r.ServerCores = (cpu1 - cpu0).Seconds() / at1.Sub(at0).Seconds()
	r.ClientCores = (self1 - self0).Seconds() / at1.Sub(at0).Seconds()
	r.RSS, r.RSSPeak = l.srv.rss()
	l.srv.mu.Lock()
	r.RSSMaxSampled, r.AnonMax, r.FileMax = l.srv.maxRSS, l.srv.maxAnon, l.srv.maxFile
	l.srv.mu.Unlock()
	r.delta(m0, m1)
	r.Overruns = overrunLines(l.srv.logPath)[logLines0:]
	r.Times = map[string]Dist{}
	for _, name := range l.times.names() {
		r.Times[name] = dist(l.times.get(name))
	}
	r.Codes = map[string]int64{}
	l.codes.Range(func(k, v any) bool {
		r.Codes[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	r.unseen(l)
	r.regions(l)
	// MP-05.1: the page holds every keystroke the server took.
	for _, e := range l.editors {
		e.mu.Lock()
		r.Typed += e.typed
		e.mu.Unlock()
	}
	probe := newTab("check", 9001, l.base)
	if _, body, err := probe.load(l.ctx, "/spaces/"+l.man.SpaceKey+"/pages/"+l.man.Page); err == nil {
		r.DocChars = typedChars(body)
	}
	r.DBBytesAfter = fileSize(l.srv.db)
	return nil
}

// typedChars counts the editors' letters at the start of the page's first
// paragraph.
func typedChars(page []byte) int {
	s := string(page)
	i := strings.Index(s, `id="page-body"`)
	if i < 0 {
		return -1
	}
	s = s[i:]
	j := strings.Index(s, "<p>")
	if j < 0 {
		return -1
	}
	n := 0
	for _, c := range s[j+3:] {
		if c < 'a' || c > 't' {
			break
		}
		n++
	}
	return n
}

// restart stops the server as a deploy does and starts it again on the
// same database; every stream reconnects by itself (NFR-42).
func (l *load) restart(r *Results) error {
	rev, _ := l.lastRev.Load().(string)
	versions := map[string]int64{}
	l.issueV.Range(func(k, v any) bool { versions[k.(string)] = v.(int64); return true })
	chans := make([]chan struct{}, len(l.all))
	for i, t := range l.all {
		chans[i] = t.expect()
	}
	r.StopTook = l.srv.stop()
	down := time.Now()
	healthy, err := l.srv.start()
	if err != nil {
		return err
	}
	r.RestartTook = healthy.Sub(down)
	var lat []time.Duration
	for i, t := range l.all {
		select {
		case <-chans[i]:
			t.mu.Lock()
			lat = append(lat, t.firstAt.Sub(healthy))
			t.mu.Unlock()
		case <-time.After(30 * time.Second):
			r.NotBack++
		}
	}
	r.Reconnect = dist(lat)
	time.Sleep(time.Second)
	now, _ := l.lastRev.Load().(string)
	r.BoardSame = rev != "" && rev == now
	same := 0
	for id, v := range versions {
		if w, ok := l.issueV.Load(id); ok && w.(int64) == v {
			same++
		}
	}
	r.IssuesSame = fmt.Sprintf("%d of %d", same, len(versions))
	r.EditorStates = map[string]int{}
	for _, e := range l.editors {
		e.mu.Lock()
		r.EditorStates[e.state]++
		e.mu.Unlock()
	}
	r.RSSAfterRestart, _ = l.srv.rss()
	slog.Info("restarted", "stop", r.StopTook.Round(time.Millisecond), "reconnect p95", r.Reconnect.P95, "max", r.Reconnect.Max, "not back", r.NotBack)
	return nil
}

// sustain keeps the load up for d after the restart and samples, every
// minute, the database's size, the events by stream kind and the fold's
// numbers (NFR-43).
func (l *load) sustain(r *Results, db string, d time.Duration) error {
	ro, err := sqlite.Open(l.ctx, db, sqlite.Options{ReadOnly: true, Readers: 1})
	if err != nil {
		return err
	}
	defer ro.Close()
	ctx, stop := context.WithCancel(l.ctx)
	wg := l.drive(ctx)
	start := time.Now()
	take := func() Growth {
		g := Growth{Minute: time.Since(start).Minutes(), Bytes: fileSize(db), Events: map[string]int64{}}
		if fi, err := os.Stat(db + "-wal"); err == nil {
			g.WAL = fi.Size()
		}
		if rows, err := ro.R.QueryContext(l.ctx, `SELECT stream, count(*) FROM lith_events GROUP BY stream`); err == nil {
			for rows.Next() {
				var k string
				var n int64
				rows.Scan(&k, &n)
				g.Events[k] = n
			}
			rows.Close()
		}
		var free sql.NullInt64
		ro.R.QueryRowContext(l.ctx, `PRAGMA freelist_count`).Scan(&free)
		g.FreePages = free.Int64
		if m, err := l.srv.scrape(l.ctx); err == nil {
			g.Folded = m.sum("tickit_folded_events_total")
			g.Folds = m["tickit_fold_seconds_count"]
			g.FoldsOver5ms = g.Folds - m[`tickit_fold_seconds_bucket{le="0.005"}`]
			if g.Folds > 0 {
				g.FoldMean = m["tickit_fold_seconds_sum"] / g.Folds
			}
			g.RSS = int64(m["process_resident_memory_bytes"])
		}
		return g
	}
	r.Growth = append(r.Growth, take())
	for time.Since(start) < d {
		time.Sleep(min(time.Minute, d-time.Since(start)))
		g := take()
		r.Growth = append(r.Growth, g)
		slog.Info("sustain", "minute", fmt.Sprintf("%.1f", g.Minute), "bytes", g.Bytes, "wal", g.WAL, "events", g.Events, "folded", g.Folded, "folds", g.Folds, "over 5 ms", g.FoldsOver5ms)
	}
	stop()
	wg.Wait()
	_, r.RSSPeakSustain = l.srv.rss()
	return nil
}
