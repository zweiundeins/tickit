package views

import (
	"encoding/json"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// ListsID is the editor page's element that the view pagelists:<page>
// patches with every issue list's rows, which the editor's node views
// show (FR-104); the editor's host names it in its lists attribute.
const ListsID = "wk-lists"

// EmbeddedList is what an issue list on a page shows (FR-104): its
// description, its rows as one reader may see them, and the issue list
// with its filter.
type EmbeddedList struct {
	Desc    string // trusted HTML: doc.ListDescription
	Note    string
	Missing string // why it shows no rows: its saved filter was deleted
	Results tracker.Results
	Limit   int
	People  map[string]people.Person
	Href    string // the issue list with this filter
}

// Shown are the rows the list shows.
func (l EmbeddedList) Shown() []tracker.Issue {
	if len(l.Results.Issues) > l.Limit {
		return l.Results.Issues[:l.Limit]
	}
	return l.Results.Issues
}

// ListRows is one issue list's rows in the editor's lists element, by
// the list's key (doc.IssueList.Key): trusted HTML of IssueListEmbed.
type ListRows struct {
	Key  string
	HTML string
}

// PageListsProps is the editor page's lists element: the saved filters an
// issue list may name and the rows of the page's lists.
type PageListsProps struct {
	Filters []tracker.SavedFilter
	Lists   []ListRows
}

// PageIssueProps is "Create issue" on a selection of a page's text
// (FR-105): the projects the person may create issues in and the one
// preselected.
type PageIssueProps struct {
	Page     string
	Projects []tracker.Project
	Project  string
}

// PageIssueID is the id of the dialog that creates an issue from a
// page's selected text.
const PageIssueID = "page-issue"

func pageIssueSignals(project string) string {
	b, _ := json.Marshal(map[string]any{"_pissue": map[string]any{
		"op": "", "msg": "", "project": project, "summary": "", "find": "", "block": -1, "nth": 0,
		"track": "", "key": "", "chip": false, "done": "",
	}})
	return string(b)
}

// The expressions of "Create issue" (FR-105). selWatch shows the button
// under a selection of text: in the editor from its sb-select events, in
// view mode from the document's selection inside the page's body. The
// button keeps the selection (mousedown), takes its text as the summary
// and opens the dialog: in the editor it keeps the selection under an id
// the editor maps through every change (track), in view mode it notes the
// top-level block, the text and which of its occurrences in that block
// it is, for the server to find.
const (
	selWatch = `(() => { const pop = el, body = document.getElementById('` + PageBodyID + `'), ed = document.getElementById('` + EditorID + `'); ` +
		`const place = (r) => { pop.hidden = false; const o = (pop.offsetParent || document.body).getBoundingClientRect(); ` +
		`pop.style.top = (r.bottom - o.top + 6) + 'px'; pop.style.left = Math.max(0, r.left - o.left) + 'px' }; ` +
		`if (ed) { ed.addEventListener('sb-select', (e) => e.detail.empty ? (pop.hidden = true) : place(e.detail)); return } ` +
		`document.addEventListener('selectionchange', () => { const s = getSelection(); ` +
		`if (!body || !s || s.isCollapsed || !s.rangeCount || !body.contains(s.anchorNode) || !body.contains(s.focusNode) || !s.toString().trim()) { pop.hidden = true; return } ` +
		`place(s.getRangeAt(0).getBoundingClientRect()) }) })()`
	selCreate = `((ed) => { const s = getSelection(); ` +
		`$_pissue.summary = (ed ? ed.selectionText() : String(s || '')).replace(/\s+/g, ' ').trim().slice(0, 255); ` +
		`$_pissue.msg = '', $_pissue.done = '', $_pissue.find = '', $_pissue.block = -1, $_pissue.nth = 0, $_pissue.track = ''; ` +
		`if (ed) { $_pissue.track = ` + newOp + `; ed.track($_pissue.track) } ` +
		`else if (s && s.rangeCount) { const body = document.getElementById('` + PageBodyID + `'), r = s.getRangeAt(0); let b = r.startContainer; ` +
		`while (b && b.parentNode !== body) b = b.parentNode; ` +
		`if (b) { const find = s.toString(), pre = document.createRange(); pre.setStart(b, 0); pre.setEnd(r.startContainer, r.startOffset); ` +
		`const at = pre.toString().length, all = b.textContent; let n = 0; for (let i = all.indexOf(find); i >= 0 && i < at; i = all.indexOf(find, i + 1)) n++; ` +
		`$_pissue.find = find, $_pissue.block = [...body.children].indexOf(b), $_pissue.nth = n } } ` +
		`el.closest('.wk-sel').hidden = true; document.getElementById('` + PageIssueID + `').showModal() })(document.getElementById('` + EditorID + `'))`
	keepSelection = `evt.preventDefault()`
)

var (
	pageIssueCreate = `$_pissue.summary.trim() && ` + Act("_pissue.op", "/cmd/pages/issue",
		"page: el.dataset.page, project: $_pissue.project, summary: $_pissue.summary, find: $_pissue.find, block: $_pissue.block, nth: $_pissue.nth")
	pageIssueSettled = Settle("_pissue.op", "o.ok ? document.getElementById('"+PageIssueID+"').close() : ($_pissue.msg = o.msg)")
	pageIssueFailed  = Failed("_pissue.op")
	// pageIssueDone puts the new issue's chip in place of the selection
	// in the editor (an editor commit, which co-editors apply), and says
	// what happened.
	pageIssueDone = `$_pissue.key && ((ed, key) => { const chip = ed && $_pissue.track ? ed.chipTracked($_pissue.track, key) : $_pissue.chip; ` +
		`$_pissue.done = 'Created ' + key + ', linked to this page' + (chip ? '.' : '. Its chip was not added: the text changed, or the selection spans more than one paragraph.'); ` +
		`$_pissue.key = '', $_pissue.track = '' })(document.getElementById('` + EditorID + `'), $_pissue.key)`
	pageIssueClosed = `document.getElementById('` + EditorID + `')?.focus()`
)
