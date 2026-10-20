package demo

import (
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/google/uuid"
)

// The seed's ids are the same at every run, so that an address a visitor
// kept, /issues/WEB-12 or a page's, names the same thing after a reset
// (DM-02.4). They are UUIDv7s whose time is a fixed day plus the thing's
// place in the seed, and whose random bits come from a hash of its key:
// time-ordered like every other id (lith's rule 10), and older than any id
// a visitor's action makes. A page keeps its id across resets, but its
// stream starts anew; Epoch tells an editor that it did (MP-13.2).

// seedEpoch is the time the seed's ids start from.
var seedEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Each kind of id has its own range of times, a day apart.
const (
	kindProject = iota
	kindIssue
	kindComment
	kindSpace
	kindPage
	kindPageComment
	kindTemplate
	kindImage
)

// stableID is the id of the n-th thing of a kind with key.
func stableID(kind, n int, key string) string {
	var u uuid.UUID
	ms := uint64(seedEpoch.Add(time.Duration(kind)*24*time.Hour).UnixMilli()) + uint64(n)
	binary.BigEndian.PutUint64(u[:8], ms<<16)
	h := sha256.Sum256([]byte(key))
	copy(u[6:], h[:10])
	u[6] = u[6]&0x0f | 0x70 // version 7
	u[8] = u[8]&0x3f | 0x80 // RFC 9562 variant
	return u.String()
}
