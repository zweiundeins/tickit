package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/demo/seed"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/people"
)

// Space is a space as pages show it.
type Space struct {
	ID, Key, Name, Description string
	Home                       string
	Open, Archived             bool
	CreatedBy                  string
	CreatedAt                  time.Time
	Pages                      int // current pages
}

const spaceColumns = `s.id, s.key, s.name, s.description, s.home_id, s.open, s.archived, s.created_by, s.created_at,
	(SELECT count(*) FROM pages p WHERE p.space_id = s.id AND p.state = 'current')`

func scanSpace(r interface{ Scan(...any) error }) (Space, error) {
	var s Space
	var at int64
	err := r.Scan(&s.ID, &s.Key, &s.Name, &s.Description, &s.Home, &s.Open, &s.Archived, &s.CreatedBy, &at, &s.Pages)
	s.CreatedAt = time.UnixMilli(at)
	return s, err
}

// Spaces lists every space by name (FR-75.1).
func Spaces(ctx context.Context, q sqlite.Querier) ([]Space, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+spaceColumns+` FROM spaces s ORDER BY s.archived, s.name COLLATE NOCASE, s.key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Space
	for rows.Next() {
		s, err := scanSpace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SpaceByKey reads a space by its key, without regard to case.
func SpaceByKey(ctx context.Context, q sqlite.Querier, key string) (Space, error) {
	s, err := scanSpace(q.QueryRowContext(ctx, `SELECT `+spaceColumns+` FROM spaces s WHERE s.key = ?`, strings.ToUpper(key)))
	if errors.Is(err, sql.ErrNoRows) {
		return s, notFound("space", key)
	}
	return s, err
}

// SpaceByID reads a space.
func SpaceByID(ctx context.Context, q sqlite.Querier, id string) (Space, error) {
	s, err := scanSpace(q.QueryRowContext(ctx, `SELECT `+spaceColumns+` FROM spaces s WHERE s.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return s, notFound("space", id)
	}
	return s, err
}

