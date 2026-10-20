package tracker

import (
	"slices"
	"strings"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/search"
)

// The search index's projectors of the tracker's streams (FR-57,
// NFR-44): every issue as a document of search.Issue, every comment, on
// an issue or a page, as one of search.Comment. Their handlers build a
// document from the event and from the document as indexed, never from
// another projection, so a rebuild of the index stands alone.

func issuePeople(reporter, assignee string) []string {
	var p []string
	for _, id := range []string{reporter, assignee} {
		if id != "" && !slices.Contains(p, id) {
			p = append(p, id)
		}
	}
	return p
}

func issueHref(key string) string { return "/" + key }

func patchIssue(tx *writer.Tx, id string, r events.Meta, fn func(*search.Doc)) error {
	return search.Patch(tx, search.Issue, id, func(d *search.Doc) {
		fn(d)
		d.Updated = r.At
	})
}

var issueSearchProjector = events.NewProjector(IssueStream, search.IssuesProjector, search.IssuesVersion,
	events.Handle(IssueCreatedType, func(tx *writer.Tx, r events.Record[string, IssueCreated]) error {
		e := r.Data
		info, _ := readDoc(e.Description)
		return search.Put(tx, search.Issue, search.Doc{
			ID: r.ID, Scope: e.Project, Key: e.Key, Title: e.Summary, Body: info.Text, Href: issueHref(e.Key),
			Labels: slices.Sorted(slices.Values(e.Labels)), People: issuePeople(e.Reporter, e.Assignee), Updated: r.At,
		})
	}),
	events.Handle(SummaryChangedType, func(tx *writer.Tx, r events.Record[string, SummaryChanged]) error {
		return patchIssue(tx, r.ID, r.Meta, func(d *search.Doc) { d.Title = r.Data.To })
	}),
	events.Handle(DescriptionChangedType, func(tx *writer.Tx, r events.Record[string, DescriptionChanged]) error {
		info, _ := readDoc(r.Data.To)
		return patchIssue(tx, r.ID, r.Meta, func(d *search.Doc) { d.Body = info.Text })
	}),
	events.Handle(AssigneeChangedType, func(tx *writer.Tx, r events.Record[string, AssigneeChanged]) error {
		return patchIssue(tx, r.ID, r.Meta, func(d *search.Doc) {
			reporter := ""
			if len(d.People) > 0 {
				reporter = d.People[0]
			}
			d.People = issuePeople(reporter, r.Data.To)
		})
	}),
	events.Handle(LabelsChangedType, func(tx *writer.Tx, r events.Record[string, LabelsChanged]) error {
		return patchIssue(tx, r.ID, r.Meta, func(d *search.Doc) {
			d.Labels = slices.Sorted(slices.Values(applySet(d.Labels, r.Data.Added, r.Data.Removed)))
		})
	}),
	events.Handle(ProjectChangedType, func(tx *writer.Tx, r events.Record[string, ProjectChanged]) error {
		return patchIssue(tx, r.ID, r.Meta, func(d *search.Doc) {
			d.Scope, d.Key, d.Href = r.Data.Project, r.Data.Key, issueHref(r.Data.Key)
			if !slices.Contains(d.OldKeys, strings.ToUpper(r.Data.FromKey)) {
				d.OldKeys = append(d.OldKeys, r.Data.FromKey)
			}
		})
	}),
	events.Handle(IssueDeletedType, func(tx *writer.Tx, r events.Record[string, IssueDeleted]) error {
		return search.Remove(tx, search.Issue, r.ID)
	}),
	events.Handle(IssueHiddenType, func(tx *writer.Tx, r events.Record[string, IssueHidden]) error {
		return search.Hide(tx, search.Issue, r.ID, true)
	}),
	events.Handle(IssueUnhiddenType, func(tx *writer.Tx, r events.Record[string, IssueUnhidden]) error {
		return search.Hide(tx, search.Issue, r.ID, false)
	}),
	events.Ignore(TypeChangedType),
	events.Ignore(StatusChangedType),
	events.Ignore(MovedType),
	events.Ignore(PriorityChangedType),
	events.Ignore(ParentChangedType),
	events.Ignore(DueChangedType),
	events.Ignore(PointsChangedType),
	events.Ignore(EstimateChangedType),
	events.Ignore(RemainingChangedType),
	events.Ignore(SprintChangedType),
	events.Ignore(ComponentsChangedType),
	events.Ignore(WatchersAddedType),
	events.Ignore(WatcherRemovedType),
	events.Ignore(LinkAddedType),
	events.Ignore(PageLinkedType),
	events.Ignore(PageUnlinkedType),
	events.Ignore(LinkRemovedType),
).Tables(search.Tables(search.Issue)...)

var commentSearchProjector = events.NewProjector(CommentStream, search.CommentsProjector, search.CommentsVersion,
	events.Handle(CommentAddedType, func(tx *writer.Tx, r events.Record[string, CommentAdded]) error {
		e := r.Data
		info, _ := readDoc(e.Body)
		return search.Put(tx, search.Comment, search.Doc{ID: r.ID, Scope: e.SubjectID, Key: e.Subject, Body: info.Text, People: []string{r.Actor}, Updated: r.At})
	}),
	events.Handle(CommentEditedType, func(tx *writer.Tx, r events.Record[string, CommentEdited]) error {
		info, _ := readDoc(r.Data.Body)
		return search.Patch(tx, search.Comment, r.ID, func(d *search.Doc) { d.Body, d.Updated = info.Text, r.At })
	}),
	events.Handle(CommentDeletedType, func(tx *writer.Tx, r events.Record[string, CommentDeleted]) error {
		return search.Remove(tx, search.Comment, r.ID)
	}),
	events.Handle(CommentHiddenType, func(tx *writer.Tx, r events.Record[string, CommentHidden]) error {
		return search.Hide(tx, search.Comment, r.ID, true)
	}),
	events.Handle(CommentUnhiddenType, func(tx *writer.Tx, r events.Record[string, CommentUnhidden]) error {
		return search.Hide(tx, search.Comment, r.ID, false)
	}),
).Tables(search.Tables(search.Comment)...)

// DocText is the plain text of a document (a description, a comment, a
// page's body), as search indexes it.
func DocText(raw []byte) string {
	info, _ := readDoc(raw)
	return info.Text
}
