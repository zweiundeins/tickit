// Package demo runs tickit as a public demo: the nightly reset and the
// notice that warns of it (DM-03, DM-10).
package demo

import (
	"fmt"
	"time"
	_ "time/tzdata" // Europe/Zurich, wherever the binary runs
)

// Topic is touched by everything the demo view shows: a reset to come or
// under way.
const Topic = "demo"

// zurich is where the reset's hour is kept (Q21).
var zurich = func() *time.Location {
	l, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		panic(err)
	}
	return l
}()

// ResetHour is the hour of the nightly reset, in Europe/Zurich (DM-03.1).
const ResetHour = 4

// NextReset is the first nightly reset after now.
func NextReset(now time.Time) time.Time {
	z := now.In(zurich)
	next := time.Date(z.Year(), z.Month(), z.Day(), ResetHour, 0, 0, 0, zurich)
	if !next.After(z) {
		next = time.Date(z.Year(), z.Month(), z.Day()+1, ResetHour, 0, 0, 0, zurich)
	}
	return next
}

// Notice is the demo notice of every page at now (DM-10.1).
func Notice(now time.Time) string {
	return "Public demo. Everything you write is visible to everyone and deleted at the nightly reset (" + Until(now) + "). Do not enter personal data."
}

// Until says when the next reset comes: "next in 5 h", or in minutes
// within the last hour.
func Until(now time.Time) string {
	d := NextReset(now).Sub(now)
	if d < time.Hour {
		return fmt.Sprintf("next in %d min", int((d+time.Minute-1)/time.Minute))
	}
	return fmt.Sprintf("next in %d h", int((d+30*time.Minute)/time.Hour))
}
