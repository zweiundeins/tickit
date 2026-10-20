package doc

import (
	"errors"
	"fmt"
)

// Step is one of the five step types the server applies: ReplaceStep,
// ReplaceAroundStep, AddMarkStep, RemoveMarkStep and AttrStep, ported from
// prosemirror-transform.
type Step interface {
	// Apply applies the step. A *StepFailed error is prosemirror-transform's
	// failed StepResult (the step does not fit); any other error is an
	// exception prosemirror-transform throws.
	Apply(doc Doc) (Doc, error)
	// Map maps the step through a mapping; nil when its range was deleted.
	Map(m Mappable) Step
	// StepMap returns the map of the step's change.
	StepMap() *StepMap
	appendJSON(b []byte) []byte
}

// StepFailed is a failed StepResult: the step did not fit the document.
type StepFailed struct{ Reason string }

func (e *StepFailed) Error() string { return "doc: step failed: " + e.Reason }

// IsStepFailed reports whether err is a failed StepResult.
func IsStepFailed(err error) bool {
	var f *StepFailed
	return errors.As(err, &f)
}

// fromReplace is StepResult.fromReplace: a replaceError fails the step,
// other panics are exceptions.
func fromReplace(doc *Node, from, to int, s Slice) (out *Node, err error) {
	defer func() {
		if r := recover(); r != nil {
			if re, ok := r.(*replaceError); ok {
				out, err = nil, &StepFailed{Reason: re.msg}
				return
			}
			panic(r)
		}
	}()
	return doc.replace(from, to, s), nil
}

// guard turns the model's panics into errors at a step's surface.
func guard(err *error) {
	switch r := recover().(type) {
	case nil:
	case *ModelError:
		*err = r
	case splitPanic:
		*err = ErrSurrogateSplit
	case *replaceError:
		*err = &StepFailed{Reason: r.msg}
	default:
		panic(r)
	}
}

// ReplaceStep replaces From..To with a slice.
type ReplaceStep struct {
	From, To  int
	Slice     Slice
	Structure bool
}

// Apply applies the step.
func (s *ReplaceStep) Apply(doc Doc) (out Doc, err error) {
	defer guard(&err)
	if s.Structure && contentBetween(doc, s.From, s.To) {
		return nil, &StepFailed{Reason: "Structure replace would overwrite content"}
	}
	return fromReplace(doc, s.From, s.To, s.Slice)
}

// StepMap returns the step's map.
func (s *ReplaceStep) StepMap() *StepMap {
	return NewStepMap([]int{s.From, s.To - s.From, s.Slice.Size()}, false)
}

// Map maps the step.
func (s *ReplaceStep) Map(m Mappable) Step {
	to := m.MapResult(s.To, -1)
	from := m.MapResult(s.From, 1)
	if from.DeletedAcross() && to.DeletedAcross() {
		return nil
	}
	return &ReplaceStep{From: from.Pos, To: max(from.Pos, to.Pos), Slice: s.Slice, Structure: s.Structure}
}

func (s *ReplaceStep) appendJSON(b []byte) []byte {
	b = append(b, `{"stepType":"replace","from":`...)
	b = appendNumber(b, float64(s.From))
	b = append(b, `,"to":`...)
	b = appendNumber(b, float64(s.To))
	if s.Slice.Size() != 0 {
		b = append(b, `,"slice":`...)
		b = s.Slice.appendJSON(b)
	}
	if s.Structure {
		b = append(b, `,"structure":true`...)
	}
	return append(b, '}')
}

// ReplaceAroundStep replaces From..To with a slice, keeping the content
// between GapFrom and GapTo at Insert in the slice.
type ReplaceAroundStep struct {
	From, To, GapFrom, GapTo int
	Slice                    Slice
	Insert                   int
	Structure                bool
}

// Apply applies the step.
func (s *ReplaceAroundStep) Apply(doc Doc) (out Doc, err error) {
	defer guard(&err)
	if s.Structure && (contentBetween(doc, s.From, s.GapFrom) || contentBetween(doc, s.GapTo, s.To)) {
		return nil, &StepFailed{Reason: "Structure gap-replace would overwrite content"}
	}
	gap := doc.slice(s.GapFrom, s.GapTo)
	if gap.OpenStart != 0 || gap.OpenEnd != 0 {
		return nil, &StepFailed{Reason: "Gap is not a flat range"}
	}
	inserted, ok := s.Slice.insertAt(s.Insert, gap.Content)
	if !ok {
		return nil, &StepFailed{Reason: "Content does not fit in gap"}
	}
	return fromReplace(doc, s.From, s.To, inserted)
}

