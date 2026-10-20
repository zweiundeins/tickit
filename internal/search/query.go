package search

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zweiundeins/lith/sqlite"
)

// Limits of a query.
const (
	MaxText  = 200 // characters of a search text
	MaxWords = 12
	PageSize = 20 // hits per page of results
	Suggest  = 5  // hits of each kind while typing (FR-57.1)
)

// Query is a search as a person asks it: the text, the kinds and filters,
// and the scopes the person may read. A kind without readable scopes
// finds nothing.
type Query struct {
	Text string
	// Kinds limits the hits to Issue or Page; none means both.
	Kinds []Kind
	// Projects and Spaces are the ids of the projects and spaces the person
	// may read (access): nothing else is found.
	Projects, Spaces []string
	// InProjects and InSpaces narrow the hits to these scopes (ids);
	// Labels to documents with one of them, People to documents of one of
	// these persons (FR-57.2).
	InProjects, InSpaces []string
	Labels, People       []string
	Offset, Limit        int // Limit 0 is PageSize
}

// Part is a piece of a snippet: text, and whether it matched.
type Part struct {
	Text string
	Hit  bool
}

// Hit is a document a search found.
type Hit struct {
	Kind                     Kind
	ID, Scope, Key, Href     string
	Title, Place             string
	Snippet                  []Part
	Exact                    bool   // the exact key or title
	InTitle                  bool   // the title or key matched
	ViaComment               string // the comment that matched, when only a comment did
	tier                     int
	score                    float64
	updated, n, commentN     int64
	titleFold, keys, oldKeys string
}

// Results are a page of hits, how many there are in all of the asked
// kinds, and how many of each kind (for the kind filter's counts).
type Results struct {
	Hits   []Hit
	Total  int
	Counts map[Kind]int
}

// expr is a search text as FTS5 queries: every word, the last one as a
// prefix while someone types, in any column (match) or in the key and
// title only (title).
type expr struct {
	match, title string
	exactKey     string // an issue key the text is, upper case
	exactTitle   string // the text folded, for an exact title
}

var issueKey = regexp.MustCompile(`^[A-Z]{2,6}-[1-9][0-9]{0,8}$`)

func parse(text string) (expr, bool) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > MaxText {
		text = string([]rune(text)[:MaxText])
	}
	ws := words(text)
	if len(ws) == 0 {
		return expr{}, false
	}
	if len(ws) > MaxWords {
		ws = ws[:MaxWords]
	}
	q := make([]string, len(ws))
	for i, w := range ws {
		q[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}
	if last := ws[len(ws)-1]; utf8.RuneCountInString(last) >= 2 && strings.HasSuffix(text, last) {
		q[len(q)-1] += "*"
	}
	m := strings.Join(q, " ")
	e := expr{match: m, title: "{key title} : (" + m + ")", exactTitle: fold(text)}
	if k := strings.ToUpper(text); issueKey.MatchString(k) {
		e.exactKey = k
	}
	return e, true
}

// filters is the part of a statement that keeps documents d within the
// query's scopes and filters, with its arguments from ?3 on.
func filters(k Kind, query Query) (string, []any) {
	readable, in := query.Projects, query.InProjects
	if k == Page {
		readable, in = query.Spaces, query.InSpaces
	}
	return ` AND d.hidden = 0
		AND d.scope IN (SELECT value FROM json_each(?3))
		AND (?4 = '[]' OR d.scope IN (SELECT value FROM json_each(?4)))
		AND (?5 = '[]' OR EXISTS (SELECT 1 FROM json_each(d.labels) l WHERE l.value IN (SELECT value FROM json_each(?5))))
		AND (?6 = '[]' OR EXISTS (SELECT 1 FROM json_each(d.people) p WHERE p.value IN (SELECT value FROM json_each(?6))))`,
		[]any{jsonList(readable), jsonList(in), jsonList(query.Labels), jsonList(query.People)}
}

const docColumns = `d.n, d.id, d.scope, d.key, d.old_keys, d.title, d.href, d.place, d.updated`

func (h *Hit) scan(s interface{ Scan(...any) error }, extra ...any) error {
	return s.Scan(append([]any{&h.n, &h.ID, &h.Scope, &h.Key, &h.oldKeys, &h.titleFold, &h.Href, &h.Place, &h.updated}, extra...)...)
}

