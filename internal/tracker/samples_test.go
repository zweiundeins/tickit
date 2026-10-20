package tracker

import (
	"os"
	"testing"

	"github.com/zweiundeins/lith/events/eventstest"
)

// Every event type has a sample of its JSON as first stored; the samples
// never change, so a renamed field fails here and not in production.
func TestSamples(t *testing.T) {
	eventstest.Samples(t, Store(), ProjectStream, os.DirFS("testdata/project"))
	eventstest.Samples(t, Store(), IssueStream, os.DirFS("testdata/issue"))
	eventstest.Samples(t, Store(), CommentStream, os.DirFS("testdata/comment"))
	eventstest.Samples(t, Store(), DescDocStream, os.DirFS("testdata/issuedoc"))
}