// SpaceName is a space's name, without the counts of SpaceByID.
func SpaceName(ctx context.Context, q sqlite.Querier, id string) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT name FROM spaces WHERE id = ?`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", notFound("space", id)
	}
	return name, err
}

// Page is a page as pages show it.
type Page struct {
	ID, Space, Parent, Rank, Title string
	Home                           bool
	State                          string
	Labels                         []string
	CreatedBy                      string
	CreatedAt                      time.Time
	EditedBy                       string
	EditedAt                       time.Time
	TrashedBy                      string
	TrashedAt                      time.Time
	Version                        int64
	Versions                       int // the number of the newest version
	SpaceKey, SpaceName            string
}

const pageColumns = `p.id, p.space_id, p.parent_id, p.rank, p.title, p.home, p.state, p.labels, p.created_by, p.created_at,
	p.edited_by, p.edited_at, p.trashed_by, coalesce(p.trashed_at, 0), p.version,
	coalesce((SELECT max(number) FROM page_versions v WHERE v.page_id = p.id), 0), s.key, s.name`

const pageFrom = ` FROM pages p JOIN spaces s ON s.id = p.space_id`

func scanPage(r interface{ Scan(...any) error }) (Page, error) {
	var p Page
	var labels string
	var created, edited, trashed int64
	err := r.Scan(&p.ID, &p.Space, &p.Parent, &p.Rank, &p.Title, &p.Home, &p.State, &labels, &p.CreatedBy, &created,
		&p.EditedBy, &edited, &p.TrashedBy, &trashed, &p.Version, &p.Versions, &p.SpaceKey, &p.SpaceName)
	if err != nil {
		return p, err
	}
	json.Unmarshal([]byte(labels), &p.Labels)
	p.CreatedAt, p.EditedAt = time.UnixMilli(created), time.UnixMilli(edited)
	if trashed > 0 {
		p.TrashedAt = time.UnixMilli(trashed)
	}
	return p, nil
}

func pages(ctx context.Context, q sqlite.Querier, where string, args ...any) ([]Page, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+pageColumns+pageFrom+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PageByID reads a page, current or trashed; a deleted one answers
// access.ErrNotFound.
func PageByID(ctx context.Context, q sqlite.Querier, id string) (Page, error) {
	if !ValidID(id) {
		return Page{}, notFound("page", id)
	}
	p, err := scanPage(q.QueryRowContext(ctx, `SELECT `+pageColumns+pageFrom+` WHERE p.id = ? AND p.state != 'deleted'`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, notFound("page", id)
	}
	return p, err
}

// Node is a page in its space's tree.
type Node struct {
	Page
	Children []*Node
}

// Tree is a space's current pages as a tree, in their order (FR-78).
func Tree(ctx context.Context, q sqlite.Querier, space string) ([]*Node, error) {
	list, err := pages(ctx, q, `p.space_id = ? AND p.state = 'current' ORDER BY p.home DESC, p.rank, p.id`, space)
	if err != nil {
		return nil, err
	}
	byID := map[string]*Node{}
	for _, p := range list {
		byID[p.ID] = &Node{Page: p}
	}
	var roots []*Node
	for _, p := range list {
		n := byID[p.ID]
		if parent, ok := byID[p.Parent]; ok && p.Parent != "" {
			parent.Children = append(parent.Children, n)
		} else {
			roots = append(roots, n)
		}
	}
	return roots, nil
}

// Ancestors are a page's ancestors, the top first.
func Ancestors(ctx context.Context, q sqlite.Querier, page string) ([]Page, error) {
	var out []Page
	seen := map[string]bool{page: true}
	p, err := PageByID(ctx, q, page)
	if err != nil {
		return nil, err
	}
	for p.Parent != "" && !seen[p.Parent] && len(out) < 64 {
		seen[p.Parent] = true
		if p, err = PageByID(ctx, q, p.Parent); err != nil {
			break
		}
		out = append([]Page{p}, out...)
	}
	return out, nil
}

// Trash lists a space's trashed pages, the latest first (FR-94.2).
func Trash(ctx context.Context, q sqlite.Querier, space string) ([]Page, error) {
	return pages(ctx, q, `p.space_id = ? AND p.state = 'trashed' ORDER BY p.trashed_at DESC`, space)
}

// Labelled lists the current pages with a label (FR-93), by title.
func Labelled(ctx context.Context, q sqlite.Querier, label string) ([]Page, error) {
	return pages(ctx, q, `p.state = 'current' AND p.id IN (SELECT page_id FROM page_labels WHERE label = ?) ORDER BY p.title`, label)
}

// Watching reports whether a person watches a page (FR-95).
func Watching(ctx context.Context, q sqlite.Querier, page, person string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM page_watchers WHERE page_id = ? AND person_id = ?`, page, person).Scan(&n)
	return n > 0, err
}

// Watchers are the people who watch a page, for notifications (FR-114).
func Watchers(ctx context.Context, q sqlite.Querier, page string) ([]string, error) {
	return strings2(ctx, q, `SELECT person_id FROM page_watchers WHERE page_id = ? ORDER BY since`, page)
}

func strings2(ctx context.Context, q sqlite.Querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
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

// VersionInfo is a version in a page's history (FR-87.2).
type VersionInfo struct {
	Number  int
	Upto    int64
	Title   string
	Editors []string
	Message string
	Reason  string
	By      string
	At      time.Time
	Doc     json.RawMessage // Version only
	Hidden  bool            // by a moderator (moderation.go)
}

// Versions lists a page's versions, the newest first.
func Versions(ctx context.Context, q sqlite.Querier, page string) ([]VersionInfo, error) {
	rows, err := q.QueryContext(ctx, `SELECT number, upto, title, editors, message, reason, created_by, created_at, hidden_at IS NOT NULL
		FROM page_versions WHERE page_id = ? ORDER BY number DESC`, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VersionInfo
	for rows.Next() {
		var v VersionInfo
		var editors string
		var at int64
		if err := rows.Scan(&v.Number, &v.Upto, &v.Title, &editors, &v.Message, &v.Reason, &v.By, &at, &v.Hidden); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(editors), &v.Editors)
		v.At = time.UnixMilli(at)
		out = append(out, v)
	}
	return out, rows.Err()
}

// Version reads one version with its document.
func Version(ctx context.Context, q sqlite.Querier, page string, number int) (VersionInfo, doc.Doc, error) {
	var v VersionInfo
	var editors, raw string
	var at int64
	err := q.QueryRowContext(ctx, `SELECT number, upto, title, editors, message, reason, created_by, created_at, doc, hidden_at IS NOT NULL
		FROM page_versions WHERE page_id = ? AND number = ?`, page, number).
		Scan(&v.Number, &v.Upto, &v.Title, &editors, &v.Message, &v.Reason, &v.By, &at, &raw, &v.Hidden)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil, notFound("version", page)
	}
	if err != nil {
		return v, nil, err
	}
	json.Unmarshal([]byte(editors), &v.Editors)
	v.At = time.UnixMilli(at)
	d, err := doc.ParseDoc([]byte(raw))
	return v, d, err
}

