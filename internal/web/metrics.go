package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
)

// namespace prefixes tickit's own metrics, as lith's collector's
// (cmd/tickit): tickit_*.
const namespace = "tickit"

// metrics are tickit's own numbers (NFR-76.1), beside lith's collector.
// Areas count theirs here: the editor its batches and editors, moderation
// its actions, the demo its resets.
type metrics struct {
	limited      *prometheus.CounterVec // requests over a rate limit, by class
	refusals     *prometheus.CounterVec // refused actions, by route
	logins       *prometheus.CounterVec // logins, by outcome
	editBatches  prometheus.Counter     // batches of page edits applied (editor)
	editors      prometheus.Gauge       // the most editors of one page now (editor)
	moderations  *prometheus.CounterVec // moderation actions, by action (moderation)
	resets       *prometheus.CounterVec // demo resets, by outcome (demo)
	resetSeconds prometheus.Histogram   // how long a reset took (demo)
	guests       prometheus.GaugeFunc   // guests who acted in the last 24 hours
	folded       prometheus.Counter     // events the fold deleted (NFR-43)
	foldSeconds  prometheus.Histogram   // a fold command's time in the writer (NFR-43.3)
	opened       prometheus.Counter     // streams the hub took (NFR-76.2)
	closed       prometheus.Counter     // streams that ended after it took them (NFR-76.2)
}

func newMetrics(db *sqlite.DB) *metrics {
	m := &metrics{
		limited: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: namespace, Name: "rate_limited_total",
			Help: "Requests answered 429, by rate-limit class."}, []string{"class"}),
		refusals: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: namespace, Name: "refusals_total",
			Help: "Actions refused, with the reason in the tab, by route."}, []string{"route"}),
		logins: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: namespace, Name: "logins_total",
			Help: "Logins, by outcome."}, []string{"outcome"}),
		editBatches: prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "edit_batches_total",
			Help: "Batches of page edits applied."}),
		editors: prometheus.NewGauge(prometheus.GaugeOpts{Namespace: namespace, Name: "page_editors_max",
			Help: "The most people editing one page now."}),
		moderations: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: namespace, Name: "moderation_actions_total",
			Help: "Moderation actions, by action."}, []string{"action"}),
		resets: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: namespace, Name: "resets_total",
			Help: "Resets of the demo, by outcome: ok or failed."}, []string{"outcome"}),
		resetSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: namespace, Name: "reset_seconds",
			Help: "How long a reset of the demo took.", Buckets: []float64{1, 2, 5, 10, 20, 30, 60}}),
		folded: prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "folded_events_total",
			Help: "Page edits the fold folded into snapshots."}),
		foldSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: namespace, Name: "fold_seconds",
			Help: "How long one fold command held the writer.", Buckets: []float64{0.0005, 0.001, 0.002, 0.003, 0.004, 0.005, 0.0075, 0.01, 0.02, 0.05}}),
		opened: prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "streams_opened_total",
			Help: "Streams the hub took and started to send on."}),
		closed: prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "streams_closed_total",
			Help: "Streams that ended after the hub took them."}),
		guests: prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: namespace, Name: "guests_active",
			Help: "Guests who acted in the last 24 hours."}, func() float64 {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var n int
			db.R.QueryRowContext(ctx, `SELECT count(*) FROM people WHERE kind = 'guest' AND last_seen_at > ?`, time.Now().Add(-24*time.Hour).Unix()).Scan(&n)
			return float64(n)
		}),
	}
	// The alerts read both outcomes from 0 on (deploy/observability/rules-tickit.yaml).
	m.resets.WithLabelValues("ok")
	m.resets.WithLabelValues("failed")
	return m
}

// reset counts a reset of the demo and how long it took.
func (m *metrics) reset(ok bool, took time.Duration) {
	outcome := "ok"
	if !ok {
		outcome = "failed"
	}
	m.resets.WithLabelValues(outcome).Inc()
	m.resetSeconds.Observe(took.Seconds())
}

// Collectors are tickit's own metrics, for the registry of the metrics
// listener.
func (s *Server) Collectors() []prometheus.Collector {
	m := s.metrics
	return []prometheus.Collector{m.limited, m.refusals, m.logins, m.editBatches, m.editors, m.moderations, m.resets, m.resetSeconds, m.guests, m.folded, m.foldSeconds,
		m.opened, m.closed, newViewsCollector(s.hub)}
}

// viewsCollector reads the hub's renders by view key (MP-11.4, NFR-76.2)
// and sums them by the key's kind, the part before its first colon, so
// that the labels stay few whatever the ids.
type viewsCollector struct {
	hub              *live.Hub
	watched, renders *prometheus.Desc
}

func newViewsCollector(hub *live.Hub) *viewsCollector {
	return &viewsCollector{hub: hub,
		watched: prometheus.NewDesc(namespace+"_views_watched", "Views watched now, by kind.", []string{"kind"}, nil),
		renders: prometheus.NewDesc(namespace+"_view_renders", "Renders of the views watched now since each was first watched, by kind.", []string{"kind"}, nil),
	}
}

func (c *viewsCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.watched; ch <- c.renders }

func (c *viewsCollector) Collect(ch chan<- prometheus.Metric) {
	if c.hub == nil {
		return
	}
	watched, renders := map[string]int{}, map[string]int64{}
	for key, n := range c.hub.Stats().RendersByKey {
		k := keyKind(key)
		watched[k]++
		renders[k] += n
	}
	for k, n := range watched {
		ch <- prometheus.MustNewConstMetric(c.watched, prometheus.GaugeValue, float64(n), k)
		ch <- prometheus.MustNewConstMetric(c.renders, prometheus.GaugeValue, float64(renders[k]), k)
	}
}

// keyKind is the kind of a view's key: "issue" of "issue:<id>", "other"
// for a key that does not start with a short lower-case word.
func keyKind(key string) string {
	k, _, _ := strings.Cut(key, ":")
	if k == "" || len(k) > 24 || strings.Trim(k, "abcdefghijklmnopqrstuvwxyz-") != "" {
		return "other"
	}
	return k
}

// countedStream is a stream's ResponseWriter that counts the stream as
// opened when the hub answers 200, and closed once it ends.
type countedStream struct {
	http.ResponseWriter
	m      *metrics
	sent   bool // the status went out, by WriteHeader or implied by a Write or Flush
	opened bool
}

func (w *countedStream) WriteHeader(code int) {
	if !w.sent && code >= 200 {
		w.sent = true
		w.count(code)
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *countedStream) Write(p []byte) (int, error) {
	w.implied()
	return w.ResponseWriter.Write(p)
}

func (w *countedStream) Flush() { w.FlushError() }

func (w *countedStream) FlushError() error {
	w.implied()
	return http.NewResponseController(w.ResponseWriter).Flush()
}

// implied notes the 200 that net/http sends for a Write or Flush before
// any WriteHeader, without writing it again.
func (w *countedStream) implied() {
	if !w.sent {
		w.sent = true
		w.count(http.StatusOK)
	}
}

func (w *countedStream) count(code int) {
	if code == http.StatusOK {
		w.opened = true
		w.m.opened.Inc()
	}
}

func (w *countedStream) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *countedStream) done() {
	if w.opened {
		w.m.closed.Inc()
	}
}
