package demo

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"
)

// The nightly reset (DM-03). Reset empties every table of visitor content
// in one writer command, then runs the seed:
//
//  1. Actions answer 503 from here on (Resetting; the guard refuses in the
//     writer whatever got past the check), and pages show MsgResetting.
//  2. The writer is flushed, and the reset waits until the events runner's
//     catch-up projectors (notifications) have handled every event: none
//     of them is then left behind on an event about to go.
//  3. One command deletes the rows of every table but those kept (Kept):
//     the event log and every stream's state with the inline projections
//     built from them, in one transaction, so a rebuild after the reset
//     equals the live tables. A projection copy of a rebuild under way is
//     emptied too; the rebuild goes on from its position. Positions keep
//     growing (sqlite_sequence stays), so the catch-up projectors' and
//     rebuilds' checkpoints stay true: every event after them is new. The
//     same command bumps the epoch (MP-13.2) and logs the reset. Such a
//     DELETE without WHERE is SQLite's truncation, a few milliseconds for
//     a day of the demo.
//  4. The seed runs, a batch of steps per command (Seed).
//  5. Actions go through again, and every view renders once more
//     (MarkAll), so open pages show the seeded state on their next tick.
//     Until then views render nothing (Quiet) and documents show
//     MsgResetting, so no page shows the emptied or half-seeded state.

// Kept are the tables a reset keeps: staff accounts with their login
// sessions and the preferences of every session (people rows of guests go;
// display preferences are no content), the security and moderation logs,
// blocks, the demo's own state, and lith's bookkeeping.
var Kept = []string{
	"accounts", "login_sessions", "security_log", "prefs",
	"demo_state", "demo_resets", "moderation_log", "blocks",
	"lith_projections", "lith_event_kinds", "sqlite_sequence",
}

// Reset resets the demo now: by is the site admin's person id, "" for the
// schedule. It returns ErrResetting while another reset is under way. A
// failure is logged at ERROR and counted.
func (d *Demo) Reset(ctx context.Context, by string) error {
	if !d.reset.TryLock() {
		return ErrResetting
	}
	defer d.reset.Unlock()
	start := d.cfg.Now()
	d.hold(true)
	d.cfg.Changed([]string{Topic})
	n, err := d.resetNow(ctx, by, start)
	d.hold(false)
	d.cfg.MarkAll()
	took := d.cfg.Now().Sub(start)
	if d.cfg.OnReset != nil {
		d.cfg.OnReset(err == nil, took)
	}
	if err != nil {
		d.cfg.Log.Error("demo reset failed", "by", by, "took", took.Round(time.Millisecond), "err", err)
		return err
	}
	d.cfg.Log.Info("demo reset", "by", by, "took", took.Round(time.Millisecond), "issues", n.Issues, "comments", n.Comments, "steps", n.Steps)
	return nil
}

