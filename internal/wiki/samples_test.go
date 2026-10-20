package wiki

import (
	"os"
	"testing"

	"github.com/zweiundeins/lith/events/eventstest"
)

// Every event type has a sample of its JSON as first stored; the samples
// never change, so a renamed field fails here and not in production.
func TestSamples(t *testing.T) {
	eventstest.Samples(t, Store(), SpaceStream, os.DirFS("testdata/space"))
	eventstest.Samples(t, Store(), PageStream, os.DirFS("testdata/page"))
	eventstest.Samples(t, Store(), PageDocStream, os.DirFS("testdata/pagedoc"))
}
