package tracker

import (
	"math/rand/v2"
	"slices"
	"testing"
)

func TestRankBetween(t *testing.T) {
	for _, c := range []struct{ a, b, want string }{
		{"", "", "a0"},
		{"", "a0", "Zz"},
		{"a0", "", "a1"},
		{"a0", "a1", "a0V"},
		{"a1", "a2", "a1V"},
		{"a0V", "a1", "a0l"},
		{"Zz", "a0", "ZzV"},
		{"Zz", "a01", "a0"},
		{"", "Zz", "Zy"},
		{"a0", "a0V", "a0G"},
		{"az", "", "b00"},
		{"a0", "b00", "a1"},
	} {
		got, err := RankBetween(c.a, c.b)
		if err != nil || got != c.want {
			t.Errorf("RankBetween(%q, %q) = %q, %v; want %q", c.a, c.b, got, err, c.want)
		}
		if !(c.a == "" || c.a < got) || !(c.b == "" || got < c.b) || !ValidRank(got) {
			t.Errorf("RankBetween(%q, %q) = %q is not between", c.a, c.b, got)
		}
	}
	for _, c := range [][2]string{{"a1", "a0"}, {"a0", "a0"}, {"x", ""}, {"a00", ""}, {"", "a0!"}} {
		if got, err := RankBetween(c[0], c[1]); err == nil {
			t.Errorf("RankBetween(%q, %q) = %q, want an error", c[0], c[1], got)
		}
	}
}

// FR-41.1: a move between two cards never renumbers the others, and
// ranks stay short when issues are appended.
func TestRanksStayOrdered(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var ranks []string
	for range 2000 {
		i := r.IntN(len(ranks) + 1)
		lo, hi := "", ""
		if i > 0 {
			lo = ranks[i-1]
		}
		if i < len(ranks) {
			hi = ranks[i]
		}
		k, err := RankBetween(lo, hi)
		if err != nil {
			t.Fatalf("between %q and %q: %v", lo, hi, err)
		}
		ranks = slices.Insert(ranks, i, k)
	}
	if !slices.IsSorted(ranks) || len(slices.Compact(slices.Clone(ranks))) != len(ranks) {
		t.Fatal("the ranks are out of order or repeat")
	}
	last := ""
	for range 10000 {
		k, err := RankBetween(last, "")
		if err != nil {
			t.Fatal(err)
		}
		last = k
	}
	if len(last) > 5 {
		t.Errorf("after 10,000 appends the rank is %q", last)
	}
}
