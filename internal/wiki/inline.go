package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/zweiundeins/coedit/collab"
	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"
	"github.com/zweiundeins/prosemirror-go"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/docnodes"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Inline comments (FR-91) are page comments on a range of the page's
// text. The comment, its replies, edits, hiding and notifications are the
// page comment's (comments.go); its stream "inline", keyed by the
// comment's id, holds the range it was made on and its resolution. Where
// the text is now lives in page_anchors, outside the document: every
// commit maps the page's anchors through its steps (pageDocs' OnCommit),
// in the writer's order, so every reader and editor sees the same
// positions at a version, and an anchor whose text was deleted is
// detached. docs/editor.md, "Inline comments", says why not a mark.

// InlineEvent is an event of an inline comment's stream.
type InlineEvent interface{ inlineEvent() }

// InlineAnchored starts the stream: the page, the range at version V of
// its document (the head when it was made), and the text it held.
type InlineAnchored struct {
	Page  string `json:"page"`
	V     int64  `json:"v"`
	From  int    `json:"from"`
	To    int    `json:"to"`
	Quote string `json:"quote"`
}

// InlineResolved hides the highlight and lists the comment as resolved
// (FR-91.3); InlineReopened takes it back.
type InlineResolved struct{}
type InlineReopened struct{}

func (InlineAnchored) inlineEvent() {}
func (InlineResolved) inlineEvent() {}
func (InlineReopened) inlineEvent() {}

// InlineStream is the kind of the inline comments' streams, keyed by the
// comment's id.
var InlineStream = events.NewStream[string, InlineEvent]("inline")

func defineInline[T InlineEvent](name string) events.Type[string, InlineEvent, T] {
	return events.Define(InlineStream, name, func(t T) InlineEvent { return t })
}

// The inline event types; the names are what the log stores.
var (
	InlineAnchoredType = defineInline[InlineAnchored]("inline.anchored")
	InlineResolvedType = defineInline[InlineResolved]("inline.resolved")
	InlineReopenedType = defineInline[InlineReopened]("inline.reopened")
)

// InlineState is what an inline comment's commands decide on.
type InlineState struct {
	Exists   bool   `json:"exists"`
	Page     string `json:"page"`
	Resolved bool   `json:"resolved,omitempty"`
}

const inlineFold = 1

func evolveInline(s InlineState, e InlineEvent) InlineState {
	switch e := e.(type) {
	case InlineAnchored:
		s.Exists, s.Page = true, e.Page
	case InlineResolved:
		s.Resolved = true
	case InlineReopened:
		s.Resolved = false
	}
	return s
}

func inlineDecider[C any](decide func(C, InlineState) ([]InlineEvent, error)) events.Decider[C, InlineEvent, InlineState] {
	return events.Decider[C, InlineEvent, InlineState]{Decide: decide, Evolve: evolveInline, Version: inlineFold}
}

var peekInlineDecider = peekDecider(InlineState{}, evolveInline, inlineFold)

// InlineTopic is touched by every change to a page's inline comments: a
// new one, a resolution, a detached anchor, and every change to any of
// the page's comments.
func InlineTopic(page string) string { return "inline:" + page }

// MaxQuote is how much of the text a comment is on its list shows.
const MaxQuote = 200

// AddInlineComment comments on a range of a page's text (FR-91.1): From
// and To at version Base of its document, whose text there is Text (the
// characters of its text nodes, without separators). The range maps over
// the commits since Base; when its text changed meanwhile, or nothing of
// it is left, the person selects again. Anyone who may comment on the
// page does (3.2). Its result is {"id"} as JSON.
type AddInlineComment struct {
	By       tabs.By
	ID       string // optional
	Page     string
	Epoch    string
	Base     int64
	From, To int
	Text     string
	Body     json.RawMessage
	// On, for the seed, puts it on the first text of the head that holds
	// On instead of From and To at Base.
	On string
}

func (c AddInlineComment) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.ID != "" && !ValidID(c.ID) || !ValidID(c.Page) || c.Base < 0 || c.From < 0 || c.To <= c.From && c.On == "" || len(c.Text) > docnodes.SchemaLimits().StoredBytes {
		return errors.New("wiki: a malformed inline comment")
	}
	return nil
}

var anchorDecider = inlineDecider(func(c InlineAnchored, s InlineState) ([]InlineEvent, error) {
	if s.Exists {
		return nil, errors.New("wiki: the inline comment exists")
	}
	return []InlineEvent{c}, nil
})

