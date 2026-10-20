package wiki

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// PageEvent is an event of a page's stream: its title, place, labels,
// watchers, state and versions. Its document is the page's pagedoc
// stream.
type PageEvent interface{ pageEvent() }

// PageCreated starts a page's stream (FR-81): in a space, under a parent
// ("" at the top), at a rank among its siblings. Its creator watches it
// from the start (FR-95.1).
type PageCreated struct {
	Space    string `json:"space"`
	Parent   string `json:"parent,omitempty"`
	Rank     string `json:"rank"`
	Title    string `json:"title"`
	Home     bool   `json:"home,omitempty"`
	Creator  string `json:"creator"`
	Template string `json:"template,omitempty"`
}

// PageRenamed changes the title (FR-86.2).
type PageRenamed struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// PageMoved puts a page in another place (FR-79), or another space.
// Carried names the page whose move or trash carried this one along:
// children move with their page, and up when it goes to the trash.
type PageMoved struct {
	FromSpace  string `json:"from_space,omitempty"`
	Space      string `json:"space"`
	FromParent string `json:"from_parent,omitempty"`
	Parent     string `json:"parent,omitempty"`
	Rank       string `json:"rank"`
	Carried    string `json:"carried,omitempty"`
}

// PageLabelsChanged adds and removes labels (FR-93).
type PageLabelsChanged struct {
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

// PageWatched starts or ends a person's watch of the page (FR-95).
type PageWatched struct {
	Person string `json:"person"`
	Watch  bool   `json:"watch"`
}

// PageTrashed moves a page to its space's trash (FR-94); Parent and Rank
// are where it was, for a restore. Its children moved up first.
type PageTrashed struct {
	Parent string `json:"parent,omitempty"`
	Rank   string `json:"rank"`
}

// PageRestored puts a page back from the trash.
type PageRestored struct {
	Parent string `json:"parent,omitempty"`
	Rank   string `json:"rank"`
}

// PageDeleted deletes a trashed page for good; its title is free again.
type PageDeleted struct{}

// VersionCut is a version of the page (FR-87): its number from 1, the
// pagedoc version it holds, the title and document then, who edited
// since the version before, an optional message, and why it was cut:
// "created", "idle", "left", "interval", "save", "restore", "seed".
type VersionCut struct {
	Number  int             `json:"number"`
	Upto    int64           `json:"upto"`
	Title   string          `json:"title"`
	Doc     json.RawMessage `json:"doc"`
	Editors []string        `json:"editors,omitempty"`
	Message string          `json:"message,omitempty"`
	Reason  string          `json:"reason"`
}

func (PageCreated) pageEvent()       {}
func (PageRenamed) pageEvent()       {}
func (PageMoved) pageEvent()         {}
func (PageLabelsChanged) pageEvent() {}
func (PageWatched) pageEvent()       {}
func (PageTrashed) pageEvent()       {}
func (PageRestored) pageEvent()      {}
func (PageDeleted) pageEvent()       {}
func (VersionCut) pageEvent()        {}

// PageStream is the kind of the pages' streams, keyed by page id.
var PageStream = events.NewStream[string, PageEvent]("page")

func definePage[T PageEvent](name string) events.Type[string, PageEvent, T] {
	return events.Define(PageStream, name, func(t T) PageEvent { return t })
}

// The page event types; the names are what the log stores.
var (
	PageCreatedType       = definePage[PageCreated]("page.created")
	PageRenamedType       = definePage[PageRenamed]("page.renamed")
	PageMovedType         = definePage[PageMoved]("page.moved")
	PageLabelsChangedType = definePage[PageLabelsChanged]("page.labels_changed")
	PageWatchedType       = definePage[PageWatched]("page.watched")
	PageTrashedType       = definePage[PageTrashed]("page.trashed")
	PageRestoredType      = definePage[PageRestored]("page.restored")
	PageDeletedType       = definePage[PageDeleted]("page.deleted")
	VersionCutType        = definePage[VersionCut]("page.version_cut")
)

// The states of a page.
const (
	Current = "current"
	Trashed = "trashed"
	Deleted = "deleted"
)

// PageState is what a page's commands decide on.
type PageState struct {
	Exists   bool     `json:"exists"`
	Space    string   `json:"space"`
	Parent   string   `json:"parent,omitempty"`
	Rank     string   `json:"rank"`
	Title    string   `json:"title"`
	Home     bool     `json:"home,omitempty"`
	State    string   `json:"state"`
	Creator  string   `json:"creator"`
	Labels   []string `json:"labels,omitempty"`
	Watchers []string `json:"watchers,omitempty"`
	// The last version: its number, the pagedoc version and the title it
	// holds, and this stream's version when it was cut.
	Versions int    `json:"versions"`
	CutUpto  int64  `json:"cut_upto"`
	CutTitle string `json:"cut_title"`
	CutAt    int64  `json:"cut_at"`
	Version  int64  `json:"version"`
	// HiddenVersions are the numbers of the versions a moderator hid
	// (moderation.go).
	HiddenVersions []int `json:"hidden_versions,omitempty"`
}

const pageFold = 1

func evolvePage(s PageState, e PageEvent) PageState {
	s.Version++
	switch e := e.(type) {
	case PageCreated:
		s.Exists, s.Space, s.Parent, s.Rank, s.Title, s.Home, s.State, s.Creator = true, e.Space, e.Parent, e.Rank, e.Title, e.Home, Current, e.Creator
		s.Watchers = []string{e.Creator}
	case PageRenamed:
		s.Title = e.To
	case PageMoved:
		s.Space, s.Parent, s.Rank = e.Space, e.Parent, e.Rank
	case PageLabelsChanged:
		s.Labels = applySet(s.Labels, e.Added, e.Removed)
	case PageWatched:
		if e.Watch {
			s.Watchers = applySet(s.Watchers, []string{e.Person}, nil)
		} else {
			s.Watchers = applySet(s.Watchers, nil, []string{e.Person})
		}
	case PageTrashed:
		s.State, s.Parent = Trashed, ""
	case PageRestored:
		s.State, s.Parent, s.Rank = Current, e.Parent, e.Rank
	case PageDeleted:
		s.State = Deleted
	case VersionCut:
		s.Versions, s.CutUpto, s.CutTitle, s.CutAt = e.Number, e.Upto, e.Title, s.Version
	case VersionHidden:
		if !slices.Contains(s.HiddenVersions, e.Number) {
			s.HiddenVersions = append(slices.Clone(s.HiddenVersions), e.Number)
		}
	case VersionUnhidden:
		s.HiddenVersions = slices.DeleteFunc(slices.Clone(s.HiddenVersions), func(n int) bool { return n == e.Number })
	}
	return s
}

// applySet is set with add and without remove, in order, each once.
func applySet(set, add, remove []string) []string {
	out := slices.Clone(set)
	for _, a := range add {
		if !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return slices.DeleteFunc(out, func(x string) bool { return slices.Contains(remove, x) })
}

func pageDecider[C any](decide func(C, PageState) ([]PageEvent, error)) events.Decider[C, PageEvent, PageState] {
	return events.Decider[C, PageEvent, PageState]{Decide: decide, Evolve: evolvePage, Version: pageFold}
}

var peekPageDecider = peekDecider(PageState{}, evolvePage, pageFold)

// pageState reads a page's state inside a command; a page that does not
// exist, or was deleted for good, answers access.ErrNotFound.
func pageState(tx *writer.Tx, id string) (PageState, error) {
	var s PageState
	if !ValidID(id) {
		return s, notFound("page", id)
	}
	if err := events.Execute(Store(), tx, peekPageDecider, PageStream.ID(id), events.Any, peek[PageState]{&s}, events.Meta{}); err != nil {
		return s, err
	}
	if !s.Exists || s.State == Deleted {
		return s, notFound("page", id)
	}
	return s, nil
}

// errGone refuses a change to a page in the trash.
var errGone = &writer.Refusal{Msg: "This page is in the trash."}

// current reads a page that must not be in the trash, and checks the
// actor's right in its space.
func current(tx *writer.Tx, by tabs.By, id string, r access.Right) (PageState, error) {
	s, err := pageState(tx, id)
	if err != nil {
		return s, err
	}
	if err := require(tx, by, r, s.Space); err != nil {
		return s, err
	}
	if s.State != Current {
		return s, errGone
	}
	return s, nil
}

// titleKey is a title compared without case (FR-81.2).
func titleKey(title string) string { return strings.ToLower(title) }

// claimTitle reserves a title in a space for a page, or refuses next to
// the title field when another page of the space has it.
func claimTitle(tx *writer.Tx, space, page, title string) error {
	var other, state string
	err := tx.QueryRow(`SELECT t.page_id, coalesce(p.state, '') FROM page_titles t LEFT JOIN pages p ON p.id = t.page_id
		WHERE t.space_id = ? AND t.title_key = ?`, space, titleKey(title)).Scan(&other, &state)
	switch {
	case err == nil && other == page:
		return nil
	case err == nil && state == Trashed:
		return tabs.Refuse("title", fmt.Sprintf("A page titled %q is in this space's trash.", title))
	case err == nil:
		return tabs.Refuse("title", fmt.Sprintf("A page titled %q exists in this space.", title))
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	if _, err := tx.Exec(`DELETE FROM page_titles WHERE page_id = ?`, page); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO page_titles (space_id, title_key, page_id) VALUES (?, ?, ?)`, space, titleKey(title), page)
	return err
}

// checkTitle checks a title's text (DM-05.1).
func checkTitle(t string) (string, error) {
	title, ok := text(t, 1, MaxTitle)
	if !ok {
		return "", tabs.Refuse("title", "A title has 1 to 255 characters.")
	}
	return title, nil
}

// rankAfter is a rank after sibling after ("" for the last place), or
// before sibling before, among the pages under parent in space.
func rankAt(tx *writer.Tx, space, parent, after, before, self string) (string, error) {
	type sib struct{ id, rank string }
	rows, err := tx.Query(`SELECT id, rank FROM pages WHERE space_id = ? AND parent_id = ? AND state = 'current' AND id != ? ORDER BY rank, id`, space, parent, self)
	if err != nil {
		return "", err
	}
	var sibs []sib
	for rows.Next() {
		var s sib
		if err := rows.Scan(&s.id, &s.rank); err != nil {
			rows.Close()
			return "", err
		}
		sibs = append(sibs, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}
	i := len(sibs) // the place: before sibs[i]
	switch {
	case after != "":
		j := slices.IndexFunc(sibs, func(s sib) bool { return s.id == after })
		if j < 0 {
			return "", tabs.Refuse("place", "That page is not under the parent any more.")
		}
		i = j + 1
	case before != "":
		j := slices.IndexFunc(sibs, func(s sib) bool { return s.id == before })
		if j < 0 {
			return "", tabs.Refuse("place", "That page is not under the parent any more.")
		}
		i = j
	}
	var lo, hi string
	if i > 0 {
		lo = sibs[i-1].rank
	}
	if i < len(sibs) {
		hi = sibs[i].rank
	}
	if lo != "" && hi != "" && lo >= hi {
		hi = "" // two siblings share a rank: go after the lower one
	}
	if lo == "" && hi == "" {
		return tracker.FirstRank, nil
	}
	return tracker.RankBetween(lo, hi)
}

// parentOK checks that parent is "" or a current page of space.
func parentOK(tx *writer.Tx, space, parent string) error {
	if parent == "" {
		return nil
	}
	var sp string
	err := tx.QueryRow(`SELECT space_id FROM page_parents WHERE page_id = ?`, parent).Scan(&sp)
	if errors.Is(err, sql.ErrNoRows) || err == nil && sp != space {
		return tabs.Refuse("parent", "The parent page is not in this space any more.")
	}
	return err
}

// descendants are the pages under page, its children first, from
// page_parents.
func descendants(tx *writer.Tx, page string) ([]string, error) {
	rows, err := tx.Query(`WITH RECURSIVE d(id, depth) AS (SELECT page_id, 1 FROM page_parents WHERE parent_id = ?
		UNION ALL SELECT p.page_id, d.depth + 1 FROM page_parents p JOIN d ON p.parent_id = d.id WHERE d.depth < 64)
		SELECT id FROM d ORDER BY depth, id`, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func setParent(tx *writer.Tx, page, space, parent string) error {
	_, err := tx.Exec(`INSERT INTO page_parents (page_id, space_id, parent_id) VALUES (?, ?, ?)
		ON CONFLICT (page_id) DO UPDATE SET space_id = excluded.space_id, parent_id = excluded.parent_id`, page, space, parent)
	return err
}

// newPage is what createPage makes.
type newPage struct {
	id, space, parent, after, title, template, message string
	home                                               bool
	doc                                                json.RawMessage
}

var createPageDecider = pageDecider(func(c PageCreated, s PageState) ([]PageEvent, error) {
	if s.Exists {
		return nil, errors.New("wiki: the page exists")
	}
	return []PageEvent{c}, nil
})

// createPage appends a page's first events: page.created, its document's
// pagedoc.created and version 1 (FR-81).
func createPage(tx *writer.Tx, by tabs.By, p newPage) error {
	title, err := checkTitle(p.title)
	if err != nil {
		return err
	}
	d, err := parseDoc(p.doc)
	if err != nil {
		return err
	}
	if err := parentOK(tx, p.space, p.parent); err != nil {
		return err
	}
	if err := claimTitle(tx, p.space, p.id, title); err != nil {
		return err
	}
	rank, err := rankAt(tx, p.space, p.parent, p.after, "", p.id)
	if err != nil {
		return err
	}
	if err := setParent(tx, p.id, p.space, p.parent); err != nil {
		return err
	}
	m := meta(by)
	e := PageCreated{Space: p.space, Parent: p.parent, Rank: rank, Title: title, Home: p.home, Creator: by.Actor.ID, Template: p.template}
	if err := events.Execute(Store(), tx, createPageDecider, PageStream.ID(p.id), events.NoStream, e, m); err != nil {
		return err
	}
	raw := marshalDoc(d)
	epoch, err := newEpoch(tx)
	if err != nil {
		return err
	}
	if err := events.Execute(Store(), tx, createDocDecider, PageDocStream.ID(p.id), events.NoStream, DocCreated{Epoch: epoch, Doc: raw}, m); err != nil {
		return err
	}
	msg, _ := text(p.message, 0, MaxMessage)
	cut := VersionCut{Number: 1, Upto: 1, Title: title, Doc: raw, Editors: []string{by.Actor.ID}, Message: msg, Reason: "created"}
	return events.Execute(Store(), tx, cutDecider, PageStream.ID(p.id), events.Any, cut, m)
}

// CreatePage creates a page in a space, under Parent ("" at the top),
// after the sibling After or last, blank or with a template's document
// (FR-81, FR-92). Its result is {"id"} as JSON.
type CreatePage struct {
	By       tabs.By
	ID       string // optional
	Space    string
	Parent   string
	After    string
	Title    string
	Doc      json.RawMessage // empty: a blank page
	Template string          // the template's name, for the record
	Message  string          // version 1's, optional
}

func (c CreatePage) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	for _, id := range []string{c.ID, c.Parent, c.After} {
		if id != "" && !ValidID(id) {
			return errors.New("wiki: a malformed page id")
		}
	}
	return nil
}

func (c CreatePage) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c CreatePage) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if _, err := spaceState(tx, c.Space); err != nil {
		return nil, err
	}
	if err := require(tx, c.By, access.EditIssues, c.Space); err != nil {
		return nil, err
	}
	id := c.ID
	if id == "" {
		id = NewID()
	}
	if err := createPage(tx, c.By, newPage{id: id, space: c.Space, parent: c.Parent, after: c.After, title: c.Title, doc: c.Doc, template: c.Template, message: c.Message}); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": id})
}

// RenamePage changes a page's title (FR-86.2). A title that another page
// of the space has, compared without case, is refused next to the title.
type RenamePage struct {
	By    tabs.By
	Page  string
	Title string
}

func (c RenamePage) Validate() error { return checkBy(c.By) }

var renamePage = pageDecider(func(c PageRenamed, s PageState) ([]PageEvent, error) {
	if s.Title == c.To {
		return nil, nil
	}
	c.From = s.Title
	return []PageEvent{c}, nil
})

func (c RenamePage) Apply(tx *writer.Tx) error {
	s, err := current(tx, c.By, c.Page, access.EditIssues)
	if err != nil {
		return err
	}
	title, err := checkTitle(c.Title)
	if err != nil {
		return err
	}
	if title == s.Title {
		return nil
	}
	if err := claimTitle(tx, s.Space, c.Page, title); err != nil {
		return err
	}
	return events.Execute(Store(), tx, renamePage, PageStream.ID(c.Page), events.Any, PageRenamed{To: title}, meta(c.By))
}

// MovePage puts a page under another parent ("" for the top of the
// space), after the sibling After or before Before (neither: last), in
// its space or in Space. Its children move with it (FR-79).
type MovePage struct {
	By            tabs.By
	Page          string
	Space         string // empty: the page's own
	Parent        string
	After, Before string
	First         bool // the first place among the new siblings
}

func (c MovePage) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	for _, id := range []string{c.Parent, c.After, c.Before} {
		if id != "" && !ValidID(id) {
			return errors.New("wiki: a malformed page id")
		}
	}
	return nil
}

var movePage = pageDecider(func(c PageMoved, s PageState) ([]PageEvent, error) {
	switch {
	case !s.Exists:
		return nil, errors.New("wiki: no such page")
	case s.Space == c.Space && s.Parent == c.Parent && s.Rank == c.Rank:
		return nil, nil
	}
	c.FromSpace, c.FromParent = s.Space, s.Parent
	if c.FromSpace == c.Space {
		c.FromSpace = ""
	}
	return []PageEvent{c}, nil
})

func (c MovePage) Apply(tx *writer.Tx) error {
	s, err := current(tx, c.By, c.Page, access.EditIssues)
	if err != nil {
		return err
	}
	if s.Home {
		return tabs.Refuse("parent", "The home page stays at the top of its space.")
	}
	space := c.Space
	if space == "" {
		space = s.Space
	}
	if space != s.Space {
		if _, err := spaceState(tx, space); err != nil {
			return err
		}
		if err := require(tx, c.By, access.EditIssues, space); err != nil {
			return err
		}
	}
	if c.Parent == c.Page {
		return tabs.Refuse("parent", "A page cannot move under itself.")
	}
	under, err := descendants(tx, c.Page)
	if err != nil {
		return err
	}
	if slices.Contains(under, c.Parent) {
		return tabs.Refuse("parent", "A page cannot move under one of its own pages.")
	}
	if err := parentOK(tx, space, c.Parent); err != nil {
		return err
	}
	before := c.Before
	if c.First && c.After == "" && before == "" {
		err := tx.QueryRow(`SELECT id FROM pages WHERE space_id = ? AND parent_id = ? AND state = 'current' AND id != ? ORDER BY rank, id LIMIT 1`,
			space, c.Parent, c.Page).Scan(&before)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	rank, err := rankAt(tx, space, c.Parent, c.After, before, c.Page)
	if err != nil {
		return err
	}
	m := meta(c.By)
	if space != s.Space {
		// FR-79.3: every title that moves must be free in the target space.
		for _, id := range append([]string{c.Page}, under...) {
			ps, err := pageState(tx, id)
			if err != nil {
				return err
			}
			if err := claimTitle(tx, space, id, ps.Title); err != nil {
				return err
			}
		}
		for _, id := range under {
			ps, err := pageState(tx, id)
			if err != nil {
				return err
			}
			if err := setParent(tx, id, space, ps.Parent); err != nil {
				return err
			}
			e := PageMoved{Space: space, Parent: ps.Parent, Rank: ps.Rank, Carried: c.Page}
			if err := events.Execute(Store(), tx, movePage, PageStream.ID(id), events.Any, e, m); err != nil {
				return err
			}
		}
	}
	if err := setParent(tx, c.Page, space, c.Parent); err != nil {
		return err
	}
	return events.Execute(Store(), tx, movePage, PageStream.ID(c.Page), events.Any, PageMoved{Space: space, Parent: c.Parent, Rank: rank}, m)
}

// SetPageLabels adds and removes a page's labels (FR-93).
type SetPageLabels struct {
	By          tabs.By
	Page        string
	Add, Remove []string
}

func (c SetPageLabels) Validate() error { return checkBy(c.By) }

var setLabels = pageDecider(func(c SetPageLabels, s PageState) ([]PageEvent, error) {
	norm := func(in []string) ([]string, error) {
		var out []string
		for _, l := range in {
			n, ok := tracker.NormalizeLabel(l)
			if !ok {
				return nil, tabs.Refuse("labels", "A label has 1 to 50 characters and no spaces.")
			}
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
		return out, nil
	}
	add, err := norm(c.Add)
	if err != nil {
		return nil, err
	}
	remove, err := norm(c.Remove)
	if err != nil {
		return nil, err
	}
	var e PageLabelsChanged
	for _, l := range add {
		if !slices.Contains(s.Labels, l) && !slices.Contains(remove, l) {
			e.Added = append(e.Added, l)
		}
	}
	for _, l := range remove {
		if slices.Contains(s.Labels, l) {
			e.Removed = append(e.Removed, l)
		}
	}
	if len(e.Added) == 0 && len(e.Removed) == 0 {
		return nil, nil
	}
	if len(s.Labels)+len(e.Added)-len(e.Removed) > MaxLabels {
		return nil, tabs.Refuse("labels", "A page has at most 20 labels.")
	}
	return []PageEvent{e}, nil
})

func (c SetPageLabels) Apply(tx *writer.Tx) error {
	if _, err := current(tx, c.By, c.Page, access.EditIssues); err != nil {
		return err
	}
	return events.Execute(Store(), tx, setLabels, PageStream.ID(c.Page), events.Any, c, meta(c.By))
}

// WatchPage starts or ends the actor's watch of a page (FR-95): anyone
// who may read it.
type WatchPage struct {
	By    tabs.By
	Page  string
	Watch bool
}

func (c WatchPage) Validate() error { return checkBy(c.By) }

var watchPage = pageDecider(func(c PageWatched, s PageState) ([]PageEvent, error) {
	if slices.Contains(s.Watchers, c.Person) == c.Watch {
		return nil, nil
	}
	return []PageEvent{c}, nil
})

func (c WatchPage) Apply(tx *writer.Tx) error {
	if _, err := current(tx, c.By, c.Page, access.Read); err != nil {
		return err
	}
	return events.Execute(Store(), tx, watchPage, PageStream.ID(c.Page), events.Any, PageWatched{Person: c.By.Actor.ID, Watch: c.Watch}, meta(c.By))
}

// TrashPage moves a page to its space's trash, and its children up to
// its parent, where it was (FR-94.1): its creator does, the space's
// Admin, or a moderator. The home page stays.
type TrashPage struct {
	By   tabs.By
	Page string
}

func (c TrashPage) Validate() error { return checkBy(c.By) }

var trashPage = pageDecider(func(c PageTrashed, s PageState) ([]PageEvent, error) {
	if s.State != Current {
		return nil, nil
	}
	c.Parent, c.Rank = s.Parent, s.Rank
	return []PageEvent{c}, nil
})

// mayTrash checks the right to trash or restore page s.
func mayTrash(tx *writer.Tx, by tabs.By, s PageState) error {
	if s.Creator == by.Actor.ID {
		if err := require(tx, by, access.TrashOwnPage, s.Space); err == nil {
			return nil
		}
	}
	return require(tx, by, access.TrashPage, s.Space)
}

func (c TrashPage) Apply(tx *writer.Tx) error {
	s, err := pageState(tx, c.Page)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.Read, s.Space); err != nil {
		return err
	}
	if s.State != Current {
		return nil
	}
	if s.Home {
		return &writer.Refusal{Msg: "The home page of a space cannot go to the trash."}
	}
	if err := mayTrash(tx, c.By, s); err != nil {
		return err
	}
	m := meta(c.By)
	// The children move up to the page's place, in their order.
	rows, err := tx.Query(`SELECT id FROM pages WHERE parent_id = ? AND state = 'current' ORDER BY rank, id`, c.Page)
	if err != nil {
		return err
	}
	var children []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		children = append(children, id)
	}
	rows.Close()
	after := c.Page
	for _, id := range children {
		rank, err := rankAt(tx, s.Space, s.Parent, after, "", id)
		if err != nil {
			return err
		}
		if err := setParent(tx, id, s.Space, s.Parent); err != nil {
			return err
		}
		e := PageMoved{Space: s.Space, Parent: s.Parent, Rank: rank, Carried: c.Page}
		if err := events.Execute(Store(), tx, movePage, PageStream.ID(id), events.Any, e, m); err != nil {
			return err
		}
		after = id
	}
	if _, err := tx.Exec(`DELETE FROM page_parents WHERE page_id = ?`, c.Page); err != nil {
		return err
	}
	return events.Execute(Store(), tx, trashPage, PageStream.ID(c.Page), events.Any, PageTrashed{}, m)
}

// RestorePage puts a trashed page back where it was, or at the top of
// its space when its parent is gone (FR-94.2): the space's Admin does.
type RestorePage struct {
	By   tabs.By
	Page string
}

func (c RestorePage) Validate() error { return checkBy(c.By) }

var restorePage = pageDecider(func(c PageRestored, s PageState) ([]PageEvent, error) {
	if s.State != Trashed {
		return nil, nil
	}
	return []PageEvent{c}, nil
})

func (c RestorePage) Apply(tx *writer.Tx) error {
	s, err := pageState(tx, c.Page)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.TrashPage, s.Space); err != nil {
		return err
	}
	if s.State != Trashed {
		return nil
	}
	// Where it was, from its trash event, and who trashed it.
	var was PageTrashed
	var by string
	for r, err := range events.Load(tx.Context(), Store(), tx, PageStream.ID(c.Page), 0) {
		if err != nil {
			return err
		}
		if t, ok := r.Data.(PageTrashed); ok {
			was, by = t, r.Actor
		}
	}
	// What a moderator hid, only a moderator restores (DM-07.1).
	if !access.SiteCan(c.By.Actor, access.Moderate) {
		staff, err := people.Staff(tx.Context(), tx, by)
		if err != nil {
			return err
		}
		if staff {
			return &access.Denial{Refusal: writer.Refusal{Msg: "A moderator hid this page: only a moderator restores it."}}
		}
	}
	parent := was.Parent
	if parentOK(tx, s.Space, parent) != nil {
		parent = ""
	}
	rank, err := rankAt(tx, s.Space, parent, "", "", c.Page)
	if err != nil {
		return err
	}
	if parent == was.Parent && was.Rank != "" {
		var taken int
		if err := tx.QueryRow(`SELECT count(*) FROM pages WHERE space_id = ? AND parent_id = ? AND rank = ? AND state = 'current'`, s.Space, parent, was.Rank).Scan(&taken); err != nil {
			return err
		}
		if taken == 0 {
			rank = was.Rank
		}
	}
	if err := setParent(tx, c.Page, s.Space, parent); err != nil {
		return err
	}
	return events.Execute(Store(), tx, restorePage, PageStream.ID(c.Page), events.Any, PageRestored{Parent: parent, Rank: rank}, meta(c.By))
}

// DeletePage deletes a trashed page for good (FR-94.2): the space's
// Admin does. Its title is free again; links to it show "a removed page".
type DeletePage struct {
	By   tabs.By
	Page string
}

func (c DeletePage) Validate() error { return checkBy(c.By) }

var deletePage = pageDecider(func(c DeletePage, s PageState) ([]PageEvent, error) {
	if s.State != Trashed {
		return nil, &writer.Refusal{Msg: "Only a page in the trash can be deleted."}
	}
	return []PageEvent{PageDeleted{}}, nil
})

func (c DeletePage) Apply(tx *writer.Tx) error {
	s, err := pageState(tx, c.Page)
	if err != nil {
		return err
	}
	if err := require(tx, c.By, access.TrashPage, s.Space); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM page_titles WHERE page_id = ?`, c.Page); err != nil {
		return err
	}
	return events.Execute(Store(), tx, deletePage, PageStream.ID(c.Page), events.Any, c, meta(c.By))
}
