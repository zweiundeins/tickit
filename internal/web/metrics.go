package web

import (
	"context"
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
	renders      *renderCounts          // renders by view kind (MP-11.4, renders.go)
}

func newMetrics(db *sqlite.DB, hub *live.Hub) *metrics {
	m := &metrics{
		renders: newRenderCounts(hub),
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
	return []prometheus.Collector{m.limited, m.refusals, m.logins, m.editBatches, m.editors, m.moderations, m.resets, m.resetSeconds, m.guests, m.folded, m.foldSeconds, m.renders}
}
