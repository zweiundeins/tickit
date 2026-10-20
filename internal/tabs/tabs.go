// Package tabs keeps the state of every loaded page (a tab) in its row of
// tab_state: the newest outcomes of its actions, each area's own state,
// and the views it added to its stream. Every action runs inside Act,
// which records its outcome whatever it was, so the tab's view (View)
// brings the op id back on the stream and the page's control ends its
// pending state (MP-09). A row appears with the tab's first command: a
// GET writes nothing.
package tabs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/people"
)

const (
	// MaxOutcomes is how many outcomes a tab's row keeps, newest last: two
	// quick actions in one tab both settle.
	MaxOutcomes = 16
	// MaxWatched is how many views a tab may add to its stream; adding one
	// more drops the oldest.
	MaxWatched = 8
	// TTL is how long a tab's row outlives its last action (requirements 4.1).
	TTL = 3 * 24 * time.Hour
	// PrefsTTL is how long a session's preferences outlive their last change.
	PrefsTTL = 365 * 24 * time.Hour
)

// Topic is touched by every change to a tab's row; the tab's view lists it.
func Topic(tab string) string { return "tab:" + tab }

// By is who acts, in which action, when: the part of every command that
// comes from the request. Handlers make it from the session; commands
// take the actor from it, never from the payload (NFR-02.2).
type By struct {
	Actor people.Person
	Op    string
	At    time.Time
}

// Meta is the event metadata of an action (FR-155): its time, its actor's
// person id and its op id.
func (b By) Meta() events.Meta {
	return events.Meta{At: b.At, Actor: b.Actor.ID, Causation: b.Op}
}

// Outcome is what became of one action of a tab.
type Outcome struct {
	Op    string `json:"op"`
	OK    bool   `json:"ok"`
	Msg   string `json:"msg,omitempty"`   // why it was refused
	Field string `json:"field,omitempty"` // the field the refusal belongs to
	At    int64  `json:"at"`              // unix ms
}

// Watched is a view a tab added to its stream (a side panel): its spec
// ("issue:<id>") and its key, so a reconnect restores it.
type Watched struct {
	View string `json:"view"`
	Key  string `json:"key"`
}

// State is a tab's row.
type State struct {
	Kind     string                     `json:"-"` // the page's kind, in its own column
	Outcomes []Outcome                  `json:"outcomes,omitempty"`
	Areas    map[string]json.RawMessage `json:"areas,omitempty"`
	Watched  []Watched                  `json:"watched,omitempty"`
}

// Outcome is the outcome of op, if the row still has it.
func (s State) Outcome(op string) (Outcome, bool) {
	for _, o := range s.Outcomes {
		if o.Op == op {
			return o, true
		}
	}
	return Outcome{}, false
}

// record keeps o as the newest outcome, in place of an older one of the
// same op.
func (s *State) record(o Outcome) {
	s.Outcomes = slices.DeleteFunc(s.Outcomes, func(x Outcome) bool { return x.Op == o.Op })
	s.Outcomes = append(s.Outcomes, o)
	if n := len(s.Outcomes) - MaxOutcomes; n > 0 {
		s.Outcomes = slices.Delete(s.Outcomes, 0, n)
	}
}

// Load is an area's state of the tab, the zero T when the area stored none.
func Load[T any](s State, area string) (T, error) {
	var v T
	raw, ok := s.Areas[area]
	if !ok {
		return v, nil
	}
	err := json.Unmarshal(raw, &v)
	return v, err
}

// Store sets an area's state of the tab; Save writes it.
func Store[T any](s *State, area string, v T) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if s.Areas == nil {
		s.Areas = map[string]json.RawMessage{}
	}
	s.Areas[area] = raw
	return nil
}

