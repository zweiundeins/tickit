package wiki

import (
	"encoding/json"
	"errors"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/tracker"
)

// Page comments (FR-90) are the tracker's comments with the subject
// "page": one stream, one table, one search index and one set of
// notifications for the comments on issues and on pages. The wiki
// registers the subject (its scope is the page's space; replies go one
// level deep) and keeps the threads (page_comment_threads). Edit, delete
// and hide them with tracker.EditComment, DeleteComment and HideComment.
func init() {
	tracker.RegisterSubject(tracker.SubjectPage, tracker.Subject{
		Scope: func(tx *writer.Tx, id string) (access.Scope, error) {
			s, err := pageState(tx, id)
			if err != nil {
				return access.Scope{}, err
			}
			return access.SpaceScope(s.Space), nil
		},
		Topics:  func(id string) []string { return []string{PageTopic(id), PagesTopic} }, // PagesTopic: the feeds
		Replies: true,
	})
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

func (c AddComment) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c AddComment) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if _, err := current(tx, c.By, c.Page, access.Comment); err != nil {
		return nil, err
	}
	return tracker.AddSubjectComment{By: c.By, ID: c.ID, Subject: tracker.SubjectPage, SubjectID: c.Page, Parent: c.Parent, Body: c.Body}.ApplyResult(tx)
}

const threadsVersion = 1

// threadsProjector keeps page_comment_threads from the comment stream.
var threadsProjector = events.NewProjector(tracker.CommentStream, "wiki_threads", threadsVersion,
	events.Handle(tracker.CommentAddedType, func(tx *writer.Tx, r events.Record[string, tracker.CommentAdded]) error {
		if r.Data.Subject != tracker.SubjectPage {
			return nil
		}
		_, err := tx.Exec(`INSERT INTO page_comment_threads (comment_id, page_id, parent_id) VALUES (?, ?, ?)`, r.ID, r.Data.SubjectID, r.Data.Parent)
		return err
	}),
	events.Ignore(tracker.CommentEditedType),
	events.Ignore(tracker.CommentDeletedType),
	events.Ignore(tracker.CommentHiddenType),
	events.Ignore(tracker.CommentUnhiddenType),
).Tables("page_comment_threads")
