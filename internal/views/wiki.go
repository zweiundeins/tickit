package views

import (
	"context"
	"io"
	"time"

	"github.com/a-h/templ"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/ui"
	"github.com/mbolli/tickit/internal/wiki"
)

// The wiki's regions, which the stream patches by id.
const (
	SpacesID    = "spaces-list"
	PageHeadID  = "page-head"
	PageBodyID  = "page-body"
	PageRefsID  = "page-refs"
	CommentsID  = "page-comments"
	TreeID      = "page-tree"
	EditorID    = "editor"
	HistoryID   = "page-history"
	TrashListID = "space-trash"
	PickID      = "wiki-pick"
)

// PagePresenceID is the id of a page's presence row, with editors marked.
func PagePresenceID(page string) string { return "presence-page-" + page }

// SpaceItem is a space in the list of spaces (FR-75.1).
type SpaceItem struct {
	wiki.Space
}

// SpacesProps is the page of all spaces.
type SpacesProps struct {
	Spaces    []wiki.Space
	CanCreate bool
	Now       time.Time
}

// PageProps is a page in view (FR-80).
type PageProps struct {
	Page     wiki.Page
	Space    wiki.Space
	URL      string
	Me       people.Person
	CanEdit  bool
	CanTrash bool
	CanAdmin bool // the space's Admin or a moderator: comments, trash
	Watching bool
	// Head is the shared head: breadcrumbs, title, labels, byline.
	Head PageHead
	// Body is the document's HTML for the first paint.
	Body     string
	Refs     PageRefs
	Comments []CommentItem
	// Spaces and Pages are the targets of the Move dialog.
	Spaces    []wiki.Space
	Targets   []MoveTarget
	Templates []wiki.Template
	// Activity is the space's feed, on its home page (FR-69).
	Activity templ.Component
	Now      time.Time
}

// PageHead is what the page's shared view shows above the body.
type PageHead struct {
	Crumbs   []ui.Crumb
	Title    string
	Labels   []string
	State    string
	Creator  people.Person
	Created  time.Time
	Editor   people.Person
	Edited   time.Time
	Versions int
	History  string // the history's address
	Now      time.Time
}

// PageRefs are the issues a page shows as chips and the pages that link
// to it (FR-80.1, FR-103.2).
type PageRefs struct {
	Issues     []ui.IssueChipProps
	LinkedFrom []wiki.PageRef
}

// CommentItem is a comment with its author and replies (FR-90).
type CommentItem struct {
	wiki.Comment
	Author  people.Person
	HTML    string
	Replies []CommentItem
}

// MoveTarget is a place a page can move under.
type MoveTarget struct {
	Value, Label string
	Depth        int
}

// EditorProps is the editor's page (FR-86, docs/editor.md).
type EditorProps struct {
	PageProps
	Host EditorHost
}

// EditorHost is the sb-rich-editor element, as the GET and every frame of
// the tab's editor view render it: every attribute every time, because
// Datastar's morph removes those a frame leaves out.
type EditorHost struct {
	Page, Epoch, Client, State string
	Base, Head                 int64
	Batches, Chips, Refusal    string
	Images                     string
	Readonly                   bool
	// Doc is the document at Base, for the GET only; frames leave the
	// script empty and the morph keeps it.
	Doc string
}

// VersionsProps is a page's history (FR-87.2).
type VersionsProps struct {
	PageProps
	Versions []VersionItem
}

// VersionItem is a version in the history.
type VersionItem struct {
	wiki.VersionInfo
	Editors []people.Person
	URL     string
}

// VersionProps is an old version in view (FR-87.3), or a comparison of
// two (FR-88).
type VersionProps struct {
	PageProps
	Version  VersionItem
	Newest   int
	HTML     string
	Compare  bool
	From, To int
	Numbers  []int
	Inserted int
	Deleted  int
}

// TrashProps is a space's trash (FR-94.2).
type TrashProps struct {
	Space    wiki.Space
	Pages    []TrashItem
	CanAdmin bool
	Now      time.Time
}

// TrashItem is a trashed page with who trashed it.
type TrashItem struct {
	wiki.Page
	By people.Person
}

// PickItem is a result of an editor picker: a page, a person or an issue.
type PickItem struct {
	Kind, ID, Label, Meta string
}

// ownStyle shows the controls of the viewer's own comments: shared views
// know nothing of who looks at them.
func ownStyle(me people.Person, moderates, edits bool) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		css := `.wk-own{display:none}`
		if !edits {
			css += `.wk-edit{display:none}`
		}
		if people.ValidID(me.ID) {
			css += `[data-author="` + me.ID + `"]>.wk-comment__actions .wk-own{display:inline-flex}`
		}
		if moderates {
			css += `.wk-mod{display:inline-flex}`
		} else {
			css += `.wk-mod{display:none}`
		}
		_, err := io.WriteString(w, `<style>`+css+`</style>`)
		return err
	})
}

