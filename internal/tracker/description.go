package tracker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/zweiundeins/lith-coedit/collab"
	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"
	"github.com/zweiundeins/prosemirror-go"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/docnodes"
	"github.com/zweiundeins/tickit/internal/editing"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// An issue's description is co-edited as a page is (FR-15, Q22,
// docs/editor.md): a document of lith-coedit's collab of the kind "issuedoc",
// keyed by the issue's id, whose head is issue_doc and whose editing
// tabs are issue_editors. CreateIssue starts it with the description it
// is created with.
//
// Versions of it are cut as the issue's own issue.description_changed,
// with the whole document, the stream version of the document it was cut
// at (Upto) and who edited it since the last one: after CutIdle without a
// commit, when the last editor leaves, at least every CutInterval, and
// at once for SetDescription. So the issue's history, its state as of a
// version (FR-70), search, the description's mentions (FR-112) and its
// readers' lists see a description as it was at its versions, while the
// issue's page shows the document live. The commits up to a version are
// folded FoldAfter after it (NFR-43).
var descDocs = editing.NewKind(collab.Config{
	Name: "issuedoc", Noun: "description", Store: func() *events.Store { return Store() },
	Head: "issue_doc", Editors: "issue_editors", Key: "issue_id",
	EditorTopic: DescEditorTopic, CaretsTopic: DescCaretsTopic,
})

// DescDocStream is the kind of the descriptions' streams, keyed by issue
// id; its event types are issuedoc.created, .committed and .snapshot.
var DescDocStream = descDocs.Stream

// The description stream's event types.
var (
	DescCreatedType   = descDocs.Created
	DescCommittedType = descDocs.Committed
	DescSnapshotType  = descDocs.Snapshot
)

// DescTopic is touched by every commit to an issue's description: its
// reader view and its editors' views.
func DescTopic(issue string) string { return descDocs.Topic(issue) }

// DescCaretsTopic is touched by every presence post of a description's
// editors.
func DescCaretsTopic(issue string) string { return "desc-carets:" + issue }

// DescEditorTopic is touched by every change to one tab's row of
// issue_editors: its refusals, its close.
func DescEditorTopic(tab string) string { return "desc-editor:" + tab }

// DescEditorView is the spec of a tab's editor view of a description;
// DescCaretsView that of the carets of its editors.
func DescEditorView(issue, tab string) string { return "desc-editor:" + issue + "." + tab }
func DescCaretsView(issue string) string      { return "desc-carets:" + issue }

// descProjector keeps issue_doc.
var descProjector = descDocs.Projector("tracker_descs", 1, nil)

// When description versions are cut and folded: as pages' are.
const (
	CutIdle     = 60 * time.Second
	CutInterval = 10 * time.Minute
)

// DescHead is an issue's description as it is now.
type DescHead = collab.Head

// DescriptionHead reads an issue's description as it is now;
// sql.ErrNoRows for an issue without one (created before descriptions
// were documents of their own).
func DescriptionHead(ctx context.Context, q sqlite.Querier, issue string) (DescHead, error) {
	return descDocs.Head(ctx, q, issue)
}

// DescriptionBatches are a description's commits after version after,
// for an editor's frame (collab.Kind.Batches).
func DescriptionBatches(ctx context.Context, q sqlite.Querier, issue, epoch string, after, head int64) ([]collab.Batch, bool, error) {
	return descDocs.Batches(ctx, q, issue, epoch, after, head)
}

// DescriptionEditor reads a person's tab's row of issue_editors;
// access.ErrNotFound when there is none.
func DescriptionEditor(ctx context.Context, q sqlite.Querier, tab, person string) (collab.EditorRow, error) {
	return descDocs.Editor(ctx, q, tab, person)
}

// DescriptionEditors are the rows of a description's editing tabs.
func DescriptionEditors(ctx context.Context, q sqlite.Querier, issue string) ([]collab.EditorRow, error) {
	return descDocs.Editors(ctx, q, issue)
}

// descDoc parses a description a command brings: a document of the
// schema within the rules, the blank document when empty.
func descDoc(raw json.RawMessage) (prosemirror.Doc, error) {
	if docnodes.IsBlank(raw) {
		raw = json.RawMessage(docnodes.Blank)
	}
	d, err := docnodes.ParseDoc(raw)
	if err != nil {
		return nil, tabs.Refuse(FieldDescription, "The description is not a document.")
	}
	if err := editing.LibraryRules.Check(nil, d); err != nil {
		var re *prosemirror.RuleError
		if errors.As(err, &re) {
			return nil, tabs.Refuse(FieldDescription, re.Msg)
		}
		return nil, tabs.Refuse(FieldDescription, "The description is not a document.")
	}
	return d, nil
}

