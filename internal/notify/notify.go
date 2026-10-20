// Package notify tells people what concerns them (FR-110 to FR-117): one
// row of notifications per recipient, made only by reactions to events.
//
// # Reactions
//
// Catch-up projectors (events.After) watch the issue and comment streams:
// an assignment to someone else (FR-111), a new mention in a description
// or a comment (FR-112, once: the tracker's events carry the mentions a
// text adds), a comment on or a status change of a watched issue, to its
// watchers but the actor (FR-113), several of those by one person on one
// issue within a minute merged into one notification (FR-113.2). Each
// sends one command with writer.Once keyed by the event's position, and
// the table's UNIQUE (recipient, source, kind) and the merge's last
// source make a replay create nothing twice. The projectors skip history
// (SkipHistory): a new or changed projector, a restart, a deploy notify
// nobody of the past (FR-116). Only guests and accounts are told: the
// seeded people never read their notifications.
//
// Wiki reactions plug in with one file of this package (wiki.go): its
// init adds the page stream's projector to extra (Parts returns it) and
// registers the "page" subject of comments in commentables, with a
// function that names a page and lists its watchers as of a position;
// page comments then notify the page's watchers (FR-114) and mentions in
// them notify as issue comments do.
//
// # Reads and commands
//
// Unread, List and HasUnread read a person's notifications. Read,
// ReadAll, Dismiss, DismissAll, Open and Seen are the person's actions
// (FR-115), each run in tabs.Act; one on someone else's notification is
// refused with an *access.Denial (403). Sweep keeps the table bounded
// (FR-117). Every change touches people.PersonTopic of the recipient: the
// person view renders the bell.
package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"
)

// Kind is what a notification is about.
type Kind string

const (
	Assigned      Kind = "assigned"       // FR-111
	Mentioned     Kind = "mentioned"      // FR-112
	Commented     Kind = "commented"      // FR-113: a comment on a watched issue
	StatusChanged Kind = "status_changed" // FR-113: a watched issue's status
	PageCommented Kind = "page_commented" // FR-114: a comment on a watched page
)

// watching are the kinds a watcher gets, which merge (FR-113.2).
var watching = []Kind{Commented, StatusChanged, PageCommented}

// Subject kinds.
const (
	SubjectIssue = "issue"
	SubjectPage  = "page"
)

// Subject is what a notification links to.
type Subject struct {
	Kind  string // SubjectIssue or SubjectPage
	ID    string
	Label string // the issue key or the page title, as it was then
	Href  string
}

// IssueHref is an issue's address: /WEB-12 (FR-12).
func IssueHref(key string) string { return "/" + key }

// Detail is what several changes merged into one notification hold.
type Detail struct {
	Status   string `json:"status,omitempty"`   // the newest status's name
	Comments int    `json:"comments,omitempty"` // how many comments
}

// Notification is a notification as the panel shows it.
type Notification struct {
	ID        int64
	Recipient string
	Kind      Kind
	Subject   Subject
	Actor     string // a person id
	Text      string // the sentence around a mention, a comment's start
	Detail    Detail
	Created   time.Time
	Updated   time.Time // the newest change merged into it
	Read      bool
}

// Phrase is what the actor did, around the subject's label: before is
// "assigned", after "to you", for "Sunny Otter assigned WEB-12 to you".
func (n Notification) Phrase() (before, after string) {
	switch n.Kind {
	case Assigned:
		return "assigned", "to you"
	case Mentioned:
		return "mentioned you in", ""
	}
	comments := ""
	switch c := n.Detail.Comments; {
	case c == 1:
		comments = "commented"
	case c > 1:
		comments = "left " + strconv.Itoa(c) + " comments"
	}
	switch {
	case n.Detail.Status != "" && comments != "":
		return "moved", "to " + n.Detail.Status + " and " + comments
	case n.Detail.Status != "":
		return "moved", "to " + n.Detail.Status
	case n.Detail.Comments > 1:
		return comments + " on", ""
	}
	return "commented on", ""
}

// Sentence is the notification as one line of text, with the actor's
// name: "Sunny Otter assigned WEB-12 to you".
func (n Notification) Sentence(actor string) string {
	before, after := n.Phrase()
	s := actor + " " + before + " " + n.Subject.Label
	if after != "" {
		s += " " + after
	}
	return s
}

// Panel is how many of a person's notifications are unread, the newest
// of them, read or not, and how many older ones the panel leaves out.
type Panel struct {
	Unread int
	Items  []Notification
	More   int
}

