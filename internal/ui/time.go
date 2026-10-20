package ui

import (
	"strconv"
	"time"
)

// Ago says when t was, seen from now, in English (NFR-51): "just now",
// "5 minutes ago", "yesterday", "3 days ago", then the date ("Oct 2", with
// the year when it is not now's). Future times read "in 5 minutes",
// "tomorrow". Days count calendar dates in t's location.
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	future := d < 0
	if future {
		d = -d
	}
	unit := func(n int, one string) string {
		s := strconv.Itoa(n) + " " + one
		if n != 1 {
			s += "s"
		}
		if future {
			return "in " + s
		}
		return s + " ago"
	}
	switch {
	case d < 45*time.Second:
		if future {
			return "in a moment"
		}
		return "just now"
	case d < 90*time.Second:
		return unit(1, "minute")
	case d < 45*time.Minute:
		return unit(int((d+30*time.Second)/time.Minute), "minute")
	case d < 90*time.Minute:
		return unit(1, "hour")
	case d < 20*time.Hour:
		return unit(int((d+30*time.Minute)/time.Hour), "hour")
	}
	days := calendarDays(t, now)
	if days < 0 {
		days = -days
	}
	switch {
	case days <= 1 && future:
		return "tomorrow"
	case days <= 1:
		return "yesterday"
	case days < 7:
		return unit(days, "day")
	}
	return Date(t, now)
}

// Date is a short date: "Oct 2", or "Oct 2, 2025" outside now's year.
func Date(t, now time.Time) string {
	if t.Year() == now.In(t.Location()).Year() {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}

// Absolute is the full date and time with its zone, for a title: "Friday,
// 16 October 2026, 13:24 CEST".
func Absolute(t time.Time) string {
	return t.Format("Monday, 2 January 2006, 15:04 MST")
}

// calendarDays counts the dates from t to now in t's location.
func calendarDays(t, now time.Time) int {
	now = now.In(t.Location())
	a := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}
