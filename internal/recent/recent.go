// Package recent keeps what each person viewed lately (FR-59.1): the
// newest Keep issues and pages per person, which the home page lists as
// "Recently viewed". GET never writes, so a page that opens an issue or a
// page posts the view once (Record, through tabs.Act); Sweep drops what
// is older than TTL. The rows are a person's own and relational: no
// event, no projection.
package recent

import (
	"context"
	"errors"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/tabs"
)

// The kinds of what a person views.
const (
	Issue = "issue"
	Page  = "page"
)

// Keep is how many views a person keeps; TTL how long one stays.
const (
	Keep = 20
	TTL  = 30 * 24 * time.Hour
)

// Topic is a person's recently viewed list, which the home page's view of
// that person listens to: a root of its own, so a view renders nothing
// else.
func Topic(person string) string { return "recent:" + person }

// Record notes that the actor viewed an issue or a page now. The caller
// checks that they may read it.
type Record struct {
	By      tabs.By
	Kind    string
	Subject string
}

func (c Record) Validate() error {
	if c.Kind != Issue && c.Kind != Page {
		return errors.New("recent: a view of an issue or a page")
	}
	if c.Subject == "" || len(c.Subject) > 64 {
		return errors.New("recent: a malformed subject")
	}
	if c.By.Actor.IsZero() || c.By.At.IsZero() {
		return errors.New("recent: a view needs an actor and a time")
	}
	return nil
}

func (c Record) Apply(tx *writer.Tx) error {
	p := c.By.Actor.ID
	if _, err := tx.Exec(`INSERT INTO recent_views (person_id, subject_id, kind, seen_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (person_id, subject_id) DO UPDATE SET seen_at = excluded.seen_at, kind = excluded.kind`, p, c.Subject, c.Kind, c.By.At.UnixMilli()); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM recent_views WHERE person_id = ?1 AND subject_id NOT IN
		(SELECT subject_id FROM recent_views WHERE person_id = ?1 ORDER BY seen_at DESC LIMIT ?2)`, p, Keep); err != nil {
		return err
	}
	tx.Touch(Topic(p))
	return nil
}

// Seen is a view as the home page lists it.
type Seen struct {
	Kind    string
	Subject string
	At      time.Time
}

// List is a person's views, newest first.
func List(ctx context.Context, q sqlite.Querier, person string) ([]Seen, error) {
	rows, err := q.QueryContext(ctx, `SELECT kind, subject_id, seen_at FROM recent_views WHERE person_id = ? ORDER BY seen_at DESC LIMIT ?`, person, Keep)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Seen
	for rows.Next() {
		var s Seen
		var at int64
		if err := rows.Scan(&s.Kind, &s.Subject, &at); err != nil {
			return nil, err
		}
		s.At = time.UnixMilli(at)
		out = append(out, s)
	}
	return out, rows.Err()
}

// Sweep drops views older than TTL and those of people who are gone; the
// hourly sweep sends it.
type Sweep struct{ At time.Time }

func (c Sweep) Apply(tx *writer.Tx) error {
	if _, err := tx.Exec(`DELETE FROM recent_views WHERE seen_at < ?`, c.At.Add(-TTL).UnixMilli()); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM recent_views WHERE person_id NOT IN (SELECT id FROM people)`)
	return err
}
