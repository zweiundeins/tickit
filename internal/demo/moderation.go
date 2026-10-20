package demo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// Moderation (DM-07): moderators and site admins hide, restore or delete
// what visitors wrote, and block a guest for BlockFor. Each kind of thing
// registers how (RegisterModerated): issues and comments here, pages,
// page versions and images from the wiki. Every action is stored in the
// moderation log and the security log (DM-07.4, FR-156) with moderator,
// target, reason and time.

// ModerationTopic is touched by every moderation action: the moderation
// page listens, with the tracker's topics for what visitors write.
const ModerationTopic = "moderation"

// BlockFor is how long a block lasts (DM-07.2).
const BlockFor = 24 * time.Hour

// Actions of the moderation log.
const (
	ActHide    = "hide"
	ActRestore = "restore"
	ActDelete  = "delete"
	ActBlock   = "block"
	ActUnblock = "unblock"
)

// Target is something a visitor wrote, as the moderation page lists it.
type Target struct {
	Kind    string // a Moderated's Kind
	ID      string
	Label   string // "WEB-12", a page's title
	Text    string // the summary, the start of a comment
	Href    string
	Author  string // a person id
	At      time.Time
	Hidden  bool
	Scope   access.Scope // where it is, for the reader's right
	Context string       // where it is, in words: "on WEB-12"
}

// Moderated is a kind of thing moderators act on.
type Moderated struct {
	Kind string // "issue", "comment", "page", "version", "image"
	Name string // "Issue", as the page says it
	// Hide hides the thing with id, or shows it again when hidden is
	// false. The command checks the moderator's right itself.
	Hide func(by tabs.By, id string, hidden bool, reason string) writer.Command
	// Delete deletes it; nil when this kind is hidden only.
	Delete func(by tabs.By, id string) writer.Command
	// Label names it for the log, before the change.
	Label func(tx *writer.Tx, id string) (string, error)
	// Newest lists the newest of this kind that visitors (guests and
	// accounts, not the seed) wrote, hidden ones too, newest first.
	Newest func(ctx context.Context, q sqlite.Querier, limit int) ([]Target, error)
}

var (
	moderatedMu sync.Mutex
	moderated   []Moderated
)

// RegisterModerated adds a kind of thing to moderate: the wiki registers
// pages, page versions and images from an init function.
func RegisterModerated(m Moderated) {
	if m.Kind == "" || m.Hide == nil || m.Label == nil || m.Newest == nil {
		panic("demo: a moderated kind needs a kind, Hide, Label and Newest")
	}
	moderatedMu.Lock()
	defer moderatedMu.Unlock()
	if slices.ContainsFunc(moderated, func(o Moderated) bool { return o.Kind == m.Kind }) {
		panic("demo: moderated kind " + m.Kind + " registered twice")
	}
	moderated = append(moderated, m)
}

// ModeratedKinds are the kinds registered, in order.
func ModeratedKinds() []Moderated {
	moderatedMu.Lock()
	defer moderatedMu.Unlock()
	return slices.Clone(moderated)
}

func moderatedKind(kind string) (Moderated, bool) {
	for _, m := range ModeratedKinds() {
		if m.Kind == kind {
			return m, true
		}
	}
	return Moderated{}, false
}

func init() {
	RegisterModerated(Moderated{
		Kind: "issue", Name: "Issue",
		Hide: func(by tabs.By, id string, hidden bool, reason string) writer.Command {
			return tracker.HideIssue{By: by, Issue: id, Hidden: hidden, Reason: reason}
		},
		Delete: func(by tabs.By, id string) writer.Command { return tracker.DeleteIssue{By: by, Issue: id} },
		Label: func(tx *writer.Tx, id string) (string, error) {
			var key, summary string
			err := tx.QueryRow(`SELECT key, summary FROM issues WHERE id = ?`, id).Scan(&key, &summary)
			if errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("issue %s: %w", id, access.ErrNotFound)
			}
			return key + " " + summary, err
		},
		Newest: newestIssues,
	})
	RegisterModerated(Moderated{
		Kind: "comment", Name: "Comment",
		Hide: func(by tabs.By, id string, hidden bool, reason string) writer.Command {
			return tracker.HideComment{By: by, Comment: id, Hidden: hidden, Reason: reason}
		},
		Delete: func(by tabs.By, id string) writer.Command { return tracker.DeleteComment{By: by, Comment: id} },
		Label: func(tx *writer.Tx, id string) (string, error) {
			var body sql.NullString
			var key string
			err := tx.QueryRow(`SELECT c.body, coalesce(i.key, '') FROM comments c LEFT JOIN issues i ON c.subject = 'issue' AND i.id = c.subject_id WHERE c.id = ?`, id).Scan(&body, &key)
			if errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("comment %s: %w", id, access.ErrNotFound)
			}
			label := excerpt(body.String, 60)
			if key != "" {
				label = key + ": " + label
			}
			return label, err
		},
		Newest: newestComments,
	})
}

