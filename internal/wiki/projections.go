package wiki

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/doc"
	"github.com/mbolli/tickit/internal/tracker"
)

// The projectors' versions: bump one when its handlers change, and the
// runner rebuilds its tables from the events.
const (
	spacesVersion   = 1
	pagesVersion    = 1
	versionsVersion = 1
	docsVersion     = 1
)

// Checkpoint is how many commits page_doc's document may lag behind the
// head: a reader that misses the cache applies at most that many.
const Checkpoint = 50

// spacesProjector keeps spaces.
var spacesProjector = events.NewProjector(SpaceStream, "wiki_spaces", spacesVersion,
	events.Handle(SpaceCreatedType, func(tx *writer.Tx, r events.Record[string, SpaceCreated]) error {
		e := r.Data
		_, err := tx.Exec(`INSERT INTO spaces (id, key, name, description, home_id, open, archived, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`, r.ID, e.Key, e.Name, e.Description, e.Home, e.Open, r.Actor, millis(r.At))
		tx.Touch(SpacesTopic, SpaceTopic(r.ID))
		return err
	}),
	events.Handle(SpaceEditedType, func(tx *writer.Tx, r events.Record[string, SpaceEdited]) error {
		_, err := tx.Exec(`UPDATE spaces SET name = coalesce(?, name), description = coalesce(?, description) WHERE id = ?`, r.Data.Name, r.Data.Description, r.ID)
		tx.Touch(SpacesTopic, SpaceTopic(r.ID), PagesTopic)
		return err
	}),
	events.Handle(SpaceArchivedType, func(tx *writer.Tx, r events.Record[string, SpaceArchived]) error {
		_, err := tx.Exec(`UPDATE spaces SET archived = 1 WHERE id = ?`, r.ID)
		tx.Touch(SpacesTopic, SpaceTopic(r.ID))
		return err
	}),
	events.Handle(SpaceUnarchivedType, func(tx *writer.Tx, r events.Record[string, SpaceUnarchived]) error {
		_, err := tx.Exec(`UPDATE spaces SET archived = 0 WHERE id = ?`, r.ID)
		tx.Touch(SpacesTopic, SpaceTopic(r.ID))
		return err
	}),
).Tables("spaces")

