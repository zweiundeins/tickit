package web

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/zweiundeins/tickit/internal/views"
)

// Renders per view (MP-11.4, NFR-76): the hub counts the renders of each
// view watched now (live.Stats.RendersByKey), from its first watcher on.
// The metrics sum them by kind (tickit_view_renders{kind}, metrics.go);
// the admin statistics list the keys rendered most.

// TopViews is how many view keys the admin statistics list.
const TopViews = 20

// topViews are the n view keys rendered most, most first. Keys that hold a
// tab id show as their kind and a hash: whoever has a tab id can pace or
// end that tab's stream.
func (s *Server) topViews(n int) []views.ViewRenders {
	byKey := s.hub.Stats().RendersByKey
	out := make([]views.ViewRenders, 0, len(byKey))
	for k, r := range byKey {
		out = append(out, views.ViewRenders{Key: k, Renders: r})
	}
	slices.SortFunc(out, func(a, b views.ViewRenders) int {
		return cmp.Or(cmp.Compare(b.Renders, a.Renders), strings.Compare(a.Key, b.Key))
	})
	out = out[:min(n, len(out))]
	for i := range out {
		out[i].Key = shownKey(out[i].Key)
	}
	return out
}

// shownKey is a view key as the admin statistics show it.
func shownKey(key string) string {
	switch k := keyKind(key); k {
	case "tab", "prefs", "editor", "revoked":
		h := sha256.Sum256([]byte(key))
		return k + ":#" + hex.EncodeToString(h[:4])
	}
	return key
}