// visitor is the SQL condition of an author who is no seeded person.
const visitor = ` NOT LIKE 's-%'`

func newestIssues(ctx context.Context, q sqlite.Querier, limit int) ([]Target, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, key, summary, reporter, created_at, hidden_at IS NOT NULL, project_id FROM issues
		WHERE reporter`+visitor+` AND (deleted_at IS NULL OR hidden_at IS NOT NULL) ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		t := Target{Kind: "issue"}
		var at int64
		var project string
		if err := rows.Scan(&t.ID, &t.Label, &t.Text, &t.Author, &at, &t.Hidden, &project); err != nil {
			return nil, err
		}
		t.At, t.Href, t.Scope = time.UnixMilli(at), "/"+t.Label, access.ProjectScope(project)
		out = append(out, t)
	}
	return out, rows.Err()
}

func newestComments(ctx context.Context, q sqlite.Querier, limit int) ([]Target, error) {
	rows, err := q.QueryContext(ctx, `SELECT c.id, coalesce(c.body, ''), c.author, c.created_at, c.hidden_at IS NOT NULL, c.project_id, coalesce(i.key, '')
		FROM comments c LEFT JOIN issues i ON i.id = c.subject_id
		WHERE c.subject = 'issue' AND c.author`+visitor+` AND c.deleted_at IS NULL ORDER BY c.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		t := Target{Kind: "comment"}
		var at int64
		var body, project, key string
		if err := rows.Scan(&t.ID, &body, &t.Author, &at, &t.Hidden, &project, &key); err != nil {
			return nil, err
		}
		t.At, t.Text, t.Label, t.Scope = time.UnixMilli(at), excerpt(body, 140), key, access.ProjectScope(project)
		t.Href, t.Context = "/"+key, "on "+key
		out = append(out, t)
	}
	return out, rows.Err()
}

// excerpt is the start of a document's text.
func excerpt(doc string, n int) string {
	s := notify.Excerpt(json.RawMessage(doc))
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n])) + "…"
}

// Moderate is one moderation action on a thing: hide, restore or delete,
// by a moderator or a site admin (access.Moderate), with an optional
// reason of at most 200 characters.
type Moderate struct {
	By     tabs.By
	Action string // ActHide, ActRestore or ActDelete
	Kind   string
	ID     string
	Reason string
}

func (c Moderate) Validate() error {
	if c.By.Actor.IsZero() {
		return errors.New("demo: moderation needs a moderator")
	}
	if c.Action != ActHide && c.Action != ActRestore && c.Action != ActDelete {
		return fmt.Errorf("demo: no moderation action %q", c.Action)
	}
	if _, ok := moderatedKind(c.Kind); !ok || c.ID == "" {
		return fmt.Errorf("demo: nothing to moderate of kind %q", c.Kind)
	}
	return nil
}

func (c Moderate) Apply(tx *writer.Tx) error {
	if err := access.Require(tx, c.By.Actor, access.Moderate, access.Scope{}); err != nil {
		return err
	}
	reason := strings.TrimSpace(c.Reason)
	if utf8.RuneCountInString(reason) > 200 {
		return tabs.Refuse("reason", "A reason has at most 200 characters.")
	}
	m, _ := moderatedKind(c.Kind)
	label, err := m.Label(tx, c.ID)
	if err != nil {
		return err
	}
	var cmd writer.Command
	switch c.Action {
	case ActHide, ActRestore:
		cmd = m.Hide(c.By, c.ID, c.Action == ActHide, reason)
	case ActDelete:
		if m.Delete == nil {
			return &writer.Refusal{Msg: m.Name + "s are hidden, not deleted."}
		}
		cmd = m.Delete(c.By, c.ID)
	}
	if err := cmd.Apply(tx); err != nil {
		return err
	}
	return logModeration(tx, c.By, c.Action, c.Kind, c.ID, label, reason)
}

func logModeration(tx *writer.Tx, by tabs.By, action, kind, id, label, reason string) error {
	at := by.At
	if at.IsZero() {
		at = time.Now()
	}
	if _, err := tx.Exec(`INSERT INTO moderation_log (at, moderator, action, target_kind, target_id, target_label, reason) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		at.Unix(), by.Actor.ID, action, kind, id, clip(label, 200), reason); err != nil {
		return err
	}
	tx.Touch(ModerationTopic)
	return people.LogSecurity{Kind: people.EventModeration, Outcome: people.OutcomeOK, Person: by.Actor.ID, Actor: by.Actor.ID,
		Detail: action + " " + kind + " " + id, At: at}.Apply(tx)
}