// descRaw is the description an issue's events hold for a document: nil
// for a blank one, so that an empty description stays empty.
func descRaw(d prosemirror.Doc) json.RawMessage {
	raw := docnodes.Marshal(d)
	if string(raw) == docnodes.Blank {
		return nil
	}
	return canonical(raw)
}

// checkDesc is DM-05's limits of a description beyond the schema's, for
// a commit: its text and its mentions.
func checkDesc(_, next prosemirror.Doc) string {
	if utf8.RuneCountInString(docnodes.Text(next)) > MaxIssueText {
		return "A description has at most 20,000 characters."
	}
	if len(docnodes.References(next).People) > MaxDescriptionMentions {
		return "A description mentions 50 people at most."
	}
	return ""
}

// editable reads an issue whose description the actor may edit now: it
// exists, is neither deleted nor hidden (collab.ErrGone), and the actor
// may edit issues in its open project (a denial otherwise).
func editable(tx *writer.Tx, by tabs.By, issue string) (IssueState, error) {
	s, err := issueState(tx, issue)
	if err != nil {
		return s, err
	}
	if err := require(tx, by, access.EditIssues, s.Project); err != nil {
		return s, err
	}
	if s.Deleted || s.Hidden {
		return s, collab.ErrGone
	}
	return s, nil
}

// DescriptionEditable checks in the writer that by may change the issue's
// description now, as a commit does: collab.ErrGone for a deleted or
// hidden issue.
func DescriptionEditable(tx *writer.Tx, by tabs.By, issue string) error {
	if !ValidID(issue) {
		return notFound("issue", issue)
	}
	_, err := editable(tx, by, issue)
	return err
}

// ensureDesc starts the document of an issue created before descriptions
// were documents of their own, from its description now; the issue's
// version 1 then matches the document's (IssueState.DescUpto).
func ensureDesc(tx *writer.Tx, by tabs.By, issue string, s IssueState) error {
	if _, err := descDocs.State(tx, issue); !errors.Is(err, access.ErrNotFound) {
		return err
	}
	d, err := docnodes.ParseDoc(s.Description)
	if err != nil || docnodes.IsBlank(s.Description) {
		d, _ = docnodes.ParseDoc(json.RawMessage(docnodes.Blank))
	}
	_, err = descDocs.Create(tx, issue, d, meta(by))
	return err
}

// CommitDescription applies an editor's commit to an issue's description
// (collab.Commit; docs/editor.md, "Wire format"). Its result is a
// collab.Answer.
type CommitDescription struct {
	By           tabs.By
	Session, Tab string
	Issue        string
	Epoch        string
	Base         int64
	Ref          string
	Client       string
	Steps        json.RawMessage

	// Took is how long the writer spent on it, for the measurements.
	Took time.Duration

	c *collab.Commit
}

func (c *CommitDescription) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	by, issue := c.By, c.Issue
	c.c = &collab.Commit{Kind: descDocs, Meta: c.By.Meta(), Session: c.Session, Tab: c.Tab, ID: c.Issue, Epoch: c.Epoch, Base: c.Base,
		Ref: c.Ref, Client: c.Client, Steps: c.Steps, Check: checkDesc,
		Gate: func(tx *writer.Tx) error { return DescriptionEditable(tx, by, issue) }}
	if err := c.c.Validate(); err != nil {
		return errors.New("tracker: " + err.Error())
	}
	return nil
}

func (c *CommitDescription) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c *CommitDescription) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if c.c == nil {
		if err := c.Validate(); err != nil {
			return nil, err
		}
	}
	defer func() { c.Took = c.c.Took }()
	return c.c.ApplyResult(tx)
}

// OpenDescription starts editing an issue's description in a tab: the
// tab's row in issue_editors, acknowledging version Base (the one its
// editor started from). A Member of the issue's project does.
type OpenDescription struct {
	By           tabs.By
	Session, Tab string
	Issue        string
	Base         int64
	Client       string // the tab as others see it: 12 hex digits the page chose
}

func (c OpenDescription) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.Session == "" || c.Tab == "" || !ValidID(c.Issue) || c.Base < 0 || !collab.ValidClient(c.Client) {
		return errors.New("tracker: a malformed editor")
	}
	return nil
}

func (c OpenDescription) Apply(tx *writer.Tx) error {
	s, err := editable(tx, c.By, c.Issue)
	if errors.Is(err, collab.ErrGone) {
		return errDeleted
	}
	if err != nil {
		return err
	}
	if err := ensureDesc(tx, c.By, c.Issue, s); err != nil {
		return err
	}
	return descDocs.Open(tx, collab.Open{Session: c.Session, Tab: c.Tab, ID: c.Issue, Person: c.By.Actor.ID, Base: c.Base, Client: c.Client, At: at(c.By)})
}

