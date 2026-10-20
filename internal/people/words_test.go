package people_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/dbtest"
	"github.com/zweiundeins/tickit/internal/demo/words"
	"github.com/zweiundeins/tickit/internal/people"
)

// DM-06.4, FR-121.1: a display name with a word of the list is refused
// next to the name field.
func TestRenameWordList(t *testing.T) {
	e := dbtest.Open(t)
	g := people.GuestOf("session-words")
	e.Do(t, writer.Func(func(tx *writer.Tx) error { return people.Ensure(tx, g, now) }))
	err := e.W.Do(context.Background(), people.Rename{Person: g.ID, Name: "Cheap V1agra"})
	var refusal *writer.Refusal
	var field interface{ OnField() string }
	if !errors.As(err, &refusal) || refusal.Msg != words.Refusal || !errors.As(err, &field) || field.OnField() != "name" {
		t.Errorf("a listed word: %v", err)
	}
	e.Do(t, people.Rename{Person: g.ID, Name: "Grape Specialist"})
}