// Block pauses a guest's changes for BlockFor, and those from the client
// address they last acted from (DM-07.2); Unblock lifts it. A moderator's
// or a site admin's action.
func (d *Demo) Block(by tabs.By, person, reason string) writer.Command {
	return blockCmd{d: d, by: by, person: person, reason: reason, block: true}
}

// Unblock lifts a guest's block.
func (d *Demo) Unblock(by tabs.By, person string) writer.Command {
	return blockCmd{d: d, by: by, person: person}
}

type blockCmd struct {
	d      *Demo
	by     tabs.By
	person string
	reason string
	block  bool
}

func (c blockCmd) Validate() error {
	if c.by.Actor.IsZero() || !people.ValidID(c.person) {
		return errors.New("demo: a block names a person")
	}
	return nil
}

func (c blockCmd) Apply(tx *writer.Tx) error {
	if err := access.Require(tx, c.by.Actor, access.Moderate, access.Scope{}); err != nil {
		return err
	}
	if !strings.HasPrefix(c.person, "g-") {
		return &writer.Refusal{Msg: "Only guests are blocked."}
	}
	reason := strings.TrimSpace(c.reason)
	if utf8.RuneCountInString(reason) > 200 {
		return tabs.Refuse("reason", "A reason has at most 200 characters.")
	}
	at := c.by.At
	if at.IsZero() {
		at = time.Now()
	}
	p, err := people.Get(tx.Context(), tx, c.person)
	if err != nil {
		return err
	}
	action := ActUnblock
	until := time.Time{}
	if c.block {
		action, until = ActBlock, at.Add(BlockFor)
		_, err = tx.Exec(`INSERT INTO blocks (person_id, until, by, reason, at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (person_id) DO UPDATE SET until = excluded.until, by = excluded.by, reason = excluded.reason, at = excluded.at`,
			c.person, until.Unix(), c.by.Actor.ID, reason, at.Unix())
	} else {
		_, err = tx.Exec(`DELETE FROM blocks WHERE person_id = ?`, c.person)
	}
	if err != nil {
		return err
	}
	if c.d != nil {
		tx.AfterCommit(func() { c.d.addrs.block(c.person, until) })
	}
	return logModeration(tx, c.by, action, "guest", c.person, p.Name, reason)
}

// Blocked is a guest whose changes are paused.
type Blocked struct {
	Person, By, Reason string
	At, Until          time.Time
}

// Blocks are the blocks in force at now, newest first.
func Blocks(ctx context.Context, q sqlite.Querier, now time.Time) ([]Blocked, error) {
	rows, err := q.QueryContext(ctx, `SELECT person_id, by, reason, at, until FROM blocks WHERE until > ? ORDER BY at DESC`, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Blocked
	for rows.Next() {
		var b Blocked
		var at, until int64
		if err := rows.Scan(&b.Person, &b.By, &b.Reason, &at, &until); err != nil {
			return nil, err
		}
		b.At, b.Until = time.Unix(at, 0), time.Unix(until, 0)
		out = append(out, b)
	}
	return out, rows.Err()
}

// LogEntry is one entry of the moderation log.
type LogEntry struct {
	At                              time.Time
	Moderator, Action, Kind, Target string
	Label, Reason                   string
}

// ModerationLog is the newest entries of the moderation log.
func ModerationLog(ctx context.Context, q sqlite.Querier, limit int) ([]LogEntry, error) {
	rows, err := q.QueryContext(ctx, `SELECT at, moderator, action, target_kind, target_id, target_label, reason FROM moderation_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var e LogEntry
		var at int64
		if err := rows.Scan(&at, &e.Moderator, &e.Action, &e.Kind, &e.Target, &e.Label, &e.Reason); err != nil {
			return nil, err
		}
		e.At = time.Unix(at, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Newest lists what visitors wrote lately, of every kind registered,
// newest first: the moderation page (DM-07.3).
func Newest(ctx context.Context, q sqlite.Querier, limit int) ([]Target, error) {
	var all []Target
	for _, m := range ModeratedKinds() {
		ts, err := m.Newest(ctx, q, limit)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Kind, err)
		}
		all = append(all, ts...)
	}
	slices.SortStableFunc(all, func(a, b Target) int { return b.At.Compare(a.At) })
	return all[:min(len(all), limit)], nil
}
