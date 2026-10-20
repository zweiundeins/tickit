package tracker

import (
	"testing"

	"github.com/zweiundeins/lith/events/eventstest"
)

// FR-102: a page links to an issue once; unlinking a page not linked does
// nothing; a deleted issue takes no link.
func TestPageLinkDecider(t *testing.T) {
	d := eventstest.For(pageLinkDecider).Given(story)
	d.When(pageLink{LinkPage: LinkPage{Page: "p1", Title: "Plan"}}).Then(t, PageLinked{Page: "p1", Title: "Plan"})
	d.When(pageLink{LinkPage: LinkPage{Page: "p1"}, linked: true}).ThenError(t, refuse(FieldPages, "This page is linked to the issue already."))
	d.When(pageLink{LinkPage: LinkPage{Page: "p1", Title: "Plan", Unlink: true}, linked: true}).Then(t, PageUnlinked{Page: "p1", Title: "Plan"})
	d.When(pageLink{LinkPage: LinkPage{Page: "p1", Unlink: true}}).Then(t)
	eventstest.For(pageLinkDecider).Given(story, IssueDeleted{}).When(pageLink{LinkPage: LinkPage{Page: "p1"}}).ThenError(t, errDeleted)
	eventstest.For(pageLinkDecider).When(pageLink{LinkPage: LinkPage{Page: "p1"}}).ThenError(t, errNoIssue)
}
