// Package activity reads feeds of recent changes from the event log
// (FR-69): issues created, moved to another status and commented on,
// pages created, edited and commented on, each with its actor, subject
// and time, newest first. Nothing is stored a second time: a feed reads
// the end of lith_events with events.SinceUntil, a window at a time
// going back, decodes the events of the types it shows with the domain's
// streams (events.Decode) and keeps those whose subject the person may
// read.
//
// Home, Project and Space are the feeds of the home page, a project and a
// space. A live view of a feed lists events.Topic (every append) and
// people.Topic (names), and is shared by everyone who reads the same
// scopes (Readable, Key).
//
// Each stream kind has a source that turns its events into entries:
// issues and comments here (tracker.go); the wiki adds its page stream,
// and the "page" subject of comments, with one file (wiki.go) whose init
// calls addSource and sets commentSubjects["page"].
package activity

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/people"
)

// Action is what an entry says happened.
type Action string

const (
	IssueCreated   Action = "issue_created"
	IssueMoved     Action = "issue_moved" // to another status
	IssueCommented Action = "issue_commented"
	PageCreated    Action = "page_created"
	PageEdited     Action = "page_edited"
	PageCommented  Action = "page_commented"
)

// Subject is what an entry is about.
type Subject struct {
	Kind    string // "issue" or "page"
	ID      string
	Label   string // the issue key or the page title, as it is now
	Summary string // an issue's summary
	Type    string // an issue's type: "story", "bug"
	Href    string
	Scope   access.Scope // the project or space it belongs to
}

// Entry is one line of a feed.
type Entry struct {
	Position int64 // of its newest event
	At       time.Time
	Actor    string // a person id
	Action   Action
	Subject  Subject
	Status   string // for a move: the new status's name
	Category string // and its category: "todo", "inprogress", "done"
	Text     string // a comment's start
	// Edits counts the edits of a page one person made within EditWindow
	// of each other, shown as one entry (FR-69.3); Since is the oldest.
	Edits int
	Since time.Time
}

// PageSize is how many entries a feed shows at a time (FR-69.1).
const PageSize = 50

// EditWindow is how close consecutive edits of one page by one person
// are to become one entry (FR-69.3).
const EditWindow = 10 * time.Minute

// MaxScan is how many events a feed reads back at most: a quiet scope in
// a busy log shows what the newest MaxScan events hold.
const MaxScan = 20000

// window is how many positions a feed reads at a time, at first.
const window = 512

// source turns a window's events of one stream kind, of the types it
// names, into entries; an entry with an empty Subject.ID is dropped (a
// deleted issue).
type source struct {
	types   map[string]bool
	list    string // the types as a JSON list, for SQL
	entries func(ctx context.Context, q sqlite.Querier, evs []events.Raw) ([]Entry, error)
}

var sources = map[string]source{}

// addSource registers the source of a stream kind.
func addSource(kind string, types []string, entries func(ctx context.Context, q sqlite.Querier, evs []events.Raw) ([]Entry, error)) {
	s := source{types: map[string]bool{}, entries: entries}
	for _, t := range types {
		s.types[t] = true
	}
	list, _ := json.Marshal(types)
	s.list = string(list)
	sources[kind] = s
}

// commentSubjects name the subjects of comments by their ids, by the
// comment's subject kind: issues here, pages from wiki.go.
var commentSubjects = map[string]func(ctx context.Context, q sqlite.Querier, ids []string) (map[string]Subject, error){}

