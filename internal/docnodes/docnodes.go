// Package docnodes is tickit's rich-text document on prosemirror-go: the
// schema in schema.json (which the editor reads too), its limits, and the
// renderers and rules of its node types. Four node types are tickit's own:
// issue_chip (an issue by key, live at render time, FR-100), mention (a
// person, FR-67.2), page_link (a page by id, FR-103) and issue_list (the
// rows of a filter, FR-104); the others are those of a typical editor.
//
// # Rules
//
// Rules.Check refuses a commit with a sentence a person can read (a
// *prosemirror.RuleError), with prosemirror-go's checks of the changed
// blocks and these, the numbers from schema.json:
//
//   - The document: 200 KB of stored JSON and 2,000 block nodes (DM-05.1),
//     refused only when a commit grows a document past them.
//   - Text: no control characters but tab and newline in code blocks.
//   - heading.level 1, 2 or 3; code_block.language empty or one of
//     codeLanguages; ordered_list.order a whole number from 1 to 9,999.
//   - Tables (FR-83): 50 rows, 20 columns counted with colspans and the
//     rowspans of the rows above; colspan 1 to 20, rowspan 1 to 50;
//     colwidth null or one number from 0 to 10,000 per spanned column.
//   - image (FR-84): imageId in the library (the Rules.Image callback);
//     src empty or /img/lib/<imageId>; alt at most 500 characters.
//   - issue_chip.key matches ^[A-Z]{2,10}-[1-9][0-9]{0,6}$ (FR-100).
//   - mention.personId is a person id (g- and 20 characters of base32, a-
//     and a UUIDv7, s- and a slug); its label is empty or 2 to 30
//     characters.
//   - page_link.pageId is a UUIDv7 (FR-103).
//   - issue_list (FR-104): savedFilter empty or a UUIDv7; filter at most
//     listFilter (500) characters without control characters, a query
//     string as the issue list's address writes it; limit a whole number
//     from 1 to listRows (25).
//   - link.href passes prosemirror.SafeHref: http, https or mailto, with a
//     host for http and https, at most 2,048 characters (DM-06.2).
//   - Every attribute string holds at most attrLength (2,048) characters.
//
// # Rendering
//
// RenderHTML produces the HTML table of docs/editor.md. It escapes every
// text and attribute, renders links with rel="nofollow ugc noopener" and
// their host in title, an unsafe href as a span, images from imageId alone
// and never from src, and no <a> for a link mark around an atom. What
// chips, mentions and page links show comes from a Resolver at render
// time; an issue list's rows come from a ListResolver, and without one it
// shows its filter (ListDescription). BlockCache keeps the HTML of
// unchanged blocks, except those holding an issue list. Text gives the
// search text, References the issue keys, person ids, page ids and images
// of a document. Markdown exports a page (FR-152).
package docnodes

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"slices"
	"strconv"
	"sync"

	"github.com/zweiundeins/prosemirror-go"
)

//go:embed schema.json
var schemaJSON []byte

var schema = prosemirror.MustSchema(schemaJSON)

// Schema is tickit's schema.
func Schema() *prosemirror.Schema { return schema }

// SchemaJSON returns schema.json: the one source the browser and this
// package read.
func SchemaJSON() []byte { return slices.Clone(schemaJSON) }

// Limits are tickit's limits from schema.json (DM-05, DM-06), which the
// editor's filterTransaction reads too.
type Limits struct {
	StoredBytes   int      `json:"storedBytes"`
	Blocks        int      `json:"blocks"`
	TableRows     int      `json:"tableRows"`
	TableColumns  int      `json:"tableColumns"`
	AttrLength    int      `json:"attrLength"`
	ListFilter    int      `json:"listFilter"` // characters of an issue list's filter (FR-104)
	ListRows      int      `json:"listRows"`   // rows an issue list shows at most
	LinkSchemes   []string `json:"linkSchemes"`
	CodeLanguages []string `json:"codeLanguages"`
	HeadingLevels []int    `json:"headingLevels"`
}

var limits = sync.OnceValue(func() Limits {
	var spec struct {
		Limits Limits `json:"limits"`
	}
	if err := json.Unmarshal(schemaJSON, &spec); err != nil {
		panic(err)
	}
	return spec.Limits
})

// SchemaLimits returns the limits of schema.json.
func SchemaLimits() Limits { return limits() }

// ParseDoc reads a document of tickit's schema and checks it.
func ParseDoc(data []byte) (prosemirror.Doc, error) { return schema.ParseDoc(data) }

// ParseSteps reads a JSON array of steps of tickit's schema.
func ParseSteps(data []byte) ([]prosemirror.Step, error) { return schema.ParseSteps(data) }

// Blank is the document of a new blank page or an empty description.
const Blank = `{"type":"doc","content":[{"type":"paragraph"}]}`

// Marshal is a document's JSON as prosemirror-go writes it.
func Marshal(d prosemirror.Doc) json.RawMessage {
	b, _ := prosemirror.MarshalDoc(d)
	return b
}

// Canonical is a document's JSON as prosemirror-go writes it: the stored
// event's JSON escapes <, > and &, the event in a live command does not,
// and a rebuild must write what the live projection wrote.
func Canonical(raw json.RawMessage) string {
	d, err := ParseDoc(raw)
	if err != nil {
		return string(raw)
	}
	return string(Marshal(d))
}

// IsBlank reports whether raw is empty, null or the blank document.
func IsBlank(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) == 0 || string(raw) == "null" || Canonical(raw) == Blank
}

// ChipStep is the step that puts an issue's chip in place of the range
// from, to (FR-105).
func ChipStep(from, to int, key string) (prosemirror.Step, error) {
	b := []byte(`{"stepType":"replace","from":` + strconv.Itoa(from) + `,"to":` + strconv.Itoa(to) + `,"slice":{"content":[{"type":"issue_chip","attrs":{"key":`)
	b = prosemirror.AppendValue(b, key)
	b = append(b, `}}]}}`...)
	return schema.ParseStep(b)
}
