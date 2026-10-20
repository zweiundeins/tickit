package tracker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/tabs"
)

// What an issue's page reads besides IssueDetail: the workflow its status
// menu offers, and who types where (MP-07).

// ProjectWorkflow reads a project's workflow, for the transitions an
// issue's status menu offers (FR-16.1): from the project's state row when
// it is current, else folded from its stream.
func ProjectWorkflow(ctx context.Context, q sqlite.Querier, project string) (Workflow, error) {
	var state string
	err := q.QueryRowContext(ctx, `SELECT state FROM lith_event_state WHERE stream = ? AND stream_id = ? AND fold = ?`,
		ProjectStream.Kind(), project, projectFold).Scan(&state)
	if err == nil {
		var s ProjectState
		if json.Unmarshal([]byte(state), &s) == nil && s.Exists {
			return s.Workflow, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Workflow{}, err
	}
	s, _, err := events.StateAt(ctx, Store(), q, peekProjectDecider, ProjectStream.ID(project), 0)
	if err != nil {
		return Workflow{}, err
	}
	if !s.Exists {
		return Workflow{}, notFound("project", project)
	}
	return s.Workflow, nil
}

// The fields people type in, for Typing.
const (
	TypingSummary     = "summary"
	TypingDescription = "description"
	TypingComment     = "comment"
)

// TypingFor is how long an indicator stays after the last keystroke
// (MP-07.4).
const TypingFor = 5 * time.Second

// TypingTopic is touched when someone starts or stops typing on an issue.
func TypingTopic(issue string) string { return "issue-typing:" + issue }

// SetTyping says that the actor types in a field of an issue now, or,
// with Stop, that they closed it (MP-07). It writes no event: typing is
// no change to the issue. Anyone who may read the issue may type.
type SetTyping struct {
	By    tabs.By
	Issue string
	Field string
	Stop  bool
}

func (c SetTyping) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if !ValidID(c.Issue) {
		return fmt.Errorf("%w: no issue id", writer.ErrInvalid)
	}
	switch c.Field {
	case TypingSummary, TypingDescription, TypingComment:
		return nil
	}
	return fmt.Errorf("%w: no such field %q", writer.ErrInvalid, c.Field)
}

func (c SetTyping) Apply(tx *writer.Tx) error {
	var project string
	if err := tx.QueryRow(`SELECT project_id FROM issues WHERE id = ? AND deleted_at IS NULL`, c.Issue).Scan(&project); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("issue", c.Issue)
		}
		return err
	}
	if err := require(tx, c.By, access.Read, project); err != nil {
		return err
	}
	now := at(c.By).UnixMilli()
	if _, err := tx.Exec(`DELETE FROM issue_typing WHERE issue_id = ? AND at < ?`, c.Issue, now-time.Minute.Milliseconds()); err != nil {
		return err
	}
	var err error
	if c.Stop {
		_, err = tx.Exec(`DELETE FROM issue_typing WHERE issue_id = ? AND person_id = ? AND field = ?`, c.Issue, c.By.Actor.ID, c.Field)
	} else {
		_, err = tx.Exec(`INSERT INTO issue_typing (issue_id, person_id, field, at) VALUES (?, ?, ?, ?)
			ON CONFLICT (issue_id, person_id, field) DO UPDATE SET at = excluded.at`, c.Issue, c.By.Actor.ID, c.Field, now)
	}
	if err != nil {
		return err
	}
	tx.Touch(TypingTopic(c.Issue))
	return nil
}

// Typist is someone typing in a field of an issue.
type Typist struct {
	Person string
	Field  string
	At     time.Time
}

// Typing lists who typed in a field of an issue within TypingFor before
// now, by field, then in the order they started.
func Typing(ctx context.Context, q sqlite.Querier, issue string, now time.Time) ([]Typist, error) {
	rows, err := q.QueryContext(ctx, `SELECT person_id, field, at FROM issue_typing WHERE issue_id = ? AND at >= ? ORDER BY field, at`,
		issue, now.Add(-TypingFor).UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Typist
	for rows.Next() {
		var t Typist
		var ms int64
		if err := rows.Scan(&t.Person, &t.Field, &ms); err != nil {
			return nil, err
		}
		t.At = time.UnixMilli(ms)
		out = append(out, t)
	}
	return out, rows.Err()
}

// RefByKey returns the issue a key names now, as a chip shows it (FR-100):
// nil for a key that names no issue. An old key names the moved issue.
func RefByKey(ctx context.Context, q sqlite.Querier, key string) (*IssueRef, error) {
	k, err := IssueByKey(ctx, q, key)
	if errors.Is(err, access.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ref(ctx, q, k.Issue)
}
