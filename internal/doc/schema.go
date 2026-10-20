package doc

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

//go:embed schema.json
var schemaJSON []byte

// SchemaJSON returns the schema the package was built with: the one source
// the browser, the oracle and this package read.
func SchemaJSON() []byte { return slices.Clone(schemaJSON) }

var tickit = mustSchema(schemaJSON)

// Tickit returns tickit's schema.
func Tickit() *Schema { return tickit }

func mustSchema(data []byte) *Schema {
	s, err := NewSchema(data)
	if err != nil {
		panic(err)
	}
	return s
}

// Schema holds the node and mark types of a schema in the format of
// schema.json, compiled as prosemirror-model's Schema does.
type Schema struct {
	nodes    []*NodeType
	marks    []*MarkType
	nodeByID map[string]*NodeType
	markByID map[string]*MarkType
	top      *NodeType
	text     *NodeType
	limits   Limits
}

// Limits are the server's limits from schema.json (DM-05, DM-06).
type Limits struct {
	StoredBytes   int      `json:"storedBytes"`
	Blocks        int      `json:"blocks"`
	TableRows     int      `json:"tableRows"`
	TableColumns  int      `json:"tableColumns"`
	TextLength    int      `json:"textLength"`
	AttrLength    int      `json:"attrLength"`
	LinkSchemes   []string `json:"linkSchemes"`
	CodeLanguages []string `json:"codeLanguages"`
	HeadingLevels []int    `json:"headingLevels"`
}

// NodeType is a node type of the schema.
type NodeType struct {
	name          string
	schema        *Schema
	groups        []string
	attrs         []*attribute
	defaultAttrs  []any // nil when an attribute has no default
	contentMatch  *ContentMatch
	inlineContent bool
	isBlock       bool
	isText        bool
	markSet       []*MarkType // nil with allMarks: every mark
	allMarks      bool
	atom          bool
	code          bool
	defining      bool
	isolating     bool
	tableRole     string
}

// MarkType is a mark type of the schema.
type MarkType struct {
	name      string
	rank      int
	schema    *Schema
	attrs     []*attribute
	instance  *Mark // the mark without attributes, when all have defaults
	excluded  []*MarkType
	inclusive bool
	spanning  bool
	groups    []string
}

type attribute struct {
	name       string
	hasDefault bool
	def        any
	validate   []string // the allowed typeof names; nil: anything
}

type nodeSpec struct {
	Name      string                     `json:"name"`
	Content   *string                    `json:"content"`
	Marks     *string                    `json:"marks"`
	Group     string                     `json:"group"`
	Inline    bool                       `json:"inline"`
	Atom      bool                       `json:"atom"`
	Code      bool                       `json:"code"`
	Defining  bool                       `json:"defining"`
	Isolating bool                       `json:"isolating"`
	TableRole string                     `json:"tableRole"`
	Attrs     map[string]json.RawMessage `json:"attrs"`
}

type markSpec struct {
	Name      string                     `json:"name"`
	Inclusive *bool                      `json:"inclusive"`
	Excludes  *string                    `json:"excludes"`
	Group     string                     `json:"group"`
	Spanning  *bool                      `json:"spanning"`
	Attrs     map[string]json.RawMessage `json:"attrs"`
}

