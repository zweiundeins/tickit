package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/demo/seed"
	"github.com/mbolli/tickit/internal/doc"
	"github.com/mbolli/tickit/internal/tabs"
)

// DocEvent is an event of a page's document stream (docs/editor.md).
type DocEvent interface{ docEvent() }

// DocCreated starts a page's document: its epoch and first document.
type DocCreated struct {
	Epoch string          `json:"epoch"`
	Doc   json.RawMessage `json:"doc"`
}

// DocCommitted is a commit as the writer applied it: its random ref, the
// tab that made it (Client, as others see the tab), the version it was
// made on, and its steps after the rebase. Steps may be empty: a commit
// whose steps all dropped still confirms its ref.
type DocCommitted struct {
	Ref    string          `json:"ref"`
	Client string          `json:"client,omitempty"`
	Base   int64           `json:"base"`
	Steps  json.RawMessage `json:"steps"`
}

// DocSnapshot is the document as of the commits the fold replaced
// (NFR-43): Fold writes it in place of the last of them.
type DocSnapshot struct {
	Epoch string          `json:"epoch"`
	Doc   json.RawMessage `json:"doc"`
}

func (DocCreated) docEvent()   {}
func (DocCommitted) docEvent() {}
func (DocSnapshot) docEvent()  {}

// PageDocStream is the kind of the documents' streams, keyed by page id.
// Its version is the collab version of docs/editor.md.
var PageDocStream = events.NewStream[string, DocEvent]("pagedoc")

func defineDoc[T DocEvent](name string) events.Type[string, DocEvent, T] {
	return events.Define(PageDocStream, name, func(t T) DocEvent { return t })
}

// The document event types; the names are what the log stores.
var (
	DocCreatedType   = defineDoc[DocCreated]("pagedoc.created")
	DocCommittedType = defineDoc[DocCommitted]("pagedoc.committed")
	DocSnapshotType  = defineDoc[DocSnapshot]("pagedoc.snapshot")
)

// DocState is what a commit decides on: the stream's epoch, its version
// and the ref of its last commit. It never holds the document, which is
// up to 200 KB: the document cache and page_doc do. Only a fold from the
// events, as Fold's (events.Compact), carries the document along.
type DocState struct {
	Exists  bool   `json:"exists"`
	Epoch   string `json:"epoch"`
	Version int64  `json:"version"`
	Ref     string `json:"ref,omitempty"`

	doc doc.Doc // in a fold from the events only
}

const docFold = 1

func evolveDoc(s DocState, e DocEvent) DocState {
	s.Version++
	switch e := e.(type) {
	case DocCreated:
		s.Exists, s.Epoch, s.Ref = true, e.Epoch, ""
		s.doc, _ = doc.ParseDoc(e.Doc)
	case DocSnapshot:
		s.Exists, s.Epoch, s.Ref = true, e.Epoch, ""
		s.doc, _ = doc.ParseDoc(e.Doc)
	case DocCommitted:
		s.Ref = e.Ref
		if s.doc != nil {
			if steps, err := doc.ParseSteps(e.Steps); err == nil {
				s.doc, _ = doc.Apply(s.doc, steps)
			} else {
				s.doc = nil
			}
		}
	}
	return s
}

func docDecider[C any](decide func(C, DocState) ([]DocEvent, error)) events.Decider[C, DocEvent, DocState] {
	return events.Decider[C, DocEvent, DocState]{Decide: decide, Evolve: evolveDoc, Version: docFold}
}

var peekDocDecider = peekDecider(DocState{}, evolveDoc, docFold)

var createDocDecider = docDecider(func(c DocCreated, s DocState) ([]DocEvent, error) {
	if s.Exists {
		return nil, errors.New("wiki: the document exists")
	}
	return []DocEvent{c}, nil
})

func docState(tx *writer.Tx, page string) (DocState, error) {
	var s DocState
	if err := events.Execute(Store(), tx, peekDocDecider, PageDocStream.ID(page), events.Any, peek[DocState]{&s}, events.Meta{}); err != nil {
		return s, err
	}
	if !s.Exists {
		return s, notFound("document", page)
	}
	return s, nil
}

