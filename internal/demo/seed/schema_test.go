package seed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// schema is internal/doc/schema.json, read the way the Go core reads it.
type schema struct {
	TopNode string       `json:"topNode"`
	Nodes   []schemaNode `json:"nodes"`
	Marks   []schemaNode `json:"marks"`
	Limits  struct {
		StoredBytes   int      `json:"storedBytes"`
		Blocks        int      `json:"blocks"`
		TableRows     int      `json:"tableRows"`
		TableColumns  int      `json:"tableColumns"`
		TextLength    int      `json:"textLength"`
		AttrLength    int      `json:"attrLength"`
		LinkSchemes   []string `json:"linkSchemes"`
		CodeLanguages []string `json:"codeLanguages"`
		HeadingLevels []int    `json:"headingLevels"`
	} `json:"limits"`
}

type schemaNode struct {
	Name    string                `json:"name"`
	Group   string                `json:"group"`
	Content *string               `json:"content"`
	Marks   *string               `json:"marks"`
	Inline  bool                  `json:"inline"`
	Attrs   map[string]schemaAttr `json:"attrs"`
	// Flags the validator does not need.
	Defining, Atom, Draggable, Code, Isolating, Selectable, Inclusive *bool
	TableRole                                                         string `json:"tableRole"`
}

type schemaAttr struct {
	Default  json.RawMessage `json:"default"`
	Validate string          `json:"validate"`
}

// validator checks a document against the schema: node and mark names,
// attributes, the content expression of every node, mark order, and the
// limits.
type validator struct {
	s       schema
	nodes   map[string]schemaNode
	content map[string]*regexp.Regexp
	rank    map[string]int
}

