package web

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/zweiundeins/lith/live"

	"github.com/zweiundeins/tickit/internal/views"
)

// Renders per view (MP-11.4, NFR-76): the hub counts the renders of each
// view watched now (live.Stats.RendersByKey), from its first watcher on.
// tickit_view_renders_total adds them up by the view's kind, the key
// before its first colon, so the metric's labels stay few however many
// lists and boards people open; the admin statistics show the keys.

// MaxRenderKinds bounds the kinds the metric labels; the rest count as
// "other".
const MaxRenderKinds = 64

// TopViews is how many view keys the admin statistics list.
const TopViews = 20

// renderCounts keeps the renders of the views by kind, as counters that
// only grow while views come and go.
type renderCounts struct {
	hub  *live.Hub
	desc *prometheus.Desc

	mu    sync.Mutex
	last  map[string]int64 // renders of each key at the last sample
	kinds map[string]int64 // renders by kind, since the start
}

func newRenderCounts(hub *live.Hub) *renderCounts {
	return &renderCounts{
		hub: hub,
		desc: prometheus.NewDesc(namespace+"_view_renders_total",
			"Views rendered, by kind (the view key before its first colon).", []string{"kind"}, nil),
		last: map[string]int64{}, kinds: map[string]int64{},
	}
}

// kindOf is the kind of a view key: "list" of "list:3f2a...".
func kindOf(key string) string {
	k, _, _ := strings.Cut(key, ":")
	if !kindPattern.MatchString(k) {
		return "other"
	}
	return k
}

// sample adds the renders since the last sample. A view watched again
// since starts its count from 0; renders of a view that went between two
// samples after the first are lost, so the hub is sampled every few
// seconds (run).
func (c *renderCounts) sample() {
	now := c.hub.Stats().RendersByKey
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, n := range now {
		d := n - c.last[key]
		if d < 0 {
			d = n
		}
		if d == 0 {
			continue
		}
		kind := kindOf(key)
		if _, ok := c.kinds[kind]; !ok && len(c.kinds) >= MaxRenderKinds {
			kind = "other"
		}
		c.kinds[kind] += d
	}
	c.last = now
}

// run samples the hub every 5 s until it stops.
func (c *renderCounts) run() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.hub.Done():
			return
		case <-t.C:
			c.sample()
		}
	}
}

func (c *renderCounts) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *renderCounts) Collect(ch chan<- prometheus.Metric) {
	c.sample()
	c.mu.Lock()
	defer c.mu.Unlock()
	for kind, n := range c.kinds {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, float64(n), kind)
	}
}

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
	switch kindOf(key) {
	case "tab", "prefs", "editor", "revoked":
		h := sha256.Sum256([]byte(key))
		return kindOf(key) + ":#" + hex.EncodeToString(h[:4])
	}
	return key
}
