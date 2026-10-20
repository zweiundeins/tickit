package tracker

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/zweiundeins/lith/writer"
)

// Conflict refuses a change to a field that someone else changed after
// the version the page showed (MP-08.3), or a board move of a card that
// someone else moved first (MP-04.3). It is a *writer.Refusal for the
// field (tabs.Act records Msg and Field), and its OutcomeData is the
// JSON the page renders the message and "Use Critical anyway" from:
//
//	{"kind":"conflict","field":"priority","label":"priority","by":"g-…",
//	 "name":"Sunny Otter","at":1760600000000,"theirs":"High","yours":"Critical",
//	 "version":7}
//
// To use the refused value anyway, the page sends the same change again
// with Seen set to version.
type Conflict struct {
	Kind           string `json:"kind"`             // "conflict"
	Field          string `json:"field"`            // the field the page shows the message at: FieldPriority, FieldMove
	Label          string `json:"label"`            // the field as the message names it: "priority", "due date"
	By             string `json:"by"`               // who changed it
	Name           string `json:"name"`             // their display name
	At             int64  `json:"at"`               // when, in unix ms
	Theirs         string `json:"theirs"`           // the value they set, as the page shows it
	Yours          string `json:"yours"`            // the value refused
	Version        int64  `json:"version"`          // the issue's version now: resend with Seen = Version
	Key            string `json:"key,omitempty"`    // a move's issue key
	Status         string `json:"status,omitempty"` // a move's status now, by id
	writer.Refusal `json:"-"`
}

func (c *Conflict) Error() string   { return c.Msg }
func (c *Conflict) Unwrap() error   { return &c.Refusal }
func (c *Conflict) OnField() string { return c.Field }

// OutcomeData is the conflict as the tab's outcome carries it.
func (c *Conflict) OutcomeData() json.RawMessage {
	b, err := json.Marshal(c)
	if err != nil {
		return nil
	}
	return b
}

// conflictErr is a decider's conflict, before the command adds the names
// and values a person reads.
type conflictErr struct {
	Field   string
	Version int64 // the version that changed it last
	Yours   string
}

func (e *conflictErr) Error() string {
	return fmt.Sprintf("tracker: %s changed at version %d", e.Field, e.Version)
}

// labels are the fields as messages name them.
var labels = map[string]string{
	FieldSummary:     "summary",
	FieldDescription: "description",
	FieldType:        "type",
	FieldStatus:      "status",
	FieldPriority:    "priority",
	FieldAssignee:    "assignee",
	FieldParent:      "parent",
	FieldDue:         "due date",
	FieldPoints:      "story points",
	FieldEstimate:    "original estimate",
	FieldRemaining:   "remaining estimate",
	FieldSprint:      "sprint",
}

// FieldLabel is a field as messages and the history name it: "due date".
func FieldLabel(field string) string {
	if l, ok := labels[field]; ok {
		return l
	}
	return field
}

// conflictMessage is MP-08.3's sentence.
func conflictMessage(field, name, theirs, yours string, ago string) string {
	switch field {
	case FieldDescription:
		return fmt.Sprintf("%s changed the description %s. Your changes were not saved.", name, ago)
	case FieldSummary:
		return fmt.Sprintf("%s changed the summary to %q %s. Your %q was not saved.", name, theirs, ago, yours)
	}
	return fmt.Sprintf("%s changed the %s to %s %s. Your %s was not saved.", name, FieldLabel(field), theirs, ago, yours)
}

// Ago says how long ago then was, seen from now: "just now", "5 seconds
// ago", "a minute ago", "3 hours ago", "2 days ago".
func Ago(then, now time.Time) string {
	d := now.Sub(then)
	unit := func(n int64, one, many string) string {
		if n == 1 {
			return one + " ago"
		}
		return strconv.FormatInt(n, 10) + " " + many + " ago"
	}
	switch {
	case d < time.Second:
		return "just now"
	case d < time.Minute:
		return unit(int64(d/time.Second), "1 second", "seconds")
	case d < time.Hour:
		return unit(int64(d/time.Minute), "a minute", "minutes")
	case d < 24*time.Hour:
		return unit(int64(d/time.Hour), "an hour", "hours")
	}
	return unit(int64(d/(24*time.Hour)), "a day", "days")
}

// Minutes shows an estimate: "2h 30m", "45m", "3h".
func Minutes(m int) string {
	h, r := m/60, m%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", r)
	case r == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, r)
}

// Date shows a date YYYY-MM-DD as "21 October 2026".
func Date(d string) string {
	t, err := time.Parse(time.DateOnly, d)
	if err != nil {
		return d
	}
	return t.Format("2 January 2006")
}

func intString(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}
