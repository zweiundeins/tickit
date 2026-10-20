package tracker

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// CommentEvent is an event of a comment's stream. Comments on issues use
// it now; comments on pages will use it with Subject "page".
type CommentEvent interface{ commentEvent() }

// Subjects of comments.
const (
	SubjectIssue = "issue"
	SubjectPage  = "page"
)

// CommentAdded starts a comment's stream (FR-65). Mentions are the
// people its body mentions, whom notifications tell (FR-67).
type CommentAdded struct {
	Subject   string          `json:"subject"`    // SubjectIssue
	SubjectID string          `json:"subject_id"` // the issue's id
	Project   string          `json:"project,omitempty"`
	Body      json.RawMessage `json:"body"`
	Mentions  []string        `json:"mentions,omitempty"`
	// Parent is the comment this one replies to, on a subject with
	// replies (a page: one level, FR-90).
	Parent string `json:"parent,omitempty"`
}

// CommentEdited holds the new body and the people it mentions for the
// first time (FR-67.3: notified once).
type CommentEdited struct {
	Body     json.RawMessage `json:"body"`
	Mentions []string        `json:"mentions_added,omitempty"`
}

// CommentDeleted leaves "Comment deleted" in the thread (FR-66.1).
type CommentDeleted struct{}

// CommentHidden and CommentUnhidden: an Admin or a moderator hides a
// comment (FR-66.2, DM-07).
type CommentHidden struct {
	Reason string `json:"reason,omitempty"`
}
type CommentUnhidden struct{}

func (CommentAdded) commentEvent()    {}
func (CommentEdited) commentEvent()   {}
func (CommentDeleted) commentEvent()  {}
func (CommentHidden) commentEvent()   {}
func (CommentUnhidden) commentEvent() {}

// CommentStream is the kind of the comments' streams, keyed by comment id.
var CommentStream = events.NewStream[string, CommentEvent]("comment")

func defineComment[T CommentEvent](name string) events.Type[string, CommentEvent, T] {
	return events.Define(CommentStream, name, func(t T) CommentEvent { return t })
}

// The comment event types; the names are what the log stores.
var (
	CommentAddedType    = defineComment[CommentAdded]("comment.added")
	CommentEditedType   = defineComment[CommentEdited]("comment.edited")
	CommentDeletedType  = defineComment[CommentDeleted]("comment.deleted")
	CommentHiddenType   = defineComment[CommentHidden]("comment.hidden")
	CommentUnhiddenType = defineComment[CommentUnhidden]("comment.unhidden")
)

// CommentState is what a comment's commands decide on.
type CommentState struct {
	Exists    bool            `json:"exists"`
	Deleted   bool            `json:"deleted,omitempty"`
	Hidden    bool            `json:"hidden,omitempty"`
	Subject   string          `json:"subject"`
	SubjectID string          `json:"subject_id"`
	Project   string          `json:"project,omitempty"`
	Parent    string          `json:"parent,omitempty"`
	Body      json.RawMessage `json:"body,omitempty"`
	Mentioned []string        `json:"mentioned,omitempty"` // everyone it ever mentioned
	Version   int64           `json:"version"`
}

const commentFold = 1