func (d *Demo) resetNow(ctx context.Context, by string, start time.Time) (Seeded, error) {
	ctx = context.WithoutCancel(ctx) // a reset that began finishes, or the demo stays empty
	if err := d.cfg.W.Flush(ctx); err != nil {
		return Seeded{}, err
	}
	if err := d.caughtUp(ctx, 15*time.Second); err != nil {
		d.cfg.Log.Warn("demo reset: the catch-up projectors are behind", "err", err)
	}
	var id, epoch int64
	err := d.cfg.W.Do(ctx, writer.Func(func(tx *writer.Tx) error {
		var err error
		if epoch, err = Wipe(tx); err != nil {
			return err
		}
		return tx.QueryRow(`INSERT INTO demo_resets (started_at, by, epoch) VALUES (?, ?, ?) RETURNING id`, start.Unix(), by, epoch).Scan(&id)
	}))
	if err != nil {
		return Seeded{}, fmt.Errorf("demo reset: empty the database: %w", err)
	}
	n, err := Seed(ctx, d.cfg.W, d.cfg.Content, d.cfg.Now(), epoch)
	outcome, detail := "ok", fmt.Sprintf("%d projects, %d issues, %d changes, %d comments, %d page steps in %s", n.Projects, n.Issues, n.Changes, n.Comments, n.PageSteps, n.Took.Round(time.Millisecond))
	if err != nil {
		outcome, detail = "failed", err.Error()
	}
	if lerr := d.cfg.W.Do(ctx, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE demo_resets SET finished_at = ?, outcome = ?, detail = ? WHERE id = ?`, d.cfg.Now().Unix(), outcome, clip(detail, 500), id)
		tx.Touch(Topic)
		return err
	})); lerr != nil && err == nil {
		err = lerr
	}
	return n, err
}

// caughtUp waits until every catch-up projector has handled the last
// event, or one is parked, or wait passed.
func (d *Demo) caughtUp(ctx context.Context, wait time.Duration) error {
	if d.cfg.Projectors == nil {
		return nil
	}
	deadline := time.Now().Add(wait)
	for {
		behind := ""
		for _, p := range d.cfg.Projectors().Projectors {
			if p.CatchUp && p.Status == "live" && p.Lag > 0 {
				behind = p.Name
			}
		}
		if behind == "" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s is behind", behind)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Wipe empties every table but Kept, deletes the guests' rows of people,
// empties the event log, and bumps the epoch, which it returns. It runs in
// one command: the reset's, and `tickit seed -force`'s.
func Wipe(tx *writer.Tx) (int64, error) {
	// Tables and virtual tables (a search index), not the shadow tables
	// that a virtual table keeps for itself.
	rows, err := tx.QueryUncached(`SELECT name FROM pragma_table_list WHERE schema = 'main' AND type IN ('table', 'virtual')
		AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return 0, err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return 0, err
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	// Children before parents: memberships before people and scopes.
	slices.SortStableFunc(tables, func(a, b string) int { return rank(a) - rank(b) })
	for _, t := range tables {
		if slices.Contains(Kept, t) {
			continue
		}
		q := `DELETE FROM ` + quote(t)
		if t == "people" {
			q += ` WHERE kind = 'guest'`
		}
		if _, err := tx.ExecUncached(q); err != nil {
			return 0, fmt.Errorf("empty %s: %w", t, err)
		}
	}
	var epoch int64
	if err := tx.QueryRow(`UPDATE demo_state SET epoch = epoch + 1 WHERE id = 1 RETURNING epoch`).Scan(&epoch); err != nil {
		return 0, err
	}
	tx.Touch(Topic, events.Topic)
	return epoch, nil
}

func rank(table string) int {
	switch table {
	case "people", "scopes":
		return 1
	}
	return 0
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

// Empty reports whether the database holds no demo content yet: no event
// and no project.
func Empty(ctx context.Context, q sqlite.Querier) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM projects) + (SELECT count(*) FROM scopes)`).Scan(&n)
	if err != nil {
		return false, err
	}
	var events int
	err = q.QueryRowContext(ctx, `SELECT count(*) FROM lith_events`).Scan(&events)
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return false, err
	}
	return n == 0 && events == 0, nil
}

// Run resets the demo every night at 04:00 Europe/Zurich (DM-03.1, Q21),
// and tells every page WarnBefore ahead (DM-03.2), until ctx ends.
func (d *Demo) Run(ctx context.Context) {
	for {
		now := d.cfg.Now()
		next := NextReset(now)
		if warn := next.Add(-WarnBefore); now.Before(warn) {
			if !sleepUntil(ctx, warn) {
				return
			}
			d.cfg.Changed([]string{Topic})
		}
		if !sleepUntil(ctx, next) {
			return
		}
		if err := d.Reset(ctx, ""); err != nil && !errors.Is(err, ErrResetting) {
			// logged by Reset; the next night tries again
			continue
		}
	}
}

func sleepUntil(ctx context.Context, t time.Time) bool {
	timer := time.NewTimer(time.Until(t))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// ResetEntry is one reset of the log.
type ResetEntry struct {
	Started, Finished time.Time
	Outcome           string
	By                string
	Epoch             int64
	Detail            string
}

// Resets are the newest resets, newest first.
func Resets(ctx context.Context, q sqlite.Querier, limit int) ([]ResetEntry, error) {
	rows, err := q.QueryContext(ctx, `SELECT started_at, coalesce(finished_at, 0), outcome, by, epoch, detail FROM demo_resets ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ResetEntry
	for rows.Next() {
		var e ResetEntry
		var s, f int64
		if err := rows.Scan(&s, &f, &e.Outcome, &e.By, &e.Epoch, &e.Detail); err != nil {
			return nil, err
		}
		e.Started = time.Unix(s, 0)
		if f > 0 {
			e.Finished = time.Unix(f, 0)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// clip cuts s to at most n bytes, on a character boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xc0 == 0x80 {
		n--
	}
	return s[:n]
}

// Hold makes actions answer as during a reset, without one, until the
// returned func is called: for tests, and for work on the database that
// must not race visitors.
func (d *Demo) Hold() (release func()) {
	d.reset.Lock()
	d.hold(true)
	d.cfg.Changed([]string{Topic})
	return func() {
		d.hold(false)
		d.reset.Unlock()
		d.cfg.MarkAll()
	}
}
