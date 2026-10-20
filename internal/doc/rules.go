package doc

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// RuleError is a refusal by a server rule; its message is a sentence a
// person can read.
type RuleError struct{ Msg string }

func (e *RuleError) Error() string { return e.Msg }

func refuse(format string, args ...any) error { return &RuleError{Msg: fmt.Sprintf(format, args...)} }

// Rules are the server's rules beyond the schema (DM-05, DM-06, FR-83,
// FR-84, FR-100, FR-103). The zero value checks the formats; Image adds the
// image library.
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

// SafeHref reports whether a link target may be rendered (DM-06.2): it
// parses with net/url, its scheme is http, https or mailto, http and https
// name a host, and it is at most 2,048 characters without spaces or
// control characters. It returns what the link's title shows: the host, or
// the address of a mailto link.
func SafeHref(href string) (title string, ok bool) {
	if href == "" || len(href) > 2048 {
		return "", false
	}
	for i := 0; i < len(href); i++ {
		if c := href[i]; c <= ' ' || c == 0x7f {
			return "", false
		}
	}
	u, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	switch u.Scheme {
	case "http", "https":
		if u.Opaque != "" || u.Hostname() == "" {
			return "", false
		}
		return strings.ToLower(u.Host), true
	case "mailto":
		addr := u.Opaque
		if i := strings.IndexByte(addr, '?'); i >= 0 {
			addr = addr[:i]
		}
		if a, err := url.PathUnescape(addr); err == nil {
			addr = a
		}
		return addr, addr != ""
	}
	return "", false
}

func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// changedBlocks returns the top-level blocks of after that are not, by
// pointer, at the same place from the start or the end of before.
func changedBlocks(before, after Doc) []*Node {
	var old []*Node
	if before != nil {
		old = before.content.nodes
	}
	cur := after.content.nodes
	lo := 0
	for lo < len(old) && lo < len(cur) && old[lo] == cur[lo] {
		lo++
	}
	hi := 0
	for hi < len(old)-lo && hi < len(cur)-lo && old[len(old)-1-hi] == cur[len(cur)-1-hi] {
		hi++
	}
	return cur[lo : len(cur)-hi]
}

// Check validates a commit's result: the document's size limits, and the
// schema and the rules on the top-level blocks of after that are not
// blocks of before (unchanged blocks keep their node, so a pointer
// comparison finds the changed ones). before is nil for a new document,
// which checks every block. A size limit refuses only a commit that grows
// a document past it.
func (r Rules) Check(before, after Doc) error {
	s := after.typ.schema
	lim := s.limits
	if after.typ != s.top {
		return refuse("This is not a page.")
	}
	if lim.StoredBytes > 0 {
		if n := after.JSONLen(); n > lim.StoredBytes && (before == nil || n > before.JSONLen()) {
			return refuse("The page is at its limit of %s KB.", thousands(lim.StoredBytes/1024))
		}
	}
	if lim.Blocks > 0 {
		if n := after.Blocks(); n > lim.Blocks && (before == nil || n > before.Blocks()) {
			return refuse("The page is at its limit of %s blocks.", thousands(lim.Blocks))
		}
	}
	if !after.typ.validContent(after.content) {
		return refuse("The page holds something the editor cannot make.")
	}
	for _, b := range changedBlocks(before, after) {
		if err := b.Check(); err != nil {
			return refuse("The page holds something the editor cannot make.")
		}
		if err := r.checkNode(b, lim, false); err != nil {
			return err
		}
	}
	return nil
}

func (r Rules) checkNode(n *Node, lim Limits, inCode bool) error {
	if err := checkStrings(n.typ.attrs, n.attrs, lim); err != nil {
		return err
	}
	for _, m := range n.marks {
		if err := checkStrings(m.typ.attrs, m.attrs, lim); err != nil {
			return err
		}
		if m.typ.name == "link" {
			if _, ok := SafeHref(m.AttrString("href")); !ok {
				return refuse("A link must start with http://, https:// or mailto: and be at most 2,048 characters.")
			}
		}
	}
	switch n.typ.name {
	case "text":
		for _, c := range n.text {
			if (c < 0x20 && !(inCode && (c == '\t' || c == '\n'))) || (c >= 0x7f && c <= 0x9f) {
				return refuse("The text holds a control character.")
			}
		}
		return nil
	case "heading":
		if l, ok := n.AttrInt("level"); !ok || (len(lim.HeadingLevels) > 0 && !slices.Contains(lim.HeadingLevels, l)) {
			return refuse("A heading has level 1, 2 or 3.")
		}
	case "code_block":
		if len(lim.CodeLanguages) > 0 && !slices.Contains(lim.CodeLanguages, n.AttrString("language")) {
			return refuse("Code blocks take one of the listed languages.")
		}
		inCode = true
	case "ordered_list":
		if _, ok := wholeIn(n.Attr("order"), 1, 9999); !ok {
			return refuse("A numbered list starts at a number from 1 to 9,999.")
		}
	case "table":
		if err := checkTable(n, lim); err != nil {
			return err
		}
	case "image":
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
	case "issue_chip":
		if !IssueKey(n.AttrString("key")) {
			return refuse("An issue chip needs an issue key such as WEB-12.")
		}
	case "mention":
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
	case "page_link":
		id := n.AttrString("pageId")
		ok := PageID(id)
		if r.Page != nil {
			ok = r.Page(id)
		}
		if !ok {
			return refuse("A page link names a page of this site.")
		}
	}
	for _, c := range n.content.nodes {
		if err := r.checkNode(c, lim, inCode); err != nil {
			return err
		}
	}
	return nil
}

func checkStrings(specs []*attribute, values []any, lim Limits) error {
	if lim.AttrLength <= 0 {
		return nil
	}
	for i := range specs {
		if s, ok := values[i].(string); ok && len(s) > lim.AttrLength && utf16Len(s) > lim.AttrLength {
			return refuse("A link or attribute holds at most %s characters.", thousands(lim.AttrLength))
		}
	}
	return nil
}

func wholeIn(v any, lo, hi int) (int, bool) {
	f, ok := v.(float64)
	if !ok || f != math.Trunc(f) || f < float64(lo) || f > float64(hi) {
		return 0, false
	}
	return int(f), true
}

// checkTable checks a table against DM-05.1: at most 50 rows and 20
// columns, counting colspans and the rowspans of the rows above.
func checkTable(t *Node, lim Limits) error {
	rows, cols := lim.TableRows, lim.TableColumns
	if rows <= 0 {
		rows = 50
	}
	if cols <= 0 {
		cols = 20
	}
	if len(t.content.nodes) > rows {
		return refuse("A table holds at most %d rows.", rows)
	}
	carry := make([]int, len(t.content.nodes))
	for ri, row := range t.content.nodes {
		width := carry[ri]
		for _, cell := range row.content.nodes {
			cs, ok1 := wholeIn(cell.Attr("colspan"), 1, cols)
			rs, ok2 := wholeIn(cell.Attr("rowspan"), 1, rows)
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
