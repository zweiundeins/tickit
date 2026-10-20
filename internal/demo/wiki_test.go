package demo_test

import (
	"github.com/zweiundeins/lith/events"

	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// One store of the tracker's and the wiki's parts, as cmd/tickit binds
// them: page comments' threads and the wiki's touches run in the
// tracker's commands.
func init() {
	st := events.Must(events.New(append(tracker.Parts(), wiki.Parts()...)...))
	tracker.Bind(st)
	wiki.Bind(st)
}