// The image library (FR-84), from the seed's images.json.
var library = sync.OnceValue(func() map[string]seed.Image {
	m := map[string]seed.Image{}
	c, err := seed.Load()
	if err != nil {
		return m
	}
	for _, im := range c.Images {
		m[im.ID] = im
	}
	return m
})

// Images are the library's images in the seed's order.
func Images() []seed.Image {
	c, err := seed.Load()
	if err != nil {
		return nil
	}
	return c.Images
}

// ImageExists reports whether id is in the image library.
func ImageExists(id string) bool { _, ok := library()[id]; return ok }

// rules are the server's rules beyond the schema (internal/doc).
var rules = doc.Rules{Image: ImageExists}

// blank is the document of a new blank page.
const blank = `{"type":"doc","content":[{"type":"paragraph"}]}`

// parseDoc reads a whole document that a command brings (a template, a
// restore) and checks it; empty is a blank page.
func parseDoc(raw json.RawMessage) (doc.Doc, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		raw = json.RawMessage(blank)
	}
	d, err := doc.ParseDoc(raw)
	if err != nil {
		return nil, tabs.Refuse("body", "This is not a page's document.")
	}
	if err := rules.Check(nil, d); err != nil {
		var re *doc.RuleError
		if errors.As(err, &re) {
			return nil, tabs.Refuse("body", re.Msg)
		}
		return nil, tabs.Refuse("body", "This is not a page's document.")
	}
	return d, nil
}

func marshalDoc(d doc.Doc) json.RawMessage {
	b, _ := doc.MarshalDoc(d)
	return b
}

// loadCommits reads a page's commits after version after up to upto, the
// snapshot that a fold left among them included (its Snapshot set),
// through the commit cache. A render's or a GET's reads may fill the
// cache: they see committed events only. A command's may not, as its
// batch could still fail; it passes fill false.
func loadCommits(ctx context.Context, q sqlite.Querier, page, epoch string, after, upto int64, fill bool) ([]commitRec, int64, error) {
	out := make([]commitRec, 0, upto-after)
	first := after + 1
	for v := after + 1; v <= upto; v++ {
		c, ok := commits.get(commitKey{page, epoch, v})
		if !ok {
			break
		}
		out = append(out, c)
	}
	if int64(len(out)) == upto-after {
		return out, first, nil
	}
	from := after + int64(len(out))
	n := 0
	for r, err := range events.LoadUntil(ctx, Store(), q, PageDocStream.ID(page), from, 1<<62) {
		if err != nil {
			return nil, 0, err
		}
		if r.Version > upto {
			break
		}
		if n == 0 && len(out) == 0 {
			first = r.Version
		}
		n++
		var c commitRec
		switch e := r.Data.(type) {
		case DocCommitted:
			steps, err := doc.ParseSteps(e.Steps)
			if err != nil {
				return nil, 0, fmt.Errorf("wiki: page %s version %d: %w", page, r.Version, err)
			}
			c = commitRec{Ref: e.Ref, Client: e.Client, Base: e.Base, Steps: steps, Raw: e.Steps}
		case DocSnapshot:
			d, err := doc.ParseDoc(e.Doc)
			if err != nil {
				return nil, 0, err
			}
			c = commitRec{Snapshot: d}
		case DocCreated:
			d, err := doc.ParseDoc(e.Doc)
			if err != nil {
				return nil, 0, err
			}
			c = commitRec{Snapshot: d}
		}
		if fill && c.Snapshot == nil {
			commits.put(commitKey{page, epoch, r.Version}, c)
		}
		out = append(out, c)
	}
	return out, first, nil
}

// errStale is a document state the reader has to start again from: its
// version was folded away, or the page was reset.
var errStale = errors.New("wiki: the document moved on")