func evolveComment(s CommentState, e CommentEvent) CommentState {
	s.Version++
	switch e := e.(type) {
	case CommentAdded:
		s.Exists, s.Subject, s.SubjectID, s.Project, s.Parent, s.Body, s.Mentioned = true, e.Subject, e.SubjectID, e.Project, e.Parent, e.Body, e.Mentions
	case CommentEdited:
		s.Body = e.Body
		s.Mentioned = watchersOf(append([]string(nil), s.Mentioned...), e.Mentions...)
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
	return s, nil
}

// commentBody checks a comment's body: a document of 1 to 10,000
// characters, without headings and tables, mentioning 10 people at most
// (FR-65.1, DM-05).
func commentBody(raw json.RawMessage) (docInfo, error) {
	info, err := readDoc(raw)
	switch {
	case err != nil && err != errNotDoc:
		return info, tabs.Refuse("body", "A comment has at most 10,000 characters.")
	case err != nil:
		return info, tabs.Refuse("body", "The comment is not a document.")
	case info.Length == 0 && len(info.Mentions) == 0:
		return info, tabs.Refuse("body", "Write something first.")
	case info.Length > MaxCommentText:
		return info, tabs.Refuse("body", "A comment has at most 10,000 characters.")
	case info.blocks["heading"] || info.blocks["table"]:
		return info, tabs.Refuse("body", "A comment has no headings and no tables.")
	case len(info.Mentions) > MaxMentions:
		return info, tabs.Refuse("body", "A comment mentions 10 people at most.")
	}
	for _, m := range info.Mentions {
		if !people.ValidID(m) {
			return info, tabs.Refuse("body", "A mention names nobody.")
		}
	}
	return info, nil
}

// AddComment comments on an issue (FR-65). The author and the people the
// comment mentions become the issue's watchers (FR-27.2). Its result is
// the comment's id as JSON: {"id": "..."}.
type AddComment struct {
	By    tabs.By
	ID    string // optional
	Issue string
	Body  json.RawMessage
}

func (c AddComment) Validate() error {
	if c.ID != "" && !ValidID(c.ID) {
		return errors.New("tracker: a malformed comment id")
	}
	return checkBy(c.By)
}

type newComment struct {
	AddComment
	project string
	deleted bool
}

var addComment = commentDecider(func(c newComment, s CommentState) ([]CommentEvent, error) {
	if s.Exists {
		return nil, errors.New("tracker: the comment exists")
	}
	if c.deleted {
		return nil, errDeleted
	}
	info, err := commentBody(c.Body)
	if err != nil {
		return nil, err
	}
	return []CommentEvent{CommentAdded{Subject: SubjectIssue, SubjectID: c.Issue, Project: c.project, Body: canonical(c.Body), Mentions: info.Mentions}}, nil
})

func (c AddComment) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c AddComment) ApplyResult(tx *writer.Tx) ([]byte, error) {
	s, err := issueState(tx, c.Issue)
	if err != nil {
		return nil, err
	}
	if err := require(tx, c.By, access.Comment, s.Project); err != nil {
		return nil, err
	}
	if c.ID == "" {
		c.ID = NewID()
	}
	m := meta(c.By)
	n := newComment{AddComment: c, project: s.Project, deleted: s.Deleted}
	if err := events.Execute(Store(), tx, addComment, CommentStream.ID(c.ID), events.NoStream, n, m); err != nil {
		return nil, err
	}
	info, _ := readDoc(c.Body)
	add := watch{add: watchersOf([]string{c.By.Actor.ID}, info.Mentions...)}
	if err := events.Execute(Store(), tx, watchDecider, IssueStream.ID(c.Issue), events.Any, add, m); err != nil {
		return nil, err
	}
	if err := acted(tx, s.Project, c.By.Actor.ID, at(c.By)); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": c.ID})
}

// EditComment changes a comment's body: its author does (FR-66.1), or a
// moderator.
type EditComment struct {
	By      tabs.By
	Comment string
	Body    json.RawMessage
}

func (c EditComment) Validate() error { return checkBy(c.By) }

type editComment struct{ Body json.RawMessage }

var editCommentDecider = commentDecider(func(c editComment, s CommentState) ([]CommentEvent, error) {
	switch {
	case !s.Exists:
		return nil, notFound("comment", "")
	case s.Deleted:
		return nil, &writer.Refusal{Msg: "This comment was deleted."}
	}
	info, err := commentBody(c.Body)
	if err != nil {
		return nil, err
	}
	body := canonical(c.Body)
	if bytes.Equal(body, s.Body) {
		return nil, nil
	}
	return []CommentEvent{CommentEdited{Body: body, Mentions: added(s.Mentioned, info.Mentions)}}, nil
})

