package demo

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/demo/seed"
)

// Config is what a Demo works with.
type Config struct {
	DB  *sqlite.DB
	W   *writer.Writer
	Log *slog.Logger
	// Content is the seed; nil loads internal/demo/seed's.
	Content *seed.Content
	// Changed tells the views of topics that changed outside a command:
	// the hub's Changed.
	Changed func(topics []string)
	// MarkAll renders every view again: the hub's MarkAll, once a reset
	// is done (DM-03.4).
	MarkAll func()
	// Projectors are the events runner's stats: a reset waits until the
	// catch-up projectors have handled every event it is about to delete.
	// Nil waits for nothing.
	Projectors func() events.Stats
	// OnReset hears how each reset went, for the metrics.
	OnReset func(ok bool, took time.Duration)
	// Now is the clock; time.Now by default.
	Now func() time.Time
}

// Demo runs tickit as a public demo: the nightly reset and its warning
// (DM-03), the caps of DM-04.3, blocks of guests (DM-07.2). One per
// process.
type Demo struct {
	cfg       Config
	resetting atomic.Bool
	reset     sync.Mutex // one reset at a time
	addrs     addrBlocks
}

// New returns the demo of cfg.
func New(cfg Config) (*Demo, error) {
	if cfg.DB == nil || cfg.W == nil {
		return nil, errors.New("demo: a database and a writer")
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Changed == nil {
		cfg.Changed = func([]string) {}
	}
	if cfg.MarkAll == nil {
		cfg.MarkAll = func() {}
	}
	if cfg.Content == nil {
		c, err := seed.Load()
		if err != nil {
			return nil, err
		}
		cfg.Content = c
	}
	return &Demo{cfg: cfg, addrs: addrBlocks{seen: map[string]seenAddr{}, blocked: map[string]time.Time{}}}, nil
}

// Content is the seed the demo resets to.
func (d *Demo) Content() *seed.Content { return d.cfg.Content }

// WarnBefore is how long before a reset every page warns of it (DM-03.2).
const WarnBefore = 10 * time.Minute

// Messages of the demo.
const (
	// MsgWarning is what every open page shows WarnBefore a reset.
	MsgWarning = "The demo resets in 10 minutes. Your changes will be deleted."
	// MsgResetting is what pages show, and actions answer, during a reset.
	MsgResetting = "The demo is resetting"
	// MsgFull is what a guest hears who would create past a cap.
	MsgFull = "The demo is full until the next reset"
	// MsgBlocked is what a blocked guest hears for every change.
	MsgBlocked = "Changes from this browser are paused."
)

// EventReset is the security log's kind of a "Reset now" (FR-156).
const EventReset = "demo_reset"

// ErrResetting is the error of an action while the demo resets: it
// answers 503 with MsgResetting.
var ErrResetting = errors.New(MsgResetting)

// Status is what the demo view shows at a time.
type Status struct {
	Remaining string // to the next reset: "5 h", "12 min" (DM-10.1)
	Soon      bool   // the reset comes within WarnBefore (DM-03.2)
	Resetting bool   // a reset is under way (DM-03.4)
}

// Status is the demo's status at now.
func (d *Demo) Status(now time.Time) Status {
	return Status{Remaining: Remaining(now), Soon: NextReset(now).Sub(now) <= WarnBefore, Resetting: d.Resetting()}
}

// Resetting reports whether a reset is under way.
func (d *Demo) Resetting() bool { return d != nil && d.resetting.Load() }

// Epoch is the demo's epoch (MP-13.2), which every reset bumps. The wiki's
// editor names the epoch it opened in with every batch of edits, and the
// wiki refuses a batch of an older one.
func Epoch(ctx context.Context, q sqlite.Querier) (int64, error) {
	var e int64
	err := q.QueryRowContext(ctx, `SELECT epoch FROM demo_state WHERE id = 1`).Scan(&e)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return e, err
}