// StepMap returns the step's map.
func (s *ReplaceAroundStep) StepMap() *StepMap {
	return NewStepMap([]int{s.From, s.GapFrom - s.From, s.Insert, s.GapTo, s.To - s.GapTo, s.Slice.Size() - s.Insert}, false)
}

// Map maps the step.
func (s *ReplaceAroundStep) Map(m Mappable) Step {
	from, to := m.MapResult(s.From, 1), m.MapResult(s.To, -1)
	gapFrom := from.Pos
	if s.From != s.GapFrom {
		gapFrom = m.Map(s.GapFrom, -1)
	}
	gapTo := to.Pos
	if s.To != s.GapTo {
		gapTo = m.Map(s.GapTo, 1)
	}
	if (from.DeletedAcross() && to.DeletedAcross()) || gapFrom < from.Pos || gapTo > to.Pos {
		return nil
	}
	return &ReplaceAroundStep{From: from.Pos, To: to.Pos, GapFrom: gapFrom, GapTo: gapTo, Slice: s.Slice, Insert: s.Insert, Structure: s.Structure}
}

func (s *ReplaceAroundStep) appendJSON(b []byte) []byte {
	b = append(b, `{"stepType":"replaceAround","from":`...)
	b = appendNumber(b, float64(s.From))
	b = append(b, `,"to":`...)
	b = appendNumber(b, float64(s.To))
	b = append(b, `,"gapFrom":`...)
	b = appendNumber(b, float64(s.GapFrom))
	b = append(b, `,"gapTo":`...)
	b = appendNumber(b, float64(s.GapTo))
	b = append(b, `,"insert":`...)
	b = appendNumber(b, float64(s.Insert))
	if s.Slice.Size() != 0 {
		b = append(b, `,"slice":`...)
		b = s.Slice.appendJSON(b)
	}
	if s.Structure {
		b = append(b, `,"structure":true`...)
	}
	return append(b, '}')
}

func contentBetween(doc *Node, from, to int) bool {
	rf := resolve(doc, from)
	dist, depth := to-from, rf.Depth
	for dist > 0 && depth > 0 && rf.indexAfter(depth) == len(rf.Node(depth).content.nodes) {
		depth--
		dist--
	}
	if dist > 0 {
		next := rf.Node(depth).content.maybeChild(rf.indexAfter(depth))
		for dist > 0 {
			if next == nil || next.IsLeaf() {
				return true
			}
			next = next.content.firstChild()
			dist--
		}
	}
	return false
}

func mapFragment(f Fragment, fn func(child, parent *Node) *Node, parent *Node) Fragment {
	mapped := make([]*Node, 0, len(f.nodes))
	for _, child := range f.nodes {
		if child.content.size > 0 {
			child = child.copy(mapFragment(child.content, fn, child))
		}
		if !child.typ.isBlock {
			child = fn(child, parent)
		}
		mapped = append(mapped, child)
	}
	return fragmentFromArray(mapped)
}

// AddMarkStep adds a mark to the inline content between From and To.
type AddMarkStep struct {
	From, To int
	Mark     *Mark
}

// Apply applies the step.
func (s *AddMarkStep) Apply(doc Doc) (out Doc, err error) {
	defer guard(&err)
	old := doc.slice(s.From, s.To)
	rf := resolve(doc, s.From)
	parent := rf.Node(rf.sharedDepth(s.To))
	sl := Slice{mapFragment(old.Content, func(n, p *Node) *Node {
		if !n.IsAtom() || !p.typ.allowsMarkType(s.Mark.typ) {
			return n
		}
		return n.mark(s.Mark.addToSet(n.marks))
	}, parent), old.OpenStart, old.OpenEnd}
	return fromReplace(doc, s.From, s.To, sl)
}

// StepMap returns the step's map: marks move nothing.
func (s *AddMarkStep) StepMap() *StepMap { return emptyStepMap }

// Map maps the step.
func (s *AddMarkStep) Map(m Mappable) Step {
	from, to := m.MapResult(s.From, 1), m.MapResult(s.To, -1)
	if (from.Deleted() && to.Deleted()) || from.Pos >= to.Pos {
		return nil
	}
	return &AddMarkStep{From: from.Pos, To: to.Pos, Mark: s.Mark}
}

func (s *AddMarkStep) appendJSON(b []byte) []byte {
	b = append(b, `{"stepType":"addMark","mark":`...)
	b = s.Mark.appendJSON(b)
	b = append(b, `,"from":`...)
	b = appendNumber(b, float64(s.From))
	b = append(b, `,"to":`...)
	b = appendNumber(b, float64(s.To))
	return append(b, '}')
}