// headDoc returns a page's document at version, whose last commit has
// ref, through the cache: from the cache, or from page_doc's checkpoint
// and the commits since.
func headDoc(ctx context.Context, q sqlite.Querier, page, epoch string, version int64, ref string, fill bool) (doc.Doc, error) {
	if d, ok := docs.get(docKey{page, epoch, version, ref}); ok {
		return d, nil
	}
	var rowEpoch, raw string
	var checkpoint int64
	err := q.QueryRowContext(ctx, `SELECT epoch, checkpoint, doc FROM page_doc WHERE page_id = ?`, page).Scan(&rowEpoch, &checkpoint, &raw)
	if err != nil {
		return nil, err
	}
	if rowEpoch != epoch || checkpoint > version {
		return nil, errStale
	}
	d, err := doc.ParseDoc([]byte(raw))
	if err != nil {
		return nil, err
	}
	if checkpoint < version {
		list, _, err := loadCommits(ctx, q, page, epoch, checkpoint, version, fill)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			if c.Snapshot != nil {
				d = c.Snapshot
				continue
			}
			if d, err = doc.Apply(d, c.Steps); err != nil {
				return nil, err
			}
		}
	}
	docs.put(docKey{page, epoch, version, ref}, d)
	return d, nil
}

// Head is a page's document as it is now, from the cache: for the reader
// view, the versions and the jobs.
type Head struct {
	Page    string
	Epoch   string
	Version int64
	Ref     string
	Doc     doc.Doc
	LastAt  int64
}

// HeadOf reads a page's head document.
func HeadOf(ctx context.Context, q sqlite.Querier, page string) (Head, error) {
	h := Head{Page: page}
	err := q.QueryRowContext(ctx, `SELECT epoch, version, ref, last_at FROM page_doc WHERE page_id = ?`, page).Scan(&h.Epoch, &h.Version, &h.Ref, &h.LastAt)
	if err != nil {
		return h, err
	}
	h.Doc, err = headDoc(ctx, q, page, h.Epoch, h.Version, h.Ref, true)
	return h, err
}

// The answers of a commit (docs/editor.md, "Wire format").
type CommitAnswer byte

const (
	Applied  CommitAnswer = 'a' // 204: applied, or applied before
	Stale    CommitAnswer = 's' // 409: another epoch, the page is gone, or base is older than the fold
	Refused  CommitAnswer = 'r' // 422: a rule refused it; the editor's row says why
	Readonly CommitAnswer = 'o' // 403: the actor may not edit the page
)

// Commit applies an editor's commit to a page's document: it maps its
// steps over the commits since Base (doc.Rebase), checks the result
// (doc.Rules) and appends it with events.Exactly(head). Run it as
// writer.OnceResult("commit:" + page + ":" + ref, c): a resent commit
// applies once. Its result is a CommitAnswer; a refusal by a rule goes
// into the editing tab's row (page_editors.refusal) for its view.
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

	steps []doc.Step
	bad   error
}

// Validate parses the steps outside the writer: ParseCommit is the
// costly part that needs no transaction.
func (c *Commit) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if !ValidID(c.Page) || c.Ref == "" || len(c.Ref) > 64 || c.Base < 0 || len(c.Client) > 64 {
		return errors.New("wiki: a malformed commit")
	}
	steps, err := doc.ParseCommit(c.Steps)
	var re *doc.RuleError
	switch {
	case errors.As(err, &re):
		c.bad = re
	case err != nil:
		return fmt.Errorf("wiki: malformed steps: %w", err)
	}
	c.steps = steps
	return nil
}

func (c *Commit) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c *Commit) ApplyResult(tx *writer.Tx) ([]byte, error) {
	start := time.Now()
	defer func() { c.Took = time.Since(start) }()
	var refusal string
	err := tx.Savepoint(func(tx *writer.Tx) error {
		var err error
		refusal, err = c.apply(tx)
		return err
	})
	var denial interface{ Denied() bool }
	switch {
	case errors.Is(err, errStale) || errors.Is(err, errGone) || errors.Is(err, access.ErrNotFound):
		return []byte{byte(Stale)}, nil
	case errors.As(err, &denial) && denial.Denied():
		return []byte{byte(Readonly)}, nil
	case err != nil:
		return nil, err
	case refusal != "":
		b, _ := json.Marshal(map[string]string{"ref": c.Ref, "message": refusal})
		if _, err := tx.Exec(`UPDATE page_editors SET refusal = ? WHERE sid = ? AND tab_id = ? AND page_id = ?`, string(b), c.Session, c.Tab, c.Page); err != nil {
			return nil, err
		}
		tx.Touch(EditorTopic(c.Tab))
		return []byte{byte(Refused)}, nil
	}
	return []byte{byte(Applied)}, nil
}