// Comment is a comment below a page, with its replies (FR-90).
type Comment struct {
	ID, Parent, Author string
	Body               json.RawMessage
	CreatedAt          time.Time
	EditedAt           time.Time
	Deleted, Hidden    bool
	Replies            []Comment
}

// Comments lists a page's comments in the order they came, each with its
// replies; the inline ones are InlineComments'.
func Comments(ctx context.Context, q sqlite.Querier, page string) ([]Comment, error) {
	list, err := allComments(ctx, q, page)
	if err != nil {
		return nil, err
	}
	inline := map[string]bool{}
	rows, err := q.QueryContext(ctx, `SELECT comment_id FROM page_inline WHERE page_id = ?`, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		inline[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(c Comment) bool { return inline[c.ID] }), nil
}

// allComments are a page's comments, inline ones too.
func allComments(ctx context.Context, q sqlite.Querier, page string) ([]Comment, error) {
	rows, err := q.QueryContext(ctx, `SELECT c.id, coalesce(t.parent_id, ''), c.author, coalesce(c.body, ''), c.created_at, coalesce(c.edited_at, 0),
		c.deleted_at IS NOT NULL, c.hidden_at IS NOT NULL
		FROM comments c LEFT JOIN page_comment_threads t ON t.comment_id = c.id
		WHERE c.subject = 'page' AND c.subject_id = ? ORDER BY c.created_at, c.id`, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var top []Comment
	var replies []Comment
	for rows.Next() {
		var c Comment
		var body string
		var created, edited int64
		if err := rows.Scan(&c.ID, &c.Parent, &c.Author, &body, &created, &edited, &c.Deleted, &c.Hidden); err != nil {
			return nil, err
		}
		c.Body, c.CreatedAt = json.RawMessage(body), time.UnixMilli(created)
		if edited > 0 {
			c.EditedAt = time.UnixMilli(edited)
		}
		if c.Parent == "" {
			top = append(top, c)
		} else {
			replies = append(replies, c)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, r := range replies {
		for i := range top {
			if top[i].ID == r.Parent {
				top[i].Replies = append(top[i].Replies, r)
			}
		}
	}
	return top, nil
}

// PageRef is another page as a list names it.
type PageRef struct {
	ID, Title, SpaceKey, SpaceName string
}

// URL is the page's address.
func (p PageRef) URL() string { return PageURL(p.SpaceKey, p.ID, p.Title) }

func pageRefs(ctx context.Context, q sqlite.Querier, query string, args ...any) ([]PageRef, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PageRef
	for rows.Next() {
		var p PageRef
		if err := rows.Scan(&p.ID, &p.Title, &p.SpaceKey, &p.SpaceName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const refSelect = `SELECT p.id, p.title, s.key, s.name FROM pages p JOIN spaces s ON s.id = p.space_id `

// PagesMentioning lists the current pages whose head document holds a
// chip of the issue key, by space and title (FR-101). The issue page
// lists them, and watches IssueTopic, which the wiki touches when a
// page's chips of the issue come or go, or the page's title or state
// changes. Old keys of a moved issue count too: pass each of them.
func PagesMentioning(ctx context.Context, q sqlite.Querier, issueKey string) ([]PageRef, error) {
	return pageRefs(ctx, q, refSelect+`WHERE p.state = 'current' AND p.id IN (SELECT page_id FROM page_chips WHERE key = ?)
		ORDER BY s.name, p.title`, strings.ToUpper(issueKey))
}

// LinkedFrom lists the current pages whose head document links to page
// (FR-103.2).
func LinkedFrom(ctx context.Context, q sqlite.Querier, page string) ([]PageRef, error) {
	return pageRefs(ctx, q, refSelect+`WHERE p.state = 'current' AND p.id IN (SELECT page_id FROM page_links WHERE target_id = ?)
		ORDER BY s.name, p.title`, page)
}

// SearchPages finds current pages whose title holds the text, without
// regard to case, the titles that start with it first (FR-103.1): for
// the link dialog. The search area indexes bodies (FR-57).
func SearchPages(ctx context.Context, q sqlite.Querier, text string, limit int) ([]PageRef, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	return pageRefs(ctx, q, refSelect+`WHERE p.state = 'current' AND instr(p.search, ?) > 0
		ORDER BY instr(p.search, ?) != 1, p.title COLLATE NOCASE LIMIT ?`, t, t, limit)
}

// LinkedIssues are the issue keys the page's head document shows as
// chips (FR-80.1).
func LinkedIssues(ctx context.Context, q sqlite.Querier, page string) ([]string, error) {
	return strings2(ctx, q, `SELECT key FROM page_chips WHERE page_id = ? ORDER BY key`, page)
}

// Resolve is what a document's chips, mentions and page links show now
// (docs/editor.md): the issues by key from the tracker's tables, people's
// names, pages' titles.
func Resolve(ctx context.Context, q sqlite.Querier, refs doc.Refs) (doc.StaticResolver, error) {
	r := doc.StaticResolver{Issues: map[string]doc.Chip{}, People: map[string]string{}, Pages: map[string]string{}}
	for _, k := range refs.Issues {
		var c doc.Chip
		var deleted bool
		err := q.QueryRowContext(ctx, `SELECT i.summary, coalesce(s.name, i.status), i.status_category, i.type, i.deleted_at IS NOT NULL OR i.hidden_at IS NOT NULL
			FROM issue_keys k JOIN issues i ON i.id = k.issue_id LEFT JOIN statuses s ON s.project_id = i.project_id AND s.id = i.status
			WHERE k.key = ?`, k).Scan(&c.Summary, &c.Status, &c.Category, &c.Type, &deleted)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			continue
		case err != nil:
			return r, err
		}
		c.Deleted = deleted
		if c.Category == "inprogress" { // the tracker's name; documents say "progress"
			c.Category = "progress"
		}
		r.Issues[k] = c
	}
	if len(refs.People) > 0 {
		found, err := people.Lookup(ctx, q, refs.People...)
		if err != nil {
			return r, err
		}
		for id, p := range found { // an erased account: DeletedName (NFR-27.1)
			r.People[id] = p.Name
		}
	}
	if len(refs.Images) > 0 {
		hidden, err := HiddenImages(ctx, q, refs.Images...)
		if err != nil {
			return r, err
		}
		if len(hidden) > 0 {
			r.Hidden = hidden
		}
	}
	for _, id := range refs.Pages {
		var title string
		err := q.QueryRowContext(ctx, `SELECT title FROM pages WHERE id = ? AND state = 'current'`, id).Scan(&title)
		if err == nil {
			r.Pages[id] = title
		} else if !errors.Is(err, sql.ErrNoRows) {
			return r, err
		}
	}
	return r, nil
}

// Template is a page template (FR-92): one of the five seeded ones.
type Template struct {
	Name, Description string
	Doc               json.RawMessage
}

// Templates are the seeded templates, as documents.
func Templates() []Template {
	c, err := seed.Load()
	if err != nil {
		return nil
	}
	var out []Template
	for _, t := range c.Templates {
		d, err := seed.Parse(t.Body, templateResolver{})
		if err != nil {
			continue
		}
		out = append(out, Template{Name: t.Name, Description: t.Description, Doc: d})
	}
	return out
}

// templateResolver lets templates name the library's images and nothing
// else.
type templateResolver struct{}

func (templateResolver) Issue(string) bool            { return false }
func (templateResolver) Person(string) (string, bool) { return "", false }
func (templateResolver) Page(string) (string, bool)   { return "", false }
func (templateResolver) Image(id string) bool         { return ImageExists(id) }

// PageURL is a page's address: its space's key, its id, and its title as
// a slug, which only reads well (titles change: the id finds the page).
func PageURL(spaceKey, id, title string) string {
	return "/spaces/" + spaceKey + "/pages/" + id + "/" + Slug(title)
}

// EditURL is the address of a page's editor.
func EditURL(spaceKey, id string) string { return "/spaces/" + spaceKey + "/pages/" + id + "/edit" }

// HistoryURL is the address of a page's history (FR-87.2).
func HistoryURL(spaceKey, id string) string {
	return "/spaces/" + spaceKey + "/pages/" + id + "/history"
}

// VersionURL is the address of a version of a page (FR-87.3).
func VersionURL(spaceKey, id string, n int) string {
	return "/spaces/" + spaceKey + "/pages/" + id + "/versions/" + strconv.Itoa(n)
}

// CompareURL is the address of the comparison of two versions (FR-88).
func CompareURL(spaceKey, id string, from, to int) string {
	return "/spaces/" + spaceKey + "/pages/" + id + "/compare?from=" + strconv.Itoa(from) + "&to=" + strconv.Itoa(to)
}

// reserved slugs would name a page's other addresses.
var reserved = map[string]bool{"edit": true, "history": true, "versions": true, "compare": true}

// Slug is a title in lower-case ASCII letters and digits between dashes.
func Slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		var s string
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			s = string(r)
		case r == 'ä' || r == 'æ':
			s = "ae"
		case r == 'ö' || r == 'ø' || r == 'œ':
			s = "oe"
		case r == 'ü':
			s = "ue"
		case r == 'ß':
			s = "ss"
		case strings.ContainsRune("àáâãå", r):
			s = "a"
		case strings.ContainsRune("èéêë", r):
			s = "e"
		case strings.ContainsRune("ìíîï", r):
			s = "i"
		case strings.ContainsRune("òóôõ", r):
			s = "o"
		case strings.ContainsRune("ùúû", r):
			s = "u"
		case r == 'ç':
			s = "c"
		case r == 'ñ':
			s = "n"
		default:
			dash = true
			continue
		}
		if dash && b.Len() > 0 {
			b.WriteByte('-')
		}
		dash = false
		b.WriteString(s)
		if b.Len() > 80 {
			break
		}
	}
	out := b.String()
	if out == "" {
		return "page"
	}
	if reserved[out] {
		return out + "-page"
	}
	return out
}

// PageAt is a page as its stream held it at a position, with its space's
// key and its address then: for reactions that decide as of an event.
func PageAt(tx *writer.Tx, id string, upto int64) (s PageState, key, href string, err error) {
	s, _, err = events.StateAt(tx.Context(), Store(), tx, peekPageDecider, PageStream.ID(id), upto)
	if err != nil {
		return s, "", "", err
	}
	if key, err = spaceKey(tx, s.Space); err != nil {
		return s, "", "", err
	}
	return s, key, PageURL(key, id, s.Title), nil
}

// VersionBefore is the document of the version before number of a page,
// from the page's stream as of upto; nil for the first.
func VersionBefore(tx *writer.Tx, id string, number int, upto int64) (json.RawMessage, error) {
	var prev json.RawMessage
	for r, err := range events.LoadUntil(tx.Context(), Store(), tx, PageStream.ID(id), 0, upto) {
		if err != nil {
			return nil, err
		}
		if v, ok := r.Data.(VersionCut); ok && v.Number < number {
			prev = v.Doc
		}
	}
	return prev, nil
}

// PagesMentioningIssue is PagesMentioning for every key an issue ever
// had (issue_keys): the old keys of a moved issue still name it.
func PagesMentioningIssue(ctx context.Context, q sqlite.Querier, issue string) ([]PageRef, error) {
	return pageRefs(ctx, q, refSelect+`WHERE p.state = 'current' AND p.id IN (SELECT c.page_id FROM page_chips c JOIN issue_keys k ON k.key = c.key WHERE k.issue_id = ?)
		ORDER BY s.name, p.title`, issue)
}
