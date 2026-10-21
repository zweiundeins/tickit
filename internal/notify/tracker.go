package notify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/tracker"
)

// The reactions to the tracker's streams. Each handler sends one
// reaction, whose Build reads the issue as the log held it at the
// event's position.

// issueReactions notify assignees (FR-111), the people a description
// mentions (FR-112) and an issue's watchers of status changes (FR-113).
var issueReactions = events.NewProjector(tracker.IssueStream, "notify_issues", 1,
	events.After(tracker.IssueCreatedType, func(ctx context.Context, r events.Record[string, tracker.IssueCreated]) error {
		e := r.Data
		return react(ctx, r.Position, r.Meta, func(tx *writer.Tx) (Subject, []item, error) {
			var items []item
			if e.Assignee != "" {
				items = append(items, item{Recipient: e.Assignee, Kind: Assigned})
			}
			return issueSubject(r.ID, e.Key), append(items, mentions(e.Mentions, e.Description)...), nil
		})
	}),
	events.After(tracker.AssigneeChangedType, func(ctx context.Context, r events.Record[string, tracker.AssigneeChanged]) error {
		if r.Data.To == "" {
			return nil
		}
		return react(ctx, r.Position, r.Meta, func(tx *writer.Tx) (Subject, []item, error) {
			is, err := issueAt(tx, r.ID, r.Position)
			return is.subject, []item{{Recipient: r.Data.To, Kind: Assigned}}, err
		})
	}),
	events.After(tracker.DescriptionChangedType, func(ctx context.Context, r events.Record[string, tracker.DescriptionChanged]) error {
		if len(r.Data.Mentions) == 0 {
			return nil
		}
		return react(ctx, r.Position, r.Meta, func(tx *writer.Tx) (Subject, []item, error) {
			is, err := issueAt(tx, r.ID, r.Position)
			return is.subject, mentions(r.Data.Mentions, r.Data.To), err
		})
	}),
	events.After(tracker.StatusChangedType, func(ctx context.Context, r events.Record[string, tracker.StatusChanged]) error {
		return statusChanged(ctx, r.ID, r.Position, r.Meta, r.Data.To)
	}),
	events.After(tracker.MovedType, func(ctx context.Context, r events.Record[string, tracker.Moved]) error {
		if r.Data.Status == r.Data.FromStatus {
			return nil // a new rank in the same column
		}
		return statusChanged(ctx, r.ID, r.Position, r.Meta, r.Data.Status)
	}),
	events.Ignore(tracker.SummaryChangedType),
	events.Ignore(tracker.CategoryChangedType),
	events.Ignore(tracker.TypeChangedType),
	events.Ignore(tracker.PriorityChangedType),
	events.Ignore(tracker.ParentChangedType),
	events.Ignore(tracker.DueChangedType),
	events.Ignore(tracker.PointsChangedType),
	events.Ignore(tracker.EstimateChangedType),
	events.Ignore(tracker.RemainingChangedType),
	events.Ignore(tracker.SprintChangedType),
	events.Ignore(tracker.LabelsChangedType),
	events.Ignore(tracker.ComponentsChangedType),
	events.Ignore(tracker.WatchersAddedType),
	events.Ignore(tracker.WatcherRemovedType),
	events.Ignore(tracker.LinkAddedType),
	events.Ignore(tracker.PageLinkedType),
	events.Ignore(tracker.PageUnlinkedType),
	events.Ignore(tracker.LinkRemovedType),
	events.Ignore(tracker.IssueDeletedType),
	events.Ignore(tracker.ProjectChangedType),
	events.Ignore(tracker.IssueHiddenType),
	events.Ignore(tracker.IssueUnhiddenType),
).SkipHistory()

// commentReactions notify the people a comment mentions (FR-112) and the
// watchers of what it comments on (FR-113, FR-114).
var commentReactions = events.NewProjector(tracker.CommentStream, "notify_comments", 1,
	events.After(tracker.CommentAddedType, func(ctx context.Context, r events.Record[string, tracker.CommentAdded]) error {
		e := r.Data
		on, ok := commentables[e.Subject]
		if !ok {
			return nil // a subject this package does not know yet
		}
		return react(ctx, r.Position, r.Meta, func(tx *writer.Tx) (Subject, []item, error) {
			s, watching, err := on.At(tx, e.SubjectID, r.Position)
			if err != nil {
				return s, nil, err
			}
			items := mentions(e.Mentions, e.Body)
			return s, append(items, watchers(watching, on.Kind, Excerpt(e.Body), Detail{Comments: 1})...), nil
		})
	}),
	events.After(tracker.CommentEditedType, func(ctx context.Context, r events.Record[string, tracker.CommentEdited]) error {
		if len(r.Data.Mentions) == 0 {
			return nil
		}
		return react(ctx, r.Position, r.Meta, func(tx *writer.Tx) (Subject, []item, error) {
			kind, id, err := commentSubject(tx, r.ID, r.Position)
			if err != nil {
				return Subject{}, nil, err
			}
			on, ok := commentables[kind]
			if !ok {
				return Subject{}, nil, nil
			}
			s, _, err := on.At(tx, id, r.Position)
			return s, mentions(r.Data.Mentions, r.Data.Body), err
		})
	}),
	events.Ignore(tracker.CommentDeletedType),
	events.Ignore(tracker.CommentHiddenType),
	events.Ignore(tracker.CommentUnhiddenType),
).SkipHistory()

