// Package tracker is the Jira side of tickit: projects with their
// workflows, components and sprints, issues with their fields, links,
// labels, watchers and ranks, and the comments on issues. Every change is
// a command on an event stream (lith/events) whose decider decides from
// the stream's state; inline projectors keep the tables that pages read.
//
// # Streams
//
// Three kinds, keyed by UUIDv7 strings: "project", "issue" and
// "comment". A command that creates an issue runs on the project's stream
// (the next number) and the issue's in one savepoint; a link appends to
// both issues; a comment appends to its own stream and to its issue's
// (new watchers).
//
// # Commands
//
// Every command carries tabs.By (actor, op id, time), checks its right
// with access.Require inside Apply and stores events with By.Meta(). A
// change to an issue's field carries Seen, the issue's version the page
// showed; a field someone else changed after Seen refuses the change
// with a *Conflict (MP-08), and a board move whose card moved after Seen
// with a *Conflict of the field "move" (MP-04.3). A change to the value
// a field holds stores nothing (FR-14.3).
//
// The seed runs the same commands with a seeded person (people.Seed) as
// the actor and a past By.At: the access checks let seeded people act,
// since no request ever acts as one.
//
// # Reads
//
// Queries take a sqlite.Querier (a GET's transaction, a render's, a
// command's) and read the projections: ProjectByKey, ProjectByID,
// Projects, Readable, Statuses, Components, Sprints, IssueByKey,
// IssueDetail, Watchers, EpicChildren, Epics, Board, Backlog, Search,
// Comments, Labels, Assignable, SavedFilters, RefByKey, Typing,
// IssuesByID, SearchAll and ListOptions (the list's filter values);
// History, Activity and StateAt read an issue's own stream,
// ProjectWorkflow a project's. SetTyping records who types in an issue's
// field (MP-07): no event, a row of issue_typing. The search index's
// projectors of issues and comments are among Parts (search_index.go).
package tracker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Topics, besides access's project:<id> (ProjectTopic) and people's
// person:<id>.
const (
	// IssuesTopic is touched by every change to any issue or comment:
	// lists, search results, chips and the projects' open counts listen.
	IssuesTopic = "issues"
	// ProjectsTopic is touched by every change to any project: the list
	// of projects and the pickers listen.
	ProjectsTopic = "projects"
	// FiltersTopic is touched by every change to the saved filters.
	FiltersTopic = "filters"
)

// IssueTopic is touched by every change an issue's page shows: its
// fields, comments, children, links and watchers.
func IssueTopic(id string) string { return "issue:" + id }

// ProjectTopic is touched by every change to a project's settings.
func ProjectTopic(id string) string { return access.ProjectScope(id).Topic() }

// BoardTopic is touched by every change to a project's board or backlog:
// its issues, ranks, columns and sprints.
func BoardTopic(projectID string) string { return "board:" + projectID }

var (
	bound atomic.Pointer[events.Store]
	own   func() *events.Store // set in init: the description's Kind calls Store
)

func init() {
	own = sync.OnceValue(func() *events.Store { return events.Must(events.New(Parts()...)) })
}

// Parts are the tracker's projectors, for the app's one event store.
func Parts() []events.Part {
	return []events.Part{projectsProjector, issuesProjector, commentsProjector, issueSearchProjector, commentSearchProjector, descProjector}
}

// Bind gives the tracker's commands the app's event store, which holds
// every package's parts: cmd/tickit calls it once the store is made.
// Without it, as in tests, they use a store of the tracker's parts alone.
func Bind(st *events.Store) { bound.Store(st) }

// Store is the event store the commands append with.
func Store() *events.Store {
	if st := bound.Load(); st != nil {
		return st
	}
	return own()
}

// NewID returns a new id for a project, an issue, a comment, a sprint or
// a saved filter: a UUIDv7.
func NewID() string { return uuid.Must(uuid.NewV7()).String() }

// ValidID reports whether s is an id NewID could have made.
func ValidID(s string) bool {
	u, err := uuid.Parse(s)
	return err == nil && u.String() == s
}

// notFound is the error of a command or query that names something that
// does not exist: it answers 404.
func notFound(what, id string) error { return fmt.Errorf("%s %s: %w", what, id, access.ErrNotFound) }

// require checks the actor's right in a project. Seeded people act only
// in the seed, which writes the demo's history: people.Middleware never
// makes one the actor of a request.
func require(tx *writer.Tx, by tabs.By, r access.Right, project string) error {
	if by.Actor.Kind == people.Seed {
		return nil
	}
	return access.Require(tx, by.Actor, r, access.ProjectScope(project))
}

// acted records that a person acted in a project now: they stay
// assignable there for a day (FR-17.1).
func acted(tx *writer.Tx, project, person string, at time.Time) error {
	_, err := tx.Exec(`INSERT INTO tracker_actors (project_id, person_id, at) VALUES (?, ?, ?)
		ON CONFLICT (project_id, person_id) DO UPDATE SET at = max(at, excluded.at)`, project, person, at.UnixMilli())
	return err
}

// at is the command's time, or now.
func at(by tabs.By) time.Time {
	if by.At.IsZero() {
		return time.Now()
	}
	return by.At
}

// meta is the events' metadata of a command, with its time set.
func meta(by tabs.By) events.Meta {
	m := by.Meta()
	m.At = at(by)
	return m
}

// checkBy refuses a command without an actor: Validate runs it.
func checkBy(by tabs.By) error {
	if by.Actor.IsZero() {
		return errors.New("tracker: a command needs an actor")
	}
	return nil
}

// peek is a command that decides nothing: its decider hands out the
// stream's state, so that a command reads a state before it decides.
type peek[S any] struct{ out *S }

func peekDecider[E, S any](initial S, evolve func(S, E) S, version int) events.Decider[peek[S], E, S] {
	return events.Decider[peek[S], E, S]{
		Initial: initial,
		Decide: func(c peek[S], s S) ([]E, error) {
			*c.out = s
			return nil, nil
		},
		Evolve:  evolve,
		Version: version,
	}
}

// eventMeta reads who appended an event of a stream and when.
func eventMeta[K events.Key, E any](ctx context.Context, q sqlite.Querier, id events.StreamID[K, E], version int64) (events.Meta, error) {
	for r, err := range events.Load(ctx, Store(), q, id, version-1) {
		if err != nil {
			return events.Meta{}, err
		}
		return r.Meta, nil
	}
	return events.Meta{}, nil
}

func millis(t time.Time) int64 { return t.UnixMilli() }

func fromMillis(ms int64) time.Time { return time.UnixMilli(ms) }

func nullTime(ms sql.NullInt64) time.Time {
	if !ms.Valid {
		return time.Time{}
	}
	return time.UnixMilli(ms.Int64)
}
