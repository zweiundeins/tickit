package wiki

import (
	"bytes"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/doc"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
)

// CommentEvent is an event of a page comment's stream (FR-90). Comments
// on pages have a stream kind of their own, "page_comment": a page's
// comment has a parent (one level of replies) and checks its rights in a
// space, an issue's in a project.
type CommentEvent interface{ commentEvent() }

// CommentAdded starts a comment's stream: on a page, as a reply to
// Parent ("" for none), mentioning people (FR-67).
type CommentAdded struct {
	Page     string          `json:"page"`
	Parent   string          `json:"parent,omitempty"`
	Body     json.RawMessage `json:"body"`
	Mentions []string        `json:"mentions,omitempty"`
}

// CommentEdited holds the new body and the people it mentions for the
// first time.
type CommentEdited struct {
	Body     json.RawMessage `json:"body"`
	Mentions []string        `json:"mentions_added,omitempty"`
}

// CommentDeleted leaves "Comment deleted" in the thread.
type CommentDeleted struct{}

// CommentHidden and CommentUnhidden: a space's Admin or a moderator.
type CommentHidden struct {
	Reason string `json:"reason,omitempty"`
}
type CommentUnhidden struct{}

func (CommentAdded) commentEvent()    {}
func (CommentEdited) commentEvent()   {}
func (CommentDeleted) commentEvent()  {}
func (CommentHidden) commentEvent()   {}
func (CommentUnhidden) commentEvent() {}

// CommentStream is the kind of the page comments' streams.
var CommentStream = events.NewStream[string, CommentEvent]("page_comment")

func defineComment[T CommentEvent](name string) events.Type[string, CommentEvent, T] {
	return events.Define(CommentStream, name, func(t T) CommentEvent { return t })
}

// The page comment event types.
var (
	CommentAddedType    = defineComment[CommentAdded]("page_comment.added")
	CommentEditedType   = defineComment[CommentEdited]("page_comment.edited")
	CommentDeletedType  = defineComment[CommentDeleted]("page_comment.deleted")
	CommentHiddenType   = defineComment[CommentHidden]("page_comment.hidden")
	CommentUnhiddenType = defineComment[CommentUnhidden]("page_comment.unhidden")
)

// CommentState is what a comment's commands decide on.
type CommentState struct {
	Exists    bool     `json:"exists"`
	Deleted   bool     `json:"deleted,omitempty"`
	Hidden    bool     `json:"hidden,omitempty"`
	Page      string   `json:"page"`
	Parent    string   `json:"parent,omitempty"`
	Author    string   `json:"author"`
	Body      []byte   `json:"body,omitempty"`
	Mentioned []string `json:"mentioned,omitempty"`
}

const commentFold = 1

func evolveComment(s CommentState, e CommentEvent) CommentState {
	switch e := e.(type) {
	case CommentAdded:
		s.Exists, s.Page, s.Parent, s.Body, s.Mentioned = true, e.Page, e.Parent, e.Body, e.Mentions
	case CommentEdited:
		s.Body = e.Body
		s.Mentioned = applySet(s.Mentioned, e.Mentions, nil)
	case CommentDeleted:
		s.Deleted = true
	case CommentHidden:
		s.Hidden = true
	case CommentUnhidden:
		s.Hidden = false
	}
	return s
}

func commentDecider[C any](decide func(C, CommentState) ([]CommentEvent, error)) events.Decider[C, CommentEvent, CommentState] {
	return events.Decider[C, CommentEvent, CommentState]{Decide: decide, Evolve: evolveComment, Version: commentFold}
}

var peekCommentDecider = peekDecider(CommentState{}, evolveComment, commentFold)

func commentState(tx *writer.Tx, id string) (CommentState, error) {
	var s CommentState
	if !ValidID(id) {
		return s, notFound("comment", id)
	}
	if err := events.Execute(Store(), tx, peekCommentDecider, CommentStream.ID(id), events.Any, peek[CommentState]{&s}, events.Meta{}); err != nil {
		return s, err
	}
	if !s.Exists {
		return s, notFound("comment", id)
	}
	for r, err := range events.Load(tx.Context(), Store(), tx, CommentStream.ID(id), 0) {
		if err != nil {
			return s, err
		}
		s.Author = r.Actor
		break
	}
	return s, nil
}

// commentBody checks a comment's body: a document of 1 to 10,000
// characters, without headings and tables, mentioning 10 people at most
// (FR-65, DM-05), and returns it as stored with the people it mentions.
func commentBody(raw json.RawMessage) (json.RawMessage, []string, error) {
	d, err := doc.ParseDoc(bytes.TrimSpace(raw))
	if err != nil {
		return nil, nil, tabs.Refuse("body", "The comment is not a document.")
	}
	if err := rules.Check(nil, d); err != nil {
		var re *doc.RuleError
		if errors.As(err, &re) {
			return nil, nil, tabs.Refuse("body", re.Msg)
		}
		return nil, nil, tabs.Refuse("body", "The comment is not a document.")
	}
	refs := doc.References(d)
	n := utf8.RuneCountInString(doc.Text(d))
	var outlawed bool
	d.Descendants(func(n *doc.Node, _ int, _ *doc.Node, _ int) bool {
		if t := n.Type().Name(); t == "heading" || t == "table" {
			outlawed = true
		}
		return !outlawed
	})
	switch {
	case n == 0 && len(refs.People) == 0 && len(refs.Issues) == 0:
		return nil, nil, tabs.Refuse("body", "Write something first.")
	case n > MaxCommentText:
		return nil, nil, tabs.Refuse("body", "A comment has at most 10,000 characters.")
	case outlawed:
		return nil, nil, tabs.Refuse("body", "A comment has no headings and no tables.")
	case len(refs.People) > MaxMentions:
		return nil, nil, tabs.Refuse("body", "A comment mentions 10 people at most.")
	}
	return marshalDoc(d), refs.People, nil
}