var commitDecider = docDecider(func(c DocCommitted, s DocState) ([]DocEvent, error) {
	if !s.Exists {
		return nil, errStale
	}
	return []DocEvent{c}, nil
})

// apply runs the commit; a refusal by a rule comes back as its message,
// with nothing changed.
func (c *Commit) apply(tx *writer.Tx) (string, error) {
	ctx := tx.Context()
	ps, err := pageState(tx, c.Page)
	if err != nil {
		return "", err
	}
	if err := require(tx, c.By, access.EditIssues, ps.Space); err != nil {
		return "", err
	}
	if ps.State != Current {
		return "", errGone
	}
	ds, err := docState(tx, c.Page)
	if err != nil {
		return "", err
	}
	if ds.Epoch != c.Epoch {
		return "", errStale // MP-13.2: never across a reset
	}
	if c.Base > ds.Version {
		return "", errStale // a version the page never had: the editor starts again
	}
	if c.bad != nil {
		return c.bad.Error(), nil
	}
	since, first, err := loadCommits(ctx, tx, c.Page, ds.Epoch, c.Base, ds.Version, false)
	if err != nil {
		return "", err
	}
	if c.Base < ds.Version && first != c.Base+1 {
		return "", errStale // folded away: the editor starts again
	}
	var mapped []doc.Step
	for _, s := range since {
		if s.Snapshot != nil {
			return "", errStale
		}
		mapped = append(mapped, s.Steps...)
	}
	head, err := headDoc(ctx, tx, c.Page, ds.Epoch, ds.Version, ds.Ref, false)
	if err != nil {
		return "", err
	}
	next, applied, err := doc.Rebase(head, mapped, c.steps)
	if err != nil {
		return "This change no longer fits the page.", nil
	}
	if err := rules.Check(head, next); err != nil {
		var re *doc.RuleError
		if errors.As(err, &re) {
			return re.Msg, nil
		}
		return "This change does not fit the page.", nil
	}
	raw, err := doc.MarshalSteps(applied)
	if err != nil {
		return "", err
	}
	v := ds.Version + 1
	docs.put(docKey{c.Page, ds.Epoch, v, c.Ref}, next)
	e := DocCommitted{Ref: c.Ref, Client: c.Client, Base: c.Base, Steps: raw}
	if err := events.Execute(Store(), tx, commitDecider, PageDocStream.ID(c.Page), events.Exactly(ds.Version), e, meta(c.By)); err != nil {
		return "", err
	}
	page, epoch, rec := c.Page, ds.Epoch, commitRec{Ref: c.Ref, Client: c.Client, Base: c.Base, Steps: applied, Raw: raw}
	tx.AfterCommit(func() { commits.put(commitKey{page, epoch, v}, rec) })
	return "", nil
}

// Batch is a commit as an editor's frame carries it (docs/editor.md).
type Batch struct {
	V      int64           `json:"v"`
	Ref    string          `json:"ref"`
	Client string          `json:"client"`
	Steps  json.RawMessage `json:"steps"`
}

// MaxBatches is how many commits a frame carries at most.
const MaxBatches = 500

// Batches are a page's commits after version after, up to head and at
// most MaxBatches, for an editor's frame. ok is false when the commits
// right after it were folded away: the editor starts again ("gap").
func Batches(ctx context.Context, q sqlite.Querier, page, epoch string, after, head int64) (list []Batch, ok bool, err error) {
	if after >= head {
		return nil, true, nil
	}
	upto := min(head, after+MaxBatches)
	recs, first, err := loadCommits(ctx, q, page, epoch, after, upto, true)
	if err != nil {
		return nil, false, err
	}
	if first != after+1 {
		return nil, false, nil
	}
	list = make([]Batch, 0, len(recs))
	for i, c := range recs {
		if c.Snapshot != nil {
			return nil, false, nil
		}
		raw := c.Raw
		if len(raw) == 0 {
			raw = json.RawMessage(`[]`)
		}
		list = append(list, Batch{V: after + 1 + int64(i), Ref: c.Ref, Client: c.Client, Steps: raw})
	}
	return list, true, nil
}
