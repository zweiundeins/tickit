package demo

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// The guard of every action: web runs each action's command wrapped by
// Guard, inside tabs.Act. In the writer, so that nothing races it, it
// refuses
//
//   - every change while the demo resets, with ErrResetting (503, no
//     outcome: the page tries again later);
//   - every change of a blocked guest, by their person or their client
//     address, with MsgBlocked as a denial (403, and the outcome says
//     why, DM-07.2);
//   - a guest's creation past the demo-wide cap of its kind, with MsgFull
//     (DM-04.3), and counts the ones that go through.

// Caps are the most issues, pages and images guests together create
// between two resets (DM-04.3).
var Caps = map[string]int{CapIssue: 1000, CapPage: 300, CapImage: 200}

// Kinds of creations the caps count.
const (
	CapIssue = "issue"
	CapPage  = "page"
	CapImage = "image"
)

var (
	capsMu   sync.Mutex
	capKinds = []func(writer.Command) string{func(cmd writer.Command) string {
		if _, ok := cmd.(tracker.CreateIssue); ok {
			return CapIssue
		}
		return ""
	}}
)

// CountCreations adds a kind of command the caps count: kind returns
// CapPage for the wiki's command that creates a page, CapImage for an
// upload, "" for every other command. The wiki calls it from an init
// function.
func CountCreations(kind func(writer.Command) string) {
	capsMu.Lock()
	defer capsMu.Unlock()
	capKinds = append(capKinds, kind)
}

func capOf(cmd writer.Command) string {
	capsMu.Lock()
	defer capsMu.Unlock()
	for _, f := range capKinds {
		if k := f(cmd); k != "" {
			return k
		}
	}
	return ""
}

// Guard wraps the command of an action by actor from the client address
// addr (edge.ClientKey). Put it inside tabs.Act. It notes the address as
// the person's, for a block to come.
func (d *Demo) Guard(actor people.Person, addr string, cmd writer.Command) writer.Command {
	g := guarded{d: d, actor: actor, cmd: cmd, kind: capOf(cmd)}
	if d != nil && actor.Kind == people.Guest {
		now := d.cfg.Now()
		d.addrs.saw(actor.ID, addr, now)
		g.addrBlocked = d.addrs.isBlocked(addr, now)
	}
	return g
}

type guarded struct {
	d           *Demo
	actor       people.Person
	addrBlocked bool
	cmd         writer.Command
	kind        string
}

func (g guarded) Validate() error {
	if v, ok := g.cmd.(writer.Validator); ok {
		return v.Validate()
	}
	return nil
}

func (g guarded) Apply(tx *writer.Tx) error {
	_, err := g.ApplyResult(tx)
	return err
}

func (g guarded) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if g.d.Resetting() {
		return nil, ErrResetting
	}
	guest := g.actor.Kind == people.Guest
	if guest {
		blocked := g.addrBlocked
		if !blocked {
			var err error
			if blocked, err = isBlocked(tx.Context(), tx, g.actor.ID, time.Now()); err != nil {
				return nil, err
			}
		}
		if blocked {
			return nil, &access.Denial{Refusal: writer.Refusal{Msg: MsgBlocked}}
		}
	}
	count := guest && g.kind != ""
	if count {
		var n int
		err := tx.QueryRow(`SELECT count FROM demo_caps WHERE kind = ?`, g.kind).Scan(&n)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if n >= Caps[g.kind] {
			return nil, &writer.Refusal{Msg: MsgFull}
		}
	}
	var res []byte
	var err error
	if rc, ok := g.cmd.(writer.ResultCommand); ok {
		res, err = rc.ApplyResult(tx)
	} else {
		err = g.cmd.Apply(tx)
	}
	if err != nil || !count {
		return res, err
	}
	_, err = tx.Exec(`INSERT INTO demo_caps (kind, count) VALUES (?, 1) ON CONFLICT (kind) DO UPDATE SET count = count + 1`, g.kind)
	return res, err
}

// Created is how many things of each kind guests created since the
// reset, against Caps.
func Created(ctx context.Context, q sqlite.Querier) (map[string]int, error) {
	rows, err := q.QueryContext(ctx, `SELECT kind, count FROM demo_caps`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// Check is the guard for a change sent without waiting, outside the
// writer (typing, carets): ErrResetting while the demo resets, a denial
// for a blocked guest, else nil.
func (d *Demo) Check(ctx context.Context, q sqlite.Querier, actor people.Person, addr string) error {
	switch {
	case d == nil:
		return nil
	case d.Resetting():
		return ErrResetting
	case actor.Kind != people.Guest:
		return nil
	}
	now := d.cfg.Now()
	blocked := d.addrs.isBlocked(addr, now)
	if !blocked {
		var err error
		if blocked, err = isBlocked(ctx, q, actor.ID, now); err != nil {
			return err
		}
	}
	if blocked {
		return &access.Denial{Refusal: writer.Refusal{Msg: MsgBlocked}}
	}
	return nil
}

// isBlocked reports whether a guest's changes are paused at now.
func isBlocked(ctx context.Context, q sqlite.Querier, person string, now time.Time) (bool, error) {
	var until int64
	err := q.QueryRowContext(ctx, `SELECT until FROM blocks WHERE person_id = ?`, person).Scan(&until)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	}
	return until > now.Unix(), nil
}

// addrBlocks keeps client addresses in memory only (NFR-25.2): the
// address each guest acted from last, for a block to come, and the
// blocked ones. A restart forgets them; the blocks of persons stay.
type addrBlocks struct {
	mu      sync.Mutex
	seen    map[string]seenAddr  // person id to the address they last acted from
	blocked map[string]time.Time // address to the end of its block
}

type seenAddr struct {
	addr string
	at   time.Time
}

// maxSeen bounds the addresses kept for blocks to come.
const maxSeen = 50_000

func (a *addrBlocks) saw(person, addr string, now time.Time) {
	if addr == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.seen) >= maxSeen {
		for p, s := range a.seen {
			if now.Sub(s.at) > BlockFor {
				delete(a.seen, p)
			}
		}
		if len(a.seen) >= maxSeen {
			clear(a.seen) // a flood: forget them all rather than grow
		}
	}
	a.seen[person] = seenAddr{addr, now}
}

func (a *addrBlocks) isBlocked(addr string, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	until, ok := a.blocked[addr]
	if ok && !now.Before(until) {
		delete(a.blocked, addr)
		return false
	}
	return ok
}

// block blocks the address person last acted from, until; unblock (until
// zero) lifts it.
func (a *addrBlocks) block(person string, until time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.seen[person]
	if !ok {
		return
	}
	if until.IsZero() {
		delete(a.blocked, s.addr)
		return
	}
	a.blocked[s.addr] = until
}