// Search finds the documents that match a query (FR-57): ranked, then a
// page of them with their snippets.
func Search(ctx context.Context, q sqlite.Querier, query Query) (Results, error) {
	res := Results{Counts: map[Kind]int{}}
	e, ok := parse(query.Text)
	if !ok {
		return res, nil
	}
	found := map[string]*Hit{}
	keep := func(h Hit) {
		k := string(h.Kind) + ":" + h.ID
		if old, ok := found[k]; ok && (old.tier < h.tier || old.tier == h.tier && old.score <= h.score) {
			return
		}
		found[k] = &h
	}
	for _, k := range Kinds {
		where, args := filters(k, query)
		t := k.table()
		rows, err := q.QueryContext(ctx, `SELECT `+docColumns+`, bm25(`+t+`_fts, 8.0, 4.0, 1.0),
			`+t+`_fts.rowid IN (SELECT rowid FROM `+t+`_fts WHERE `+t+`_fts MATCH ?2)
			FROM `+t+`_fts JOIN `+t+` d ON d.n = `+t+`_fts.rowid
			WHERE `+t+`_fts MATCH ?1`+where, append([]any{e.match, e.title}, args...)...)
		if err != nil {
			return res, err
		}
		for rows.Next() {
			h := Hit{Kind: k}
			if err := h.scan(rows, &h.score, &h.InTitle); err != nil {
				rows.Close()
				return res, err
			}
			h.tier = 2
			if h.InTitle {
				h.tier = 1
			}
			if h.Exact = exact(k, e, h); h.Exact {
				h.tier = 0
			}
			keep(h)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return res, err
		}
		// Comments lead to their subject, below its own body's matches:
		// their scores come from another table.
		rows, err = q.QueryContext(ctx, `SELECT `+docColumns+`, c.n, bm25(search_comments_fts, 0.0, 0.0, 1.0)
			FROM search_comments_fts JOIN search_comments c ON c.n = search_comments_fts.rowid
			JOIN `+t+` d ON d.id = c.scope
			WHERE search_comments_fts MATCH ?1 AND ?2 = ?2 AND c.hidden = 0 AND c.key = ?7`+where,
			append(append([]any{e.match, e.title}, args...), string(k))...)
		if err != nil {
			return res, err
		}
		for rows.Next() {
			h := Hit{Kind: k, tier: 3}
			if err := h.scan(rows, &h.commentN, &h.score); err != nil {
				rows.Close()
				return res, err
			}
			keep(h)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return res, err
		}
	}
	all := make([]Hit, 0, len(found))
	for _, h := range found {
		res.Counts[h.Kind]++
		if len(query.Kinds) == 0 || slices.Contains(query.Kinds, h.Kind) {
			all = append(all, *h)
		}
	}
	slices.SortFunc(all, func(a, b Hit) int {
		return cmp.Or(cmp.Compare(a.tier, b.tier), cmp.Compare(a.score, b.score), cmp.Compare(b.updated, a.updated),
			cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
	})
	res.Total = len(all)
	limit := query.Limit
	if limit <= 0 {
		limit = PageSize
	}
	from := min(max(query.Offset, 0), len(all))
	res.Hits = all[from:min(from+limit, len(all))]
	if err := snippets(ctx, q, e, res.Hits); err != nil {
		return res, err
	}
	return res, nil
}

// exact reports whether a hit is the exact key or title the text names.
func exact(k Kind, e expr, h Hit) bool {
	if k == Issue && e.exactKey != "" {
		return h.Key == e.exactKey || slices.Contains(strings.Fields(h.oldKeys), e.exactKey)
	}
	return h.titleFold == e.exactTitle
}

// Marks of a snippet's matches, which clean keeps out of the text.
const (
	markOn  = "\x01"
	markOff = "\x02"
)

// snippets fills in the hits' snippets and titles: from the body of the
// document, or of the comment that matched.
func snippets(ctx context.Context, q sqlite.Querier, e expr, hits []Hit) error {
	type ref struct {
		table string
		n     int64
	}
	want := map[string][]string{}
	for _, h := range hits {
		t := h.Kind.table()
		want[t] = append(want[t], strconv.FormatInt(h.n, 10))
		if h.commentN != 0 {
			want["search_comments"] = append(want["search_comments"], strconv.FormatInt(h.commentN, 10))
		}
	}
	snip, title := map[ref]string{}, map[ref]string{}
	for t, ns := range want {
		rows, err := q.QueryContext(ctx, `SELECT rowid, title, snippet(`+t+`_fts, 2, char(1), char(2), '…', 24)
			FROM `+t+`_fts WHERE rowid IN (SELECT value FROM json_each(?2)) AND `+t+`_fts MATCH ?1`, e.match, "["+strings.Join(ns, ",")+"]")
		if err != nil {
			return err
		}
		for rows.Next() {
			var n int64
			var ti, s string
			if err := rows.Scan(&n, &ti, &s); err != nil {
				rows.Close()
				return err
			}
			snip[ref{t, n}], title[ref{t, n}] = s, ti
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		// A document found by a comment alone may not match by itself:
		// its title still shows.
		rows, err = q.QueryContext(ctx, `SELECT rowid, title, substr(body, 1, 200) FROM `+t+`_fts WHERE rowid IN (SELECT value FROM json_each(?1))`, "["+strings.Join(ns, ",")+"]")
		if err != nil {
			return err
		}
		for rows.Next() {
			var n int64
			var ti, s string
			if err := rows.Scan(&n, &ti, &s); err != nil {
				rows.Close()
				return err
			}
			if _, ok := title[ref{t, n}]; !ok {
				snip[ref{t, n}], title[ref{t, n}] = s, ti
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	for i := range hits {
		h := &hits[i]
		r := ref{h.Kind.table(), h.n}
		h.Title = title[r]
		s := snip[r]
		if h.commentN != 0 {
			s = snip[ref{"search_comments", h.commentN}]
			h.ViaComment = "comment"
		}
		h.Snippet = parts(s)
	}
	return nil
}

// parts cuts a snippet at its marks.
func parts(s string) []Part {
	var out []Part
	hit := false
	for s != "" {
		mark := markOn
		if hit {
			mark = markOff
		}
		i := strings.Index(s, mark)
		if i < 0 {
			out = append(out, Part{Text: s, Hit: hit})
			break
		}
		if i > 0 {
			out = append(out, Part{Text: s[:i], Hit: hit})
		}
		s, hit = s[i+1:], !hit
	}
	return out
}
