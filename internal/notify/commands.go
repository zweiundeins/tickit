package notify

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
)

// The person's actions on their notifications (FR-115). Each takes the
// actor from tabs.By; a notification of someone else's is refused with
// an *access.Denial (403), an unknown one answers access.ErrNotFound.

// Read marks one notification read.
type Read struct {
	By tabs.By
	ID int64
}

// ReadAll marks every notification of the actor read.
type ReadAll struct{ By tabs.By }

// Dismiss takes one notification out of the panel; the sweep deletes it.
type Dismiss struct {
	By tabs.By
	ID int64
}

// DismissAll dismisses every notification of the actor.
type DismissAll struct{ By tabs.By }

// Open marks a notification read as its subject opens from the panel
// (FR-115.2). Its result is where to go, as JSON: {"href": "/WEB-12"}.
type Open struct {
	By tabs.By
	ID int64
}

// Seen marks read the actor's notifications about a subject that a page
// opened (FR-115.2): the page posts it as it opens, since GET never
// writes.
type Seen struct {
	By      tabs.By
	Subject string
}

func (c Read) Validate() error       { return checkBy(c.By) }
func (c ReadAll) Validate() error    { return checkBy(c.By) }
func (c Dismiss) Validate() error    { return checkBy(c.By) }
func (c DismissAll) Validate() error { return checkBy(c.By) }
func (c Open) Validate() error       { return checkBy(c.By) }

func (c Seen) Validate() error {
	if c.Subject == "" || len(c.Subject) > 100 {
		return errors.New("notify: a malformed subject")
	}
	return checkBy(c.By)
}

func checkBy(by tabs.By) error {
	if by.Actor.IsZero() || by.At.IsZero() {
		return errors.New("notify: an action needs an actor and a time")
	}
	return nil
}

// errNotYours is the refusal of an action on someone else's notification.
var errNotYours = &access.Denial{Refusal: writer.Refusal{Msg: "This notification is someone else's."}}

// own checks that a notification is the actor's, and returns its href.
func own(tx *writer.Tx, by tabs.By, id int64) (string, error) {
	var recipient, href string
	err := tx.QueryRow(`SELECT recipient, href FROM notifications WHERE id = ? AND dismissed_at IS NULL`, id).Scan(&recipient, &href)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", fmt.Errorf("notification %d: %w", id, access.ErrNotFound)
	case err != nil:
		return "", err
	case recipient != by.Actor.ID:
		return "", errNotYours
	}
	return href, nil
}

func (c Read) Apply(tx *writer.Tx) error {
	if _, err := own(tx, c.By, c.ID); err != nil {
		return err
	}
	return update(tx, c.By, `UPDATE notifications SET read_at = ? WHERE id = ? AND read_at IS NULL`, c.By.At.UnixMilli(), c.ID)
}

func (c ReadAll) Apply(tx *writer.Tx) error {
	return update(tx, c.By, `UPDATE notifications SET read_at = ? WHERE recipient = ? AND read_at IS NULL AND dismissed_at IS NULL`,
		c.By.At.UnixMilli(), c.By.Actor.ID)
}

func (c Dismiss) Apply(tx *writer.Tx) error {
	if _, err := own(tx, c.By, c.ID); err != nil {
		return err
	}
	return update(tx, c.By, `UPDATE notifications SET dismissed_at = ? WHERE id = ?`, c.By.At.UnixMilli(), c.ID)
}

func (c DismissAll) Apply(tx *writer.Tx) error {
	return update(tx, c.By, `UPDATE notifications SET dismissed_at = ? WHERE recipient = ? AND dismissed_at IS NULL`,
		c.By.At.UnixMilli(), c.By.Actor.ID)
}

func (c Open) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c Open) ApplyResult(tx *writer.Tx) ([]byte, error) {
	href, err := own(tx, c.By, c.ID)
	if err != nil {
		return nil, err
	}
	err = update(tx, c.By, `UPDATE notifications SET read_at = ? WHERE id = ? AND read_at IS NULL`, c.By.At.UnixMilli(), c.ID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"href": href})
}

func (c Seen) Apply(tx *writer.Tx) error {
	return update(tx, c.By, `UPDATE notifications SET read_at = ? WHERE recipient = ? AND subject_id = ? AND read_at IS NULL AND dismissed_at IS NULL`,
		c.By.At.UnixMilli(), c.By.Actor.ID, c.Subject)
}

// update runs a change of the actor's notifications and touches their
// topic when it changed any.
func update(tx *writer.Tx, by tabs.By, query string, args ...any) error {
	res, err := tx.Exec(query, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		tx.Touch(people.PersonTopic(by.Actor.ID))
	}
	return nil
}

// Sweep keeps the notifications bounded (FR-117): it deletes the
// dismissed ones, the read ones older than ReadTTL, and each person's
// beyond the newest Keep. cmd/tickit runs it with its hourly sweep.
type Sweep struct{ At time.Time }

func (c Sweep) Apply(tx *writer.Tx) error {
	if _, err := tx.Exec(`DELETE FROM notifications WHERE dismissed_at IS NOT NULL`); err != nil {
		return err
	}
	old := c.At.Add(-ReadTTL).UnixMilli()
	stale, err := recipients(tx, `SELECT DISTINCT recipient FROM notifications WHERE read_at IS NOT NULL AND read_at < ?`, old)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notifications WHERE read_at IS NOT NULL AND read_at < ?`, old); err != nil {
		return err
	}
	for _, p := range stale {
		tx.Touch(people.PersonTopic(p))
	}
	// The newest Keep of each person stay, by updated_at.
	over, err := recipients(tx, `SELECT recipient FROM notifications GROUP BY recipient HAVING count(*) > ?`, Keep)
	if err != nil {
		return err
	}
	for _, p := range over {
		_, err := tx.Exec(`DELETE FROM notifications WHERE recipient = ?1 AND id NOT IN
			(SELECT id FROM notifications WHERE recipient = ?1 ORDER BY updated_at DESC, id DESC LIMIT ?2)`, p, Keep)
		if err != nil {
			return err
		}
		tx.Touch(people.PersonTopic(p))
	}
	return nil
}

// recipients runs a query of recipients.
func recipients(tx *writer.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
