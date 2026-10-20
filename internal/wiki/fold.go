package wiki

import (
	"database/sql"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"
)

// The fold (NFR-43): the commits of a page's document up to a version are
// folded into a snapshot FoldAfter after the version is cut, at most
// FoldBatch events a command.
const (
	FoldAfter = 15 * time.Minute
	FoldBatch = 200
)

// Fold folds the commits that versions hold into snapshots, with lith's
// events.Compact: each document stream that loses commits keeps the last
// of them, rewritten as a pagedoc.snapshot of the document then. Compact
// cuts the kind at one position, the first event stored after a time;
// Fold picks the time so that no commit after its page's last version is
// folded: FoldAfter ago, or the time of the oldest commit not in a
// version yet, whichever is earlier. A page that someone edits without
// pause holds the fold back by at most CutInterval, when its next
// version is cut. Rebuilds, versions and as-of reads still work: versions
// hold their documents in page.version_cut, never folded, and a document
// stream starts at its snapshot.
//
// It reports how many events it deleted; the job sends it again while it
// deletes some.
type Fold struct {
	At time.Time

	Deleted int64
	Took    time.Duration // in the writer: NFR-43.3 wants at most 5 ms
}

func (c *Fold) Apply(tx *writer.Tx) error {
	start := time.Now()
	defer func() { c.Took = time.Since(start) }()
	since := c.At.Add(-FoldAfter)
	var oldest sql.NullInt64
	err := tx.QueryRow(`SELECT min(e.at) FROM (SELECT page_id, max(upto) upto FROM page_versions GROUP BY page_id) v
		JOIN lith_events e ON e.stream = 'pagedoc' AND e.stream_id = v.page_id AND e.version = v.upto + 1`).Scan(&oldest)
	if err != nil {
		return err
	}
	if oldest.Valid && time.UnixMilli(oldest.Int64).Before(since) {
		since = time.UnixMilli(oldest.Int64)
	}
	n, err := events.Compact(Store(), tx, DocSnapshotType, events.Retention{Since: since, Limit: FoldBatch}, peekDocDecider,
		func(s DocState) DocSnapshot { return DocSnapshot{Epoch: s.Epoch, Doc: marshalDoc(s.doc)} })
	c.Deleted = n
	return err
}
