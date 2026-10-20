package tracker

import (
	"errors"
	"strings"
)

// Ranks order the issues of a project on its board and in its backlog
// (FR-41, FR-45). A rank is a fractional index: a string over the 62
// digits 0-9, A-Z, a-z, which SQLite compares byte by byte as Go does.
// Between two ranks there is always another, so a move gives the moved
// issue a new rank and never renumbers the others. A rank is an integer
// part, whose first character says how many digits follow (a to z: 1 to
// 26, A to Z the negative ones), then an optional fraction without
// trailing zeros. Appending at the end grows the integer part, so a rank
// stays short; inserting between two neighbours grows the fraction. The
// algorithm is David Greenspan's, as rocicorp/fractional-indexing has it.

const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// FirstRank is the rank of the first issue of an empty project.
const FirstRank = "a0"

var errRank = errors.New("tracker: not a rank")

// RankBetween returns a rank that sorts after a and before b. An empty a
// means the start, an empty b the end. It fails when a is not before b
// or either is not a rank.
func RankBetween(a, b string) (string, error) {
	if a != "" {
		if err := validRank(a); err != nil {
			return "", err
		}
	}
	if b != "" {
		if err := validRank(b); err != nil {
			return "", err
		}
	}
	if a != "" && b != "" && a >= b {
		return "", errors.New("tracker: the ranks are not in order")
	}
	switch {
	case a == "" && b == "":
		return FirstRank, nil
	case a == "":
		ib := integerPart(b)
		fb := b[len(ib):]
		if ib == smallestInteger {
			return ib + midpoint("", fb), nil
		}
		if ib < b {
			return ib, nil
		}
		res, ok := decrementInteger(ib)
		if !ok {
			return "", errors.New("tracker: no rank before the smallest")
		}
		return res, nil
	case b == "":
		ia := integerPart(a)
		fa := a[len(ia):]
		i, ok := incrementInteger(ia)
		if !ok {
			return ia + midpoint(fa, ""), nil
		}
		return i, nil
	}
	ia := integerPart(a)
	fa := a[len(ia):]
	ib := integerPart(b)
	fb := b[len(ib):]
	if ia == ib {
		return ia + midpoint(fa, fb), nil
	}
	i, ok := incrementInteger(ia)
	if !ok {
		return "", errors.New("tracker: no rank after the largest")
	}
	if i < b {
		return i, nil
	}
	return ia + midpoint(fa, ""), nil
}

// ValidRank reports whether s is a rank.
func ValidRank(s string) bool { return validRank(s) == nil }

var smallestInteger = "A" + strings.Repeat("0", 26)

func integerLength(head byte) int {
	switch {
	case head >= 'a' && head <= 'z':
		return int(head-'a') + 2
	case head >= 'A' && head <= 'Z':
		return int('Z'-head) + 2
	}
	return 0
}

// integerPart is the integer part of a valid rank.
func integerPart(key string) string { return key[:integerLength(key[0])] }

func validRank(key string) error {
	if key == "" || len(key) > 512 {
		return errRank
	}
	n := integerLength(key[0])
	if n == 0 || n > len(key) || key == smallestInteger {
		return errRank
	}
	for i := 1; i < len(key); i++ {
		if strings.IndexByte(digits, key[i]) < 0 {
			return errRank
		}
	}
	if len(key) > n && key[len(key)-1] == '0' {
		return errRank // a fraction never ends in zero
	}
	return nil
}

// midpoint returns a fraction between a and b (b empty: the end); a < b,
// neither ends in zero.
func midpoint(a, b string) string {
	if b != "" {
		n := 0
		for n < len(b) {
			ca := byte('0')
			if n < len(a) {
				ca = a[n]
			}
			if ca != b[n] {
				break
			}
			n++
		}
		if n > 0 {
			rest := ""
			if n < len(a) {
				rest = a[n:]
			}
			return b[:n] + midpoint(rest, b[n:])
		}
	}
	da := 0
	if a != "" {
		da = strings.IndexByte(digits, a[0])
	}
	db := len(digits)
	if b != "" {
		db = strings.IndexByte(digits, b[0])
	}
	if db-da > 1 {
		return string(digits[(da+db+1)/2])
	}
	if len(b) > 1 {
		return b[:1]
	}
	rest := ""
	if len(a) > 1 {
		rest = a[1:]
	}
	return string(digits[da]) + midpoint(rest, "")
}

func incrementInteger(x string) (string, bool) {
	head, digs := x[0], []byte(x[1:])
	carry := true
	for i := len(digs) - 1; carry && i >= 0; i-- {
		d := strings.IndexByte(digits, digs[i]) + 1
		if d == len(digits) {
			digs[i] = digits[0]
		} else {
			digs[i] = digits[d]
			carry = false
		}
	}
	if !carry {
		return string(head) + string(digs), true
	}
	switch head {
	case 'Z':
		return "a" + digits[:1], true
	case 'z':
		return "", false
	}
	h := head + 1
	if h > 'a' {
		digs = append(digs, digits[0])
	} else {
		digs = digs[:len(digs)-1]
	}
	return string(h) + string(digs), true
}

func decrementInteger(x string) (string, bool) {
	head, digs := x[0], []byte(x[1:])
	borrow := true
	for i := len(digs) - 1; borrow && i >= 0; i-- {
		d := strings.IndexByte(digits, digs[i]) - 1
		if d == -1 {
			digs[i] = digits[len(digits)-1]
		} else {
			digs[i] = digits[d]
			borrow = false
		}
	}
	if !borrow {
		return string(head) + string(digs), true
	}
	switch head {
	case 'a':
		return "Z" + digits[len(digits)-1:], true
	case 'A':
		return "", false
	}
	h := head - 1
	if h < 'Z' {
		digs = append(digs, digits[len(digits)-1])
	} else {
		digs = digs[:len(digs)-1]
	}
	return string(h) + string(digs), true
}
