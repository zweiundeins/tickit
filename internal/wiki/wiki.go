// Package wiki is the Confluence side of tickit: spaces, the page tree,
// pages, their co-edited documents, versions, comments, labels, watchers
// and the trash. Every change is a command on an event stream
// (lith/events) whose decider decides from the stream's state; inline
// projectors keep the tables that pages read. docs/editor.md says how
// co-editing works; this package is its server side.
//
// # Streams
//
// Four kinds, keyed by UUIDv7 strings:
//
//   - "space": space.created (with its home page), .edited, .archived,
//     .unarchived.
//   - "page": page.created, .renamed, .moved, .labels_changed, .watched,
//     .trashed, .restored, .deleted, .version_cut. Never folded.
//   - "pagedoc", keyed by the page's id: pagedoc.created {epoch, doc},
//     .committed {ref, client, base, steps} with the steps as applied,
//     .snapshot {epoch, doc}, which the fold writes (Fold). Its stream
//     version is the collab version the editor speaks.
//   - "page_comment": page_comment.added, .edited, .deleted, .hidden,
//     .unhidden.
//
// Rules across pages live in tables the commands write: page_titles
// (titles unique per space without regard to case), page_parents (no
// move under a descendant, children up on trash), page_editors (the
// editing tabs). Space keys are unique through access's scopes.
//
// # Commands
//
// Every command carries tabs.By and checks its right with access.Require
// inside Apply: a Member creates and edits pages, its creator, the
// space's Admin or a moderator trashes a page (requirements 3.2). Commit
// rebases a stale commit over the commits since its base (doc.Rebase),
// checks the result (doc.Rules) and appends with events.Exactly(head);
// the web layer runs it under writer.Once by its ref. CutVersion cuts a
// version (60 s idle, the last editor gone, "Save version", every 10
// minutes); Restore turns the page into an old version with one step and
// cuts "Restored version 3"; Fold folds the commits up to a version into
// a snapshot (NFR-43).
//
// The seed runs the same commands with a seeded person as the actor and a
// past By.At, as the tracker's does.
//
// # Reads
//
// Queries take a sqlite.Querier: Spaces, SpaceByKey, SpaceByID, PageByID,
// Tree, Ancestors, Versions, Version, Trash, Comments, PagesMentioning
// (FR-101, for the issue page), LinkedFrom (FR-103.2), LinkedIssues,
// SearchPages, Labels; Head returns the head document through the
// document cache, Resolve what its chips, mentions and page links show.
package wiki

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Topics, besides access's space:<id> (SpaceTopic), which every change to
// a space or to the tree of its pages touches.
const (
	// SpacesTopic is touched by every change to any space: the list of
	// spaces and the pickers.
	SpacesTopic = "spaces"
	// PagesTopic is touched by every change to any page's title, place or
	// state: link pickers, lists of pages on issues, search.
	PagesTopic = "pages"
)

// SpaceTopic is touched by every change to a space and to its page tree.
func SpaceTopic(id string) string { return access.SpaceScope(id).Topic() }

// PageTopic is touched by every change a page's header shows: title,
// labels, byline, state, versions, links, comments.
func PageTopic(id string) string { return "page:" + id }

// DocTopic is touched by every commit to a page's document: the reader
// view and the editors' views.
func DocTopic(id string) string { return "pagedoc:" + id }

// ChipsTopic is touched when an issue that a page's document shows as a
// chip changes, and when the page's chips change (FR-100.2).
func ChipsTopic(id string) string { return "chips:" + id }

// CaretsTopic is touched by every presence post of a page's editors.
func CaretsTopic(id string) string { return "carets:" + id }

// EditorTopic is touched by every change to one editing tab's row: its
// refusals, its close.
func EditorTopic(tab string) string { return "editor:" + tab }

var (
	bound atomic.Pointer[events.Store]
	own   func() *events.Store // set in init: the projectors call Store
)

func init() {
	own = sync.OnceValue(func() *events.Store { return events.Must(events.New(Parts()...)) })
}

// Parts are the wiki's projectors, for the app's one event store.
func Parts() []events.Part {
	return []events.Part{spacesProjector, pagesProjector, versionsProjector, docsProjector, threadsProjector, chipsProjector, pageSearch}
}

// Bind gives the wiki's commands the app's event store, which holds every
// package's parts: cmd/tickit calls it once the store is made. Without
// it, as in tests, they use a store of the wiki's parts alone.
func Bind(st *events.Store) { bound.Store(st) }

// Store is the event store the commands append with.
func Store() *events.Store {
	if st := bound.Load(); st != nil {
		return st
	}
	return own()
}

// NewID returns a new id for a space, a page or a comment: a UUIDv7.
func NewID() string { return uuid.Must(uuid.NewV7()).String() }

// ValidID reports whether s is an id NewID could have made.
func ValidID(s string) bool {
	u, err := uuid.Parse(s)
	return err == nil && u.String() == s && u.Version() == 7
}

// notFound is the error of a command or query that names something that
// does not exist: it answers 404.
func notFound(what, id string) error { return fmt.Errorf("%s %s: %w", what, id, access.ErrNotFound) }

// require checks the actor's right in a space. Seeded people act only in
// the seed: people.Middleware never makes one the actor of a request.
func require(tx *writer.Tx, by tabs.By, r access.Right, space string) error {
	if by.Actor.Kind == people.Seed {
		return nil
	}
	return access.Require(tx, by.Actor, r, access.SpaceScope(space))
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
		return errors.New("wiki: a command needs an actor")
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

func millis(t time.Time) int64 { return t.UnixMilli() }

// text checks a one-line text: trimmed, min to max runes, no control
// characters.
func text(s string, min, max int) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	return s, n >= min && n <= max && strings.IndexFunc(s, unicode.IsControl) < 0
}

// longText checks a text of several lines.
func longText(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	return s, utf8.RuneCountInString(s) <= max && strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) < 0
}

// Limits of the wiki's texts (DM-05.1).
const (
	MaxTitle       = 255
	MaxSpaceName   = 100
	MaxDescription = 2000
	MaxMessage     = 200
	MaxLabel       = 50
	MaxLabels      = 20
	MaxCommentText = 10000
	MaxMentions    = 10
)
