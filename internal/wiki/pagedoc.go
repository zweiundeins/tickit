package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/zweiundeins/coedit/collab"
	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"
	"github.com/zweiundeins/prosemirror-go"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo/seed"
	"github.com/zweiundeins/tickit/internal/docnodes"
	"github.com/zweiundeins/tickit/internal/editing"
	"github.com/zweiundeins/tickit/internal/images"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// A page's document is a co-edited document of coedit's collab, of the
// kind "pagedoc", keyed by the page's id (docs/editor.md). Its head is
// page_doc, its editing tabs page_editors.
var pageDocs = editing.NewKind(collab.Config{
	Name: "pagedoc", Noun: "page", Store: func() *events.Store { return Store() },
	Head: "page_doc", Editors: "page_editors", Key: "page_id",
	EditorTopic: EditorTopic, CaretsTopic: CaretsTopic,
	OnCommit: mapAnchors,
})

// The document's events (collab's), under the wiki's names.
type (
	DocEvent     = collab.Event
	DocCreated   = collab.Created
	DocCommitted = collab.Committed
	DocSnapshot  = collab.Snapshot
	DocState     = collab.State
)

// PageDocStream is the kind of the documents' streams, keyed by page id.
// Its version is the collab version of docs/editor.md.
var PageDocStream = pageDocs.Stream

// The document event types; the names are what the log stores.
var (
	DocCreatedType   = pageDocs.Created
	DocCommittedType = pageDocs.Committed
	DocSnapshotType  = pageDocs.Snapshot
)

var (
	createDocDecider = pageDocs.CreateDecider
	commitDecider    = pageDocs.CommitDecider
	errStale         = collab.ErrStale
	evolveDoc        = pageDocs.Evolve
)

func docState(tx *writer.Tx, page string) (DocState, error) { return pageDocs.State(tx, page) }

// Images are the library's images in the seed's order.
func Images() []seed.Image { return images.Library() }

// ImageExists reports whether id is in the image library (FR-84).
func ImageExists(id string) bool { return images.InLibrary(id) }

// blank is the document of a new blank page.
const blank = docnodes.Blank

// parseDoc reads a whole document that a command brings (a template, a
// restore) and checks it; empty is a blank page.
func parseDoc(raw json.RawMessage) (prosemirror.Doc, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		raw = json.RawMessage(blank)
	}
	d, err := docnodes.ParseDoc(raw)
	if err != nil {
		return nil, tabs.Refuse("body", "This is not a page's document.")
	}
	if err := editing.LibraryRules.Check(nil, d); err != nil {
		var re *prosemirror.RuleError
		if errors.As(err, &re) {
			return nil, tabs.Refuse("body", re.Msg)
		}
		return nil, tabs.Refuse("body", "This is not a page's document.")
	}
	return d, nil
}

func marshalDoc(d prosemirror.Doc) json.RawMessage { return docnodes.Marshal(d) }

// Head is a page's document as it is now, from the cache: for the reader
// view, the versions and the jobs.
type Head = collab.Head

// HeadOf reads a page's head document.
func HeadOf(ctx context.Context, q sqlite.Querier, page string) (Head, error) {
	return pageDocs.Head(ctx, q, page)
}

// The answers of a commit (docs/editor.md, "Wire format").
type CommitAnswer = collab.Answer

const (
	Applied  = collab.Applied  // 204: applied, or applied before
	Stale    = collab.Stale    // 409: another epoch, the page is gone, or base is older than the fold
	Refused  = collab.Refused  // 422: a rule refused it; the editor's row says why
	Readonly = collab.Readonly // 403: the actor may not edit the page
)

// Commit applies an editor's commit to a page's document (collab.Commit):
// the page must be current and the actor a Member of its space.
type Commit struct {
	By           tabs.By
	Session, Tab string
	Page         string
	Epoch        string
	Base         int64
	Ref          string
	Client       string
	Steps        json.RawMessage

	// Took is how long the writer spent on it, for the measurements.
	Took time.Duration

	c *collab.Commit
}

// Validate parses the steps outside the writer.
func (c *Commit) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if !ValidID(c.Page) {
		return errors.New("wiki: a malformed commit")
	}
	by, page := c.By, c.Page
	c.c = &collab.Commit{Kind: pageDocs, Meta: c.By.Meta(), Session: c.Session, Tab: c.Tab, ID: c.Page, Epoch: c.Epoch, Base: c.Base,
		Ref: c.Ref, Client: c.Client, Steps: c.Steps,
		Gate: func(tx *writer.Tx) error { return DocEditable(tx, by, page) }}
	if err := c.c.Validate(); err != nil {
		return errors.New("wiki: " + err.Error())
	}
	return nil
}

// DocEditable checks in the writer that by may change the page's
// document now, as a commit does: the page current (collab.ErrGone) and
// by a Member of its space.
func DocEditable(tx *writer.Tx, by tabs.By, page string) error {
	ps, err := pageState(tx, page)
	if err != nil {
		return err
	}
	if err := require(tx, by, access.EditIssues, ps.Space); err != nil {
		return err
	}
	if ps.State != Current {
		return collab.ErrGone
	}
	return nil
}

func (c *Commit) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c *Commit) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if c.c == nil {
		if err := c.Validate(); err != nil {
			return nil, err
		}
	}
	defer func() { c.Took = c.c.Took }()
	return c.c.ApplyResult(tx)
}

// Batch is a commit as an editor's frame carries it (docs/editor.md).
type Batch = collab.Batch

// MaxBatches is how many commits a frame carries at most.
const MaxBatches = collab.MaxBatches

// Batches are a page's commits after version after, up to head and at
// most MaxBatches, for an editor's frame. ok is false when the commits
// right after it were folded away: the editor starts again ("gap").
func Batches(ctx context.Context, q sqlite.Querier, page, epoch string, after, head int64) ([]Batch, bool, error) {
	return pageDocs.Batches(ctx, q, page, epoch, after, head)
}