// pagesProjector keeps pages, page_labels and page_watchers.
var pagesProjector = events.NewProjector(PageStream, "wiki_pages", pagesVersion,
	events.Handle(PageCreatedType, func(tx *writer.Tx, r events.Record[string, PageCreated]) error {
		e := r.Data
		if _, err := tx.Exec(`INSERT INTO pages (id, space_id, parent_id, rank, title, home, state, created_by, created_at, edited_by, edited_at, version, search)
			VALUES (?, ?, ?, ?, ?, ?, 'current', ?, ?, ?, ?, ?, ?)`, r.ID, e.Space, e.Parent, e.Rank, e.Title, e.Home,
			r.Actor, millis(r.At), r.Actor, millis(r.At), r.Version, strings.ToLower(e.Title)); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO page_watchers (page_id, person_id, since) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, r.ID, e.Creator, millis(r.At)); err != nil {
			return err
		}
		return touchPage(tx, r.ID, e.Space)
	}),
	events.Handle(PageRenamedType, func(tx *writer.Tx, r events.Record[string, PageRenamed]) error {
		if err := setPage(tx, r, `title = ?, search = ?, edited_by = ?, edited_at = ?`, r.Data.To, strings.ToLower(r.Data.To), r.Actor, millis(r.At)); err != nil {
			return err
		}
		return touchRefs(tx, r.ID)
	}),
	events.Handle(PageMovedType, func(tx *writer.Tx, r events.Record[string, PageMoved]) error {
		e := r.Data
		if e.FromSpace != "" {
			tx.Touch(SpaceTopic(e.FromSpace))
			if _, err := tx.Exec(`UPDATE page_labels SET space_id = ? WHERE page_id = ?`, e.Space, r.ID); err != nil {
				return err
			}
		}
		return setPage(tx, r, `space_id = ?, parent_id = ?, rank = ?`, e.Space, e.Parent, e.Rank)
	}),
	events.Handle(PageLabelsChangedType, func(tx *writer.Tx, r events.Record[string, PageLabelsChanged]) error {
		var space string
		if err := tx.QueryRow(`SELECT space_id FROM pages WHERE id = ?`, r.ID).Scan(&space); err != nil {
			return err
		}
		for _, l := range r.Data.Removed {
			if _, err := tx.Exec(`DELETE FROM page_labels WHERE page_id = ? AND label = ?`, r.ID, l); err != nil {
				return err
			}
		}
		for _, l := range r.Data.Added {
			if _, err := tx.Exec(`INSERT INTO page_labels (page_id, space_id, label) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, r.ID, space, l); err != nil {
				return err
			}
		}
		return setPage(tx, r, `labels = (SELECT coalesce(json_group_array(label), '[]') FROM (SELECT label FROM page_labels WHERE page_id = pages.id ORDER BY label))`)
	}),
	events.Handle(PageWatchedType, func(tx *writer.Tx, r events.Record[string, PageWatched]) error {
		q := `DELETE FROM page_watchers WHERE page_id = ? AND person_id = ?`
		args := []any{r.ID, r.Data.Person}
		if r.Data.Watch {
			q = `INSERT INTO page_watchers (page_id, person_id, since) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`
			args = append(args, millis(r.At))
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return err
		}
		return setPage(tx, r, `version = version`)
	}),
	events.Handle(PageTrashedType, func(tx *writer.Tx, r events.Record[string, PageTrashed]) error {
		if err := setPage(tx, r, `state = 'trashed', parent_id = '', trashed_by = ?, trashed_at = ?, trash_parent = ?, trash_rank = ?`,
			r.Actor, millis(r.At), r.Data.Parent, r.Data.Rank); err != nil {
			return err
		}
		tx.Touch(DocTopic(r.ID))
		return touchRefs(tx, r.ID)
	}),
	events.Handle(PageRestoredType, func(tx *writer.Tx, r events.Record[string, PageRestored]) error {
		if err := setPage(tx, r, `state = 'current', parent_id = ?, rank = ?, trashed_by = '', trashed_at = NULL, trash_parent = '', trash_rank = ''`,
			r.Data.Parent, r.Data.Rank); err != nil {
			return err
		}
		tx.Touch(DocTopic(r.ID))
		return touchRefs(tx, r.ID)
	}),
	events.Handle(PageDeletedType, func(tx *writer.Tx, r events.Record[string, PageDeleted]) error {
		if err := setPage(tx, r, `state = 'deleted'`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM page_labels WHERE page_id = ?`, r.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM page_watchers WHERE page_id = ?`, r.ID); err != nil {
			return err
		}
		tx.Touch(DocTopic(r.ID))
		return touchRefs(tx, r.ID)
	}),
	events.Handle(VersionCutType, func(tx *writer.Tx, r events.Record[string, VersionCut]) error {
		var by string
		if n := len(r.Data.Editors); n > 0 {
			by = r.Data.Editors[n-1]
		} else {
			by = r.Actor
		}
		if r.Data.Number == 1 {
			return setPage(tx, r, `version = version`)
		}
		return setPage(tx, r, `edited_by = ?, edited_at = ?`, by, millis(r.At))
	}),
).Tables("pages", "page_labels", "page_watchers")

// setPage updates a page's row, records its stream version and touches
// what shows it.
func setPage[T any](tx *writer.Tx, r events.Record[string, T], columns string, args ...any) error {
	if _, err := tx.Exec(`UPDATE pages SET `+columns+`, version = ? WHERE id = ?`, append(args, r.Version, r.ID)...); err != nil {
		return err
	}
	var space string
	if err := tx.QueryRow(`SELECT space_id FROM pages WHERE id = ?`, r.ID).Scan(&space); err != nil {
		return err
	}
	return touchPage(tx, r.ID, space)
}

func touchPage(tx *writer.Tx, id, space string) error {
	tx.Touch(PageTopic(id), SpaceTopic(space), PagesTopic)
	return nil
}

// touchRefs touches what shows a page's title or state elsewhere: the
// pages that link to it (their chips) and the issues it mentions (their
// lists of pages, FR-101).
func touchRefs(tx *writer.Tx, id string) error {
	rows, err := tx.Query(`SELECT 'p', page_id FROM page_links WHERE target_id = ?
		UNION SELECT 'i', k.issue_id FROM page_chips c JOIN issue_keys k ON k.key = c.key WHERE c.page_id = ?`, id, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, other string
		if err := rows.Scan(&kind, &other); err != nil {
			return err
		}
		if kind == "p" {
			tx.Touch(ChipsTopic(other))
		} else {
			tx.Touch(tracker.IssueTopic(other))
		}
	}
	return rows.Err()
}

// versionsProjector keeps page_versions.
var versionsProjector = events.NewProjector(PageStream, "wiki_versions", versionsVersion,
	events.Ignore(PageCreatedType),
	events.Ignore(PageRenamedType),
	events.Ignore(PageMovedType),
	events.Ignore(PageLabelsChangedType),
	events.Ignore(PageWatchedType),
	events.Ignore(PageTrashedType),
	events.Ignore(PageRestoredType),
	events.Ignore(PageDeletedType),
	events.Handle(VersionCutType, func(tx *writer.Tx, r events.Record[string, VersionCut]) error {
		e := r.Data
		editors, _ := json.Marshal(nonNil(e.Editors))
		_, err := tx.Exec(`INSERT INTO page_versions (page_id, number, upto, title, doc, editors, message, reason, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, e.Number, e.Upto, e.Title, canonical(e.Doc), string(editors), e.Message, e.Reason, r.Actor, millis(r.At))
		tx.Touch(PageTopic(r.ID))
		return err
	}),
).Tables("page_versions")

// canonical is a document's JSON as internal/doc writes it: the stored
// event's JSON escapes <, > and &, the event in a live command does not,
// and a rebuild must write what the live projection wrote.
func canonical(raw json.RawMessage) string {
	d, err := doc.ParseDoc(raw)
	if err != nil {
		return string(raw)
	}
	return string(marshalDoc(d))
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// docsProjector keeps page_doc, page_chips and page_links: the head of
// each document, with its document as of a checkpoint, and what it
// refers to.
var docsProjector = events.NewProjector(PageDocStream, "wiki_docs", docsVersion,
	events.Handle(DocCreatedType, func(tx *writer.Tx, r events.Record[string, DocCreated]) error {
		d, err := doc.ParseDoc(r.Data.Doc)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO page_doc (page_id, epoch, version, ref, checkpoint, doc, bytes, blocks, last_at) VALUES (?, ?, ?, '', ?, ?, ?, ?, ?)
			ON CONFLICT (page_id) DO UPDATE SET epoch = excluded.epoch, version = excluded.version, ref = '', checkpoint = excluded.checkpoint,
			doc = excluded.doc, bytes = excluded.bytes, blocks = excluded.blocks, last_at = excluded.last_at`,
			r.ID, r.Data.Epoch, r.Version, r.Version, string(marshalDoc(d)), d.JSONLen(), d.Blocks(), millis(r.At)); err != nil {
			return err
		}
		docs.put(docKey{r.ID, r.Data.Epoch, r.Version, ""}, d)
		tx.Touch(DocTopic(r.ID))
		return refsChanged(tx, r.ID, d)
	}),
	events.Handle(DocSnapshotType, func(tx *writer.Tx, r events.Record[string, DocSnapshot]) error {
		d, err := doc.ParseDoc(r.Data.Doc)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO page_doc (page_id, epoch, version, ref, checkpoint, doc, bytes, blocks, last_at) VALUES (?, ?, ?, '', ?, ?, ?, ?, ?)
			ON CONFLICT (page_id) DO UPDATE SET epoch = excluded.epoch, version = excluded.version, ref = '', checkpoint = excluded.checkpoint,
			doc = excluded.doc, bytes = excluded.bytes, blocks = excluded.blocks, last_at = excluded.last_at`,
			r.ID, r.Data.Epoch, r.Version, r.Version, string(marshalDoc(d)), d.JSONLen(), d.Blocks(), millis(r.At))
		if err != nil {
			return err
		}
		docs.put(docKey{r.ID, r.Data.Epoch, r.Version, ""}, d)
		return refsChanged(tx, r.ID, d)
	}),
	events.Handle(DocCommittedType, func(tx *writer.Tx, r events.Record[string, DocCommitted]) error {
		var epoch, ref string
		var version, checkpoint int64
		if err := tx.QueryRow(`SELECT epoch, version, ref, checkpoint FROM page_doc WHERE page_id = ?`, r.ID).Scan(&epoch, &version, &ref, &checkpoint); err != nil {
			return err
		}
		d, ok := docs.get(docKey{r.ID, epoch, r.Version, r.Data.Ref})
		if !ok {
			// A rebuild, or a cache that let it go: the document before,
			// and this commit's steps.
			before, err := headDoc(tx.Context(), tx, r.ID, epoch, version, ref, false)
			if err != nil {
				return err
			}
			steps, err := doc.ParseSteps(r.Data.Steps)
			if err != nil {
				return err
			}
			if d, err = doc.Apply(before, steps); err != nil {
				return err
			}
			docs.put(docKey{r.ID, epoch, r.Version, r.Data.Ref}, d)
		}
		if r.Version-checkpoint >= Checkpoint {
			if _, err := tx.Exec(`UPDATE page_doc SET checkpoint = ?, doc = ? WHERE page_id = ?`, r.Version, string(marshalDoc(d)), r.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE page_doc SET version = ?, ref = ?, bytes = ?, blocks = ?, last_at = ? WHERE page_id = ?`,
			r.Version, r.Data.Ref, d.JSONLen(), d.Blocks(), millis(r.At), r.ID); err != nil {
			return err
		}
		tx.Touch(DocTopic(r.ID))
		if len(r.Data.Steps) <= 2 { // [] : nothing changed
			return nil
		}
		return refsChanged(tx, r.ID, d)
	}),
).Tables("page_doc", "page_chips", "page_links")

// refsChanged brings page_chips and page_links to what d refers to, and
// touches what shows them: the page's chips, the issues whose lists of
// pages change (FR-101.2), the pages that gain or lose a link from it
// (FR-103.2).
func refsChanged(tx *writer.Tx, page string, d doc.Doc) error {
	refs := doc.References(d)
	var issues []string
	keys, err := column(tx, `SELECT key FROM page_chips WHERE page_id = ? ORDER BY key`, page)
	if err != nil {
		return err
	}
	if !slices.Equal(keys, refs.Issues) {
		for _, k := range keys {
			if !slices.Contains(refs.Issues, k) {
				if _, err := tx.Exec(`DELETE FROM page_chips WHERE page_id = ? AND key = ?`, page, k); err != nil {
					return err
				}
				issues = append(issues, k)
			}
		}
		for _, k := range refs.Issues {
			if !slices.Contains(keys, k) {
				if _, err := tx.Exec(`INSERT INTO page_chips (page_id, key) VALUES (?, ?)`, page, k); err != nil {
					return err
				}
				issues = append(issues, k)
			}
		}
		tx.Touch(ChipsTopic(page), PageTopic(page))
	}
	for _, k := range issues {
		ids, err := column(tx, `SELECT issue_id FROM issue_keys WHERE key = ?`, k)
		if err != nil {
			return err
		}
		for _, id := range ids {
			tx.Touch(tracker.IssueTopic(id))
		}
	}
	targets, err := column(tx, `SELECT target_id FROM page_links WHERE page_id = ? ORDER BY target_id`, page)
	if err != nil {
		return err
	}
	if !slices.Equal(targets, refs.Pages) {
		for _, t := range targets {
			if !slices.Contains(refs.Pages, t) {
				if _, err := tx.Exec(`DELETE FROM page_links WHERE page_id = ? AND target_id = ?`, page, t); err != nil {
					return err
				}
				tx.Touch(PageTopic(t))
			}
		}
		for _, t := range refs.Pages {
			if !slices.Contains(targets, t) {
				if _, err := tx.Exec(`INSERT INTO page_links (page_id, target_id) VALUES (?, ?)`, page, t); err != nil {
					return err
				}
				tx.Touch(PageTopic(t))
			}
		}
		tx.Touch(ChipsTopic(page))
	}
	return nil
}

// column reads one text column of a query's rows.
func column(tx *writer.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// chipsProjector writes nothing: it touches the chips of every page that
// shows an issue whose chip changes (FR-100.2), from the issue's keys in
// issue_keys and the pages' in page_chips.
var chipsProjector = events.NewProjector(tracker.IssueStream, "wiki_chips", 1,
	events.Handle(tracker.IssueCreatedType, chipChanged[tracker.IssueCreated]),
	events.Handle(tracker.SummaryChangedType, chipChanged[tracker.SummaryChanged]),
	events.Ignore(tracker.DescriptionChangedType),
	events.Handle(tracker.TypeChangedType, chipChanged[tracker.TypeChanged]),
	events.Handle(tracker.StatusChangedType, chipChanged[tracker.StatusChanged]),
	events.Handle(tracker.MovedType, chipChanged[tracker.Moved]),
	events.Ignore(tracker.PriorityChangedType),
	events.Ignore(tracker.AssigneeChangedType),
	events.Ignore(tracker.ParentChangedType),
	events.Ignore(tracker.DueChangedType),
	events.Ignore(tracker.PointsChangedType),
	events.Ignore(tracker.EstimateChangedType),
	events.Ignore(tracker.RemainingChangedType),
	events.Ignore(tracker.SprintChangedType),
	events.Ignore(tracker.LabelsChangedType),
	events.Ignore(tracker.ComponentsChangedType),
	events.Ignore(tracker.WatchersAddedType),
	events.Ignore(tracker.WatcherRemovedType),
	events.Ignore(tracker.LinkAddedType),
	events.Ignore(tracker.LinkRemovedType),
	events.Handle(tracker.IssueDeletedType, chipChanged[tracker.IssueDeleted]),
	events.Handle(tracker.ProjectChangedType, chipChanged[tracker.ProjectChanged]),
	events.Handle(tracker.IssueHiddenType, chipChanged[tracker.IssueHidden]),
	events.Handle(tracker.IssueUnhiddenType, chipChanged[tracker.IssueUnhidden]),
)

func chipChanged[T any](tx *writer.Tx, r events.Record[string, T]) error {
	pages, err := column(tx, `SELECT DISTINCT c.page_id FROM issue_keys k JOIN page_chips c ON c.key = k.key WHERE k.issue_id = ?`, r.ID)
	if err != nil {
		return err
	}
	for _, p := range pages {
		tx.Touch(ChipsTopic(p))
	}
	return nil
}
