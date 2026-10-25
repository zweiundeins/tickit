package wiki

import (
	"context"
	"errors"
	"time"

	"github.com/zweiundeins/lith-coedit/collab"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Client is a tab as other editors see it, made from its id (collab.Client).
func Client(tab string) string { return collab.Client(tab) }

// ValidClient reports whether s is a client id: 12 lower-case hex digits.
func ValidClient(s string) bool { return collab.ValidClient(s) }

// EditorView is the spec of a tab's editor view, which the tab's row
// keeps so that a stream that reconnects watches it again.
func EditorView(page, tab string) string { return "editor:" + page + "." + tab }

// OpenEditor starts editing a page in a tab: the tab's row in
// page_editors, acknowledging version Base (the one its page rendered)
// (docs/editor.md). A Member does (FR-86.1); the action records its view
// in the tab's row (tabs.Act's Watch).
type OpenEditor struct {
	By           tabs.By
	Session, Tab string
	Page         string
	Base         int64
	Client       string // the tab as others see it: 12 hex digits the page chose
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
	return pageDocs.Open(tx, collab.Open{Session: c.Session, Tab: c.Tab, ID: c.Page, Person: c.By.Actor.ID, Base: c.Base, Client: c.Client, At: at(c.By)})
}

// CloseEditor ends a tab's editing: its row goes, and with it its caret.
type CloseEditor struct {
	Session, Tab string
	At           time.Time
}

func (c CloseEditor) Apply(tx *writer.Tx) error {
	page, err := pageDocs.Close(tx, c.Session, c.Tab)
	if err != nil || page == "" {
		return err
	}
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

func (c Presence) presence() collab.Presence {
	return collab.Presence{Kind: pageDocs, Session: c.Session, Tab: c.Tab, ID: c.Page, Epoch: c.Epoch, Ack: c.Ack, V: c.V, Anchor: c.Anchor, Head: c.Head, Seq: c.Seq, At: c.At}
}

func (c Presence) Validate() error {
	if err := c.presence().Validate(); err != nil {
		return errors.New("wiki: a malformed presence")
	}
	return nil
}

func (c Presence) Apply(tx *writer.Tx) error { return c.presence().Apply(tx) }

// EditorRow is an editing tab's row; its ID is the page's.
type EditorRow = collab.EditorRow

// Editor reads the row of an editing tab of a person; access.ErrNotFound
// when there is none.
func Editor(ctx context.Context, q sqlite.Querier, tab, person string) (EditorRow, error) {
	r, err := pageDocs.Editor(ctx, q, tab, person)
	if err != nil {
		return r, notFound("editor", tab)
	}
	return r, nil
}

// Editors are the rows of a page's editing tabs.
func Editors(ctx context.Context, q sqlite.Querier, page string) ([]EditorRow, error) {
	return pageDocs.Editors(ctx, q, page)
}

// SweepEditors deletes the rows of editing tabs not seen for a day.
type SweepEditors struct{ At time.Time }

func (c SweepEditors) Apply(tx *writer.Tx) error { return pageDocs.SweepEditors(tx, c.At) }

// PauseEditor pauses the tab's editor of the page (collab.Kind.Pause,
// MP-14.1).
func PauseEditor(tx *writer.Tx, session, tab, page, msg string) error {
	return pageDocs.Pause(tx, session, tab, page, msg)
}
