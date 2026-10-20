package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Reports (DM-08): any visitor reports an issue, a comment, a page, a
// page's version or an image with a reason; the moderation page lists the
// open reports first. A moderator's hide or delete of the thing closes its
// reports, and a moderator dismisses one that needs nothing done.

// Outcomes of a closed report.
const (
	ReportActed     = "acted"     // the thing was hidden or deleted
	ReportDismissed = "dismissed" // a moderator saw nothing to do
)

// Report is a visitor's report of a thing of a moderated kind.
type Report struct {
	By     tabs.By
	Kind   string
	ID     string
	Reason string
}

func (c Report) Validate() error {
	if c.By.Actor.IsZero() {
		return errors.New("demo: a report needs a reporter")
	}
	if _, ok := moderatedKind(c.Kind); !ok || c.ID == "" {
		return fmt.Errorf("demo: nothing to report of kind %q", c.Kind)
	}
	return nil
}

func (c Report) Apply(tx *writer.Tx) error {
	reason := strings.TrimSpace(c.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 200 {
		return tabs.Refuse("reason", "Say why, in at most 200 characters.")
	}
	m, _ := moderatedKind(c.Kind)
	if m.Scope != nil { // what the reporter may not read is not there for them
		sc, err := m.Scope(tx, c.ID)
		if err != nil {
			return err
		}
		if sc != (access.Scope{}) {
			if err := access.Require(tx, c.By.Actor, access.Read, sc); err != nil {
				return err
			}
		}
	}
	label, err := m.Label(tx, c.ID)
	if err != nil {
		return err
	}
	at := c.By.At
	if at.IsZero() {
		at = time.Now()
	}
	if _, err := tx.Exec(`INSERT INTO reports (kind, target_id, label, reason, by, at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (kind, target_id, by) WHERE closed_at IS NULL DO UPDATE SET reason = excluded.reason, at = excluded.at`,
		c.Kind, c.ID, clip(label, 200), reason, c.By.Actor.ID, at.Unix()); err != nil {
		return err
	}
	tx.Touch(ModerationTopic)
	return nil
}

// CloseReport dismisses an open report: a moderator's or a site admin's.
type CloseReport struct {
	By     tabs.By
	Report int64
}

func (c CloseReport) Validate() error {
	if c.By.Actor.IsZero() || c.Report < 1 {
		return errors.New("demo: dismiss names a report")
	}
	return nil
}

func (c CloseReport) Apply(tx *writer.Tx) error {
	if err := access.Require(tx, c.By.Actor, access.Moderate, access.Scope{}); err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE reports SET closed_at = ?, closed_by = ?, outcome = ? WHERE id = ? AND closed_at IS NULL`,
		unixOr(c.By.At), c.By.Actor.ID, ReportDismissed, c.Report)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("report %d: %w", c.Report, access.ErrNotFound)
	}
	tx.Touch(ModerationTopic)
	return nil
}

// closeReports closes the open reports of a thing a moderator acted on.
func closeReports(tx *writer.Tx, by tabs.By, kind, id string) error {
	_, err := tx.Exec(`UPDATE reports SET closed_at = ?, closed_by = ?, outcome = ? WHERE kind = ? AND target_id = ? AND closed_at IS NULL`,
		unixOr(by.At), by.Actor.ID, ReportActed, kind, id)
	return err
}

func unixOr(t time.Time) int64 {
	if t.IsZero() {
		t = time.Now()
	}
	return t.Unix()
}

// OpenReport is an open report, as the moderation page lists it.
type OpenReport struct {
	ID                  int64
	Kind, Target, Label string
	Reason, By          string
	At                  time.Time
}

// OpenReports are the open reports, the oldest first: they wait longest.
func OpenReports(ctx context.Context, q sqlite.Querier, limit int) ([]OpenReport, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, kind, target_id, label, reason, by, at FROM reports WHERE closed_at IS NULL ORDER BY at, id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OpenReport
	for rows.Next() {
		var r OpenReport
		var at int64
		if err := rows.Scan(&r.ID, &r.Kind, &r.Target, &r.Label, &r.Reason, &r.By, &at); err != nil {
			return nil, err
		}
		r.At = time.Unix(at, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}