func (c AddInlineComment) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c AddInlineComment) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if _, err := current(tx, c.By, c.Page, access.Comment); err != nil {
		return nil, err
	}
	ds, err := docState(tx, c.Page)
	if err != nil {
		return nil, err
	}
	if c.On != "" {
		head, err := pageDocs.StateDoc(tx.Context(), tx, c.Page, ds)
		if err != nil {
			return nil, err
		}
		var ok bool
		if c.From, c.To, ok = prosemirror.Find(head, c.On); !ok {
			return nil, tabs.Refuse("anchor", "Select some text of the page to comment on.")
		}
		c.Epoch, c.Base, c.Text = ds.Epoch, ds.Version, c.On
	}
	if ds.Epoch != c.Epoch {
		return nil, tabs.Refuse("anchor", "The page was reset since you selected the text: select it again.")
	}
	steps, err := pageDocs.StepsSince(tx, c.Page, ds, c.Base)
	if errors.Is(err, collab.ErrStale) {
		return nil, tabs.Refuse("anchor", "The page changed too much since you selected the text: select it again.")
	}
	if err != nil {
		return nil, err
	}
	head, err := pageDocs.StateDoc(tx.Context(), tx, c.Page, ds)
	if err != nil {
		return nil, err
	}
	from, to, _ := prosemirror.MapRange(head, steps, c.From, c.To)
	text, ok := prosemirror.InlineText(head, from, to)
	switch {
	case !ok || text == "":
		return nil, tabs.Refuse("anchor", "Select some text of the page to comment on.")
	case text != c.Text:
		return nil, tabs.Refuse("anchor", "The text changed while you commented: select it again.")
	}
	if c.ID == "" {
		c.ID = NewID()
	}
	if _, err := (AddComment{By: c.By, ID: c.ID, Page: c.Page, Body: c.Body}).ApplyResult(tx); err != nil {
		return nil, err
	}
	e := InlineAnchored{Page: c.Page, V: ds.Version, From: from, To: to, Quote: prosemirror.Quote(head, from, to, MaxQuote)}
	if err := events.Execute(Store(), tx, anchorDecider, InlineStream.ID(c.ID), events.NoStream, e, meta(c.By)); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO page_anchors (comment_id, page_id, from_pos, to_pos) VALUES (?, ?, ?, ?)`, c.ID, c.Page, from, to); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": c.ID})
}

// ResolveInline resolves an inline comment, or reopens it (FR-91.3):
// anyone who may comment on its page.
type ResolveInline struct {
	By       tabs.By
	Comment  string
	Resolved bool
}

func (c ResolveInline) Validate() error { return checkBy(c.By) }

var resolveDecider = inlineDecider(func(c ResolveInline, s InlineState) ([]InlineEvent, error) {
	switch {
	case !s.Exists:
		return nil, notFound("inline comment", c.Comment)
	case s.Resolved == c.Resolved:
		return nil, nil
	case c.Resolved:
		return []InlineEvent{InlineResolved{}}, nil
	}
	return []InlineEvent{InlineReopened{}}, nil
})

func (c ResolveInline) Apply(tx *writer.Tx) error {
	var s InlineState
	if !ValidID(c.Comment) {
		return notFound("inline comment", c.Comment)
	}
	if err := events.Execute(Store(), tx, peekInlineDecider, InlineStream.ID(c.Comment), events.Any, peek[InlineState]{&s}, events.Meta{}); err != nil {
		return err
	}
	if !s.Exists {
		return notFound("inline comment", c.Comment)
	}
	if _, err := current(tx, c.By, s.Page, access.Comment); err != nil {
		return err
	}
	return events.Execute(Store(), tx, resolveDecider, InlineStream.ID(c.Comment), events.Any, c, meta(c.By))
}

// mapAnchors maps a page's anchors through a commit's steps, in the
// commit's command (FR-91.2), and detaches those whose text went.
func mapAnchors(tx *writer.Tx, page string, steps []prosemirror.Step, next prosemirror.Doc) error {
	if len(steps) == 0 {
		return nil
	}
	type anchor struct {
		id       string
		from, to int
	}
	rows, err := tx.Query(`SELECT comment_id, from_pos, to_pos FROM page_anchors WHERE page_id = ? AND detached = 0`, page)
	if err != nil {
		return err
	}
	var list []anchor
	for rows.Next() {
		var a anchor
		if err := rows.Scan(&a.id, &a.from, &a.to); err != nil {
			rows.Close()
			return err
		}
		list = append(list, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, a := range list {
		from, to, gone := prosemirror.MapRange(next, steps, a.from, a.to)
		if from == a.from && to == a.to && !gone {
			continue
		}
		if _, err := tx.Exec(`UPDATE page_anchors SET from_pos = ?, to_pos = ?, detached = ? WHERE comment_id = ?`, from, to, gone, a.id); err != nil {
			return err
		}
		if gone {
			tx.Touch(InlineTopic(page))
		}
	}
	return nil
}

const inlineVersion = 1

// inlineProjector keeps page_inline.
var inlineProjector = events.NewProjector(InlineStream, "wiki_inline", inlineVersion,
	events.Handle(InlineAnchoredType, func(tx *writer.Tx, r events.Record[string, InlineAnchored]) error {
		e := r.Data
		_, err := tx.Exec(`INSERT INTO page_inline (comment_id, page_id, quote, v, from_pos, to_pos, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.ID, e.Page, e.Quote, e.V, e.From, e.To, millis(r.At))
		tx.Touch(InlineTopic(e.Page))
		return err
	}),
	events.Handle(InlineResolvedType, func(tx *writer.Tx, r events.Record[string, InlineResolved]) error {
		return setInline(tx, r.ID, `resolved_at = ?, resolved_by = ?`, millis(r.At), r.Actor)
	}),
	events.Handle(InlineReopenedType, func(tx *writer.Tx, r events.Record[string, InlineReopened]) error {
		return setInline(tx, r.ID, `resolved_at = NULL, resolved_by = ''`)
	}),
).Tables("page_inline")