// RemoveMarkStep removes a mark from the inline content between From and To.
type RemoveMarkStep struct {
	From, To int
	Mark     *Mark
}

// Apply applies the step.
func (s *RemoveMarkStep) Apply(doc Doc) (out Doc, err error) {
	defer guard(&err)
	old := doc.slice(s.From, s.To)
	sl := Slice{mapFragment(old.Content, func(n, _ *Node) *Node {
		return n.mark(s.Mark.removeFromSet(n.marks))
	}, doc), old.OpenStart, old.OpenEnd}
	return fromReplace(doc, s.From, s.To, sl)
}

// StepMap returns the step's map: marks move nothing.
func (s *RemoveMarkStep) StepMap() *StepMap { return emptyStepMap }

// Map maps the step.
func (s *RemoveMarkStep) Map(m Mappable) Step {
	from, to := m.MapResult(s.From, 1), m.MapResult(s.To, -1)
	if (from.Deleted() && to.Deleted()) || from.Pos >= to.Pos {
		return nil
	}
	return &RemoveMarkStep{From: from.Pos, To: to.Pos, Mark: s.Mark}
}

func (s *RemoveMarkStep) appendJSON(b []byte) []byte {
	b = append(b, `{"stepType":"removeMark","mark":`...)
	b = s.Mark.appendJSON(b)
	b = append(b, `,"from":`...)
	b = appendNumber(b, float64(s.From))
	b = append(b, `,"to":`...)
	b = appendNumber(b, float64(s.To))
	return append(b, '}')
}

// AttrStep sets one attribute of the node at Pos. Value may be Undefined
// (the attribute's default) as in prosemirror-transform.
type AttrStep struct {
	Pos   int
	Attr  string
	Value any
}

// Apply applies the step.
func (s *AttrStep) Apply(doc Doc) (out Doc, err error) {
	defer guard(&err)
	node := doc.nodeAt(s.Pos)
	if node == nil {
		return nil, &StepFailed{Reason: "No node at attribute step's position"}
	}
	if node.typ.isText {
		return nil, &ModelError{Msg: "NodeType.create can't construct text nodes"}
	}
	o := NewObject()
	for i, a := range node.typ.attrs {
		o.Set(a.name, node.attrs[i])
	}
	o.Set(s.Attr, s.Value)
	attrs, err := computeAttrs(node.typ.name, node.typ.attrs, node.typ.defaultAttrs, o)
	if err != nil {
		return nil, err
	}
	updated := newNode(node.typ, attrs, Fragment{}, node.marks)
	openEnd := 1
	if node.IsLeaf() {
		openEnd = 0
	}
	return fromReplace(doc, s.Pos, s.Pos+1, Slice{Fragment{nodes: []*Node{updated}, size: updated.size}, 0, openEnd})
}

// StepMap returns the step's map: attributes move nothing.
func (s *AttrStep) StepMap() *StepMap { return emptyStepMap }

// Map maps the step.
func (s *AttrStep) Map(m Mappable) Step {
	p := m.MapResult(s.Pos, 1)
	if p.DeletedAfter() {
		return nil
	}
	return &AttrStep{Pos: p.Pos, Attr: s.Attr, Value: s.Value}
}

func (s *AttrStep) appendJSON(b []byte) []byte {
	b = append(b, `{"stepType":"attr","pos":`...)
	b = appendNumber(b, float64(s.Pos))
	b = append(b, `,"attr":`...)
	b = appendString(b, s.Attr)
	if s.Value != Undefined {
		b = append(b, `,"value":`...)
		b = appendValue(b, s.Value)
	}
	return append(b, '}')
}

// MarshalStep writes a step as JSON.stringify(step.toJSON()) does.
func MarshalStep(s Step) []byte { return s.appendJSON(nil) }

// MarshalSteps writes steps as a JSON array.
func MarshalSteps(steps []Step) ([]byte, error) {
	b := []byte{'['}
	for i, s := range steps {
		if i > 0 {
			b = append(b, ',')
		}
		b = s.appendJSON(b)
	}
	return append(b, ']'), nil
}

// ParseSteps reads a JSON array of steps of the five allowed types.
func ParseSteps(data []byte) ([]Step, error) { return tickit.ParseSteps(data) }

