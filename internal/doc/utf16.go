package doc

import (
	"errors"
	"unicode/utf8"
)

// Text is kept as UTF-8. Sizes and offsets count UTF-16 code units, as
// ProseMirror's positions do in the browser.

// ErrSurrogateSplit is returned when a step would cut a character outside
// the Basic Multilingual Plane (an emoji, say) in half: a position inside a
// surrogate pair. Browsers never put a caret there.
var ErrSurrogateSplit = errors.New("doc: a position falls inside a surrogate pair")

// splitPanic is raised by text cuts inside a surrogate pair; step
// application turns it into ErrSurrogateSplit.
type splitPanic struct{}

func utf16Len(s string) int {
	n := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			n++
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
		i += size
	}
	return n
}

// byteOffset turns an offset in UTF-16 units into a byte offset of s, whose
// UTF-16 length is units. It panics with splitPanic inside a pair.
func byteOffset(s string, units, off int) int {
	if len(s) == units || off == 0 {
		return off
	}
	if off == units {
		return len(s)
	}
	n := 0
	for i := 0; i < len(s); {
		if n == off {
			return i
		}
		c := s[i]
		if c < 0x80 {
			n++
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r >= 0x10000 {
			n += 2
			if n > off {
				panic(splitPanic{})
			}
		} else {
			n++
		}
		i += size
	}
	return len(s)
}

// truncateUTF16 keeps the first n UTF-16 units of s; half a pair at the end
// becomes U+FFFD, as goja hands such a JavaScript string to Go.
func truncateUTF16(s string, n int) string {
	units := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if units+w > n {
			if units < n {
				return s[:i] + "�"
			}
			return s[:i]
		}
		units += w
		i += size
	}
	return s
}
