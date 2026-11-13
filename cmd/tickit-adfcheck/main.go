// Command tickit-adfcheck converts Atlassian Document Format from Jira and
// Confluence with tickit's mapping and schema, and prints what that does:
// counts only, never a document's text.
//
//	tickit-adfcheck DIR
//
// DIR holds JSON files as the APIs return them: Jira's search results
// (/rest/api/3/search/jql with fields=description,comment), Confluence's
// pages (/wiki/api/v2/.../pages?body-format=atlas_doc_format), or ADF
// documents. It prints the converter's report, the documents that fail
// tickit's rules (docnodes.Rules) by refusal, and those past the limits
// of schema.json. Nothing is imported yet, so every issue key, page,
// person and image of the right form counts as known.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"text/tabwriter"
	"unicode/utf16"

	"github.com/zweiundeins/prosemirror-go"
	"github.com/zweiundeins/prosemirror-go/adf"
	"github.com/zweiundeins/tickit/internal/docnodes"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: tickit-adfcheck DIR")
		os.Exit(2)
	}
	c := newCheck()
	if err := c.dir(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	c.write(os.Stdout)
}

// pretend resolves every reference of the right form, as an import that
// brought everything in would.
type pretend struct{}

func (pretend) Issue(k string) (string, bool) { return k, docnodes.IssueKey(k) }
func (pretend) Page(id string) (string, bool) { return uuid7(id), id != "" }
func (pretend) Person(id string) (string, bool) {
	return "a-" + uuid7(id), id != ""
}
func (pretend) Image(id string) (string, bool) { return "adf-" + hash(id)[:20], id != "" }

func hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func uuid7(s string) string {
	h := hash(s)
	return h[:8] + "-" + h[8:12] + "-7" + h[13:16] + "-8" + h[17:20] + "-" + h[20:32]
}

type check struct {
	mapping  *adf.Mapping
	report   *adf.Report
	sources  map[string]int // documents by source
	failed   map[string]int // errors that are not refusals
	refused  map[string]int // "source: refusal"
	limits   map[string]int // documents past a limit, by limit
	bytes    []int
	blocks   []int
	errs     int
	lim      docnodes.Limits
	maxAttrs int
}

func newCheck() *check {
	return &check{mapping: docnodes.ADF(pretend{}), report: adf.NewReport(), sources: map[string]int{},
		failed: map[string]int{}, refused: map[string]int{}, limits: map[string]int{}, lim: docnodes.SchemaLimits()}
}

func (c *check) dir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("tickit-adfcheck: no JSON files in " + dir)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := c.file(b); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
	}
	return nil
}

// file finds the ADF documents of an API response.
func (c *check) file(b []byte) error {
	var v struct {
		Type   string `json:"type"`
		Issues []struct {
			Fields struct {
				Description json.RawMessage `json:"description"`
				Comment     struct {
					Comments []struct {
						Body json.RawMessage `json:"body"`
					} `json:"comments"`
				} `json:"comment"`
			} `json:"fields"`
		} `json:"issues"`
		Results []struct {
			Body struct {
				ADF struct {
					Value string `json:"value"`
				} `json:"atlas_doc_format"`
			} `json:"body"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch {
	case v.Type == "doc":
		c.doc("adf", b)
	case v.Issues != nil:
		for _, is := range v.Issues {
			c.doc("jira description", is.Fields.Description)
			for _, cm := range is.Fields.Comment.Comments {
				c.doc("jira comment", cm.Body)
			}
		}
	case v.Results != nil:
		for _, p := range v.Results {
			c.doc("confluence page", []byte(p.Body.ADF.Value))
		}
	}
	return nil
}

func (c *check) doc(source string, raw []byte) {
	if len(raw) == 0 || string(raw) == "null" {
		c.sources[source+" (empty)"]++
		return
	}
	c.sources[source]++
	doc, r, err := adf.Convert(docnodes.Schema(), c.mapping, raw)
	if err != nil {
		c.errs++
		return
	}
	c.report.Add(r)
	if err := (docnodes.Rules{}).Check(nil, doc); err != nil {
		var re *prosemirror.RuleError
		if errors.As(err, &re) {
			c.refused[source+": "+re.Msg]++
		} else {
			c.failed[source]++
		}
	}
	n, blocks := doc.JSONLen(), doc.Blocks()
	c.bytes, c.blocks = append(c.bytes, n), append(c.blocks, blocks)
	if n > c.lim.StoredBytes {
		c.limits[fmt.Sprintf("storedBytes (%d)", c.lim.StoredBytes)]++
	}
	if blocks > c.lim.Blocks {
		c.limits[fmt.Sprintf("blocks (%d)", c.lim.Blocks)]++
	}
	var v any
	json.Unmarshal(docnodes.Marshal(doc), &v)
	if l := longestAttr(v, false); l > c.lim.AttrLength {
		c.limits[fmt.Sprintf("attrLength (%d)", c.lim.AttrLength)]++
	}
	c.maxAttrs = max(c.maxAttrs, longestAttr(v, false))
}

// longestAttr is the longest string attribute of a document's JSON, in
// UTF-16 units as the rules count.
func longestAttr(v any, inAttrs bool) int {
	n := 0
	switch x := v.(type) {
	case string:
		if inAttrs {
			n = len(utf16.Encode([]rune(x)))
		}
	case []any:
		for _, e := range x {
			n = max(n, longestAttr(e, inAttrs))
		}
	case map[string]any:
		for k, e := range x {
			n = max(n, longestAttr(e, inAttrs || k == "attrs"))
		}
	}
	return n
}

func quantiles(xs []int) string {
	if len(xs) == 0 {
		return "-"
	}
	s := slices.Sorted(slices.Values(xs))
	return fmt.Sprintf("p50 %d, p95 %d, max %d", s[len(s)/2], s[len(s)*95/100], s[len(s)-1])
}

func (c *check) write(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "source\tdocuments\t")
	for _, k := range slices.Sorted(maps.Keys(c.sources)) {
		fmt.Fprintf(tw, "%s\t%d\t\n", k, c.sources[k])
	}
	fmt.Fprintf(tw, "not converted\t%d\t\n", c.errs)
	tw.Flush()
	fmt.Fprintln(w)
	c.report.Write(w)
	fmt.Fprintln(w)
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "refused by tickit's rules\tdocuments\t")
	for _, k := range slices.Sorted(maps.Keys(c.refused)) {
		fmt.Fprintf(tw, "%s\t%d\t\n", k, c.refused[k])
	}
	for _, k := range slices.Sorted(maps.Keys(c.failed)) {
		fmt.Fprintf(tw, "%s: not a valid document\t%d\t\n", k, c.failed[k])
	}
	fmt.Fprintln(tw, "\t\t\npast a limit of schema.json\tdocuments\t")
	for _, k := range slices.Sorted(maps.Keys(c.limits)) {
		fmt.Fprintf(tw, "%s\t%d\t\n", k, c.limits[k])
	}
	tw.Flush()
	fmt.Fprintf(w, "\nstored bytes: %s\nblocks: %s\nlongest attribute: %d\n", quantiles(c.bytes), quantiles(c.blocks), c.maxAttrs)
}
