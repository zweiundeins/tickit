// Package words holds the demo's word list (DM-06.4): display names,
// labels, and project and space names that hold a listed word are
// refused. It imports nothing of tickit, so that people, tracker and the
// wiki check names with it without importing the demo.
package words

import (
	_ "embed"
	"strings"
	"unicode"
)

// Refusal is what a refused name hears.
const Refusal = "Please choose another name."

//go:embed words.txt
var list string

// whole are the entries that match a whole word, inside the entries that
// also match within words and across spaces.
var whole, inside = parse(list)

func parse(src string) (map[string]bool, []string) {
	w := map[string]bool{}
	var in []string
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		star := strings.HasPrefix(line, "*")
		e := rot13(strings.TrimPrefix(line, "*"))
		if star {
			in = append(in, e)
		}
		w[e] = true
	}
	return w, in
}

func rot13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case 'a' <= r && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case 'A' <= r && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}

// swaps are the digits and signs people type for letters.
var swaps = strings.NewReplacer("0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t", "@", "a", "$", "s", "!", "i")

// Blocked reports whether s holds a word of the list: a whole word after
// lower case and the usual swaps, or a starred entry anywhere in its
// letters.
func Blocked(s string) bool {
	s = swaps.Replace(strings.ToLower(s))
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if whole[w] {
			return true
		}
	}
	letters := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			return r
		}
		return -1
	}, s)
	for _, w := range inside {
		if strings.Contains(letters, w) {
			return true
		}
	}
	return false
}