// CloseDescription ends a tab's editing of a description: its row goes,
// and with it its caret. Issue is the one the tab edited, "" for none.
type CloseDescription struct {
	Session, Tab string

	Issue string
}

func (c *CloseDescription) Apply(tx *writer.Tx) (err error) {
	c.Issue, err = descDocs.Close(tx, c.Session, c.Tab)
	return err
}

// DescriptionPresence is an editing tab's selection and acknowledged
// version (MP-06), sent without waiting (collab.Presence).
func DescriptionPresence(session, tab, issue, epoch string, ack, v int64, anchor, head int, seq int64, at time.Time) collab.Presence {
	return collab.Presence{Kind: descDocs, Session: session, Tab: tab, ID: issue, Epoch: epoch, Ack: ack, V: v, Anchor: anchor, Head: head, Seq: seq, At: at}
}

// cutDescDecider appends a version of the description: the document as
// DescriptionChanged.To holds it, with the people it mentions for the
// first time, who start watching (FR-27.2).
var cutDescDecider = issueDecider(func(c DescriptionChanged, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted:
		return nil, errDeleted
	case c.Upto <= s.DescUpto:
		return nil, nil
	}
	info, err := readDoc(c.To)
	if err != nil {
		return nil, err
	}
	c.Mentions = added(s.Mentioned, info.Mentions)
	return append([]IssueEvent{c}, newWatchers(s, info.Mentions...)...), nil
})

// cutDescription cuts a version of an issue's description from its head,
// when the document moved on since the last one. The actor is m's, or
// the last of the people who committed since. It reports whether it cut.
func cutDescription(tx *writer.Tx, m events.Meta, issue string, s IssueState) (bool, error) {
	ds, err := descDocs.State(tx, issue)
	if errors.Is(err, access.ErrNotFound) {
		return false, nil
	}
	if err != nil || ds.Version <= s.DescUpto {
		return false, err
	}
	head, err := descDocs.StateDoc(tx.Context(), tx, issue, ds)
	if err != nil {
		return false, err
	}
	editors, err := descDocs.EditorsSince(tx, issue, s.DescUpto)
	if err != nil {
		return false, err
	}
	if m.Actor == "" && len(editors) > 0 {
		m.Actor = editors[len(editors)-1]
	}
	if len(editors) == 0 && m.Actor != "" {
		editors = []string{m.Actor}
	}
	e := DescriptionChanged{To: descRaw(head), Upto: ds.Version, Editors: editors}
	if err := events.Execute(Store(), tx, cutDescDecider, IssueStream.ID(issue), events.Any, e, m); err != nil {
		return false, err
	}
	return true, descDocs.Cut(tx, issue, ds.Version)
}

// SetDescription replaces an issue's description with a whole document
// and cuts its version at once: the seed's and the tests' way to write
// one. Open editors apply it as one commit (collab.Kind.CommitWhole). A
// description equal to the one the issue has stores nothing (FR-14.3).
type SetDescription struct {
	By          tabs.By
	Issue       string
	Description json.RawMessage
}

func (c SetDescription) Validate() error { return checkBy(c.By) }

func (c SetDescription) Apply(tx *writer.Tx) error {
	s, err := editable(tx, c.By, c.Issue)
	if errors.Is(err, collab.ErrGone) {
		return errDeleted
	}
	if err != nil {
		return err
	}
	if _, err := description(c.Description); err != nil {
		return err
	}
	d, err := descDoc(c.Description)
	if err != nil {
		return err
	}
	if err := ensureDesc(tx, c.By, c.Issue, s); err != nil {
		return err
	}
	by := c.By
	if by.At.IsZero() {
		by.At = at(by)
	}
	if _, err := editing.CommitWhole(descDocs, tx, by, c.Issue, d, "set-"+c.By.Op, FieldDescription, checkDesc); err != nil {
		return err
	}
	if err := acted(tx, s.Project, c.By.Actor.ID, at(c.By)); err != nil {
		return err
	}
	_, err = cutDescription(tx, meta(by), c.Issue, s)
	return err
}

// CutDescriptions cuts the description versions that are due, as
// wiki.CutDue does pages': of each issue whose description changed since
// its last version, after CutIdle without a commit, at once when nobody
// edits it any more (Editing false), or when its first change since the
// last version is CutInterval old. The job sends it every few seconds;
// it cuts at most 20 a run.
type CutDescriptions struct {
	At time.Time
	// Editing reports whether someone has the description open in the
	// editor; nil: everyone left.
	Editing func(issue string) bool
}