// mayChange checks that the actor may edit or delete a comment: their
// own, or anyone's with EditAnyComment.
func mayChange(tx *writer.Tx, by tabs.By, s CommentState, id string) error {
	if err := requireOn(tx, by, access.Read, s); err != nil {
		return err
	}
	author, err := commentAuthor(tx, id)
	if err != nil {
		return err
	}
	if author == by.Actor.ID {
		return requireOn(tx, by, access.EditOwnComment, s)
	}
	return requireOn(tx, by, access.EditAnyComment, s)
}

// commentAuthor is who appended a comment's first event.
func commentAuthor(tx *writer.Tx, id string) (string, error) {
	m, err := eventMeta(tx.Context(), tx, CommentStream.ID(id), 1)
	return m.Actor, err
}

func (c EditComment) Apply(tx *writer.Tx) error {
	s, err := commentState(tx, c.Comment)
	if err != nil {
		return err
	}
	if err := mayChange(tx, c.By, s, c.Comment); err != nil {
		return err
	}
	m := meta(c.By)
	if err := events.Execute(Store(), tx, editCommentDecider, CommentStream.ID(c.Comment), events.Any, editComment{c.Body}, m); err != nil {
		return err
	}
	info, _ := readDoc(c.Body)
	if s.Subject == SubjectIssue && len(info.Mentions) > 0 {
		if err := events.Execute(Store(), tx, watchDecider, IssueStream.ID(s.SubjectID), events.Any, watch{add: info.Mentions}, m); err != nil && !errors.Is(err, errDeleted) {
			return err
		}
	}
	return nil
}

// DeleteComment deletes a comment: its author does, or a moderator.
type DeleteComment struct {
	By      tabs.By
	Comment string
}

func (c DeleteComment) Validate() error { return checkBy(c.By) }

var deleteCommentDecider = commentDecider(func(c DeleteComment, s CommentState) ([]CommentEvent, error) {
	if !s.Exists {
		return nil, notFound("comment", c.Comment)
	}
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
	if err := mayChange(tx, c.By, s, c.Comment); err != nil {
		return err
	}
	return events.Execute(Store(), tx, deleteCommentDecider, CommentStream.ID(c.Comment), events.Any, c, meta(c.By))
}

// keepStaffHide refuses to show a comment again that a moderator or a
// site admin hid: a project's Admin shows only what they hid (DM-07.1).
func keepStaffHide(tx *writer.Tx, comment string) error {
	var by string
	for r, err := range events.Load(tx.Context(), Store(), tx, CommentStream.ID(comment), 0) {
		if err != nil {
			return err
		}
		if _, ok := r.Data.(CommentHidden); ok {
			by = r.Actor
		}
	}
	staff, err := people.Staff(tx.Context(), tx, by)
	if err != nil || !staff {
		return err
	}
	return &access.Denial{Refusal: writer.Refusal{Msg: "A moderator hid this comment: only a moderator shows it again."}}
}

// HideComment hides a comment, or shows it again: a project's Admin, a
// moderator or a site admin (FR-66.2).
type HideComment struct {
	By      tabs.By
	Comment string
	Hidden  bool
	Reason  string
}

func (c HideComment) Validate() error { return checkBy(c.By) }

var hideCommentDecider = commentDecider(func(c HideComment, s CommentState) ([]CommentEvent, error) {
	switch {
	case !s.Exists:
		return nil, notFound("comment", c.Comment)
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
	if err := requireOn(tx, c.By, access.HideComment, s); err != nil {
		return err
	}
	if s.Hidden && !c.Hidden && !access.SiteCan(c.By.Actor, access.Moderate) {
		if err := keepStaffHide(tx, c.Comment); err != nil {
			return err
		}
	}
	return events.Execute(Store(), tx, hideCommentDecider, CommentStream.ID(c.Comment), events.Any, c, meta(c.By))
}