// Feed returns up to limit entries whose scope keep accepts, newest
// first, and whether there are more.
func Feed(ctx context.Context, q sqlite.Querier, keep func(access.Scope) bool, limit int) ([]Entry, bool, error) {
	upto, err := head(ctx, q)
	if err != nil {
		return nil, false, err
	}
	var out []Entry
	size, scanned := int64(window), int64(0)
	for upto > 0 && len(out) <= limit && scanned < MaxScan {
		after := max(0, upto-size)
		byKind, err := sourceEvents(ctx, q, after, upto)
		if err != nil {
			return nil, false, err
		}
		var found []Entry
		for kind, evs := range byKind {
			es, err := sources[kind].entries(ctx, q, evs)
			if err != nil {
				return nil, false, err
			}
			found = append(found, es...)
		}
		slices.SortFunc(found, func(a, b Entry) int { return cmp.Compare(b.Position, a.Position) })
		for _, e := range found {
			if e.Subject.ID == "" || !keep(e.Subject.Scope) {
				continue
			}
			if e.Edits == 0 {
				e.Edits = 1
			}
			if e.Since.IsZero() {
				e.Since = e.At
			}
			if n := len(out); n > 0 && merges(out[n-1], e) {
				out[n-1].Edits += e.Edits
				out[n-1].Since = e.Since
				continue
			}
			out = append(out, e)
		}
		scanned += upto - after
		upto = after
		size = min(size*2, 8192)
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}

// sourceEvents are the events after and up to upto of the kinds and types
// the sources name, by kind. Each kind is read by the log's index of its
// positions, and only its rows of those types are decoded: the log of a
// page that people co-edit is mostly commits, which no feed shows.
func sourceEvents(ctx context.Context, q sqlite.Querier, after, upto int64) (map[string][]events.Raw, error) {
	byKind := map[string][]events.Raw{}
	for kind, s := range sources {
		rows, err := q.QueryContext(ctx, `SELECT position, version, type, data, at, coalesce(actor, ''), coalesce(correlation, ''), coalesce(causation, ''), stream_id
			FROM lith_events WHERE stream = ? AND position > ? AND position <= ? AND type IN (SELECT value FROM json_each(?)) ORDER BY position`,
			kind, after, upto, s.list)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			ev := events.Raw{Stream: kind}
			var at int64
			var data string
			if err := rows.Scan(&ev.Position, &ev.Version, &ev.Type, &data, &at, &ev.Actor, &ev.Correlation, &ev.Causation, &ev.ID); err != nil {
				rows.Close()
				return nil, err
			}
			ev.Data, ev.At = json.RawMessage(data), time.UnixMilli(at)
			byKind[kind] = append(byKind[kind], ev)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return byKind, nil
}

// merges reports whether an older entry joins a newer one (FR-69.3).
func merges(newer, older Entry) bool {
	return newer.Action == PageEdited && older.Action == PageEdited && newer.Subject.ID == older.Subject.ID &&
		newer.Actor == older.Actor && newer.Since.Sub(older.At) <= EditWindow
}

// head is the log's last position, 0 before the first event.
func head(ctx context.Context, q sqlite.Querier) (int64, error) {
	var pos int64
	err := q.QueryRowContext(ctx, `SELECT coalesce(max(position), 0) FROM lith_events`).Scan(&pos)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return 0, nil
	}
	return pos, err
}

// Readable are the projects and spaces a person may read, sorted.
func Readable(ctx context.Context, q sqlite.Querier, actor people.Person) ([]access.Scope, error) {
	rows, err := q.QueryContext(ctx, `SELECT kind, id FROM scopes ORDER BY kind, id`)
	if err != nil {
		return nil, err
	}
	var all []access.Scope
	for rows.Next() {
		var s access.Scope
		if err := rows.Scan(&s.Kind, &s.ID); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []access.Scope
	for _, s := range all {
		ok, err := access.Can(ctx, q, actor, access.Read, s)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// Key is a short stable name of a set of scopes: the key of a home feed's
// view, shared by everyone who reads the same projects and spaces.
func Key(scopes []access.Scope) string {
	h := sha256.New()
	for _, s := range scopes {
		h.Write([]byte(string(s.Kind) + ":" + s.ID + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// In accepts the scopes of a list.
func In(scopes []access.Scope) func(access.Scope) bool {
	set := make(map[access.Scope]bool, len(scopes))
	for _, s := range scopes {
		set[s] = true
	}
	return func(s access.Scope) bool { return set[s] }
}

// Home is the home page's feed: what happened in the scopes the person
// may read (FR-59, FR-69.2).
func Home(ctx context.Context, q sqlite.Querier, readable []access.Scope, limit int) ([]Entry, bool, error) {
	return Feed(ctx, q, In(readable), limit)
}

// Project is a project's feed; the page checked that the person may read
// the project.
func Project(ctx context.Context, q sqlite.Querier, project string, limit int) ([]Entry, bool, error) {
	return Feed(ctx, q, In([]access.Scope{access.ProjectScope(project)}), limit)
}

// Space is a space's feed; the page checked that the person may read the
// space.
func Space(ctx context.Context, q sqlite.Querier, space string, limit int) ([]Entry, bool, error) {
	return Feed(ctx, q, In([]access.Scope{access.SpaceScope(space)}), limit)
}

// Mine are the subjects a person changed lately, newest first, each once:
// "recently edited by you" on the home page (FR-59.1), from the same end
// of the log, of every event type the sources know of or not.
func Mine(ctx context.Context, q sqlite.Querier, person string, keep func(access.Scope) bool, limit int) ([]Entry, error) {
	// A person without a row never acted (tabs.Act makes it): most guests
	// on the home page, which renders this on every commit.
	var acted int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM people WHERE id = ?`, person).Scan(&acted); err != nil || acted == 0 {
		return nil, err
	}
	upto, err := head(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []Entry
	seen := map[string]bool{}
	size, scanned := int64(window), int64(0)
	for upto > 0 && len(out) < limit && scanned < MaxScan {
		after := max(0, upto-size)
		var issues []string
		at := map[string]Entry{}
		for ev, err := range byActor(ctx, q, person, after, upto) {
			if err != nil {
				return nil, err
			}
			id, kind := subjectOf(ev)
			if id == "" || seen[id] {
				continue
			}
			if _, ok := at[id]; !ok {
				issues = append(issues, id)
			}
			at[id] = Entry{Position: ev.Position, At: ev.Meta.At, Actor: person, Subject: Subject{Kind: kind, ID: id}}
		}
		subjects, err := mineSubjects(ctx, q, issues)
		if err != nil {
			return nil, err
		}
		found := make([]Entry, 0, len(at))
		for id, e := range at {
			s, ok := subjects[id]
			if !ok || !keep(s.Scope) {
				continue
			}
			e.Subject = s
			found = append(found, e)
		}
		slices.SortFunc(found, func(a, b Entry) int { return cmp.Compare(b.Position, a.Position) })
		for _, e := range found {
			if len(out) < limit {
				out = append(out, e)
				seen[e.Subject.ID] = true
			}
		}
		for id := range at {
			seen[id] = true
		}
		scanned += upto - after
		upto = after
		size = min(size*2, 8192)
	}
	return out, nil
}

// byActor are the events of one actor in (after, upto], as
// events.SinceUntil reads them. The filter runs in SQLite, so the rows of
// other actors never reach Go: the home page renders it for every viewer
// on every commit, and a window of a busy log holds thousands of rows.
func byActor(ctx context.Context, q sqlite.Querier, actor string, after, upto int64) iter.Seq2[events.Raw, error] {
	return func(yield func(events.Raw, error) bool) {
		rows, err := q.QueryContext(ctx, `SELECT position, version, type, data, at, coalesce(correlation, ''), coalesce(causation, ''), stream, stream_id
			FROM lith_events WHERE position > ? AND position <= ? AND actor = ? ORDER BY position`, after, upto, actor)
		if err != nil {
			yield(events.Raw{}, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			ev := events.Raw{Meta: events.Meta{Actor: actor}}
			var at int64
			var data string
			if err := rows.Scan(&ev.Position, &ev.Version, &ev.Type, &data, &at, &ev.Correlation, &ev.Causation, &ev.Stream, &ev.ID); err != nil {
				yield(events.Raw{}, err)
				return
			}
			ev.Data, ev.At = json.RawMessage(data), time.UnixMilli(at)
			if !yield(ev, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(events.Raw{}, err)
		}
	}
}

// subjectOfs name the subject of an event of a stream kind, by kind:
// issues and comments here.
var subjectOfs = map[string]func(ev events.Raw) (id, kind string){}

func subjectOf(ev events.Raw) (string, string) {
	if f, ok := subjectOfs[ev.Stream]; ok {
		return f(ev)
	}
	return "", ""
}

// mineSubjectsBy name subjects by their kind, for Mine.
var mineSubjectsBy = map[string]func(ctx context.Context, q sqlite.Querier, ids []string) (map[string]Subject, error){}

func mineSubjects(ctx context.Context, q sqlite.Querier, ids []string) (map[string]Subject, error) {
	out := map[string]Subject{}
	for _, f := range mineSubjectsBy {
		m, err := f(ctx, q, ids)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			out[k] = v
		}
	}
	return out, nil
}
