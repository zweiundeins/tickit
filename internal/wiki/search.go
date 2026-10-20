package wiki

import (
	"strings"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/search"
)

// pageSearch indexes pages for search (FR-57, docs/architecture.md
// "Search"): the title from the page's events, the body from each
// version (page.version_cut), so the index follows versions and not
// keystrokes; the space's key comes from its scopes row, which commands
// write. Page comments are a stream of their own ("page_comment") that
// this index does not cover yet.
var pageSearch = events.NewProjector(PageStream, search.PagesProjector, search.PagesVersion,
	events.Handle(PageCreatedType, func(tx *writer.Tx, r events.Record[string, PageCreated]) error {
		key, err := spaceKey(tx, r.Data.Space)
		if err != nil {
			return err
		}
		return search.Put(tx, search.Page, search.Doc{ID: r.ID, Scope: r.Data.Space, Title: r.Data.Title, Place: key,
			Href: PageURL(key, r.ID, r.Data.Title), People: []string{r.Data.Creator}, Updated: r.At})
	}),
	events.Handle(PageRenamedType, func(tx *writer.Tx, r events.Record[string, PageRenamed]) error {
		return search.Patch(tx, search.Page, r.ID, func(d *search.Doc) {
			d.Title, d.Updated = r.Data.To, r.At
			d.Href = PageURL(d.Place, r.ID, r.Data.To)
		})
	}),
	events.Handle(PageMovedType, func(tx *writer.Tx, r events.Record[string, PageMoved]) error {
		if r.Data.FromSpace == "" {
			return nil
		}
		key, err := spaceKey(tx, r.Data.Space)
		if err != nil {
			return err
		}
		return search.Patch(tx, search.Page, r.ID, func(d *search.Doc) {
			d.Scope, d.Place, d.Href = r.Data.Space, key, PageURL(key, r.ID, d.Title)
		})
	}),
	events.Handle(PageLabelsChangedType, func(tx *writer.Tx, r events.Record[string, PageLabelsChanged]) error {
		return search.Patch(tx, search.Page, r.ID, func(d *search.Doc) { d.Labels = applySet(d.Labels, r.Data.Added, r.Data.Removed) })
	}),
	events.Ignore(PageWatchedType),
	events.Ignore(VersionHiddenType),
	events.Ignore(VersionUnhiddenType),
	events.Handle(PageTrashedType, func(tx *writer.Tx, r events.Record[string, PageTrashed]) error {
		return search.Hide(tx, search.Page, r.ID, true)
	}),
	events.Handle(PageRestoredType, func(tx *writer.Tx, r events.Record[string, PageRestored]) error {
		return search.Hide(tx, search.Page, r.ID, false)
	}),
	events.Handle(PageDeletedType, func(tx *writer.Tx, r events.Record[string, PageDeleted]) error {
		return search.Remove(tx, search.Page, r.ID)
	}),
	events.Handle(VersionCutType, func(tx *writer.Tx, r events.Record[string, VersionCut]) error {
		text := ""
		if d, err := doc.ParseDoc(r.Data.Doc); err == nil {
			text = doc.Text(d)
		}
		return search.Patch(tx, search.Page, r.ID, func(d *search.Doc) {
			d.Body, d.Updated = strings.TrimSpace(text), r.At
			d.People = applySet(d.People, r.Data.Editors, nil)
		})
	}),
).Tables(search.Tables(search.Page)...)

// spaceKey is a space's key from its scopes row.
func spaceKey(tx *writer.Tx, space string) (string, error) {
	var key string
	err := tx.QueryRow(`SELECT key FROM scopes WHERE kind = 'space' AND id = ?`, space).Scan(&key)
	return key, err
}
