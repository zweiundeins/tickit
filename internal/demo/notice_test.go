package demo_test

import (
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/demo"
)

// DM-10.1, Q21: the notice counts down to 04:00 Europe/Zurich, in summer
// and winter time.
func TestNotice(t *testing.T) {
	zurich, _ := time.LoadLocation("Europe/Zurich")
	for _, c := range []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 10, 16, 23, 0, 0, 0, zurich), "next in 5 h"},
		{time.Date(2026, 10, 16, 22, 50, 0, 0, zurich), "next in 5 h"},
		{time.Date(2026, 10, 17, 3, 20, 0, 0, zurich), "next in 40 min"},
		{time.Date(2026, 10, 17, 4, 0, 0, 0, zurich), "next in 24 h"},
		{time.Date(2026, 1, 10, 2, 0, 0, 0, time.UTC), "next in 1 h"}, // 03:00 in Zurich
		{time.Date(2026, 7, 10, 1, 59, 30, 0, time.UTC), "next in 1 min"},
	} {
		if got := demo.Until(c.at); got != c.want {
			t.Errorf("at %s: %q, want %q", c.at, got, c.want)
		}
	}
	want := "Public demo. Everything you write is visible to everyone and deleted at the nightly reset (next in 5 h). Do not enter personal data."
	if got := demo.Notice(time.Date(2026, 10, 16, 23, 0, 0, 0, zurich)); got != want {
		t.Errorf("the notice: %q", got)
	}
}
