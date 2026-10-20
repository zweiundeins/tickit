// Package search is tickit's search across issues and pages (FR-57): an
// FTS5 index per kind of document, kept as a projection of the event
// streams, and the query that ranks what it finds.
//
// # The index
//
// Each kind (Issue, Page, Comment) has two tables (Tables): its documents,
// and their text in an FTS5 table (key, title, body). They are a
// projection: only the kind's projector writes them, with Put, Patch,
// Hide and Remove inside its inline handlers, and a rebuild of that
// projector (lith/events, into copies) replaces both. The handlers build
// a document from the event and from the document as indexed (Patch), so
// that a rebuild never reads another projection that may be rebuilding.
//
// The tracker keeps search_issues (IssuesProjector, the issue stream) and
// search_comments (CommentsProjector, the comment stream, which holds
// comments on issues and on pages). The wiki keeps search_pages: its
// projector of the page stream is named PagesProjector at PagesVersion
// and writes Tables(Page), as docs/architecture.md ("Search") shows.
//
// # Queries
//
// Search matches every word of the text, the last one as a prefix while
// someone types, in the key, title and body of issues and pages and in
// the comments on them. An exact issue key or page title comes first,
// then title and key matches, then matches in the body or a comment, each
// tier by bm25. A hit carries a snippet of its body or comment with the
// matched words marked (Part), for the page to escape.
package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"
)

// Kind is a kind of indexed document.
type Kind string

const (
	Issue   Kind = "issue"
	Page    Kind = "page"
	Comment Kind = "comment" // on an issue or a page: its Scope is the subject's id, its Key the subject's kind
)

// Kinds a search finds: comments lead to their subject.
var Kinds = []Kind{Issue, Page}

// Valid reports whether k is a kind of document.
func (k Kind) Valid() bool { return k == Issue || k == Page || k == Comment }

func (k Kind) table() string {
	switch k {
	case Issue:
		return "search_issues"
	case Page:
		return "search_pages"
	case Comment:
		return "search_comments"
	}
	panic("search: no kind " + string(k))
}

// Tables are the tables of a kind's index, for its projector's Tables:
// migrations/060_search.sql creates them.
func Tables(k Kind) []string { return []string{k.table(), k.table() + "_fts"} }

// The projectors of the index: a projector's name and version, which a
// change of what it indexes bumps (and the runner rebuilds).
const (
	IssuesProjector   = "search_issues"
	IssuesVersion     = 1
	CommentsProjector = "search_comments"
	CommentsVersion   = 1
	PagesProjector    = "search_pages"
	PagesVersion      = 1
)

// Topic is touched by every change to the index: views of search results
// list it, with the projectors' topics for the swap of a rebuild
// (ProjectionTopics).
const Topic = "search"

// ProjectionTopics are the topics a rebuild of the index touches when it
// swaps the new tables in.
var ProjectionTopics = []string{"lith:projection:" + IssuesProjector, "lith:projection:" + CommentsProjector, "lith:projection:" + PagesProjector}

// Limits of what a document indexes.
const (
	MaxTitle = 500
	MaxBody  = 100_000 // characters
	MaxList  = 100     // labels, people, old keys
)

// Doc is an indexed document.
type Doc struct {
	ID string
	// Scope is where it lives, for access and filters: an issue's project
	// id, a page's space id; a comment's subject id.
	Scope string
	// Key is an issue's key ("WEB-12"), which an exact search finds
	// first; OldKeys the keys it had before moves. A comment's Key is its
	// subject's kind (Issue or Page).
	Key     string
	OldKeys []string
	Title   string
	Body    string // plain text
	// Href is the document's address, Place what results show beside its
	// title (a page's space key).
	Href, Place string
	Labels      []string
	People      []string // who a person filter finds it by: reporter and assignee, author and editors
	// Hidden keeps a document indexed but out of results (a hidden
	// comment, which may show again).
	Hidden  bool
	Updated time.Time
}

// clean keeps a document within the limits, with the control characters
// that mark a snippet's matches turned into spaces.
func clean(d Doc) Doc {
	d.Key = strings.TrimSpace(d.Key)
	d.Title = truncate(controls.Replace(d.Title), MaxTitle)
	d.Body = truncate(controls.Replace(d.Body), MaxBody)
	d.OldKeys, d.Labels, d.People = capList(d.OldKeys), capList(d.Labels), capList(d.People)
	return d
}

