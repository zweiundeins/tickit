package tracker

import (
	"encoding/json"
	"errors"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Subject is a kind of thing besides issues that comments are on: the
// wiki registers pages (RegisterSubject). Comments on it share the
// comment stream, the comments table, search and notifications with
// issue comments; their rights are checked in the subject's scope.
type Subject struct {
	// Scope is the scope of the subject with the id, or access.ErrNotFound.
	Scope func(tx *writer.Tx, id string) (access.Scope, error)
	// Topics are touched by every change to a comment on the subject.
	Topics func(id string) []string
	// Replies says that a comment may reply to a comment, one level deep.
	Replies bool
}

var subjects = map[string]Subject{}

// RegisterSubject adds a kind of subject of comments, from an init.
func RegisterSubject(name string, s Subject) { subjects[name] = s }

// requireOn checks the actor's right in a comment's subject's scope.
func requireOn(tx *writer.Tx, by tabs.By, r access.Right, s CommentState) error {
	if s.Subject == SubjectIssue || s.Subject == "" {
		return require(tx, by, r, s.Project)
	}
	sub, ok := subjects[s.Subject]
	if !ok {
		return notFound("comment subject", s.Subject)
	}
	scope, err := sub.Scope(tx, s.SubjectID)
	if err != nil {
		return err
	}
	if by.Actor.Kind == people.Seed {
		return nil
	}
	return access.Require(tx, by.Actor, r, scope)
}

// AddSubjectComment comments on a registered subject (a page, FR-90), or
// replies to one of its comments one level deep where the subject has
// replies. Anyone who may comment in the subject's scope does. Its result
// is {"id"} as JSON.
type AddSubjectComment struct {
	By        tabs.By
	ID        string // optional
	Subject   string
	SubjectID string
	Parent    string
	Body      json.RawMessage
}

func (c AddSubjectComment) Validate() error {
	if c.ID != "" && !ValidID(c.ID) || c.Parent != "" && !ValidID(c.Parent) || c.Subject == SubjectIssue {
		return errors.New("tracker: a malformed comment")
	}
	return checkBy(c.By)
}

var addSubjectComment = commentDecider(func(c CommentAdded, s CommentState) ([]CommentEvent, error) {
	if s.Exists {
		return nil, errors.New("tracker: the comment exists")
	}
	return []CommentEvent{c}, nil
})

func (c AddSubjectComment) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c AddSubjectComment) ApplyResult(tx *writer.Tx) ([]byte, error) {
	sub, ok := subjects[c.Subject]
	if !ok {
		return nil, notFound("comment subject", c.Subject)
	}
	if err := requireOn(tx, c.By, access.Comment, CommentState{Subject: c.Subject, SubjectID: c.SubjectID}); err != nil {
		return nil, err
	}
	if c.Parent != "" {
		p, err := commentState(tx, c.Parent)
		switch {
		case !sub.Replies:
			return nil, tabs.Refuse("body", "Comments here take no replies.")
		case err != nil || p.Subject != c.Subject || p.SubjectID != c.SubjectID:
			return nil, tabs.Refuse("body", "That comment is not here any more.")
		case p.Parent != "":
			return nil, tabs.Refuse("body", "Replies go one level deep: reply to the comment above.")
		}
	}
	info, err := commentBody(c.Body)
	if err != nil {
		return nil, err
	}
	if c.ID == "" {
		c.ID = NewID()
	}
	e := CommentAdded{Subject: c.Subject, SubjectID: c.SubjectID, Parent: c.Parent, Body: canonical(c.Body), Mentions: info.Mentions}
	if err := events.Execute(Store(), tx, addSubjectComment, CommentStream.ID(c.ID), events.NoStream, e, meta(c.By)); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": c.ID})
}