// AddComment comments on a page, or replies to one of its comments
// (FR-90). Anyone who may read the space comments (3.2). Its result is
// {"id"} as JSON.
type AddComment struct {
	By     tabs.By
	ID     string // optional
	Page   string
	Parent string
	Body   json.RawMessage
}

func (c AddComment) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.ID != "" && !ValidID(c.ID) || c.Parent != "" && !ValidID(c.Parent) {
		return errors.New("wiki: a malformed comment")
	}
	return nil
}

var addComment = commentDecider(func(c CommentAdded, s CommentState) ([]CommentEvent, error) {
	if s.Exists {
		return nil, errors.New("wiki: the comment exists")
	}
	return []CommentEvent{c}, nil
})

func (c AddComment) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c AddComment) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if _, err := current(tx, c.By, c.Page, access.Comment); err != nil {
		return nil, err
	}
	if c.Parent != "" {
		p, err := commentState(tx, c.Parent)
		if err != nil || p.Page != c.Page {
			return nil, tabs.Refuse("body", "That comment is not on this page any more.")
		}
		if p.Parent != "" {
			return nil, tabs.Refuse("body", "Replies go one level deep: reply to the comment above.")
		}
	}
	body, mentions, err := commentBody(c.Body)
	if err != nil {
		return nil, err
	}
	id := c.ID
	if id == "" {
		id = NewID()
	}
	e := CommentAdded{Page: c.Page, Parent: c.Parent, Body: body, Mentions: mentions}
	if err := events.Execute(Store(), tx, addComment, CommentStream.ID(id), events.NoStream, e, meta(c.By)); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": id})
}

// mayChange checks that the actor may edit or delete a comment: their
// own, or anyone's with EditAnyComment.
func mayChange(tx *writer.Tx, by tabs.By, s CommentState) error {
	ps, err := pageState(tx, s.Page)
	if err != nil {
		return err
	}
	if s.Author == by.Actor.ID {
		return require(tx, by, access.EditOwnComment, ps.Space)
	}
	if by.Actor.Kind == people.Seed {
		return nil
	}
	return require(tx, by, access.EditAnyComment, ps.Space)
}

// EditComment changes a comment's body: its author does, or a moderator.
type EditComment struct {
	By      tabs.By
	Comment string
	Body    json.RawMessage
}

func (c EditComment) Validate() error { return checkBy(c.By) }

var editComment = commentDecider(func(c CommentEdited, s CommentState) ([]CommentEvent, error) {
	switch {
	case s.Deleted:
		return nil, &writer.Refusal{Msg: "This comment was deleted."}
	case bytes.Equal(c.Body, s.Body):
		return nil, nil
	}
	var added []string
	for _, m := range c.Mentions {
		if !contains(s.Mentioned, m) {
			added = append(added, m)
		}
	}
	c.Mentions = added
	return []CommentEvent{c}, nil
})

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (c EditComment) Apply(tx *writer.Tx) error {
	s, err := commentState(tx, c.Comment)
	if err != nil {
		return err
	}
	if err := mayChange(tx, c.By, s); err != nil {
		return err
	}
	body, mentions, err := commentBody(c.Body)
	if err != nil {
		return err
	}
	return events.Execute(Store(), tx, editComment, CommentStream.ID(c.Comment), events.Any, CommentEdited{Body: body, Mentions: mentions}, meta(c.By))
}

// DeleteComment deletes a comment: its author does, or a moderator.
type DeleteComment struct {
	By      tabs.By
	Comment string
}

func (c DeleteComment) Validate() error { return checkBy(c.By) }

var deleteComment = commentDecider(func(c DeleteComment, s CommentState) ([]CommentEvent, error) {
	if s.Deleted {
		return nil, nil
	}
	return []CommentEvent{CommentDeleted{}}, nil
})

func (c DeleteComment) Apply(tx *writer.Tx) error {
	s, err := commentState(tx, c.Comment)
	if err != nil {
		return err
	}
	if err := mayChange(tx, c.By, s); err != nil {
		return err
	}
	return events.Execute(Store(), tx, deleteComment, CommentStream.ID(c.Comment), events.Any, c, meta(c.By))
}

// HideComment hides a comment or shows it again: the space's Admin, a
// moderator or a site admin (DM-07).
type HideComment struct {
	By      tabs.By
	Comment string
	Hidden  bool
	Reason  string
}

func (c HideComment) Validate() error { return checkBy(c.By) }

var hideComment = commentDecider(func(c HideComment, s CommentState) ([]CommentEvent, error) {
	switch {
	case s.Hidden == c.Hidden:
		return nil, nil
	case c.Hidden:
		reason, ok := text(c.Reason, 0, 200)
		if !ok {
			return nil, tabs.Refuse("reason", "A reason has at most 200 characters.")
		}
		return []CommentEvent{CommentHidden{Reason: reason}}, nil
	}
	return []CommentEvent{CommentUnhidden{}}, nil
})

func (c HideComment) Apply(tx *writer.Tx) error {
	s, err := commentState(tx, c.Comment)
	if err != nil {
		return err
	}
	ps, err := pageState(tx, s.Page)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.HideComment, ps.Space); err != nil {
		return err
	}
	return events.Execute(Store(), tx, hideComment, CommentStream.ID(c.Comment), events.Any, c, meta(c.By))
}
