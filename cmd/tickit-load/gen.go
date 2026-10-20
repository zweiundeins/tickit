package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/demo/seed"
	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
	"github.com/zweiundeins/tickit/migrations"
)

// Manifest names what the generator made for the load to use. It is
// written beside the database as <db>.json.
type Manifest struct {
	Board     string   `json:"board"`     // the project key of the watched board
	Hot       []string `json:"hot"`       // keys of the issues whose fields change, all on the board
	HotIDs    []string `json:"hot_ids"`   // their ids, in the same order
	Cards     []string `json:"cards"`     // keys of the board's cards
	List      string   `json:"list"`      // the issue list's query
	SpaceKey  string   `json:"space_key"` // the co-edited page's space
	Page      string   `json:"page"`      // the co-edited page's id
	Issues    int      `json:"issues"`
	Pages     int      `json:"pages"`
	Events    int64    `json:"events"`
	Took      string   `json:"took"`
	BoardSize int      `json:"board_size"`
}

// parts are cmd/tickit's: every area's projectors in one store.
func parts() []events.Part {
	var p []events.Part
	p = append(p, tracker.Parts()...)
	p = append(p, notify.Parts()...)
	p = append(p, wiki.Parts()...)
	return p
}

var words = strings.Fields(`checkout cart voucher payment login account search filter board sprint release deploy
database backup restore index query cache stock sync warehouse order invoice email newsletter mobile app crash
screen button layout banner image upload export import report chart latency timeout retry queue worker migration
schema rollout flag experiment conversion delivery address shipping returns refund support ticket customer price
discount summary detail profile password session token webhook carrier tracking label printer barcode inventory`)

