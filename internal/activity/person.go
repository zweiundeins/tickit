package activity

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/access"
)

// By is what one person did lately, newest first, as a feed shows it:
// the entries of their own events whose scope keep accepts, up to limit
// (FR-67.2: a person's page). A person without a row never acted.
func By(ctx context.Context, q sqlite.Querier, person string, keep func(access.Scope) bool, limit int) ([]Entry, error) {
	var acted int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM people WHERE id = ?`, person).Scan(&acted); err != nil || acted == 0 {
		return nil, err
	}
	upto, err := head(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []Entry
	size, scanned := int64(window), int64(0)
	for upto > 0 && len(out) < limit && scanned < MaxScan {
		after := max(0, upto-size)
		byKind, err := sourceEventsBy(ctx, q, person, after, upto)
		if err != nil {
			return nil, err
		}
		var found []Entry
		for kind, evs := range byKind {
			es, err := sources[kind].entries(ctx, q, evs)
			if err != nil {
				return nil, err
			}
			found = append(found, es...)
		}
		slices.SortFunc(found, func(a, b Entry) int { return cmp.Compare(b.Position, a.Position) })
		for _, e := range found {
			if e.Subject.ID == "" || e.Actor != person || !keep(e.Subject.Scope) {
				continue
			}
			if e.Edits == 0 {
				e.Edits = 1
			}
			if e.Since.IsZero() {
				e.Since = e.At
			}
			if n := len(out); n > 0 && merges(out[n-1], e) {
				out[n-1].Edits += e.Edits
				out[n-1].Since = e.Since
				continue
			}
			if len(out) < limit {
				out = append(out, e)
			}
		}
		scanned += upto - after
		upto = after
		size = min(size*2, 8192)
	}
	return out, nil
}

// sourceEventsBy is sourceEvents of one actor: SQLite drops the rows of
// everyone else.
func sourceEventsBy(ctx context.Context, q sqlite.Querier, actor string, after, upto int64) (map[string][]events.Raw, error) {
	byKind := map[string][]events.Raw{}
	for kind, s := range sources {
		rows, err := q.QueryContext(ctx, `SELECT position, version, type, data, at, coalesce(correlation, ''), coalesce(causation, ''), stream_id
			FROM lith_events WHERE stream = ? AND position > ? AND position <= ? AND actor = ? AND type IN (SELECT value FROM json_each(?)) ORDER BY position`,
			kind, after, upto, actor, s.list)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			ev := events.Raw{Stream: kind, Meta: events.Meta{Actor: actor}}
			var at int64
			var data string
			if err := rows.Scan(&ev.Position, &ev.Version, &ev.Type, &data, &at, &ev.Correlation, &ev.Causation, &ev.ID); err != nil {
				rows.Close()
				return nil, err
			}
			ev.Data, ev.At = json.RawMessage(data), time.UnixMilli(at)
			byKind[kind] = append(byKind[kind], ev)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return byKind, nil
}
