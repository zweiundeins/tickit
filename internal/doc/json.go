package doc

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// Attribute values are JSON values as JavaScript holds them: nil (null),
// bool, float64, string, []any and *Object. A missing value (JavaScript's
// undefined) is Undefined.

// Object is a JSON object with its keys in the order JSON.stringify writes
// them: array-index keys ascending first, then the others in the order they
// were added. A key added twice keeps its first place and its last value.
type Object struct {
	keys   []string
	values []any
}

type undefinedType struct{}

// Undefined stands for a value that is absent, as JavaScript's undefined.
var Undefined any = undefinedType{}

// NewObject returns an empty object.
func NewObject() *Object { return &Object{} }

// Len is the number of keys.
func (o *Object) Len() int { return len(o.keys) }

// Key returns the i-th key and its value.
func (o *Object) Key(i int) (string, any) { return o.keys[i], o.values[i] }

// Get returns the value of key, or Undefined.
func (o *Object) Get(key string) any {
	for i, k := range o.keys {
		if k == key {
			return o.values[i]
		}
	}
	return Undefined
}

// Set adds or replaces key.
func (o *Object) Set(key string, v any) {
	for i, k := range o.keys {
		if k == key {
			o.values[i] = v
			return
		}
	}
	if idx, ok := arrayIndex(key); ok {
		at := 0
		for at < len(o.keys) {
			j, isIdx := arrayIndex(o.keys[at])
			if !isIdx || j > idx {
				break
			}
			at++
		}
		o.keys = append(o.keys, "")
		o.values = append(o.values, nil)
		copy(o.keys[at+1:], o.keys[at:])
		copy(o.values[at+1:], o.values[at:])
		o.keys[at], o.values[at] = key, v
		return
	}
	o.keys = append(o.keys, key)
	o.values = append(o.values, v)
}

// arrayIndex reports whether key is a canonical array index, which
// JavaScript objects order before other keys.
func arrayIndex(key string) (uint64, bool) {
	if key == "" || len(key) > 10 || (len(key) > 1 && key[0] == '0') {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	return n, n < 1<<32-1
}

// truthy is JavaScript's ToBoolean for JSON values.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil, undefinedType:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case string:
		return x != ""
	}
	return true
}

// typeOf is JavaScript's typeof, with "null" for null as prosemirror-model's
// attribute validation names it.
func typeOf(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case undefinedType:
		return "undefined"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	}
	return "object"
}

// jsString is JavaScript's String(v) for JSON values, for error messages.
func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case undefinedType:
		return "undefined"
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return string(appendNumber(nil, x))
	case string:
		return x
	case []any:
		b := make([]byte, 0, 16)
		for i, e := range x {
			if i > 0 {
				b = append(b, ',')
			}
			if e != nil && e != Undefined {
				b = append(b, jsString(e)...)
			}
		}
		return string(b)
	}
	return "[object Object]"
}

// compareDeep is prosemirror-model's compareDeep for JSON values.
func compareDeep(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case undefinedType:
		return b == Undefined
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !compareDeep(x[i], y[i]) {
				return false
			}
		}
		return true
	case *Object:
		y, ok := b.(*Object)
		if !ok {
			return false
		}
		if x == y {
			return true
		}
		for i, k := range x.keys {
			v := y.Get(k)
			if v == Undefined && x.values[i] != Undefined {
				return false
			}
			if !compareDeep(x.values[i], v) {
				return false
			}
		}
		for _, k := range y.keys {
			found := false
			for _, kk := range x.keys {
				if kk == k {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	return false
}

// appendValue writes v as JSON.stringify does. Undefined members of objects
// are left out and undefined array elements written as null.
func appendValue(b []byte, v any) []byte {
	switch x := v.(type) {
	case nil, undefinedType:
		return append(b, "null"...)
	case bool:
		if x {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return append(b, "null"...)
		}
		return appendNumber(b, x)
	case string:
		return appendString(b, x)
	case []any:
		b = append(b, '[')
		for i, e := range x {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendValue(b, e)
		}
		return append(b, ']')
	case *Object:
		b = append(b, '{')
		first := true
		for i, k := range x.keys {
			if x.values[i] == Undefined {
				continue
			}
			if !first {
				b = append(b, ',')
			}
			first = false
			b = appendString(b, k)
			b = append(b, ':')
			b = appendValue(b, x.values[i])
		}
		return append(b, '}')
	}
	panic(fmt.Sprintf("doc: not a JSON value: %T", v))
}

// appendNumber writes f as JavaScript's Number.prototype.toString does.
func appendNumber(b []byte, f float64) []byte {
	if f == 0 {
		return append(b, '0')
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		if math.Abs(f) < 1<<53 {
			return strconv.AppendInt(b, int64(f), 10)
		}
	}
	// Shortest digits and exponent, then ECMAScript's layout rules.
	var buf [32]byte
	e := strconv.AppendFloat(buf[:0], math.Abs(f), 'e', -1, 64)
	mant, exp := e, 0
	for i, c := range e {
		if c == 'e' {
			mant = e[:i]
			exp, _ = strconv.Atoi(string(e[i+1:]))
			break
		}
	}
	digits := make([]byte, 0, len(mant))
	for _, c := range mant {
		if c != '.' {
			digits = append(digits, c)
		}
	}
	k := len(digits)
	n := exp + 1 // the decimal point sits after n digits
	if f < 0 {
		b = append(b, '-')
	}
	switch {
	case k <= n && n <= 21:
		b = append(b, digits...)
		for i := k; i < n; i++ {
			b = append(b, '0')
		}
	case 0 < n && n <= 21:
		b = append(b, digits[:n]...)
		b = append(b, '.')
		b = append(b, digits[n:]...)
	case -6 < n && n <= 0:
		b = append(b, "0."...)
		for i := 0; i < -n; i++ {
			b = append(b, '0')
		}
		b = append(b, digits...)
	default:
		b = append(b, digits[0])
		if k > 1 {
			b = append(b, '.')
			b = append(b, digits[1:]...)
		}
		b = append(b, 'e')
		if n-1 >= 0 {
			b = append(b, '+')
		}
		b = strconv.AppendInt(b, int64(n-1), 10)
	}
	return b
}

const hexDigits = "0123456789abcdef"

// appendString writes s quoted as JSON.stringify does: only the quote, the
// backslash and control characters are escaped.
func appendString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c != '"' && c != '\\' {
			continue
		}
		b = append(b, s[start:i]...)
		switch c {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\b':
			b = append(b, '\\', 'b')
		case '\f':
			b = append(b, '\\', 'f')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
		}
		start = i + 1
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}

// parser reads JSON as JSON.parse does, except that it refuses strings
// holding a lone surrogate (which Go's UTF-8 strings cannot hold) and
// numbers outside float64's range.
type parser struct {
	data []byte
	pos  int
}

// ErrLoneSurrogate is returned for a JSON string that holds half of a
// surrogate pair: ProseMirror would keep it, Go's strings cannot.
var ErrLoneSurrogate = errors.New("doc: a string holds half of a surrogate pair")

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("doc: invalid JSON at offset %d: %s", p.pos, fmt.Sprintf(format, args...))
}