// The wiki's Datastar expressions: constants, or built from constants;
// ids reach them through data attributes, never inside an expression
// (NFR-13).
var (
	wikiSignals = `{_wk: {create: {op: '', title: '', template: '', parent: '', msg: ''}, space: {op: '', key: '', name: '', description: '', msg: ''},` +
		` move: {op: '', target: '', place: 'last', msg: ''}, label: {op: '', name: '', msg: ''}, watch: {op: ''}, trash: {op: '', msg: ''},` +
		` comment: {op: '', body: '', to: '', toName: '', msg: ''}, cmod: {op: '', msg: ''}, version: {op: '', message: '', msg: ''}, restore: {op: '', msg: ''}}}`

	createSpace        = Act("_wk.space.op", "/cmd/spaces/create", "key: $_wk.space.key, name: $_wk.space.name, description: $_wk.space.description")
	createSpaceSettled = Settle("_wk.space.op", "$_wk.space.msg = o.ok ? '' : o.msg")
	createSpaceFailed  = Failed("_wk.space.op")

	createPage        = Act("_wk.create.op", "/cmd/pages/create", "space: el.dataset.space, parent: $_wk.create.parent, title: $_wk.create.title, template: $_wk.create.template")
	createPageSettled = Settle("_wk.create.op", "$_wk.create.msg = o.ok ? '' : o.msg")
	createPageFailed  = Failed("_wk.create.op")
	// childOf sets the new page's parent from the button.
	childOf = `$_wk.create.parent = el.dataset.parent, $_wk.create.title = '', $_wk.create.msg = ''`

	movePage        = Act("_wk.move.op", "/cmd/pages/move", "page: el.dataset.page, target: $_wk.move.target, place: $_wk.move.place")
	movePageSettled = Settle("_wk.move.op", "o.ok ? document.getElementById('move').close() : ($_wk.move.msg = o.msg)")
	movePageFailed  = Failed("_wk.move.op")

	addLabel        = `$_wk.label.name.trim() && ` + Act("_wk.label.op", "/cmd/pages/labels", "page: el.dataset.page, add: [$_wk.label.name]")
	removeLabel     = Act("_wk.label.op", "/cmd/pages/labels", "page: el.closest('[data-page]').dataset.page, remove: [el.dataset.label]")
	labelSettled    = Settle("_wk.label.op", "o.ok ? ($_wk.label.name = '', $_wk.label.msg = '') : ($_wk.label.msg = o.msg)")
	labelFailed     = Failed("_wk.label.op")
	watchPage       = Act("_wk.watch.op", "/cmd/pages/watch", "page: el.dataset.page, watch: el.getAttribute('aria-pressed') !== 'true'")
	watchSettled    = Settle("_wk.watch.op", "o.ok && document.querySelectorAll('[data-watch]').forEach(b => b.setAttribute('aria-pressed', b.getAttribute('aria-pressed') === 'true' ? 'false' : 'true'))")
	trashPage       = `confirm('Move this page to the trash? Its child pages move up to its parent.') && ` + Act("_wk.trash.op", "/cmd/pages/trash", "page: el.dataset.page")
	restoreTrashed  = Act("_wk.trash.op", "/cmd/pages/restore", "page: el.dataset.page")
	deleteTrashed   = `confirm('Delete this page for good? This cannot be undone.') && ` + Act("_wk.trash.op", "/cmd/pages/delete", "page: el.dataset.page")
	trashSettled    = Settle("_wk.trash.op", "$_wk.trash.msg = o.ok ? '' : o.msg")
	trashFailed     = Failed("_wk.trash.op")
	addComment      = `$_wk.comment.body.trim() && ` + Act("_wk.comment.op", "/cmd/pages/comment", "page: el.dataset.page, parent: $_wk.comment.to, body: $_wk.comment.body")
	commentSettled  = Settle("_wk.comment.op", "o.ok ? ($_wk.comment.body = '', $_wk.comment.to = '', $_wk.comment.msg = '') : ($_wk.comment.msg = o.msg)")
	commentFailed   = Failed("_wk.comment.op")
	replyTo         = `$_wk.comment.to = el.dataset.comment, $_wk.comment.toName = el.dataset.name, document.getElementById('comment-body').focus()`
	deleteComment   = `confirm('Delete this comment?') && ` + Act("_wk.cmod.op", "/cmd/pages/comment/delete", "comment: el.dataset.comment")
	hideComment     = Act("_wk.cmod.op", "/cmd/pages/comment/hide", "comment: el.dataset.comment, hidden: el.dataset.hidden !== 'true'")
	cmodSettled     = Settle("_wk.cmod.op", "$_wk.cmod.msg = o.ok ? '' : o.msg")
	cmodFailed      = Failed("_wk.cmod.op")
	saveVersion     = Act("_wk.version.op", "/cmd/pages/save-version", "page: el.dataset.page, message: $_wk.version.message")
	versionSettled  = Settle("_wk.version.op", "o.ok ? ($_wk.version.message = '', $_wk.version.msg = 'Saved as a new version.', document.getElementById('save-version').close()) : ($_wk.version.msg = o.msg)")
	versionFailed   = Failed("_wk.version.op")
	restoreVersion  = `confirm('Make this version the current one? Everyone editing sees the change.') && ` + Act("_wk.restore.op", "/cmd/pages/restore-version", "page: el.dataset.page, number: +el.dataset.number")
	restoreSettled  = Settle("_wk.restore.op", "o.ok ? location.assign(el.dataset.back) : ($_wk.restore.msg = o.msg)")
	restoreFailed   = Failed("_wk.restore.op")
	compareVersions = `location.assign(el.dataset.base + '?from=' + $_wk.cmp.from + '&to=' + $_wk.cmp.to)`
)