func sentence(r *rand.Rand, n int) string {
	w := make([]string, n)
	for i := range w {
		w[i] = words[r.IntN(len(words))]
	}
	s := strings.Join(w, " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// text is n words in paragraphs of about 50, one per line (PlainDoc).
func text(r *rand.Rand, n int) string {
	var b strings.Builder
	for n > 0 {
		k := min(n, 30+r.IntN(40))
		b.WriteString(sentence(r, k))
		b.WriteString(".\n")
		n -= k
	}
	return b.String()
}

// gen fills a new database with the demo's seed and then, through the
// real commands in chunks of one writer transaction each, the data of
// NFR-38.2: projects, issues, comments, spaces, pages and field changes up
// to the number of events asked for. Every command runs its inline
// projectors (the read models and the search index), so the database is
// what the server would hold had people made it.
func gen(args []string) error {
	f := flag.NewFlagSet("gen", flag.ExitOnError)
	dbPath := f.String("db", "", "the new database")
	nIssues := f.Int("issues", 10000, "issues in all, the seed's included")
	nPages := f.Int("pages", 2000, "pages in all, the seed's included")
	nEvents := f.Int64("events", 1000000, "events in all")
	nComments := f.Int("comments", 30000, "comments on issues")
	chunk := f.Int("chunk", 1000, "commands to a writer transaction")
	cpuProfile := f.String("cpuprofile", "", "write a CPU profile of the generation to this file")
	f.Parse(args)
	if *cpuProfile != "" {
		pf, err := os.Create(*cpuProfile)
		if err != nil {
			return err
		}
		pprof.StartCPUProfile(pf)
		defer pprof.StopCPUProfile()
	}
	if *dbPath == "" {
		return errors.New("gen: -db is required")
	}
	if _, err := os.Stat(*dbPath); err == nil {
		return fmt.Errorf("gen: %s exists", *dbPath)
	}
	start := time.Now()
	ctx := context.Background()
	store, err := events.New(parts()...)
	if err != nil {
		return err
	}
	tracker.Bind(store)
	wiki.Bind(store)
	db, err := sqlite.Open(ctx, *dbPath, sqlite.Options{Migrations: migrations.FS, Readers: 2})
	if err != nil {
		return err
	}
	defer db.Close()
	w, err := writer.New(db, writer.Options{})
	if err != nil {
		return err
	}
	go w.Run(context.Background())
	defer w.Close()
	// The catch-up projectors (notifications) run as they do in the
	// server, so the database holds what a server that made the events
	// would hold, and the server starts with no backlog.
	notify.Bind(w)
	runner := events.NewRunner(store, db, w, events.Options{})
	runCtx, stopRunner := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() { defer close(runDone); runner.Run(runCtx) }()
	defer func() { stopRunner(); <-runDone }()

	c, err := seed.Load()
	if err != nil {
		return err
	}
	epoch, err := demo.Epoch(ctx, db.R)
	if err != nil {
		return err
	}
	seeded, err := demo.Seed(ctx, w, c, time.Now(), epoch)
	if err != nil {
		return err
	}
	slog.Info("seeded", "issues", seeded.Issues, "took", seeded.Took.Round(time.Millisecond))

	r := rand.New(rand.NewPCG(38, 2))
	// Times run from 30 days ago to 10 minutes ago, in the order of the
	// commands, so positions follow times; never later (a change that
	// stores nothing still takes its time).
	t0, t1 := time.Now().AddDate(0, 0, -30), time.Now().Add(-10*time.Minute)
	var ops, total int64
	total = 1 // grows as the plan does; only for the times
	by := func() tabs.By {
		ops++
		p := people.Seeded[r.IntN(len(people.Seeded))]
		at := t0.Add(time.Duration(float64(t1.Sub(t0)) * min(1, float64(ops)/float64(total))))
		return tabs.By{Actor: p, Op: fmt.Sprint("load-", ops), At: at}
	}
	var queue []writer.Command
	flush := func() error {
		if len(queue) == 0 {
			return nil
		}
		batch := queue
		queue = nil
		return w.Do(ctx, writer.Func(func(tx *writer.Tx) error {
			for _, cmd := range batch {
				if v, ok := cmd.(writer.Validator); ok {
					if err := v.Validate(); err != nil {
						return fmt.Errorf("%T: %w", cmd, err)
					}
				}
				if err := cmd.Apply(tx); err != nil {
					return fmt.Errorf("%T: %w", cmd, err)
				}
			}
			return nil
		}))
	}
	add := func(cmd writer.Command) error {
		queue = append(queue, cmd)
		if len(queue) >= *chunk {
			return flush()
		}
		return nil
	}
	count := func() (n int64) {
		db.R.QueryRowContext(ctx, `SELECT count(*) FROM lith_events`).Scan(&n)
		return n
	}

	// The plan, for the times: issues, comments, pages, then changes.
	total = int64(*nIssues+*nComments+*nPages) + *nEvents - count()

	// Projects: LOAD's board holds 100 cards, the rest of its issues
	// wait in the backlog; three more projects hold the others.
	type proj struct{ id, key string }
	projects := []proj{{tracker.NewID(), "LOAD"}, {tracker.NewID(), "LDA"}, {tracker.NewID(), "LDB"}, {tracker.NewID(), "LDC"}}
	for _, p := range projects {
		if err := add(tracker.CreateProject{By: by(), ID: p.id, Key: p.key, Name: "Load " + p.key, Description: sentence(r, 12), Lead: people.Seeded[0].ID, Open: true}); err != nil {
			return err
		}
	}
	type issue struct {
		id, key string
		board   bool // on LOAD's board: its status stays
		project int
		labels  []string
	}
	var issues []issue
	numbers := make([]int, len(projects))
	board := []string{tracker.StatusToDo, tracker.StatusInProgress, tracker.StatusInReview, tracker.StatusDone}
	others := []string{tracker.StatusBacklog, tracker.StatusSelected, tracker.StatusToDo, tracker.StatusInProgress, tracker.StatusInReview, tracker.StatusBlocked, tracker.StatusDone, tracker.StatusDone}
	types := []tracker.IssueType{tracker.Story, tracker.Task, tracker.Bug, tracker.Bug, tracker.Story}
	m := Manifest{Board: "LOAD", SpaceKey: "LDSA"}
	for i := range *nIssues - seeded.Issues {
		pi := 0
		if i >= 2500 {
			pi = 1 + i%3
		}
		numbers[pi]++
		is := issue{id: tracker.NewID(), key: tracker.IssueKey(projects[pi].key, int64(numbers[pi])), project: pi}
		status := others[r.IntN(len(others))]
		if pi == 0 {
			status = tracker.StatusBacklog
			if numbers[pi] <= 100 {
				status, is.board = board[r.IntN(len(board))], true
				m.Cards = append(m.Cards, is.key)
			}
		}
		var points *int
		if r.IntN(3) == 0 {
			p := []int{1, 2, 3, 5, 8}[r.IntN(5)]
			points = &p
		}
		var labels []string
		if r.IntN(2) == 0 {
			labels = []string{words[r.IntN(12)]}
		}
		assignee := ""
		if r.IntN(5) > 0 {
			assignee = people.Seeded[r.IntN(len(people.Seeded))].ID
		}
		is.labels = labels
		issues = append(issues, is)
		if err := add(tracker.CreateIssue{By: by(), ID: is.id, Project: projects[pi].id, Type: types[r.IntN(len(types))],
			Summary: sentence(r, 3+r.IntN(6)), Description: tracker.PlainDoc(text(r, 20+r.IntN(120))), Status: status,
			Priority: tracker.Priorities[r.IntN(4)], Assignee: assignee, Labels: labels, Points: points}); err != nil {
			return err
		}
	}
	for i := range 10 {
		m.Hot = append(m.Hot, issues[i*10].key)
		m.HotIDs = append(m.HotIDs, issues[i*10].id)
	}
	m.BoardSize = len(m.Cards)
	slog.Info("issues queued", "n", len(issues), "took", time.Since(start).Round(time.Millisecond))
	for range *nComments {
		is := issues[r.IntN(len(issues))]
		if err := add(tracker.AddComment{By: by(), Issue: is.id, Body: tracker.PlainDoc(text(r, 8+r.IntN(60)))}); err != nil {
			return err
		}
	}
	if err := flush(); err != nil {
		return err
	}
	slog.Info("issues and comments", "events", count(), "took", time.Since(start).Round(time.Millisecond))

	// Spaces of about 200 pages each, every page under one made before it
	// in its space, at most a few levels deep.
	var seededPages int
	db.R.QueryRowContext(ctx, `SELECT count(*) FROM pages`).Scan(&seededPages)
	type page struct {
		id    string
		depth int
	}
	nSpaces := max(1, (*nPages-seededPages+199)/200)
	spaces := make([][]page, nSpaces)
	spaceIDs := make([]string, nSpaces)
	for i := range spaces {
		spaceIDs[i] = wiki.NewID()
		home := wiki.NewID()
		key := "LDS" + string(rune('A'+i%26))
		if i >= 26 {
			key = "LD" + string(rune('A'+i/26-1)) + string(rune('A'+i%26))
		}
		if err := add(wiki.CreateSpace{By: by(), ID: spaceIDs[i], Home: home, Key: key, Name: "Load space " + key, Description: sentence(r, 10), Open: true,
			HomeDoc: tracker.PlainDoc(text(r, 120))}); err != nil {
			return err
		}
		spaces[i] = []page{{home, 0}}
	}
	made := seededPages + nSpaces
	for made < *nPages-1 {
		si := made % nSpaces
		sp := spaces[si]
		parent := sp[r.IntN(len(sp))]
		for parent.depth >= 4 {
			parent = sp[r.IntN(len(sp))]
		}
		p := page{wiki.NewID(), parent.depth + 1}
		spaces[si] = append(spaces[si], p)
		if err := add(wiki.CreatePage{By: by(), ID: p.id, Space: spaceIDs[si], Parent: parent.id, Title: fmt.Sprintf("%s %d", sentence(r, 2+r.IntN(4)), made),
			Doc: tracker.PlainDoc(text(r, 200+r.IntN(400)))}); err != nil {
			return err
		}
		made++
	}
	// The co-edited page: one paragraph, in the first load space.
	m.Page = wiki.NewID()
	if err := add(wiki.CreatePage{By: by(), ID: m.Page, Space: spaceIDs[0], Parent: spaces[0][0].id, Title: "Co-editing under load",
		Doc: tracker.PlainDoc("Everyone types here.")}); err != nil {
		return err
	}
	made++
	m.Pages = made
	if err := flush(); err != nil {
		return err
	}
	slog.Info("pages", "pages", made, "events", count(), "took", time.Since(start).Round(time.Millisecond))

	// Field changes up to the number of events: one event each. LOAD's
	// cards keep their status, so the board keeps its 100 cards; its
	// backlog stays out of the board.
	labelOf := map[string][]string{}
	for _, is := range issues {
		labelOf[is.id] = is.labels
	}
	const huge = 1 << 40 // Seen: no conflict, as one person changing what they changed
	for n := count(); n < *nEvents; {
		left := *nEvents - n
		k := min(left, 20000)
		for range k {
			is := issues[r.IntN(len(issues))]
			b := by()
			var cmd writer.Command
			switch x := r.IntN(10); {
			case x < 3 && is.project != 0:
				cmd = tracker.SetStatus{By: b, Issue: is.id, Seen: huge, Status: others[1+r.IntN(len(others)-1)]}
			case x < 5:
				cmd = tracker.SetPriority{By: b, Issue: is.id, Seen: huge, Priority: tracker.Priorities[r.IntN(4)]}
			case x < 7:
				cmd = tracker.Assign{By: b, Issue: is.id, Seen: huge, Assignee: people.Seeded[r.IntN(len(people.Seeded))].ID}
			case x < 8:
				p := 1 + r.IntN(13)
				cmd = tracker.SetPoints{By: b, Issue: is.id, Seen: huge, Points: &p}
			case x < 9:
				cmd = tracker.SetSummary{By: b, Issue: is.id, Seen: huge, Summary: sentence(r, 3+r.IntN(6))}
			default:
				// One label replaces the one it had, as people retag
				// rather than pile labels up.
				l := words[r.IntN(30)]
				cmd = tracker.SetLabels{By: b, Issue: is.id, Add: []string{l}, Remove: labelOf[is.id]}
				labelOf[is.id] = []string{l}
			}
			if err := add(cmd); err != nil {
				return err
			}
		}
		if err := flush(); err != nil {
			return err
		}
		n = count()
		slog.Info("changes", "events", n, "took", time.Since(start).Round(time.Millisecond))
	}
	for {
		lag := int64(0)
		for _, p := range runner.Stats().Projectors {
			lag += p.Lag
		}
		if lag == 0 {
			break
		}
		slog.Info("catching up", "lag", lag, "took", time.Since(start).Round(time.Millisecond))
		time.Sleep(5 * time.Second)
	}
	m.Issues = *nIssues
	m.Events = count()
	m.List = "project=LOAD&sort=updated&dir=desc"
	m.Took = time.Since(start).Round(time.Millisecond).String()
	if err := w.Do(ctx, writer.Func(func(tx *writer.Tx) error { _, err := tx.Exec(`PRAGMA optimize`); return err })); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(*dbPath+".json", b, 0o644); err != nil {
		return err
	}
	slog.Info("generated", "db", *dbPath, "issues", m.Issues, "pages", m.Pages, "events", m.Events, "took", m.Took)
	return nil
}