// PanelSize is how many notifications the panel lists.
const PanelSize = 30

// Keep is how many notifications a person keeps at most (FR-117).
const Keep = 500

// ReadTTL is how long a read notification stays (FR-117).
const ReadTTL = 30 * 24 * time.Hour

// Unread counts a person's unread notifications (FR-110.1).
func Unread(ctx context.Context, q sqlite.Querier, person string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM notifications WHERE recipient = ? AND read_at IS NULL AND dismissed_at IS NULL`, person).Scan(&n)
	return n, err
}

// HasUnread reports whether a person has unread notifications about a
// subject: a page that opens the subject then posts /cmd/notify/seen
// (FR-115.2).
func HasUnread(ctx context.Context, q sqlite.Querier, person, subject string) (bool, error) {
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM notifications WHERE recipient = ? AND subject_id = ? AND read_at IS NULL AND dismissed_at IS NULL LIMIT 1`, person, subject).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

const columns = `id, recipient, kind, subject_kind, subject_id, subject_label, href, actor, text, detail, created_at, updated_at, read_at IS NOT NULL`

type scanner interface{ Scan(...any) error }

func scan(r scanner) (Notification, error) {
	var n Notification
	var detail string
	var created, updated int64
	err := r.Scan(&n.ID, &n.Recipient, &n.Kind, &n.Subject.Kind, &n.Subject.ID, &n.Subject.Label, &n.Subject.Href, &n.Actor, &n.Text, &detail, &created, &updated, &n.Read)
	if err != nil {
		return n, err
	}
	if err := json.Unmarshal([]byte(detail), &n.Detail); err != nil {
		return n, fmt.Errorf("notification %d: %w", n.ID, err)
	}
	n.Created, n.Updated = time.UnixMilli(created), time.UnixMilli(updated)
	return n, nil
}

// List returns a person's newest notifications that are not dismissed,
// newest first (FR-110.1).
func List(ctx context.Context, q sqlite.Querier, person string, limit int) ([]Notification, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+columns+` FROM notifications WHERE recipient = ? AND dismissed_at IS NULL
		ORDER BY updated_at DESC, id DESC LIMIT ?`, person, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		n, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// PanelOf is what a person's bell shows.
func PanelOf(ctx context.Context, q sqlite.Querier, person string) (Panel, error) {
	var p Panel
	var all int
	err := q.QueryRowContext(ctx, `SELECT count(*), coalesce(sum(read_at IS NULL), 0) FROM notifications WHERE recipient = ? AND dismissed_at IS NULL`, person).Scan(&all, &p.Unread)
	if err != nil {
		return p, err
	}
	if all == 0 {
		return p, nil
	}
	if p.Items, err = List(ctx, q, person, PanelSize); err != nil {
		return p, err
	}
	p.More = all - len(p.Items)
	return p, nil
}

// Get reads one notification, for tests and tools.
func Get(ctx context.Context, q sqlite.Querier, id int64) (Notification, error) {
	return scan(q.QueryRowContext(ctx, `SELECT `+columns+` FROM notifications WHERE id = ?`, id))
}

// notifiable reports whether a person is told anything: guests and
// accounts, who read their notifications, not the seeded people.
func notifiable(person string) bool {
	return strings.HasPrefix(person, "g-") || strings.HasPrefix(person, "a-")
}

var bound atomic.Pointer[writer.Writer]

// Bind gives the reactions the writer they send their commands to:
// cmd/tickit calls it once the writer exists, tests with theirs. Until
// then a reaction fails, is retried, and its projector parks.
func Bind(w *writer.Writer) { bound.Store(w) }

// OnceKey is the op id of the reaction to the event at position: a retry
// after a failure, or a replay, runs it once (writer.Once).
func OnceKey(position int64) string { return "notify:" + strconv.FormatInt(position, 10) }

// deliver sends a reaction's command once per event.
func deliver(ctx context.Context, position int64, cmd writer.Command) error {
	w := bound.Load()
	if w == nil {
		return errors.New("notify: no writer bound (notify.Bind)")
	}
	return w.Do(ctx, writer.Once(OnceKey(position), cmd))
}

// extra are the projectors other files of the package add (wiki.go).
var extra []events.Part

// Parts are notify's catch-up projectors, for the app's one event store.
func Parts() []events.Part {
	return append([]events.Part{issueReactions, commentReactions}, extra...)
}
