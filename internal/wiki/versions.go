package wiki

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// When versions are cut (FR-87.1, docs/editor.md): after a minute without
// edits, when the last editor leaves, and at least every 10 minutes of
// continuous editing.
const (
	CutIdle     = 60 * time.Second
	CutInterval = 10 * time.Minute
)

var cutDecider = pageDecider(func(c VersionCut, s PageState) ([]PageEvent, error) {
	if !s.Exists {
		return nil, errors.New("wiki: no such page")
	}
	c.Number = s.Versions + 1
	return []PageEvent{c}, nil
})

// cutVersion cuts the next version of a page from its head document, if
// anything changed since the last one; save makes "nothing changed" a
// refusal. It reports whether it cut.
func cutVersion(tx *writer.Tx, m events.Meta, page string, s PageState, message, reason string, save bool) (bool, error) {
	ctx := tx.Context()
	ds, err := docState(tx, page)
	if err != nil {
		return false, err
	}
	if ds.Version == s.CutUpto && s.Title == s.CutTitle {
		if save {
			return false, &writer.Refusal{Msg: fmt.Sprintf("Nothing changed since version %d.", s.Versions)}
		}
		return false, pageDocs.Cut(tx, page, ds.Version) // a title changed back leaves its row
	}
	head, err := pageDocs.StateDoc(ctx, tx, page, ds)
	if err != nil {
		return false, err
	}
	editors, err := editorsSince(tx, page, s)
	if err != nil {
		return false, err
	}
	if len(editors) == 0 && m.Actor != "" {
		editors = []string{m.Actor}
	}
	if m.Actor == "" && len(editors) > 0 {
		m.Actor = editors[len(editors)-1]
	}
	cut := VersionCut{Upto: ds.Version, Title: s.Title, Doc: marshalDoc(head), Editors: editors, Message: message, Reason: reason}
	if err := events.Execute(Store(), tx, cutDecider, PageStream.ID(page), events.Any, cut, m); err != nil {
		return false, err
	}
	return true, pageDocs.Cut(tx, page, ds.Version)
}

