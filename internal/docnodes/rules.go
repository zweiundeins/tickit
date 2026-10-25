package docnodes

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/zweiundeins/prosemirror-go"
)

// Rules are tickit's rules beyond the schema (DM-05, DM-06, FR-83, FR-84,
// FR-100, FR-103, FR-104). The zero value checks the formats; Image adds
// the image library.
type Rules struct {
	// Image reports whether an image id is in the library or an approved
	// upload (FR-84, FR-85). Nil accepts every id of the right format.
	Image func(id string) bool
	// Person and Page check ids; nil uses PersonID and PageID.
	Person func(id string) bool
	Page   func(id string) bool
}

var (
	issueKeyRe = regexp.MustCompile(`^[A-Z]{2,10}-[1-9][0-9]{0,6}$`)
	uuid7Re    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	guestRe    = regexp.MustCompile(`^g-([a-z2-7]{20}|[A-Z2-7]{20})$`)
	slugRe     = regexp.MustCompile(`^s-[a-z0-9]+(-[a-z0-9]+)*$`)
	imageIDRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

// IssueKey reports whether key has the format of an issue key (WEB-12).
func IssueKey(key string) bool { return issueKeyRe.MatchString(key) }

// PageID reports whether id has the format of a page id (a UUIDv7).
func PageID(id string) bool { return uuid7Re.MatchString(id) }

// PersonID reports whether id has the format of a person id: a guest
// (g- and 20 characters of base32), an account (a- and a UUIDv7) or a
// seeded person (s- and a slug).
func PersonID(id string) bool {
	switch {
	case strings.HasPrefix(id, "g-"):
		return guestRe.MatchString(id)
	case strings.HasPrefix(id, "a-"):
		return uuid7Re.MatchString(id[2:])
	case strings.HasPrefix(id, "s-"):
		return len(id) <= 42 && slugRe.MatchString(id)
	}
	return false
}

var refuse = prosemirror.Refuse

// Check validates a commit's result: before is the document it was made
// on, nil for a new document (prosemirror.Rules.Check).
func (r Rules) Check(before, after prosemirror.Doc) error { return r.Rules().Check(before, after) }

// Rules are r as prosemirror-go's rules, per node and mark type.
func (r Rules) Rules() prosemirror.Rules {
	lim := SchemaLimits()
	return prosemirror.Rules{
		Noun: "page", MaxBytes: lim.StoredBytes, MaxBlocks: lim.Blocks, MaxAttrLength: lim.AttrLength,
		Marks: map[string]func(m *prosemirror.Mark, n *prosemirror.Node) error{
			"link": func(m *prosemirror.Mark, _ *prosemirror.Node) error {
				if _, ok := prosemirror.SafeHref(m.AttrString("href")); !ok {
					return refuse("A link must start with http://, https:// or mailto: and be at most 2,048 characters.")
				}
				return nil
			},
		},
		Nodes: map[string]func(n, parent *prosemirror.Node) error{
			"text": func(n, parent *prosemirror.Node) error {
				inCode := parent.Type().Name() == "code_block"
				for _, c := range n.Text() {
					if (c < 0x20 && !(inCode && (c == '\t' || c == '\n'))) || (c >= 0x7f && c <= 0x9f) {
						return refuse("The text holds a control character.")
					}
				}
				return nil
			},
			"heading": func(n, _ *prosemirror.Node) error {
				if l, ok := n.AttrInt("level"); !ok || (len(lim.HeadingLevels) > 0 && !slices.Contains(lim.HeadingLevels, l)) {
					return refuse("A heading has level 1, 2 or 3.")
				}
				return nil
			},
			"code_block": func(n, _ *prosemirror.Node) error {
				if len(lim.CodeLanguages) > 0 && !slices.Contains(lim.CodeLanguages, n.AttrString("language")) {
					return refuse("Code blocks take one of the listed languages.")
				}
				return nil
			},
			"ordered_list": func(n, _ *prosemirror.Node) error {
				if _, ok := wholeIn(n, "order", 1, 9999); !ok {
					return refuse("A numbered list starts at a number from 1 to 9,999.")
				}
				return nil
			},
			"table": func(n, _ *prosemirror.Node) error { return checkTable(n, lim) },
			"image": func(n, _ *prosemirror.Node) error {
				id := n.AttrString("imageId")
				if !imageIDRe.MatchString(id) || (r.Image != nil && !r.Image(id)) {
					return refuse("Images come from the image library.")
				}
				if src := n.AttrString("src"); src != "" && src != "/img/lib/"+id {
					return refuse("Images come from the image library.")
				}
				if utf8.RuneCountInString(n.AttrString("alt")) > 500 {
					return refuse("An image's alt text holds at most 500 characters.")
				}
				return nil
			},
			"issue_chip": func(n, _ *prosemirror.Node) error {
				if !IssueKey(n.AttrString("key")) {
					return refuse("An issue chip needs an issue key such as WEB-12.")
				}
				return nil
			},
			"mention": func(n, _ *prosemirror.Node) error {
				id := n.AttrString("personId")
				ok := PersonID(id)
				if r.Person != nil {
					ok = r.Person(id)
				}
				if !ok {
					return refuse("A mention names a person of this site.")
				}
				if l := utf8.RuneCountInString(n.AttrString("label")); l != 0 && (l < 2 || l > 30) {
					return refuse("A mention's name has 2 to 30 characters.")
				}
				return nil
			},
			"issue_list": func(n, _ *prosemirror.Node) error { return checkIssueList(n, lim) },
			"page_link": func(n, _ *prosemirror.Node) error {
				id := n.AttrString("pageId")
				ok := PageID(id)
				if r.Page != nil {
					ok = r.Page(id)
				}
				if !ok {
					return refuse("A page link names a page of this site.")
				}
				return nil
			},
		},
	}
}

// checkIssueList checks an issue list's filter (FR-104): a saved filter's
// id, or the issue list's query string, and its row cap.
func checkIssueList(n *prosemirror.Node, lim Limits) error {
	if id := n.AttrString("savedFilter"); id != "" && !uuid7Re.MatchString(id) {
		return refuse("An issue list names a saved filter of this site.")
	}
	f := n.AttrString("filter")
	if utf8.RuneCountInString(f) > max(lim.ListFilter, 1) {
		return refuse("An issue list's filter holds at most %d characters.", lim.ListFilter)
	}
	for _, c := range f {
		if c < 0x20 || c == 0x7f {
			return refuse("An issue list's filter holds a control character.")
		}
	}
	if _, err := url.ParseQuery(f); err != nil {
		return refuse("An issue list's filter is written as the issue list's address writes it, such as status=todo&project=WEB.")
	}
	if _, ok := wholeIn(n, "limit", 1, max(lim.ListRows, 1)); !ok {
		return refuse("An issue list shows 1 to %d rows.", lim.ListRows)
	}
	return nil
}

// wholeIn returns a node's attribute when it is a whole number from lo
// to hi.
func wholeIn(n *prosemirror.Node, name string, lo, hi int) (int, bool) {
	v, ok := n.AttrInt(name)
	return v, ok && v >= lo && v <= hi
}

// checkTable checks a table against DM-05.1: at most 50 rows and 20
// columns, counting colspans and the rowspans of the rows above.
func checkTable(t *prosemirror.Node, lim Limits) error {
	rows, cols := lim.TableRows, lim.TableColumns
	if rows <= 0 {
		rows = 50
	}
	if cols <= 0 {
		cols = 20
	}
	if t.ChildCount() > rows {
		return refuse("A table holds at most %d rows.", rows)
	}
	carry := make([]int, t.ChildCount())
	for ri, row := range t.Content().Nodes() {
		width := carry[ri]
		for _, cell := range row.Content().Nodes() {
			cs, ok1 := wholeIn(cell, "colspan", 1, cols)
			rs, ok2 := wholeIn(cell, "rowspan", 1, rows)
			if !ok1 || !ok2 {
				return refuse("A table cell spans 1 to %d columns and 1 to %d rows.", cols, rows)
			}
			switch w := cell.Attr("colwidth").(type) {
			case nil:
			case []any:
				if len(w) != cs {
					return refuse("A table cell's column widths do not match its columns.")
				}
				for _, x := range w {
					if f, ok := x.(float64); !ok || f < 0 || f > 10000 {
						return refuse("A table cell's column widths do not match its columns.")
					}
				}
			default:
				return refuse("A table cell's column widths do not match its columns.")
			}
			width += cs
			for k := 1; k < rs && ri+k < len(carry); k++ {
				carry[ri+k] += cs
			}
		}
		if width > cols {
			return refuse("A table holds at most %d columns.", cols)
		}
	}
	return nil
}
