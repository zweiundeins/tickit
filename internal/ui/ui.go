// Package ui is tickit's design system: templ components with plain Go
// props, and the pixel icons and faces they draw. It imports no domain
// package: feature code maps its read types onto these props.
//
// Components render semantic markup and the classes of
// internal/web/static/css; they hold no feature behaviour. Where a feature
// wires behaviour (Datastar actions, pending states, refusals), a prop of
// type templ.Attributes takes its attributes, which must come from Go
// constants and never hold a person's value (NFR-13). /design renders
// every component; docs/design-system.md says how to use them.
package ui

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

// Size is a component's size.
type Size string

const (
	SizeXS Size = "xs"
	SizeSM Size = "sm"
	SizeMD Size = "md"
	SizeLG Size = "lg"
)

// Tone is the colour of a message: what it says, never alone (each tone
// has its icon and its words).
type Tone string

const (
	ToneNeutral Tone = ""
	ToneInfo    Tone = "info"
	ToneOK      Tone = "ok"
	ToneWarn    Tone = "warn"
	ToneDanger  Tone = "danger"
)

// icon is the tone's icon.
func (t Tone) icon() string {
	switch t {
	case ToneOK:
		return "check"
	case ToneWarn, ToneDanger:
		return "warning"
	}
	return "info"
}

// NavItem is a link in a navigation: the header's, the sidebar's, tabs.
type NavItem struct {
	Label   string
	Href    string
	Icon    string
	Current bool
	// Count shows after the label when it is not nil (issues in a filter).
	Count *int
}

// Count returns a pointer to n, for NavItem.Count.
func Count(n int) *int { return &n }

// Person is who a face, a byline or a presence entry shows. Seed is the
// person's id: it picks the face and its hue, and never shows.
type Person struct {
	Seed string
	Name string
	// Hue, when set, is the person's own hue, 0 to 15, in place of the
	// seed's: a seeded person's, or one a person chose.
	Hue *int
	// Photo, when set, is an uploaded picture's URL, drawn in the face's
	// tile.
	Photo string
	// Away: no input for five minutes (MP-02.4).
	Away bool
	// Deactivated: an account that can no longer log in.
	Deactivated bool
}

// HueIndex is the person's hue: their own, or the one their seed draws.
func (p Person) HueIndex() int {
	if p.Hue != nil {
		return *p.Hue & 15
	}
	return FaceOf(p.Seed).Hue
}

// classes joins class names, skipping empty ones.
func classes(names ...string) string {
	var b strings.Builder
	for _, n := range names {
		if n == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(n)
	}
	return b.String()
}

// when is s if ok, else "": a class that applies when ok.
func when(ok bool, s string) string {
	if ok {
		return s
	}
	return ""
}

func itoa(n int) string { return strconv.Itoa(n) }

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// jsonAttr encodes v for an attribute a Starbase component reads as JSON;
// templ escapes it for the attribute.
func jsonAttr(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// slug is how sb-tabs names a tab's slot: the label in lower case, each
// run of anything but letters and digits as one hyphen, none at the ends.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	return b.String()
}