func loadSchema(t testing.TB) *validator {
	t.Helper()
	b, err := os.ReadFile("../../doc/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	v := &validator{nodes: map[string]schemaNode{}, content: map[string]*regexp.Regexp{}, rank: map[string]int{}}
	if err := json.Unmarshal(b, &v.s); err != nil {
		t.Fatal(err)
	}
	groups := map[string][]string{}
	for _, n := range v.s.Nodes {
		v.nodes[n.Name] = n
		for _, g := range strings.Fields(n.Group) {
			groups[g] = append(groups[g], n.Name)
		}
	}
	for _, n := range v.s.Nodes {
		if n.Content == nil {
			continue
		}
		re, err := compileContent(*n.Content, v.nodes, groups)
		if err != nil {
			t.Fatalf("content of %s: %v", n.Name, err)
		}
		v.content[n.Name] = re
	}
	for i, m := range v.s.Marks {
		v.rank[m.Name] = i
	}
	return v
}

var tokenRe = regexp.MustCompile(`[a-z_]+|[()|*+?]`)

// compileContent turns a ProseMirror content expression into a regular
// expression over the names of the children, each written as <name>.
func compileContent(expr string, nodes map[string]schemaNode, groups map[string][]string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for _, tok := range tokenRe.FindAllString(expr, -1) {
		switch tok {
		case "(":
			b.WriteString("(?:")
		case ")", "|", "*", "+", "?":
			b.WriteString(tok)
		default:
			var alts []string
			if _, ok := nodes[tok]; ok {
				alts = []string{tok}
			} else if g, ok := groups[tok]; ok {
				alts = g
			} else {
				return nil, fmt.Errorf("unknown name %q", tok)
			}
			b.WriteString("(?:")
			for i, a := range alts {
				if i > 0 {
					b.WriteString("|")
				}
				b.WriteString("<" + a + ">")
			}
			b.WriteString(")")
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

type pmNode struct {
	Type    string                     `json:"type"`
	Attrs   map[string]json.RawMessage `json:"attrs"`
	Content []pmNode                   `json:"content"`
	Marks   []pmMark                   `json:"marks"`
	Text    *string                    `json:"text"`
}

type pmMark struct {
	Type  string                     `json:"type"`
	Attrs map[string]json.RawMessage `json:"attrs"`
}

func (v *validator) validate(doc json.RawMessage) error {
	if len(doc) > v.s.Limits.StoredBytes {
		return fmt.Errorf("%d bytes, more than %d", len(doc), v.s.Limits.StoredBytes)
	}
	var n pmNode
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&n); err != nil {
		return err
	}
	if n.Type != v.s.TopNode {
		return fmt.Errorf("top node %q", n.Type)
	}
	blocks := 0
	if err := v.node(&n, n.Type, &blocks); err != nil {
		return err
	}
	if blocks > v.s.Limits.Blocks {
		return fmt.Errorf("%d blocks, more than %d", blocks, v.s.Limits.Blocks)
	}
	return nil
}

func (v *validator) node(n *pmNode, path string, blocks *int) error {
	spec, ok := v.nodes[n.Type]
	if !ok {
		return fmt.Errorf("%s: unknown node %q", path, n.Type)
	}
	if spec.Group == "block" || n.Type == "list_item" || n.Type == "task_item" || strings.HasPrefix(n.Type, "table_") {
		*blocks++
	}
	if err := v.attrs(n.Attrs, spec.Attrs, path); err != nil {
		return err
	}
	if n.Type == "text" {
		switch {
		case n.Text == nil || *n.Text == "":
			return fmt.Errorf("%s: an empty text node", path)
		case len(n.Content) > 0:
			return fmt.Errorf("%s: a text node with content", path)
		case utf8.RuneCountInString(*n.Text) > v.s.Limits.TextLength:
			return fmt.Errorf("%s: a text of more than %d characters", path, v.s.Limits.TextLength)
		}
	} else if n.Text != nil {
		return fmt.Errorf("%s: text on a %s", path, n.Type)
	}
	re := v.content[n.Type]
	if re == nil && len(n.Content) > 0 {
		return fmt.Errorf("%s: a leaf with content", path)
	}
	if re != nil {
		var names strings.Builder
		for _, k := range n.Content {
			names.WriteString("<" + k.Type + ">")
		}
		if !re.MatchString(names.String()) {
			return fmt.Errorf("%s: children %s do not match %q", path, names.String(), *spec.Content)
		}
	}
	switch n.Type {
	case "code_block":
		var lang string
		_ = json.Unmarshal(n.Attrs["language"], &lang)
		if !slices.Contains(v.s.Limits.CodeLanguages, lang) {
			return fmt.Errorf("%s: code language %q", path, lang)
		}
	case "heading":
		var level int
		_ = json.Unmarshal(n.Attrs["level"], &level)
		if !slices.Contains(v.s.Limits.HeadingLevels, level) {
			return fmt.Errorf("%s: heading level %d", path, level)
		}
	case "table":
		if len(n.Content) > v.s.Limits.TableRows {
			return fmt.Errorf("%s: %d rows", path, len(n.Content))
		}
		for r, row := range n.Content {
			if len(row.Content) > v.s.Limits.TableColumns {
				return fmt.Errorf("%s: %d columns", path, len(row.Content))
			}
			for _, cell := range row.Content {
				if (r == 0) != (cell.Type == "table_header") {
					return fmt.Errorf("%s: table_header cells belong in the first row only", path)
				}
			}
		}
	case "image":
		var id, src string
		_ = json.Unmarshal(n.Attrs["imageId"], &id)
		_ = json.Unmarshal(n.Attrs["src"], &src)
		if id == "" || src != "/img/lib/"+id {
			return fmt.Errorf("%s: image %q at %q", path, id, src)
		}
	}
	for i := range n.Content {
		k := &n.Content[i]
		kpath := fmt.Sprintf("%s/%d:%s", path, i, k.Type)
		if len(k.Marks) > 0 && spec.Marks != nil && *spec.Marks == "" {
			return fmt.Errorf("%s: marks inside a %s", kpath, n.Type)
		}
		last := -1
		for _, m := range k.Marks {
			r, ok := v.rank[m.Type]
			if !ok {
				return fmt.Errorf("%s: unknown mark %q", kpath, m.Type)
			}
			if r <= last {
				return fmt.Errorf("%s: marks out of schema order or repeated", kpath)
			}
			last = r
			if err := v.attrs(m.Attrs, v.s.Marks[r].Attrs, kpath); err != nil {
				return err
			}
			if m.Type == "link" {
				var href string
				_ = json.Unmarshal(m.Attrs["href"], &href)
				u, err := url.Parse(href)
				if err != nil || !slices.Contains(v.s.Limits.LinkSchemes, u.Scheme) {
					return fmt.Errorf("%s: link %q", kpath, href)
				}
			}
		}
		if err := v.node(k, kpath, blocks); err != nil {
			return err
		}
	}
	return nil
}

func (v *validator) attrs(got map[string]json.RawMessage, spec map[string]schemaAttr, path string) error {
	if len(got) != len(spec) {
		return fmt.Errorf("%s: attrs %v, want the %d of the schema", path, keysOf(got), len(spec))
	}
	for name, a := range spec {
		raw, ok := got[name]
		if !ok {
			return fmt.Errorf("%s: attr %s missing", path, name)
		}
		var val any
		if err := json.Unmarshal(raw, &val); err != nil {
			return err
		}
		switch x := val.(type) {
		case nil:
			if string(a.Default) != "null" {
				return fmt.Errorf("%s: attr %s is null", path, name)
			}
		case float64:
			if a.Validate != "number" {
				return fmt.Errorf("%s: attr %s is a number", path, name)
			}
		case string:
			if a.Validate != "string" {
				return fmt.Errorf("%s: attr %s is a string", path, name)
			}
			if len(x) > v.limits().AttrLength {
				return fmt.Errorf("%s: attr %s longer than %d", path, name, v.limits().AttrLength)
			}
		case bool:
			if a.Validate != "boolean" {
				return fmt.Errorf("%s: attr %s is a boolean", path, name)
			}
		default:
			if a.Validate != "" {
				return fmt.Errorf("%s: attr %s has type %T", path, name, val)
			}
		}
	}
	return nil
}

func (v *validator) limits() struct{ AttrLength int } {
	return struct{ AttrLength int }{v.s.Limits.AttrLength}
}

func keysOf(m map[string]json.RawMessage) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestValidatorRefusesBadDocuments(t *testing.T) {
	v := loadSchema(t)
	bad := []string{
		`{"type":"doc"}`,
		`{"type":"doc","content":[{"type":"text","text":"loose"}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":""}]}]}`,
		`{"type":"doc","content":[{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"bullet_list","content":[]}]}]}]}`,
		`{"type":"doc","content":[{"type":"heading","attrs":{"level":4},"content":[{"type":"text","text":"x"}]}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"em"},{"type":"strong"}],"text":"x"}]}]}`,
		`{"type":"doc","content":[{"type":"code_block","attrs":{"language":"cobol"}}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}],"text":"x"}]}]}`,
		`{"type":"doc","content":[{"type":"image","attrs":{"imageId":"a","src":"https://example.com/a.png","alt":""}}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"personId":"s-x"}}]}]}`,
	}
	for _, doc := range bad {
		if err := v.validate(json.RawMessage(doc)); err == nil {
			t.Errorf("accepted %s", doc)
		}
	}
	good := `{"type":"doc","content":[{"type":"task_list","content":[{"type":"task_item","attrs":{"checked":true},"content":[{"type":"paragraph"}]}]}]}`
	if err := v.validate(json.RawMessage(good)); err != nil {
		t.Errorf("refused %s: %v", good, err)
	}
}
