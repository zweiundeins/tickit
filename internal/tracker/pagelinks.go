package tracker

import (
	"database/sql"
	"errors"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Pages linked to an issue by hand (FR-102): "Link page" on an issue adds
// a page beside the pages whose documents show the issue (FR-101), and
// the page lists the issue under "Linked issues". The link is an event on
// the issue's stream, so the issue's history and the log say who linked
// what; the issues projector keeps issue_pages from it, as a rebuild
// does.

// PageLinked links a page to the issue; PageUnlinked takes it off. Title
// is the page's title then, for the history.
type PageLinked struct {
	Page  string `json:"page"`
	Title string `json:"title"`
}

type PageUnlinked struct {
	Page  string `json:"page"`
	Title string `json:"title,omitempty"`
}

func (PageLinked) issueEvent()   {}
func (PageUnlinked) issueEvent() {}

var (
	PageLinkedType   = defineIssue[PageLinked]("issue.page_linked")
	PageUnlinkedType = defineIssue[PageUnlinked]("issue.page_unlinked")
)

// FieldPages is the field of an issue's linked pages, where a refusal
// shows.
const FieldPages = "pages"

func init() { labels[FieldPages] = "linked pages" }

// LinkedPageTopic is the topic of a page's own view (wiki.PageTopic),
// which a link of the page touches: its "Linked issues" follow.
func LinkedPageTopic(page string) string { return "page:" + page }

// LinkPage links a page to an issue, or takes the link off (Unlink): a
// Member's right in the issue's project. The caller checks that the page
// exists and that the actor may read it; Title is its title now.
type LinkPage struct {
	By     tabs.By
	Issue  string
	Page   string
	Title  string
	Unlink bool
}

func (c LinkPage) Validate() error {
	if !ValidID(c.Issue) || !ValidID(c.Page) {
		return errors.New("tracker: a malformed page link")
	}
	return checkBy(c.By)
}

// pageLink is LinkPage with whether the page is linked now, which the
// command reads from issue_pages for the decider.
type pageLink struct {
	LinkPage
	linked bool
}

var pageLinkDecider = issueDecider(func(c pageLink, s IssueState) ([]IssueEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoIssue
	case s.Deleted:
		return nil, errDeleted
	case c.Unlink && !c.linked:
		return nil, nil
	case c.Unlink:
		return []IssueEvent{PageUnlinked{Page: c.Page, Title: c.Title}}, nil
	case c.linked:
		return nil, tabs.Refuse(FieldPages, "This page is linked to the issue already.")
	}
	title, _ := text(c.Title, 0, 255)
	return []IssueEvent{PageLinked{Page: c.Page, Title: title}}, nil
})

func (c LinkPage) Apply(tx *writer.Tx) error {
	s, err := issueState(tx, c.Issue)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.EditIssues, s.Project); err != nil {
		return err
	}
	var one int
	err = tx.QueryRow(`SELECT 1 FROM issue_pages WHERE issue_id = ? AND page_id = ?`, c.Issue, c.Page).Scan(&one)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	cmd := pageLink{LinkPage: c, linked: err == nil}
	if err := events.Execute(Store(), tx, pageLinkDecider, IssueStream.ID(c.Issue), events.Any, cmd, meta(c.By)); err != nil {
		return err
	}
	return acted(tx, s.Project, c.By.Actor.ID, at(c.By))
}

// The issues projector's handlers of page links.
func projectPageLinked(tx *writer.Tx, r events.Record[string, PageLinked]) error {
	if _, err := tx.Exec(`INSERT INTO issue_pages (issue_id, page_id, linked_at, linked_by) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		r.ID, r.Data.Page, millis(r.At), r.Actor); err != nil {
		return err
	}
	tx.Touch(LinkedPageTopic(r.Data.Page))
	return finish(tx, r.ID, r.Version, millis(r.At), true)
}

func projectPageUnlinked(tx *writer.Tx, r events.Record[string, PageUnlinked]) error {
	if _, err := tx.Exec(`DELETE FROM issue_pages WHERE issue_id = ? AND page_id = ?`, r.ID, r.Data.Page); err != nil {
		return err
	}
	tx.Touch(LinkedPageTopic(r.Data.Page))
	return finish(tx, r.ID, r.Version, millis(r.At), true)
}