// ParseStep reads one step.
func (s *Schema) ParseStep(data []byte) (Step, error) {
	p := &parser{data: data}
	st, err := p.step(s)
	if err != nil {
		return nil, err
	}
	return st, p.end()
}

// ParseSteps reads a JSON array of steps.
func (s *Schema) ParseSteps(data []byte) ([]Step, error) {
	p := &parser{data: data}
	var steps []Step
	err := p.array(func() error {
		st, err := p.step(s)
		if err != nil {
			return fmt.Errorf("step %d: %w", len(steps), err)
		}
		steps = append(steps, st)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return steps, p.end()
}

// Commit limits beyond the schema.
const (
	MaxCommitSteps = 500
	MaxCommitBytes = 256 << 10
)

// ParseCommit reads the steps of a commit: at most 500 steps in 256 KB.
func ParseCommit(data []byte) ([]Step, error) {
	if len(data) > MaxCommitBytes {
		return nil, &RuleError{Msg: "This change is too large to save at once."}
	}
	steps, err := ParseSteps(data)
	if err != nil {
		return nil, err
	}
	if len(steps) > MaxCommitSteps {
		return nil, &RuleError{Msg: "This change has too many steps to save at once."}
	}
	return steps, nil
}

// ErrStepType is wrapped by the error for a step type the server refuses.
var ErrStepType = errors.New("doc: step type not allowed")

func (p *parser) step(s *Schema) (Step, error) {
	if p.peek() != '{' {
		if _, err := p.value(); err != nil {
			return nil, err
		}
		return nil, &ModelError{Msg: "Invalid input for Step.fromJSON"}
	}
	f := map[string]any{}
	var slice Slice
	var mark *Mark
	err := p.object(func(key string) error {
		var err error
		switch key {
		case "slice":
			slice, err = p.slice(s)
		case "mark":
			mark, err = p.mark(s)
		default:
			var v any
			v, err = p.value()
			f[key] = v
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	get := func(k string) any {
		if v, ok := f[k]; ok {
			return v
		}
		return Undefined
	}
	st := get("stepType")
	if !truthy(st) {
		return nil, &ModelError{Msg: "Invalid input for Step.fromJSON"}
	}
	nums := func(names ...string) ([]int, error) {
		out := make([]int, len(names))
		for i, n := range names {
			v, ok := get(n).(float64)
			if !ok {
				return nil, modelErrorf("Invalid input for %s.fromJSON", stepClass(st))
			}
			if out[i], ok = position(v); !ok {
				return nil, errBadStep(n + " must be a whole number from 0")
			}
		}
		return out, nil
	}
	switch st {
	case "replace":
		n, err := nums("from", "to")
		if err != nil {
			return nil, err
		}
		if n[0] > n[1] {
			return nil, errBadStep("from is after to")
		}
		return &ReplaceStep{From: n[0], To: n[1], Slice: slice, Structure: truthy(get("structure"))}, nil
	case "replaceAround":
		n, err := nums("from", "to", "gapFrom", "gapTo", "insert")
		if err != nil {
			return nil, err
		}
		if !(n[0] <= n[2] && n[2] <= n[3] && n[3] <= n[1]) || n[4] > slice.Size() {
			return nil, errBadStep("the gap is outside the range")
		}
		return &ReplaceAroundStep{From: n[0], To: n[1], GapFrom: n[2], GapTo: n[3], Insert: n[4], Slice: slice, Structure: truthy(get("structure"))}, nil
	case "addMark", "removeMark":
		n, err := nums("from", "to")
		if err != nil {
			return nil, err
		}
		if mark == nil {
			return nil, &ModelError{Msg: "Invalid input for Mark.fromJSON"}
		}
		if n[0] > n[1] {
			return nil, errBadStep("from is after to")
		}
		if st == "addMark" {
			return &AddMarkStep{From: n[0], To: n[1], Mark: mark}, nil
		}
		return &RemoveMarkStep{From: n[0], To: n[1], Mark: mark}, nil
	case "attr":
		n, err := nums("pos")
		if err != nil {
			return nil, err
		}
		attr, ok := get("attr").(string)
		if !ok {
			return nil, &ModelError{Msg: "Invalid input for AttrStep.fromJSON"}
		}
		return &AttrStep{Pos: n[0], Attr: attr, Value: get("value")}, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrStepType, jsString(st))
}

func stepClass(st any) string {
	switch st {
	case "replace":
		return "ReplaceStep"
	case "replaceAround":
		return "ReplaceAroundStep"
	case "addMark":
		return "AddMarkStep"
	case "removeMark":
		return "RemoveMarkStep"
	}
	return "AttrStep"
}