// editorsSince are the people who committed to the document, or renamed
// the page, since its last version, in the order they first did.
func editorsSince(tx *writer.Tx, page string, s PageState) ([]string, error) {
	rows, err := tx.Query(`SELECT actor FROM (
		SELECT actor, position FROM lith_events WHERE stream = 'pagedoc' AND stream_id = ? AND version > ? AND actor != ''
		UNION ALL SELECT actor, position FROM lith_events WHERE stream = 'page' AND stream_id = ? AND version > ? AND type = 'page.renamed' AND actor != '')
		GROUP BY actor ORDER BY min(position)`, page, s.CutUpto, page, s.CutAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CutVersion is "Save version" (FR-87.1): a version now, with an
// optional message. Nothing new since the last version is refused.
type CutVersion struct {
	By      tabs.By
	Page    string
	Message string
}

func (c CutVersion) Validate() error { return checkBy(c.By) }

func (c CutVersion) Apply(tx *writer.Tx) error {
	s, err := current(tx, c.By, c.Page, access.EditIssues)
	if err != nil {
		return err
	}
	msg, ok := text(c.Message, 0, MaxMessage)
	if !ok {
		return tabs.Refuse("message", "A message has at most 200 characters.")
	}
	_, err = cutVersion(tx, meta(c.By), c.Page, s, msg, "save", true)
	return err
}

// commitWhole replaces a page's document with d in one commit made on the
// head (collab.CommitWhole): open editors keep their places outside what
// changed. The client is "" (no tab); ref names it.
func commitWhole(tx *writer.Tx, by tabs.By, page string, d doc.Doc, ref string) error {
	if by.At.IsZero() {
		by.At = at(by)
	}
	_, err := pageDocs.CommitWhole(tx, by, page, d, ref, "body", nil)
	return err
}

// RestoreVersion makes an old version the page's document again (FR-89):
// one commit that open editors apply like any other, then a version with
// the message "Restored version 3". A Member does.
type RestoreVersion struct {
	By     tabs.By
	Page   string
	Number int
}

func (c RestoreVersion) Validate() error { return checkBy(c.By) }

func (c RestoreVersion) Apply(tx *writer.Tx) error {
	s, err := current(tx, c.By, c.Page, access.RestoreVersion)
	if err != nil {
		return err
	}
	var raw, title string
	var hidden bool
	err = tx.QueryRow(`SELECT doc, title, hidden_at IS NOT NULL FROM page_versions WHERE page_id = ? AND number = ?`, c.Page, c.Number).Scan(&raw, &title, &hidden)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("version", fmt.Sprint(c.Number))
	}
	if err != nil {
		return err
	}
	if hidden && !access.SiteCan(c.By.Actor, access.Moderate) { // DM-07.1
		return errVersionHidden
	}
	old, err := doc.ParseDoc([]byte(raw))
	if err != nil {
		return err
	}
	if err := commitWhole(tx, c.By, c.Page, old, "restore-"+c.By.Op); err != nil {
		return err
	}
	_, err = cutVersion(tx, meta(c.By), c.Page, s, fmt.Sprintf("Restored version %d", c.Number), "restore", false)
	return err
}

// SetDocument gives a page a whole new document as one commit and cuts a
// version with Message: the seed's versions of a page, a page made from
// a template after the fact. A Member does.
type SetDocument struct {
	By      tabs.By
	Page    string
	Doc     json.RawMessage
	Message string
}

func (c SetDocument) Validate() error { return checkBy(c.By) }

func (c SetDocument) Apply(tx *writer.Tx) error {
	s, err := current(tx, c.By, c.Page, access.EditIssues)
	if err != nil {
		return err
	}
	d, err := parseDoc(c.Doc)
	if err != nil {
		return err
	}
	if err := commitWhole(tx, c.By, c.Page, d, "set-"+c.By.Op); err != nil {
		return err
	}
	reason := "save"
	if c.By.Actor.Kind == people.Seed {
		reason = "seed"
	}
	msg, _ := text(c.Message, 0, MaxMessage)
	_, err = cutVersion(tx, meta(c.By), c.Page, s, msg, reason, false)
	return err
}

// CutDue cuts the versions that are due (FR-87.1): of each page with
// changes since its last version, after CutIdle without a commit, at once
// when nobody edits it any more (Editing false), or when its first change
// since the last version is CutInterval old. The pages are those with a
// row in collab_uncut: a commit or a rename marks one, a version clears
// it. The job sends it every few seconds; it cuts at most 20 pages a run.
type CutDue struct {
	At time.Time
	// Editing reports whether someone has the page open in the editor;
	// nil: everyone left.
	Editing func(page string) bool
}

// Due is a page with changes since its last version.
type due struct {
	page            string
	lastAt, firstAt int64
}

func (c CutDue) Apply(tx *writer.Tx) error {
	rows, err := tx.Query(`SELECT u.id, d.last_at, u.at FROM collab_uncut u
		JOIN page_doc d ON d.page_id = u.id JOIN pages p ON p.id = u.id
		WHERE u.kind = 'pagedoc' AND p.state = 'current' LIMIT 200`)
	if err != nil {
		return err
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.page, &d.lastAt, &d.firstAt); err != nil {
			rows.Close()
			return err
		}
		list = append(list, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	now := c.At.UnixMilli()
	n := 0
	for _, d := range list {
		var reason string
		switch {
		case c.Editing == nil || !c.Editing(d.page):
			reason = "left"
		case now-d.lastAt >= CutIdle.Milliseconds():
			reason = "idle"
		case now-d.firstAt >= CutInterval.Milliseconds():
			reason = "interval"
		default:
			continue
		}
		s, err := pageState(tx, d.page)
		if err != nil {
			return err
		}
		m := events.Meta{At: c.At, Causation: "cut:" + reason}
		if _, err := cutVersion(tx, m, d.page, s, "", reason, false); err != nil {
			return err
		}
		if n++; n == 20 {
			break
		}
	}
	return nil
}
