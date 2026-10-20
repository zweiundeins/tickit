package wiki

import (
	"context"

	"github.com/zweiundeins/lith/sqlite"
)

// Pages linked to issues by hand (FR-102), which the tracker's issues
// projector keeps in issue_pages from the issue stream.

// PagesLinkedToIssue are the current pages linked to an issue, by space
// and title.
func PagesLinkedToIssue(ctx context.Context, q sqlite.Querier, issue string) ([]PageRef, error) {
	return pageRefs(ctx, q, refSelect+`WHERE p.state = 'current' AND p.id IN (SELECT page_id FROM issue_pages WHERE issue_id = ?)
		ORDER BY s.name, p.title`, issue)
}

// IssuesLinkedTo are the keys of the issues a page is linked to, which it
// lists under "Linked issues" beside its chips, in key order.
func IssuesLinkedTo(ctx context.Context, q sqlite.Querier, page string) ([]string, error) {
	return strings2(ctx, q, `SELECT i.key FROM issue_pages l JOIN issues i ON i.id = l.issue_id
		WHERE l.page_id = ? AND i.deleted_at IS NULL ORDER BY i.project_key, i.number`, page)
}
