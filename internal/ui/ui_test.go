package ui

import (
	"strings"
	"testing"
	"time"
)

func TestFaceOf(t *testing.T) {
	a, b := FaceOf("s-ada"), FaceOf("s-ada")
	if a != b {
		t.Fatal("the same seed drew two faces")
	}
	hues := map[int]bool{}
	for _, seed := range []string{"s-ada", "s-bob", "g-ABCDEF", "a-0192", "s-sunny", "s-heron", "s-lynx"} {
		f := FaceOf(seed)
		if f.Hue < 0 || f.Hue > 15 || !strings.HasPrefix(f.Path, "M") {
			t.Errorf("%s: %+v", seed, f)
		}
		hues[f.Hue] = true
	}
	if len(hues) < 3 {
		t.Errorf("seven seeds gave %d hues", len(hues))
	}
}

// NFR-51: relative times are English.
func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 16, 13, 24, 0, 0, time.UTC)
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Second, "just now"},
		{time.Minute, "1 minute ago"},
		{5 * time.Minute, "5 minutes ago"},
		{65 * time.Minute, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{22 * time.Hour, "yesterday"},
		{3 * 24 * time.Hour, "3 days ago"},
		{20 * 24 * time.Hour, "Sep 26"},
		{400 * 24 * time.Hour, "Sep 11, 2025"},
		{-5 * time.Minute, "in 5 minutes"},
		{-26 * time.Hour, "tomorrow"},
	} {
		if got := Ago(now.Add(-c.d), now); got != c.want {
			t.Errorf("%v ago: %q, want %q", c.d, got, c.want)
		}
	}
	if got := Absolute(now); got != "Friday, 16 October 2026, 13:24 UTC" {
		t.Errorf("Absolute: %q", got)
	}
}

func TestIcons(t *testing.T) {
	for _, name := range IconNames() {
		if s := iconSVG(name, "tk-icon", ""); !strings.Contains(s, `d="M`) {
			t.Errorf("%s draws nothing", name)
		}
	}
	for _, ty := range IssueTypes {
		if typePaths[string(ty)] == "" {
			t.Errorf("type %s has no glyph", ty)
		}
	}
	for _, p := range Priorities {
		if prioPaths[string(p)] == "" {
			t.Errorf("priority %s has no glyph", p)
		}
	}
	// FR-18: every priority is its own shape.
	seen := map[string]Priority{}
	for _, p := range Priorities {
		if q, ok := seen[prioPaths[string(p)]]; ok {
			t.Errorf("%s and %s share a shape", p, q)
		}
		seen[prioPaths[string(p)]] = p
	}
}

// slug names sb-tabs' slots as sb-tabs does.
func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Comments": "comments", "Work log": "work-log", " History (3) ": "history-3", "Übersicht": "übersicht"} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPresenceOrder(t *testing.T) {
	var people []PresenceEntry
	for i := range 11 {
		people = append(people, PresenceEntry{Person: Person{Seed: string(rune('a' + i)), Name: string(rune('A' + i))}, You: i == 6})
	}
	shown, rest := presence(PresenceProps{People: people})
	if len(shown) != 8 || len(rest) != 3 || !shown[0].You { // MP-02.1
		t.Fatalf("shown %d, rest %d, first %+v", len(shown), len(rest), shown[0])
	}
}