func (p *parser) ws() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *parser) peek() byte {
	p.ws()
	if p.pos < len(p.data) {
		return p.data[p.pos]
	}
	return 0
}

func (p *parser) expect(c byte) error {
	if p.peek() != c {
		if p.pos >= len(p.data) {
			return p.errorf("unexpected end, want %q", c)
		}
		return p.errorf("unexpected %q, want %q", p.data[p.pos], c)
	}
	p.pos++
	return nil
}

func (p *parser) end() error {
	p.ws()
	if p.pos != len(p.data) {
		return p.errorf("unexpected %q after the value", p.data[p.pos])
	}
	return nil
}

// object calls f for each key of an object; f reads the value.
func (p *parser) object(f func(key string) error) error {
	if err := p.expect('{'); err != nil {
		return err
	}
	if p.peek() == '}' {
		p.pos++
		return nil
	}
	for {
		if p.peek() != '"' {
			return p.errorf("want a key")
		}
		key, _, err := p.string()
		if err != nil {
			return err
		}
		if err := p.expect(':'); err != nil {
			return err
		}
		if err := f(key); err != nil {
			return err
		}
		switch p.peek() {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return nil
		default:
			return p.errorf("want , or }")
		}
	}
}

// array calls f for each element; f reads the element.
func (p *parser) array(f func() error) error {
	if err := p.expect('['); err != nil {
		return err
	}
	if p.peek() == ']' {
		p.pos++
		return nil
	}
	for {
		if err := f(); err != nil {
			return err
		}
		switch p.peek() {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return nil
		default:
			return p.errorf("want , or ]")
		}
	}
}

// value reads any JSON value.
func (p *parser) value() (any, error) {
	switch c := p.peek(); {
	case c == '{':
		o := &Object{}
		err := p.object(func(key string) error {
			v, err := p.value()
			if err != nil {
				return err
			}
			o.Set(key, v)
			return nil
		})
		return o, err
	case c == '[':
		a := []any{}
		err := p.array(func() error {
			v, err := p.value()
			a = append(a, v)
			return err
		})
		return a, err
	case c == '"':
		s, _, err := p.string()
		return s, err
	case c == 't':
		return true, p.literal("true")
	case c == 'f':
		return false, p.literal("false")
	case c == 'n':
		return nil, p.literal("null")
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	case c == 0 && p.pos >= len(p.data):
		return nil, p.errorf("unexpected end")
	default:
		return nil, p.errorf("unexpected %q", c)
	}
}

func (p *parser) literal(s string) error {
	if len(p.data)-p.pos < len(s) || string(p.data[p.pos:p.pos+len(s)]) != s {
		return p.errorf("want %s", s)
	}
	p.pos += len(s)
	return nil
}

