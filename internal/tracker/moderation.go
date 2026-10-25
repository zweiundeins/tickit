package tracker

import (
	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Moderation of issues (DM-07.1): a moderator or a site admin hides an
// issue, and restores it. A hidden issue leaves every list, board and
// search as a deleted one does (the issues projector sets deleted_at), and
// its key answers that it is gone; restoring brings it back as it was.
// Comments have HideComment.

// IssueHidden hides an issue; IssueUnhidden restores it.
type IssueHidden struct {
	Reason string `json:"reason,omitempty"`
}
type IssueUnhidden struct{}

func (IssueHidden) issueEvent()   {}
func (IssueUnhidden) issueEvent() {}

var (
	IssueHiddenType   = defineIssue[IssueHidden]("issue.hidden")
	IssueUnhiddenType = defineIssue[IssueUnhidden]("issue.unhidden")
)

// HideIssue hides an issue, or restores it when Hidden is false: a
// moderator's or a site admin's right (access.Moderate). A deleted issue
// stays deleted.
type HideIssue struct {
	By     tabs.By
	Issue  string
	Hidden bool
	Reason string
}

func (c HideIssue) Validate() error { return checkBy(c.By) }

var hideIssueDecider = issueDecider(func(c HideIssue, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted:
		return nil, errDeleted
	case s.Hidden == c.Hidden:
		return nil, nil
	case c.Hidden:
		reason, ok := text(c.Reason, 0, 200)
		if !ok {
			return nil, tabs.Refuse("reason", "A reason has at most 200 characters.")
		}
		return []IssueEvent{IssueHidden{Reason: reason}}, nil
	}
	return []IssueEvent{IssueUnhidden{}}, nil
})

func (c HideIssue) Apply(tx *writer.Tx) error {
	s, err := issueState(tx, c.Issue)
	if err != nil {
		return err
	}
	if err := access.Require(tx, c.By.Actor, access.Moderate, access.Scope{}); err != nil {
		return err
	}
	if err := events.Execute(Store(), tx, hideIssueDecider, IssueStream.ID(c.Issue), events.Any, c, meta(c.By)); err != nil {
		return err
	}
	if c.Hidden {
		return descDocs.Release(tx, c.Issue)
	}
	return descDocs.Hold(tx, c.Issue, s.DescUpto)
}

// projectHidden and projectUnhidden are the issues projector's handlers.
func projectHidden(tx *writer.Tx, r events.Record[string, IssueHidden]) error {
	if _, err := tx.Exec(`UPDATE issues SET hidden_at = ?, hidden_by = ?, hidden_reason = ?, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		millis(r.At), r.Actor, r.Data.Reason, millis(r.At), r.Actor, r.ID); err != nil {
		return err
	}
	return finish(tx, r.ID, r.Version, millis(r.At), false)
}

func projectUnhidden(tx *writer.Tx, r events.Record[string, IssueUnhidden]) error {
	if _, err := tx.Exec(`UPDATE issues SET hidden_at = NULL, hidden_by = '', hidden_reason = '', deleted_at = NULL, deleted_by = '' WHERE id = ?`, r.ID); err != nil {
		return err
	}
	return finish(tx, r.ID, r.Version, millis(r.At), false)
}
