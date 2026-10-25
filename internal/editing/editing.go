// Package editing is tickit's configuration of coedit's collab, shared by
// the two kinds of co-edited documents, a page's body (internal/wiki) and
// an issue's description (internal/tracker): tickit's schema, its rules
// with the image library and the uploads of the commit's transaction, the
// demo's epoch, access.ErrNotFound and ids as UUIDs (docs/editor.md).
package editing

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/zweiundeins/coedit/collab"
	"github.com/zweiundeins/lith/writer"
	"github.com/zweiundeins/prosemirror-go"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/docnodes"
	"github.com/zweiundeins/tickit/internal/images"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// NewKind makes a Kind with tickit's settings; cfg names its stream,
// tables and topics.
func NewKind(cfg collab.Config) *collab.Kind {
	cfg.Schema = docnodes.Schema()
	cfg.Rules = Rules
	cfg.Epoch = Epoch
	cfg.NotFound = access.ErrNotFound
	cfg.ValidID = ValidID
	return collab.NewKind(cfg)
}

// Rules are the rules of a commit in tx: the images of the library and
// the uploaded ones (FR-84, FR-85).
func Rules(tx *writer.Tx, _, _ string) prosemirror.Rules {
	return docnodes.Rules{Image: images.Allowed(tx)}.Rules()
}

// Epoch is the epoch of a new document stream: the demo's epoch, which
// every reset bumps (demo.Epoch reads the same row; this package cannot
// import demo, which seeds), so that no edit made before a reset applies
// after it to the subject seeded again with its id (MP-13.2). The seed
// stays the same at every run (DM-02.4).
func Epoch(tx *writer.Tx) (string, error) {
	var n int64
	err := tx.QueryRow(`SELECT epoch FROM demo_state WHERE id = 1`).Scan(&n)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	return "e" + strconv.FormatInt(n, 10), nil
}

// ValidID reports whether s is a UUID in its canonical form, as every
// subject's id is.
func ValidID(s string) bool {
	u, err := uuid.Parse(s)
	return err == nil && u.String() == s
}

// CommitWhole is k.CommitWhole by an action of a tab: a document the rules
// refuse is a refusal on field.
func CommitWhole(k *collab.Kind, tx *writer.Tx, by tabs.By, id string, d prosemirror.Doc, ref, field string, check func(head, next prosemirror.Doc) string) (bool, error) {
	ok, err := k.CommitWhole(tx, by.Meta(), id, d, ref, check)
	var r *writer.Refusal
	if errors.As(err, &r) {
		return false, tabs.Refuse(field, r.Msg)
	}
	return ok, err
}

// LibraryRules are the rules outside a commit: the library's images only.
var LibraryRules = docnodes.Rules{Image: images.Allowed(nil)}