// commentable is a kind of subject that comments are on.
type commentable struct {
	// Kind is the notification its watchers get of a comment.
	Kind Kind
	// At names the subject and lists its watchers as of a position.
	At func(tx *writer.Tx, id string, upto int64) (Subject, []string, error)
}

// commentables are the subjects of comments, by tracker.CommentAdded's
// Subject: issues here, pages from wiki.go.
var commentables = map[string]commentable{
	tracker.SubjectIssue: {Kind: Commented, At: func(tx *writer.Tx, id string, upto int64) (Subject, []string, error) {
		is, err := issueAt(tx, id, upto)
		return is.subject, is.watchers, err
	}},
}

// react sends the reaction to the event at position, once.
func react(ctx context.Context, position int64, m events.Meta, build func(tx *writer.Tx) (Subject, []item, error)) error {
	at := m.At
	if at.IsZero() {
		at = time.Now()
	}
	return deliver(ctx, position, reaction{Position: position, At: at, Actor: m.Actor, Build: build})
}

// statusChanged tells an issue's watchers of its new status.
func statusChanged(ctx context.Context, id string, position int64, m events.Meta, status string) error {
	return react(ctx, position, m, func(tx *writer.Tx) (Subject, []item, error) {
		is, err := issueAt(tx, id, position)
		if err != nil {
			return is.subject, nil, err
		}
		name, err := statusName(tx.Context(), tx, is.project, status)
		return is.subject, watchers(is.watchers, StatusChanged, "", Detail{Status: name}), err
	})
}

func issueSubject(id, key string) Subject {
	return Subject{Kind: SubjectIssue, ID: id, Label: key, Href: IssueHref(key)}
}

// issueThen is an issue as its stream held it at a position.
type issueThen struct {
	subject  Subject
	project  string
	watchers []string
}

// issueAt folds an issue's events up to a position: its key, project and
// watchers then, whatever happened since (a replay decides the same).
func issueAt(tx *writer.Tx, id string, upto int64) (issueThen, error) {
	var is issueThen
	var key string
	for r, err := range events.LoadUntil(tx.Context(), tracker.Store(), tx, tracker.IssueStream.ID(id), 0, upto) {
		if err != nil {
			return is, err
		}
		switch e := r.Data.(type) {
		case tracker.IssueCreated:
			key, is.project, is.watchers = e.Key, e.Project, slices.Clone(e.Watchers)
		case tracker.ProjectChanged:
			key, is.project = e.Key, e.Project
		case tracker.WatchersAdded:
			for _, p := range e.People {
				if !slices.Contains(is.watchers, p) {
					is.watchers = append(is.watchers, p)
				}
			}
		case tracker.WatcherRemoved:
			is.watchers = slices.DeleteFunc(is.watchers, func(p string) bool { return p == e.Person })
		}
	}
	if key == "" {
		return is, fmt.Errorf("notify: issue %s has no events up to %d", id, upto)
	}
	is.subject = issueSubject(id, key)
	return is, nil
}

// commentSubject is what a comment comments on, from its first event.
func commentSubject(tx *writer.Tx, id string, upto int64) (kind, subject string, err error) {
	for r, err := range events.LoadUntil(tx.Context(), tracker.Store(), tx, tracker.CommentStream.ID(id), 0, upto) {
		if err != nil {
			return "", "", err
		}
		if e, ok := r.Data.(tracker.CommentAdded); ok {
			return e.Subject, e.SubjectID, nil
		}
	}
	return "", "", fmt.Errorf("notify: comment %s has no events up to %d", id, upto)
}

// statusName is a status's name in a project's workflow, its id when the
// project no longer has it.
func statusName(ctx context.Context, q sqlite.Querier, project, status string) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT name FROM statuses WHERE project_id = ? AND id = ?`, project, status).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return tracker.DefaultWorkflow().Name(status), nil
	}
	return name, err
}