func setInline(tx *writer.Tx, id, columns string, args ...any) error {
	var page string
	if err := tx.QueryRow(`UPDATE page_inline SET `+columns+` WHERE comment_id = ? RETURNING page_id`, append(args, id)...).Scan(&page); err != nil {
		return err
	}
	tx.Touch(InlineTopic(page))
	return nil
}

// InlineComment is an inline comment's thread and where its text is now.
type InlineComment struct {
	Comment
	Quote      string
	From, To   int
	Detached   bool // its text was deleted (FR-91.2)
	Resolved   bool
	ResolvedAt time.Time
	ResolvedBy string
}

// InlineComments are a page's inline comments with their replies: those
// on text in the order of the text, then the detached ones, then the
// resolved ones, the latest first. A deleted comment is left out once its
// replies are deleted too.
func InlineComments(ctx context.Context, q sqlite.Querier, page string) ([]InlineComment, error) {
	rows, err := q.QueryContext(ctx, `SELECT i.comment_id, i.quote, coalesce(a.from_pos, i.from_pos), coalesce(a.to_pos, i.to_pos),
		coalesce(a.detached, 1), coalesce(i.resolved_at, 0), i.resolved_by
		FROM page_inline i LEFT JOIN page_anchors a ON a.comment_id = i.comment_id WHERE i.page_id = ?`, page)
	if err != nil {
		return nil, err
	}
	byID := map[string]InlineComment{}
	for rows.Next() {
		var c InlineComment
		var resolved int64
		if err := rows.Scan(&c.ID, &c.Quote, &c.From, &c.To, &c.Detached, &resolved, &c.ResolvedBy); err != nil {
			rows.Close()
			return nil, err
		}
		if resolved > 0 {
			c.Resolved, c.ResolvedAt = true, time.UnixMilli(resolved)
		}
		byID[c.ID] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(byID) == 0 {
		return nil, err
	}
	threads, err := allComments(ctx, q, page)
	if err != nil {
		return nil, err
	}
	var out []InlineComment
	for _, t := range threads {
		c, ok := byID[t.ID]
		if !ok || t.Deleted && !slices.ContainsFunc(t.Replies, func(r Comment) bool { return !r.Deleted }) {
			continue
		}
		c.Comment = t
		out = append(out, c)
	}
	slices.SortStableFunc(out, func(a, b InlineComment) int {
		switch {
		case a.Resolved != b.Resolved:
			return boolOrder(a.Resolved)
		case a.Resolved:
			return b.ResolvedAt.Compare(a.ResolvedAt)
		case a.Detached != b.Detached:
			return boolOrder(a.Detached)
		case !a.Detached && a.From != b.From:
			return a.From - b.From
		}
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	return out, nil
}

func boolOrder(first bool) int {
	if first {
		return 1
	}
	return -1
}

// Highlights are the ranges of a page's head document that its readers
// and editors see marked: its inline comments on text, neither resolved,
// hidden nor deleted. They are at page_doc's version in the same read.
func Highlights(ctx context.Context, q sqlite.Querier, page string) ([]prosemirror.Highlight, error) {
	rows, err := q.QueryContext(ctx, `SELECT a.comment_id, a.from_pos, a.to_pos FROM page_anchors a
		JOIN page_inline i ON i.comment_id = a.comment_id JOIN comments c ON c.id = a.comment_id
		WHERE a.page_id = ? AND a.detached = 0 AND i.resolved_at IS NULL AND c.deleted_at IS NULL AND c.hidden_at IS NULL
		ORDER BY a.from_pos, a.comment_id`, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []prosemirror.Highlight
	for rows.Next() {
		var h prosemirror.Highlight
		if err := rows.Scan(&h.ID, &h.From, &h.To); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