func (p *parser) number() (float64, error) {
	start := p.pos
	d := p.data
	i := p.pos
	if i < len(d) && d[i] == '-' {
		i++
	}
	switch {
	case i < len(d) && d[i] == '0':
		i++
	case i < len(d) && d[i] >= '1' && d[i] <= '9':
		for i < len(d) && d[i] >= '0' && d[i] <= '9' {
			i++
		}
	default:
		p.pos = i
		return 0, p.errorf("bad number")
	}
	if i < len(d) && d[i] == '.' {
		i++
		if i >= len(d) || d[i] < '0' || d[i] > '9' {
			p.pos = i
			return 0, p.errorf("bad number")
		}
		for i < len(d) && d[i] >= '0' && d[i] <= '9' {
			i++
		}
	}
	if i < len(d) && (d[i] == 'e' || d[i] == 'E') {
		i++
		if i < len(d) && (d[i] == '+' || d[i] == '-') {
			i++
		}
		if i >= len(d) || d[i] < '0' || d[i] > '9' {
			p.pos = i
			return 0, p.errorf("bad number")
		}
		for i < len(d) && d[i] >= '0' && d[i] <= '9' {
			i++
		}
	}
	p.pos = i
	// Small integers, the usual case, without strconv.
	if i-start <= 15 {
		n, neg, ok := int64(0), false, true
		for j := start; j < i; j++ {
			c := d[j]
			if c == '-' {
				neg = true
				continue
			}
			if c < '0' || c > '9' {
				ok = false
				break
			}
			n = n*10 + int64(c-'0')
		}
		if ok {
			if neg {
				if n == 0 {
					return math.Copysign(0, -1), nil
				}
				return float64(-n), nil
			}
			return float64(n), nil
		}
	}
	f, err := strconv.ParseFloat(string(d[start:i]), 64)
	if err != nil {
		return 0, p.errorf("number out of range")
	}
	return f, nil
}

// string reads a JSON string and returns it with its length in UTF-16 units.
func (p *parser) string() (string, int, error) {
	if err := p.expect('"'); err != nil {
		return "", 0, err
	}
	d := p.data
	start := p.pos
	// Fast path: ASCII without escapes.
	i := start
	for i < len(d) {
		c := d[i]
		if c == '"' {
			p.pos = i + 1
			return string(d[start:i]), i - start, nil
		}
		if c < 0x20 || c == '\\' || c >= 0x80 {
			break
		}
		i++
	}
	buf := make([]byte, 0, i-start+16)
	buf = append(buf, d[start:i]...)
	units := i - start
	for {
		if i >= len(d) {
			p.pos = i
			return "", 0, p.errorf("unterminated string")
		}
		c := d[i]
		switch {
		case c == '"':
			p.pos = i + 1
			return string(buf), units, nil
		case c < 0x20:
			p.pos = i
			return "", 0, p.errorf("control character in string")
		case c == '\\':
			i++
			if i >= len(d) {
				p.pos = i
				return "", 0, p.errorf("unterminated string")
			}
			switch d[i] {
			case '"', '\\', '/':
				buf = append(buf, d[i])
			case 'b':
				buf = append(buf, '\b')
			case 'f':
				buf = append(buf, '\f')
			case 'n':
				buf = append(buf, '\n')
			case 'r':
				buf = append(buf, '\r')
			case 't':
				buf = append(buf, '\t')
			case 'u':
				r, ok := hex4(d, i+1)
				if !ok {
					p.pos = i
					return "", 0, p.errorf("bad \\u escape")
				}
				i += 4
				if utf16.IsSurrogate(r) {
					if r >= 0xdc00 || i+6 >= len(d) || d[i+1] != '\\' || d[i+2] != 'u' {
						p.pos = i
						return "", 0, ErrLoneSurrogate
					}
					r2, ok := hex4(d, i+3)
					if !ok || r2 < 0xdc00 || r2 > 0xdfff {
						p.pos = i
						return "", 0, ErrLoneSurrogate
					}
					i += 6
					r = utf16.DecodeRune(r, r2)
					units++
				}
				buf = utf8.AppendRune(buf, r)
				units++
				i++
				continue
			default:
				p.pos = i
				return "", 0, p.errorf("bad escape")
			}
			units++
			i++
		case c < 0x80:
			buf = append(buf, c)
			units++
			i++
		default:
			r, size := utf8.DecodeRune(d[i:])
			if r == utf8.RuneError && size <= 1 {
				p.pos = i
				return "", 0, p.errorf("invalid UTF-8")
			}
			buf = append(buf, d[i:i+size]...)
			if r >= 0x10000 {
				units += 2
			} else {
				units++
			}
			i += size
		}
	}
}

func hex4(d []byte, i int) (rune, bool) {
	if i+4 > len(d) {
		return 0, false
	}
	var r rune
	for _, c := range d[i : i+4] {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, false
		}
	}
	return r, true
}

// ParseValue reads one JSON value with JSON.parse's semantics (objects keep
// JavaScript's key order), for attribute values.
func ParseValue(data []byte) (any, error) {
	p := &parser{data: data}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	return v, p.end()
}

// AppendValue writes a JSON value as JSON.stringify does.
func AppendValue(b []byte, v any) []byte { return appendValue(b, v) }
