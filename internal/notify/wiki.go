package notify

import (
	"context"
	"slices"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// The wiki's reactions: comments on a page notify its watchers (FR-114),
// and a version that mentions someone for the first time notifies them
// (FR-112). A page's mentions are read from its versions, not from every
// keystroke: a person mentioned and taken out again between two versions
// is told nothing.

func init() {
	commentables[tracker.SubjectPage] = commentable{Kind: PageCommented, At: pageAt}
	extra = append(extra, pageReactions)
}

// pageAt names a page and lists its watchers as of a position.
func pageAt(tx *writer.Tx, id string, upto int64) (Subject, []string, error) {
	s, _, href, err := wiki.PageAt(tx, id, upto)
	if err != nil {
		return Subject{}, nil, err
	}
	return Subject{Kind: SubjectPage, ID: id, Label: s.Title, Href: href}, s.Watchers, nil
}

// pageReactions notify the people a page's new version mentions, who the
// version before did not.
var pageReactions = events.NewProjector(wiki.PageStream, "notify_pages", 1,
	events.After(wiki.VersionCutType, func(ctx context.Context, r events.Record[string, wiki.VersionCut]) error {
		d, err := doc.ParseDoc(r.Data.Doc)
		if err != nil {
			return nil
		}
		now := doc.References(d).People
		if len(now) == 0 {
			return nil
		}
		return react(ctx, r.Position, r.Meta, func(tx *writer.Tx) (Subject, []item, error) {
			s, _, err := pageAt(tx, r.ID, r.Position)
			if err != nil {
				return s, nil, err
			}
			prev, err := wiki.VersionBefore(tx, r.ID, r.Data.Number, r.Position)
			if err != nil {
				return s, nil, err
			}
			var before []string
			if pd, err := doc.ParseDoc(prev); err == nil && prev != nil {
				before = doc.References(pd).People
			}
			var added []string
			for _, p := range now {
				if !slices.Contains(before, p) {
					added = append(added, p)
				}
			}
			return s, mentions(added, r.Data.Doc), nil
		})
	}),
	events.Ignore(wiki.PageCreatedType),
	events.Ignore(wiki.PageRenamedType),
	events.Ignore(wiki.PageMovedType),
	events.Ignore(wiki.PageLabelsChangedType),
	events.Ignore(wiki.PageWatchedType),
	events.Ignore(wiki.PageTrashedType),
	events.Ignore(wiki.PageRestoredType),
	events.Ignore(wiki.PageDeletedType),
).SkipHistory()
