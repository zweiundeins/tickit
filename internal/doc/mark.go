package doc

import "slices"

// Mark is a mark on an inline node: its type and attributes. Marks are
// immutable.
type Mark struct {
	typ   *MarkType
	attrs []any
}

// Type returns the mark's type.
func (m *Mark) Type() *MarkType { return m.typ }

// Attr returns the value of the attribute called name, or Undefined.
func (m *Mark) Attr(name string) any {
	for i, a := range m.typ.attrs {
		if a.name == name {
			return m.attrs[i]
		}
	}
	return Undefined
}

// AttrString returns a string attribute, or "".
func (m *Mark) AttrString(name string) string { s, _ := m.Attr(name).(string); return s }

// Create makes a mark of the type. attrs may be nil when every attribute has
// a default.
func (t *MarkType) Create(attrs map[string]any) (*Mark, error) {
	var v any
	if attrs != nil {
		o := NewObject()
		for _, a := range t.attrs {
			if x, ok := attrs[a.name]; ok {
				o.Set(a.name, x)
			}
		}
		v = o
	}
	return t.create(v)
}

func (t *MarkType) create(value any) (*Mark, error) {
	if !truthy(value) && t.instance != nil {
		return t.instance, nil
	}
	attrs, err := computeAttrs(t.name, t.attrs, nil, value)
	if err != nil {
		return nil, err
	}
	return &Mark{typ: t, attrs: attrs}, nil
}

// Eq reports whether two marks have the same type and attributes.
func (m *Mark) Eq(o *Mark) bool {
	if m == o {
		return true
	}
	if m.typ != o.typ {
		return false
	}
	for i := range m.attrs {
		if !compareDeep(m.attrs[i], o.attrs[i]) {
			return false
		}
	}
	return true
}

// addToSet returns the set with the mark in its place, replacing marks it
// excludes; the set itself when the mark is in it already.
func (m *Mark) addToSet(set []*Mark) []*Mark {
	var cp []*Mark
	copied, placed := false, false
	for i, other := range set {
		if m.Eq(other) {
			return set
		}
		if m.typ.excludes(other.typ) {
			if !copied {
				cp = slices.Clone(set[:i])
				copied = true
			}
		} else if other.typ.excludes(m.typ) {
			return set
		} else {
			if !placed && other.typ.rank > m.typ.rank {
				if !copied {
					cp = slices.Clone(set[:i])
					copied = true
				}
				cp = append(cp, m)
				placed = true
			}
			if copied {
				cp = append(cp, other)
			}
		}
	}
	if !copied {
		cp = slices.Clone(set)
	}
	if !placed {
		cp = append(cp, m)
	}
	return cp
}

// removeFromSet returns the set without the mark; the set itself when the
// mark is not in it.
func (m *Mark) removeFromSet(set []*Mark) []*Mark {
	for i, o := range set {
		if m.Eq(o) {
			out := make([]*Mark, 0, len(set)-1)
			out = append(out, set[:i]...)
			return append(out, set[i+1:]...)
		}
	}
	return set
}

func (m *Mark) isInSet(set []*Mark) bool {
	for _, o := range set {
		if m.Eq(o) {
			return true
		}
	}
	return false
}

// removeFromSet returns the set without marks of the type.
func (t *MarkType) removeFromSet(set []*Mark) []*Mark {
	for i := 0; i < len(set); i++ {
		if set[i].typ == t {
			out := make([]*Mark, 0, len(set)-1)
			out = append(out, set[:i]...)
			set = append(out, set[i+1:]...)
			i--
		}
	}
	return set
}

func (t *MarkType) isInSet(set []*Mark) *Mark {
	for _, m := range set {
		if m.typ == t {
			return m
		}
	}
	return nil
}

func sameMarkSet(a, b []*Mark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Eq(b[i]) {
			return false
		}
	}
	return true
}

// setFrom sorts marks by rank, keeping the order of marks of equal rank.
func setFrom(marks []*Mark) []*Mark {
	if len(marks) == 0 {
		return nil
	}
	cp := slices.Clone(marks)
	slices.SortStableFunc(cp, func(a, b *Mark) int { return a.typ.rank - b.typ.rank })
	return cp
}

func (m *Mark) appendJSON(b []byte) []byte {
	b = append(b, `{"type":`...)
	b = appendString(b, m.typ.name)
	if len(m.attrs) > 0 {
		b = append(b, `,"attrs":`...)
		b = appendAttrs(b, m.typ.attrs, m.attrs)
	}
	return append(b, '}')
}

func appendAttrs(b []byte, specs []*attribute, values []any) []byte {
	b = append(b, '{')
	first := true
	for i, a := range specs {
		if values[i] == Undefined {
			continue
		}
		if !first {
			b = append(b, ',')
		}
		first = false
		b = appendString(b, a.name)
		b = append(b, ':')
		b = appendValue(b, values[i])
	}
	return append(b, '}')
}