func (c CutDescriptions) Apply(tx *writer.Tx) error {
	rows, err := tx.Query(`SELECT d.issue_id, d.last_at, coalesce((SELECT e.at FROM lith_events e WHERE e.stream = 'issuedoc' AND e.stream_id = d.issue_id AND e.version = i.desc_upto + 1), d.last_at)
		FROM issue_doc d JOIN issues i ON i.id = d.issue_id
		WHERE i.deleted_at IS NULL AND d.version > i.desc_upto LIMIT 200`)
	if err != nil {
		return err
	}
	type due struct {
		issue           string
		lastAt, firstAt int64
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.issue, &d.lastAt, &d.firstAt); err != nil {
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
		case c.Editing == nil || !c.Editing(d.issue):
			reason = "left"
		case now-d.lastAt >= CutIdle.Milliseconds():
			reason = "idle"
		case now-d.firstAt >= CutInterval.Milliseconds():
			reason = "interval"
		default:
			continue
		}
		s, err := issueState(tx, d.issue)
		if err != nil {
			return err
		}
		if _, err := cutDescription(tx, events.Meta{At: c.At, Causation: "cut:" + reason}, d.issue, s); err != nil {
			return err
		}
		if n++; n == 20 {
			break
		}
	}
	return nil
}

// FoldDescriptions folds the commits of descriptions that versions hold
// into snapshots (NFR-43), as wiki.Fold does pages': the commits stored
// before FoldAfter ago, or before the oldest commit not in a version yet,
// whichever is earlier, at most Limit (collab.FoldBatch when 0). A
// deleted issue's commits hold nothing back. It reports how many events
// it deleted; the job sends it again while it deletes some.
type FoldDescriptions struct {
	At    time.Time
	Limit int

	Deleted int64
	Took    time.Duration
}

func (c *FoldDescriptions) Apply(tx *writer.Tx) error {
	start := time.Now()
	defer func() { c.Took = time.Since(start) }()
	since := c.At.Add(-collab.FoldAfter)
	var oldest sql.NullInt64
	err := tx.QueryRow(`SELECT min(u.at) FROM collab_uncut u JOIN issues i ON i.id = u.id AND i.deleted_at IS NULL
		WHERE u.kind = 'issuedoc'`).Scan(&oldest)
	if err != nil {
		return err
	}
	if oldest.Valid && time.UnixMilli(oldest.Int64).Before(since) {
		since = time.UnixMilli(oldest.Int64)
	}
	c.Deleted, err = descDocs.Fold(tx, since, c.Limit)
	return err
}

// SweepDescriptionEditors deletes the rows of editing tabs not seen for a
// day.
type SweepDescriptionEditors struct{ At time.Time }

func (c SweepDescriptionEditors) Apply(tx *writer.Tx) error { return descDocs.SweepEditors(tx, c.At) }

// DescVersion is a version of an issue's description (FR-68): who made
// it and when, who edited it, and the document before it (nil for the
// first, the issue's creation) and after.
type DescVersion struct {
	Actor         string
	At            time.Time
	Editors       []string
	Before, After prosemirror.Doc
}

// DescriptionVersion reads the version of an issue's description that
// the issue's version v made: its creation or an issue.description_changed.
// Any other version answers access.ErrNotFound.
func DescriptionVersion(ctx context.Context, q sqlite.Querier, issue string, v int64) (DescVersion, error) {
	var out DescVersion
	var prev json.RawMessage
	found := false
	parse := func(raw json.RawMessage) prosemirror.Doc {
		if docnodes.IsBlank(raw) {
			raw = json.RawMessage(docnodes.Blank)
		}
		d, err := docnodes.ParseDoc(raw)
		if err != nil {
			d, _ = docnodes.ParseDoc(json.RawMessage(docnodes.Blank))
		}
		return d
	}
	for r, err := range events.Load(ctx, Store(), q, IssueStream.ID(issue), 0) {
		if err != nil {
			return out, err
		}
		if r.Version > v {
			break
		}
		switch e := r.Data.(type) {
		case IssueCreated:
			if r.Version == v {
				out, found = DescVersion{Actor: r.Actor, At: r.At, Editors: []string{r.Actor}, After: parse(e.Description)}, true
			}
			prev = e.Description
		case DescriptionChanged:
			if r.Version == v {
				out, found = DescVersion{Actor: r.Actor, At: r.At, Editors: e.Editors, Before: parse(prev), After: parse(e.To)}, true
			}
			prev = e.To
		}
	}
	if !found {
		return out, notFound("description version", issue)
	}
	return out, nil
}

// PauseDescriptionEditor pauses the tab's editor of the issue's
// description (collab.Kind.Pause, MP-14.1).
func PauseDescriptionEditor(tx *writer.Tx, session, tab, issue, msg string) error {
	return descDocs.Pause(tx, session, tab, issue, msg)
}