var controls = strings.NewReplacer(markOn, " ", markOff, " ", "\x00", " ")

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func capList(l []string) []string {
	if len(l) > MaxList {
		l = l[:MaxList]
	}
	out := make([]string, 0, len(l))
	for _, s := range l {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func jsonList(l []string) string {
	if len(l) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(l)
	return string(b)
}

// fold is a title as an exact search compares it: lower case, spaces
// collapsed.
func fold(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// Put indexes a document of kind k in place of the one with its id.
func Put(tx *writer.Tx, k Kind, d Doc) error {
	if d.ID == "" {
		return errors.New("search: a document without an id")
	}
	d = clean(d)
	t := k.table()
	var n int64
	err := tx.QueryRow(`SELECT n FROM `+t+` WHERE id = ?`, d.ID).Scan(&n)
	args := []any{d.Scope, d.Key, strings.Join(d.OldKeys, " "), fold(d.Title), d.Href, d.Place, jsonList(d.Labels), jsonList(d.People), d.Hidden, d.Updated.UnixMilli()}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		err = tx.QueryRow(`INSERT INTO `+t+` (scope, key, old_keys, title, href, place, labels, people, hidden, updated, id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING n`, append(args, d.ID)...).Scan(&n)
		if err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if _, err := tx.Exec(`UPDATE `+t+` SET scope = ?, key = ?, old_keys = ?, title = ?, href = ?, place = ?, labels = ?, people = ?,
			hidden = ?, updated = ? WHERE n = ?`, append(args, n)...); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM `+t+`_fts WHERE rowid = ?`, n); err != nil {
			return err
		}
	}
	keys := strings.TrimSpace(d.Key + " " + strings.Join(d.OldKeys, " "))
	if _, err := tx.Exec(`INSERT INTO `+t+`_fts (rowid, key, title, body) VALUES (?, ?, ?, ?)`, n, keys, d.Title, d.Body); err != nil {
		return err
	}
	tx.Touch(Topic)
	return nil
}

// Get reads an indexed document.
func Get(ctx context.Context, q sqlite.Querier, k Kind, id string) (Doc, bool, error) {
	t := k.table()
	var d Doc
	var oldKeys, labels, people string
	var updated int64
	err := q.QueryRowContext(ctx, `SELECT d.id, d.scope, d.key, d.old_keys, f.title, f.body, d.href, d.place, d.labels, d.people, d.hidden, d.updated
		FROM `+t+` d JOIN `+t+`_fts f ON f.rowid = d.n WHERE d.id = ?`, id).
		Scan(&d.ID, &d.Scope, &d.Key, &oldKeys, &d.Title, &d.Body, &d.Href, &d.Place, &labels, &people, &d.Hidden, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Doc{}, false, nil
	}
	if err != nil {
		return Doc{}, false, err
	}
	d.OldKeys = strings.Fields(oldKeys)
	if err := json.Unmarshal([]byte(labels), &d.Labels); err != nil {
		return Doc{}, false, err
	}
	if err := json.Unmarshal([]byte(people), &d.People); err != nil {
		return Doc{}, false, err
	}
	d.Updated = time.UnixMilli(updated).UTC()
	return d, true, nil
}

// Patch changes the indexed document with id: fn gets it as indexed and
// changes it in place. A document the index lacks stays out of it.
func Patch(tx *writer.Tx, k Kind, id string, fn func(*Doc)) error {
	d, ok, err := Get(tx.Context(), tx, k, id)
	if err != nil || !ok {
		return err
	}
	fn(&d)
	d.ID = id
	return Put(tx, k, d)
}

// Remove takes a document out of the index.
func Remove(tx *writer.Tx, k Kind, id string) error {
	t := k.table()
	var n int64
	err := tx.QueryRow(`SELECT n FROM `+t+` WHERE id = ?`, id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM `+t+`_fts WHERE rowid = ?`, n); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM `+t+` WHERE n = ?`, n); err != nil {
		return err
	}
	tx.Touch(Topic)
	return nil
}

// Hide keeps a document indexed but out of results, or shows it again.
func Hide(tx *writer.Tx, k Kind, id string, hidden bool) error {
	res, err := tx.Exec(`UPDATE `+k.table()+` SET hidden = ? WHERE id = ? AND hidden != ?`, hidden, id, hidden)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		tx.Touch(Topic)
	}
	return nil
}

// words are the words of a search text as the index's tokenizer cuts
// them: letters, digits and marks; everything else separates.
func words(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r)
	})
}