// Get reads a tab's state. A tab that never acted has no row yet, and its
// state is the zero value.
func Get(ctx context.Context, q sqlite.Querier, sid, tab string) (State, error) {
	var s State
	var data string
	err := q.QueryRowContext(ctx, `SELECT kind, data FROM tab_state WHERE sid = ? AND tab_id = ?`, sid, tab).Scan(&s.Kind, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	err = json.Unmarshal([]byte(data), &s)
	return s, err
}

// Save writes a tab's row and touches the tab's topic.
func Save(tx *writer.Tx, sid, tab string, s State, at time.Time) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO tab_state (sid, tab_id, kind, data, seen) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (sid, tab_id) DO UPDATE SET kind = excluded.kind, data = excluded.data, seen = excluded.seen`,
		sid, tab, s.Kind, string(data), at.Unix()); err != nil {
		return err
	}
	tx.Touch(Topic(tab))
	return nil
}

// Update changes one area's state of a tab inside a command: it reads the
// row, hands fn the area's state, and saves what fn left.
func Update[T any](tx *writer.Tx, sid, tab, area string, at time.Time, fn func(*T) error) error {
	s, err := Get(tx.Context(), tx, sid, tab)
	if err != nil {
		return err
	}
	v, err := Load[T](s, area)
	if err != nil {
		return err
	}
	if err := fn(&v); err != nil {
		return err
	}
	if err := Store(&s, area, v); err != nil {
		return err
	}
	return Save(tx, sid, tab, s, at)
}

// Refuse returns a refusal that belongs to one field of the page: Act
// records the field with the message, and the page shows it there.
func Refuse(field, msg string) error {
	return &FieldRefusal{Field: field, Refusal: writer.Refusal{Msg: msg}}
}

// FieldRefusal is a *writer.Refusal (errors.As) for one field.
type FieldRefusal struct {
	Field string
	writer.Refusal
}

func (r *FieldRefusal) Error() string   { return r.Msg }
func (r *FieldRefusal) Unwrap() error   { return &r.Refusal }
func (r *FieldRefusal) OnField() string { return r.Field }

// Act runs a page's action and records its outcome in the tab's row: the
// op id, and for a refusal its message and field. Cmd runs in a savepoint
// of its own, so a refusal undoes its changes and drops its topics, while
// the row is written all the same: the tab's view brings the op back
// whatever became of it. Before Cmd, the actor gets a row in people if
// they are a guest acting for the first time (people.Ensure).
//
// Run it as writer.OnceResult(sid+":"+op, act): a retry then replays the
// first answer and runs nothing. Its result (ParseResult) tells a refusal
// for want of a right, which the action answers with 403, and carries
// Cmd's own result when Cmd is a writer.ResultCommand.
type Act struct {
	Session, Tab string
	By           By
	Cmd          writer.Command
}

func (a Act) Validate() error {
	if a.Session == "" || a.Tab == "" || a.By.Op == "" || a.By.Actor.IsZero() || a.By.At.IsZero() || a.Cmd == nil {
		return errors.New("an action needs a session, a tab, an op, an actor, a time and a command")
	}
	if v, ok := a.Cmd.(writer.Validator); ok {
		return v.Validate()
	}
	return nil
}

func (a Act) Apply(tx *writer.Tx) error {
	_, err := a.ApplyResult(tx)
	return err
}

// Marks that begin Act's result.
const (
	markOK      = 'k'
	markRefused = 'r'
	markDenied  = 'd'
)

func (a Act) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if err := people.Ensure(tx, a.By.Actor, a.By.At); err != nil {
		return nil, err
	}
	if a.By.Actor.Kind == people.Account {
		if err := people.SeenSession(tx, a.Session, a.By.At); err != nil {
			return nil, err
		}
	}
	state, err := Get(tx.Context(), tx, a.Session, a.Tab)
	if err != nil {
		return nil, err
	}
	var value []byte
	err = tx.Savepoint(func(tx *writer.Tx) error {
		if rc, ok := a.Cmd.(writer.ResultCommand); ok {
			var err error
			value, err = rc.ApplyResult(tx)
			return err
		}
		return a.Cmd.Apply(tx)
	})
	o := Outcome{Op: a.By.Op, OK: true, At: a.By.At.UnixMilli()}
	mark := byte(markOK)
	var refusal *writer.Refusal
	switch {
	case errors.As(err, &refusal):
		o.OK, o.Msg, value, mark = false, refusal.Msg, nil, markRefused
		var field interface{ OnField() string }
		if errors.As(err, &field) {
			o.Field = field.OnField()
		}
		var denial interface{ Denied() bool }
		if errors.As(err, &denial) && denial.Denied() {
			mark = markDenied
		}
	case err != nil:
		return nil, err
	}
	state.record(o)
	if err := Save(tx, a.Session, a.Tab, state, a.By.At); err != nil {
		return nil, err
	}
	return append([]byte{mark}, value...), nil
}

// Result is what an Act answered.
type Result struct {
	OK     bool   // the command ran without a refusal
	Denied bool   // refused for want of a right: answer 403
	Value  []byte // the command's own result, if it has one and ran
}

// ParseResult reads the result of an Act from writer.Op.Result.
func ParseResult(b []byte) Result {
	if len(b) == 0 {
		return Result{}
	}
	return Result{OK: b[0] == markOK, Denied: b[0] == markDenied, Value: b[1:]}
}

// AddWatched records that a tab's stream watches a view, so that a stream
// that reconnects watches it again. It returns the keys of views it
// dropped to keep MaxWatched; unwatch those.
func AddWatched(tx *writer.Tx, sid, tab string, v Watched, at time.Time) (dropped []string, err error) {
	s, err := Get(tx.Context(), tx, sid, tab)
	if err != nil {
		return nil, err
	}
	s.Watched = slices.DeleteFunc(s.Watched, func(w Watched) bool { return w.Key == v.Key })
	s.Watched = append(s.Watched, v)
	for len(s.Watched) > MaxWatched {
		dropped = append(dropped, s.Watched[0].Key)
		s.Watched = s.Watched[1:]
	}
	return dropped, Save(tx, sid, tab, s, at)
}

// RemoveWatched records that a tab's stream no longer watches the view key.
func RemoveWatched(tx *writer.Tx, sid, tab, key string, at time.Time) error {
	s, err := Get(tx.Context(), tx, sid, tab)
	if err != nil {
		return err
	}
	n := len(s.Watched)
	if s.Watched = slices.DeleteFunc(s.Watched, func(w Watched) bool { return w.Key == key }); len(s.Watched) == n {
		return nil
	}
	return Save(tx, sid, tab, s, at)
}

// signal is an outcome as the page reads it from $_outcomes[op].
type signal struct {
	OK    bool   `json:"ok"`
	Msg   string `json:"msg"`
	Field string `json:"field"`
}

// View is the tab's own view: it patches the signal _outcomes, an object
// from op id to {ok, msg, field}, from the tab's row. Its key holds the
// tab id and it closes over the session: never show a tab id to anyone
// else, who could open a stream with it.
func View(sid, tab string) live.View {
	key := Topic(tab)
	return live.View{
		Key:    key,
		Topics: []string{key},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			var q sqlite.Querier = tx
			if wq, ok := sqlite.QuerierFrom(ctx); ok {
				q = wq
			}
			s, err := Get(ctx, q, sid, tab)
			if err != nil {
				return nil, err
			}
			outcomes := make(map[string]signal, len(s.Outcomes))
			for _, o := range s.Outcomes {
				outcomes[o.Op] = signal{OK: o.OK, Msg: o.Msg, Field: o.Field}
			}
			b, err := json.Marshal(map[string]any{"_outcomes": outcomes})
			if err != nil {
				return nil, err
			}
			return sse.PatchSignals(b, false), nil
		},
	}
}

// Sweep deletes the rows of tabs not seen for TTL and the preferences not
// changed for PrefsTTL. Run it every hour. Nothing shows them: it touches
// nothing.
type Sweep struct{ At time.Time }

func (c Sweep) Apply(tx *writer.Tx) error {
	if _, err := tx.Exec(`DELETE FROM tab_state WHERE seen < ?`, c.At.Add(-TTL).Unix()); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM prefs WHERE seen < ?`, c.At.Add(-PrefsTTL).Unix())
	return err
}
