package wiki

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/tabs"
)

// Client is a tab as other editors see it, made from its id: the first
// 12 hex digits of SHA-256 (docs/editor.md). The editor's page picks a
// random one instead (its GET knows no tab id); both keep tab ids private.
func Client(tab string) string {
	h := sha256.Sum256([]byte(tab))
	return hex.EncodeToString(h[:6])
}

// EditorView is the spec of a tab's editor view, which the tab's row
// keeps so that a stream that reconnects watches it again.
func EditorView(page, tab string) string { return "editor:" + page + "." + tab }

// OpenEditor starts editing a page in a tab: the tab's row in
// page_editors, acknowledging version Base (the one its page rendered),
// and its editor view in the tab's watched views (docs/editor.md). A
// Member does (FR-86.1).
type OpenEditor struct {
	By           tabs.By
	Session, Tab string
	Page         string
	Base         int64
	Client       string // the tab as others see it: 12 hex digits the page chose
}

// ValidClient reports whether s is a client id: 12 lower-case hex digits.
func ValidClient(s string) bool {
	if len(s) != 12 {
		return false
	}
	for _, c := range []byte(s) {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

func (c OpenEditor) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.Session == "" || c.Tab == "" || !ValidID(c.Page) || c.Base < 0 || !ValidClient(c.Client) {
		return errors.New("wiki: a malformed editor")
	}
	return nil
}

func (c OpenEditor) Apply(tx *writer.Tx) error {
	if _, err := current(tx, c.By, c.Page, access.EditIssues); err != nil {
		return err
	}
	ds, err := docState(tx, c.Page)
	if err != nil {
		return err
	}
	base := min(c.Base, ds.Version)
	now := millis(at(c.By))
	if _, err := tx.Exec(`INSERT INTO page_editors (sid, tab_id, page_id, person_id, client, epoch, ack, v, moved_at, seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (sid, tab_id) DO UPDATE SET page_id = excluded.page_id, person_id = excluded.person_id, epoch = excluded.epoch,
			client = excluded.client, ack = CASE WHEN page_editors.page_id = excluded.page_id AND page_editors.epoch = excluded.epoch THEN max(page_editors.ack, excluded.ack) ELSE excluded.ack END,
			v = excluded.v, refusal = '', seen_at = excluded.seen_at`,
		c.Session, c.Tab, c.Page, c.By.Actor.ID, c.Client, ds.Epoch, base, base, now, now); err != nil {
		return err
	}
	tx.Touch(EditorTopic(c.Tab), CaretsTopic(c.Page))
	_, err = tabs.AddWatched(tx, c.Session, c.Tab, tabs.Watched{View: EditorView(c.Page, c.Tab), Key: EditorView(c.Page, c.Tab)}, at(c.By))
	return err
}

// CloseEditor ends a tab's editing: its row goes, and with it its caret.
type CloseEditor struct {
	Session, Tab string
	At           time.Time
}

func (c CloseEditor) Apply(tx *writer.Tx) error {
	var page string
	err := tx.QueryRow(`DELETE FROM page_editors WHERE sid = ? AND tab_id = ? RETURNING page_id`, c.Session, c.Tab).Scan(&page)
	if err != nil {
		return nil // no row: nothing to close
	}
	tx.Touch(EditorTopic(c.Tab), CaretsTopic(page))
	return tabs.RemoveWatched(tx, c.Session, c.Tab, EditorView(page, c.Tab), c.At)
}

// Presence is an editing tab's selection and acknowledged version (MP-06):
// at most every 100 ms; a post that a newer one (Seq) overtook changes
// nothing. The handler sends it (writer.Send): the next one replaces it.
type Presence struct {
	Session, Tab string
	Page         string
	Epoch        string
	Ack, V       int64
	Anchor, Head int
	Seq          int64
	At           time.Time
}

func (c Presence) Validate() error {
	if c.Session == "" || c.Tab == "" || !ValidID(c.Page) || c.Ack < 0 || c.V < 0 || c.Anchor < 0 || c.Head < 0 || c.Seq < 0 {
		return errors.New("wiki: a malformed presence")
	}
	return nil
}

func (c Presence) Apply(tx *writer.Tx) error {
	now := c.At.UnixMilli()
	res, err := tx.Exec(`UPDATE page_editors SET ack = max(ack, ?), v = ?, anchor = ?, head = ?, seq = ?, seen_at = ?,
		moved_at = CASE WHEN anchor != ? OR head != ? OR v != ? THEN ? ELSE moved_at END
		WHERE sid = ? AND tab_id = ? AND page_id = ? AND epoch = ? AND seq < ?`,
		c.Ack, c.V, c.Anchor, c.Head, c.Seq, now, c.Anchor, c.Head, c.V, now, c.Session, c.Tab, c.Page, c.Epoch, c.Seq)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		tx.Touch(CaretsTopic(c.Page))
	}
	return nil
}

// EditorRow is an editing tab's row.
type EditorRow struct {
	Tab, Page, Person, Client, Epoch string
	Ack, V                           int64
	Anchor, Head                     int
	Refusal                          string
	MovedAt                          time.Time
}

// Editor reads the row of an editing tab of a person; access.ErrNotFound
// when there is none.
func Editor(ctx context.Context, q sqlite.Querier, tab, person string) (EditorRow, error) {
	var r EditorRow
	var moved int64
	err := q.QueryRowContext(ctx, `SELECT tab_id, page_id, person_id, client, epoch, ack, v, anchor, head, refusal, moved_at
		FROM page_editors WHERE tab_id = ? AND person_id = ?`, tab, person).
		Scan(&r.Tab, &r.Page, &r.Person, &r.Client, &r.Epoch, &r.Ack, &r.V, &r.Anchor, &r.Head, &r.Refusal, &moved)
	if err != nil {
		return r, notFound("editor", tab)
	}
	r.MovedAt = time.UnixMilli(moved)
	return r, nil
}

// Editors are the rows of a page's editing tabs.
func Editors(ctx context.Context, q sqlite.Querier, page string) ([]EditorRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT tab_id, page_id, person_id, client, epoch, ack, v, anchor, head, moved_at
		FROM page_editors WHERE page_id = ? ORDER BY moved_at`, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EditorRow
	for rows.Next() {
		var r EditorRow
		var moved int64
		if err := rows.Scan(&r.Tab, &r.Page, &r.Person, &r.Client, &r.Epoch, &r.Ack, &r.V, &r.Anchor, &r.Head, &moved); err != nil {
			return nil, err
		}
		r.MovedAt = time.UnixMilli(moved)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SweepEditors deletes the rows of editing tabs not seen for a day.
type SweepEditors struct{ At time.Time }

func (c SweepEditors) Apply(tx *writer.Tx) error {
	_, err := tx.Exec(`DELETE FROM page_editors WHERE seen_at < ?`, c.At.Add(-24*time.Hour).UnixMilli())
	return err
}
