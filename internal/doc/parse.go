package doc

import (
	"errors"
)

// The JSON readers follow prosemirror-model's fromJSON functions, with
// JavaScript's truthiness, so documents and steps read as the browser
// reads them. Children, marks and slices are read as they come; the
// node is built when its object ends.

type rawNode struct {
	typ      any
	attrs    any
	content  any // []*Node when an array, else the JSON value
	children []*Node
	marks    any
	markList []*Mark
	text     any
	units    int
}

func (p *parser) node(s *Schema) (*Node, error) {
	if p.peek() != '{' {
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		if !truthy(v) {
			return nil, &ModelError{Msg: "Invalid input for Node.fromJSON"}
		}
		return nil, modelErrorf("Unknown node type: undefined")
	}
	r := rawNode{typ: Undefined, attrs: Undefined, content: Undefined, marks: Undefined, text: Undefined}
	err := p.object(func(key string) error {
		var err error
		switch key {
		case "type":
			r.typ, err = p.value()
		case "attrs":
			r.attrs, err = p.value()
		case "text":
			if p.peek() == '"' {
				var s string
				s, r.units, err = p.string()
				r.text = s
			} else {
				r.text, err = p.value()
			}
		case "content":
			if p.peek() == '[' {
				r.children = nil
				err = p.array(func() error {
					c, err := p.node(s)
					r.children = append(r.children, c)
					return err
				})
				r.content = r.children
			} else {
				r.content, err = p.value()
			}
		case "marks":
			if p.peek() == '[' {
				r.markList = nil
				err = p.array(func() error {
					m, err := p.mark(s)
					r.markList = append(r.markList, m)
					return err
				})
				r.marks = r.markList
			} else {
				r.marks, err = p.value()
			}
		default:
			_, err = p.value()
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	var marks []*Mark
	if truthy(r.marks) {
		ml, ok := r.marks.([]*Mark)
		if !ok {
			return nil, &ModelError{Msg: "Invalid mark data for Node.fromJSON"}
		}
		marks = setFrom(ml)
	}
	if r.typ == "text" {
		t, ok := r.text.(string)
		if !ok {
			return nil, &ModelError{Msg: "Invalid text node in JSON"}
		}
		if t == "" {
			return nil, &ModelError{Msg: "Empty text nodes are not allowed"}
		}
		return newText(s.text, t, r.units, marks), nil
	}
	var content Fragment
	if truthy(r.content) {
		kids, ok := r.content.([]*Node)
		if !ok {
			return nil, &ModelError{Msg: "Invalid input for Fragment.fromJSON"}
		}
		content = fragmentFromArray(kids)
	}
	name, _ := r.typ.(string)
	t := s.nodeByID[name]
	if t == nil || r.typ != any(name) {
		return nil, modelErrorf("Unknown node type: %s", jsString(r.typ))
	}
	attrs := r.attrs
	if attrs == Undefined {
		attrs = nil
	}
	a, err := t.computeAttrs(attrs)
	if err != nil {
		return nil, err
	}
	return newNode(t, a, content, marks), nil
}

func (p *parser) mark(s *Schema) (*Mark, error) {
	if p.peek() != '{' {
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		if !truthy(v) {
			return nil, &ModelError{Msg: "Invalid input for Mark.fromJSON"}
		}
		return nil, modelErrorf("There is no mark type undefined in this schema")
	}
	typ, attrs := any(Undefined), any(Undefined)
	err := p.object(func(key string) error {
		var err error
		switch key {
		case "type":
			typ, err = p.value()
		case "attrs":
			attrs, err = p.value()
		default:
			_, err = p.value()
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	name, _ := typ.(string)
	t := s.markByID[name]
	if t == nil || typ != any(name) {
		return nil, modelErrorf("There is no mark type %s in this schema", jsString(typ))
	}
	if attrs == Undefined {
		attrs = nil
	}
	return t.create(attrs)
}

// slice reads a Slice as Slice.fromJSON does. It refuses open depths the
// content does not have, which prosemirror-model would accept and then
// misplace.
func (p *parser) slice(s *Schema) (Slice, error) {
	if p.peek() != '{' {
		v, err := p.value()
		if err != nil {
			return Slice{}, err
		}
		if !truthy(v) {
			return Slice{}, nil
		}
		return Slice{}, errBadStep("a slice must be an object")
	}
	var content any = Undefined
	var children []*Node
	openStart, openEnd := any(Undefined), any(Undefined)
	err := p.object(func(key string) error {
		var err error
		switch key {
		case "content":
			if p.peek() == '[' {
				children = nil
				err = p.array(func() error {
					c, err := p.node(s)
					children = append(children, c)
					return err
				})
				content = children
			} else {
				content, err = p.value()
			}
		case "openStart":
			openStart, err = p.value()
		case "openEnd":
			openEnd, err = p.value()
		default:
			_, err = p.value()
		}
		return err
	})
	if err != nil {
		return Slice{}, err
	}
	os, oe := 0, 0
	for i, v := range []any{openStart, openEnd} {
		if !truthy(v) {
			continue
		}
		f, ok := v.(float64)
		if !ok {
			return Slice{}, &ModelError{Msg: "Invalid input for Slice.fromJSON"}
		}
		n, ok := position(f)
		if !ok {
			return Slice{}, errBadStep("open depths are whole numbers")
		}
		if i == 0 {
			os = n
		} else {
			oe = n
		}
	}
	var frag Fragment
	if truthy(content) {
		kids, ok := content.([]*Node)
		if !ok {
			return Slice{}, &ModelError{Msg: "Invalid input for Fragment.fromJSON"}
		}
		frag = fragmentFromArray(kids)
	}
	ms, me := maxOpen(frag)
	if os > ms || oe > me || (frag.size == 0 && (os > 0 || oe > 0)) {
		return Slice{}, errBadStep("the slice is open deeper than its content")
	}
	return Slice{frag, os, oe}, nil
}

// ErrBadStep is wrapped by errors for steps the browser never makes:
// fractional or negative positions, ranges that end before they start,
// slices open deeper than their content.
var ErrBadStep = errors.New("doc: malformed step")

type badStepError struct{ msg string }

func (e *badStepError) Error() string { return "doc: malformed step: " + e.msg }
func (e *badStepError) Unwrap() error { return ErrBadStep }

func errBadStep(msg string) error { return &badStepError{msg} }

// position turns a JSON number into a document position.
func position(f float64) (int, bool) {
	if f < 0 || f > 1<<30 || f != float64(int(f)) {
		return 0, false
	}
	return int(f), true
}

// ParseNode reads a node as prosemirror-model's Node.fromJSON does, without
// checking its content against the schema.
func (s *Schema) ParseNode(data []byte) (*Node, error) {
	p := &parser{data: data}
	n, err := p.node(s)
	if err != nil {
		return nil, err
	}
	if err := p.end(); err != nil {
		return nil, err
	}
	return n, nil
}

// ParseDoc reads a document of tickit's schema and checks it: it must be a
// doc node and pass Check.
func ParseDoc(data []byte) (Doc, error) { return tickit.ParseDoc(data) }

// ParseDoc reads a document and checks it.
func (s *Schema) ParseDoc(data []byte) (Doc, error) {
	n, err := s.ParseNode(data)
	if err != nil {
		return nil, err
	}
	if n.typ != s.top {
		return nil, modelErrorf("A document is a %s node, not %s", s.top.name, n.typ.name)
	}
	if err := n.Check(); err != nil {
		return nil, err
	}
	return n, nil
}

// MarshalDoc writes a document as JSON.stringify(doc.toJSON()) does.
func MarshalDoc(d Doc) ([]byte, error) { return d.appendJSON(nil), nil }