// NewSchema compiles a schema in the format of schema.json.
func NewSchema(data []byte) (*Schema, error) {
	var spec struct {
		TopNode string            `json:"topNode"`
		Nodes   []json.RawMessage `json:"nodes"`
		Marks   []json.RawMessage `json:"marks"`
		Limits  Limits            `json:"limits"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("doc: schema: %w", err)
	}
	s := &Schema{nodeByID: map[string]*NodeType{}, markByID: map[string]*MarkType{}, limits: spec.Limits}
	nodeSpecs := make([]nodeSpec, len(spec.Nodes))
	for i, raw := range spec.Nodes {
		ns := &nodeSpecs[i]
		if err := json.Unmarshal(raw, ns); err != nil {
			return nil, fmt.Errorf("doc: schema: %w", err)
		}
		order, err := attrOrder(raw)
		if err != nil {
			return nil, err
		}
		attrs, err := compileAttrs(ns.Name, ns.Attrs, order)
		if err != nil {
			return nil, err
		}
		if _, dup := s.nodeByID[ns.Name]; dup {
			return nil, fmt.Errorf("doc: schema: node type %s twice", ns.Name)
		}
		t := &NodeType{
			name: ns.Name, schema: s, attrs: attrs,
			isBlock: !(ns.Inline || ns.Name == "text"), isText: ns.Name == "text",
			atom: ns.Atom, code: ns.Code, defining: ns.Defining, isolating: ns.Isolating, tableRole: ns.TableRole,
		}
		if ns.Group != "" {
			t.groups = strings.Split(ns.Group, " ")
		}
		t.defaultAttrs = defaultAttrs(attrs)
		s.nodes = append(s.nodes, t)
		s.nodeByID[t.name] = t
	}
	markSpecs := make([]markSpec, len(spec.Marks))
	for i, raw := range spec.Marks {
		ms := &markSpecs[i]
		if err := json.Unmarshal(raw, ms); err != nil {
			return nil, fmt.Errorf("doc: schema: %w", err)
		}
		order, err := attrOrder(raw)
		if err != nil {
			return nil, err
		}
		attrs, err := compileAttrs(ms.Name, ms.Attrs, order)
		if err != nil {
			return nil, err
		}
		t := &MarkType{name: ms.Name, rank: i, schema: s, attrs: attrs, inclusive: ms.Inclusive == nil || *ms.Inclusive, spanning: ms.Spanning == nil || *ms.Spanning}
		if ms.Group != "" {
			t.groups = strings.Split(ms.Group, " ")
		}
		if d := defaultAttrs(attrs); d != nil {
			t.instance = &Mark{typ: t, attrs: d}
		}
		s.marks = append(s.marks, t)
		s.markByID[t.name] = t
	}
	top := spec.TopNode
	if top == "" {
		top = "doc"
	}
	if s.top = s.nodeByID[top]; s.top == nil {
		return nil, fmt.Errorf("doc: schema is missing its top node type (%q)", top)
	}
	if s.text = s.nodeByID["text"]; s.text == nil {
		return nil, fmt.Errorf("doc: every schema needs a text type")
	}
	if len(s.text.attrs) > 0 {
		return nil, fmt.Errorf("doc: the text node type should not have attributes")
	}
	cache := map[string]*ContentMatch{}
	for i, t := range s.nodes {
		if _, both := s.markByID[t.name]; both {
			return nil, fmt.Errorf("doc: %s can not be both a node and a mark", t.name)
		}
		expr := ""
		if c := nodeSpecs[i].Content; c != nil {
			expr = *c
		}
		m, ok := cache[expr]
		if !ok {
			var err error
			if m, err = parseContentMatch(expr, s); err != nil {
				return nil, err
			}
			cache[expr] = m
		}
		t.contentMatch = m
		t.inlineContent = m.inlineContent()
		switch me := nodeSpecs[i].Marks; {
		case me != nil && *me == "_":
			t.allMarks = true
		case me != nil && *me != "":
			set, err := s.gatherMarks(strings.Split(*me, " "))
			if err != nil {
				return nil, err
			}
			t.markSet = set
		case me != nil || !t.inlineContent:
			t.markSet = []*MarkType{}
		default:
			t.allMarks = true
		}
	}
	for i, t := range s.marks {
		switch ex := markSpecs[i].Excludes; {
		case ex == nil:
			t.excluded = []*MarkType{t}
		case *ex == "":
			t.excluded = nil
		default:
			set, err := s.gatherMarks(strings.Split(*ex, " "))
			if err != nil {
				return nil, err
			}
			t.excluded = set
		}
	}
	return s, nil
}

// attrOrder returns the attribute names of a node or mark spec in the order
// they are written, which is the order of the attributes in JSON.
func attrOrder(raw json.RawMessage) ([]string, error) {
	p := &parser{data: raw}
	var order []string
	err := p.object(func(key string) error {
		if key != "attrs" {
			_, err := p.value()
			return err
		}
		if p.peek() != '{' {
			_, err := p.value()
			return err
		}
		order = order[:0]
		return p.object(func(name string) error {
			order = append(order, name)
			_, err := p.value()
			return err
		})
	})
	return order, err
}

func compileAttrs(typeName string, specs map[string]json.RawMessage, order []string) ([]*attribute, error) {
	var out []*attribute
	for _, name := range order {
		var spec map[string]json.RawMessage
		if err := json.Unmarshal(specs[name], &spec); err != nil {
			return nil, fmt.Errorf("doc: schema: attribute %s of %s: %w", name, typeName, err)
		}
		a := &attribute{name: name}
		if raw, ok := spec["default"]; ok {
			v, err := ParseValue(raw)
			if err != nil {
				return nil, err
			}
			a.hasDefault, a.def = true, v
		}
		if raw, ok := spec["validate"]; ok {
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, fmt.Errorf("doc: schema: validate of %s.%s must be a string", typeName, name)
			}
			a.validate = strings.Split(v, "|")
		}
		out = append(out, a)
	}
	return out, nil
}

func defaultAttrs(attrs []*attribute) []any {
	d := make([]any, len(attrs))
	for i, a := range attrs {
		if !a.hasDefault {
			return nil
		}
		d[i] = a.def
	}
	return d
}

func (s *Schema) gatherMarks(names []string) ([]*MarkType, error) {
	var found []*MarkType
	for _, name := range names {
		if m := s.markByID[name]; m != nil {
			found = append(found, m)
			continue
		}
		ok := false
		for _, m := range s.marks {
			if name == "_" || (m.groupHas(name)) {
				found = append(found, m)
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("doc: unknown mark type: '%s'", name)
		}
	}
	return found, nil
}

func (m *MarkType) groupHas(name string) bool { return slices.Contains(m.groups, name) }

// Node returns the node type called name, or nil.
func (s *Schema) Node(name string) *NodeType { return s.nodeByID[name] }

// Mark returns the mark type called name, or nil.
func (s *Schema) Mark(name string) *MarkType { return s.markByID[name] }

// Nodes returns the node types in schema order.
func (s *Schema) Nodes() []*NodeType { return slices.Clone(s.nodes) }

// Marks returns the mark types in rank order.
func (s *Schema) Marks() []*MarkType { return slices.Clone(s.marks) }

// Top returns the type of the document node.
func (s *Schema) Top() *NodeType { return s.top }

// Limits returns the limits of schema.json.
func (s *Schema) Limits() Limits { return s.limits }

// Name returns the type's name.
func (t *NodeType) Name() string { return t.name }

// IsBlock reports whether nodes of the type are blocks.
func (t *NodeType) IsBlock() bool { return t.isBlock }

// IsInline reports whether nodes of the type are inline.
func (t *NodeType) IsInline() bool { return !t.isBlock }

// IsText reports whether this is the text type.
func (t *NodeType) IsText() bool { return t.isText }

// IsTextblock reports whether the type is a block with inline content.
func (t *NodeType) IsTextblock() bool { return t.isBlock && t.inlineContent }

// IsLeaf reports whether the type allows no content.
func (t *NodeType) IsLeaf() bool { return t.contentMatch == emptyMatch }

// IsAtom reports whether the type is a leaf or declared an atom.
func (t *NodeType) IsAtom() bool { return t.IsLeaf() || t.atom }

// InlineContent reports whether the type holds inline content.
func (t *NodeType) InlineContent() bool { return t.inlineContent }

// ContentMatch returns the start state of the type's content expression.
func (t *NodeType) ContentMatch() *ContentMatch { return t.contentMatch }

func (t *NodeType) isInGroup(g string) bool { return slices.Contains(t.groups, g) }

func (t *NodeType) hasRequiredAttrs() bool {
	for _, a := range t.attrs {
		if !a.hasDefault {
			return true
		}
	}
	return false
}

func (t *NodeType) attrIndex(name string) int {
	for i, a := range t.attrs {
		if a.name == name {
			return i
		}
	}
	return -1
}

// compatibleContent reports whether the type allows some of the content of
// other.
func (t *NodeType) compatibleContent(other *NodeType) bool {
	return t == other || t.contentMatch.compatible(other.contentMatch)
}

func (t *NodeType) allowsMarkType(m *MarkType) bool {
	return t.allMarks || slices.Contains(t.markSet, m)
}

func (t *NodeType) allowsMarks(marks []*Mark) bool {
	if t.allMarks {
		return true
	}
	for _, m := range marks {
		if !t.allowsMarkType(m.typ) {
			return false
		}
	}
	return true
}

// validContent reports whether the fragment is valid content of the type.
func (t *NodeType) validContent(f Fragment) bool {
	m := t.contentMatch.matchFragment(f, 0, len(f.nodes))
	if m == nil || !m.validEnd {
		return false
	}
	for _, c := range f.nodes {
		if !t.allowsMarks(c.marks) {
			return false
		}
	}
	return true
}

func (t *NodeType) checkContent(f Fragment) error {
	if !t.validContent(f) {
		return &ModelError{Msg: "Invalid content for node " + t.name + ": " + truncateUTF16(f.String(), 50)}
	}
	return nil
}

// Name returns the mark type's name.
func (m *MarkType) Name() string { return m.name }

// Rank is the mark's place in the nesting order.
func (m *MarkType) Rank() int { return m.rank }

func (m *MarkType) excludes(other *MarkType) bool { return slices.Contains(m.excluded, other) }

// ModelError is an error prosemirror-model throws as a RangeError or
// SyntaxError; its message is prosemirror-model's.
type ModelError struct{ Msg string }

func (e *ModelError) Error() string { return e.Msg }

func modelErrorf(format string, args ...any) error {
	return &ModelError{Msg: fmt.Sprintf(format, args...)}
}

// computeAttrs builds the attributes of a node or mark from a JSON value as
// prosemirror-model's computeAttrs does, JavaScript's truthiness included.
func computeAttrs(typeName string, attrs []*attribute, defaults []any, value any) ([]any, error) {
	if !truthy(value) && defaults != nil {
		return defaults, nil
	}
	built := make([]any, len(attrs))
	obj, _ := value.(*Object)
	for i, a := range attrs {
		var given any
		switch {
		case !truthy(value):
			given = value // `value && value[name]` is value itself
		case obj != nil:
			given = obj.Get(a.name)
		default:
			given = Undefined
		}
		if given == Undefined {
			if !a.hasDefault {
				return nil, modelErrorf("No value supplied for attribute %s", a.name)
			}
			given = a.def
		} else if err := a.check(typeName, given); err != nil {
			return nil, err
		}
		built[i] = given
	}
	return built, nil
}

func (a *attribute) check(typeName string, v any) error {
	if a.validate == nil {
		return nil
	}
	name := typeOf(v)
	if !slices.Contains(a.validate, name) {
		return modelErrorf("Expected value of type %s for attribute %s on type %s, got %s", strings.Join(a.validate, ","), a.name, typeName, name)
	}
	return nil
}
