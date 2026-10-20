package web

import (
	"context"
	"net/http"
	"time"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/views"
)

// Presence rows (MP-02): the people who watch a view, one face each, and
// whether they are away (MP-02.4, people.Away). A row reads who is away
// for the faces it shows and declares their away topics (live.Depends):
// it renders again when one of them turns away or comes back, and for no
// one else.

// present are the people who watch the view with key, in the order they
// came, with their tab counts and who of them is away at now.
func (s *Server) present(ctx context.Context, q sqlite.Querier, key string, now time.Time) ([]views.Present, error) {
	var ids []string
	count := map[string]int{}
	for _, id := range s.hub.Watchers(key) {
		p := personOf(id)
		if count[p] == 0 && len(ids) < MaxPresent {
			ids = append(ids, p)
		}
		count[p]++
	}
	found, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return nil, err
	}
	away, err := people.Away(ctx, q, now, ids...)
	if err != nil {
		return nil, err
	}
	topics := make([]string, len(ids))
	list := make([]views.Present, 0, len(ids))
	for i, id := range ids {
		topics[i] = people.AwayTopic(id)
		list = append(list, views.Present{Person: found[id], Tabs: count[id], Away: away[id]})
	}
	live.Depends(ctx, topics...)
	return list, nil
}

type inputRequest struct{ Payload }

// input is POST /cmd/input {tab, op}: the person gave input in the tab
// (MP-02.4). The page sends it as it opens, then at most once a minute
// while there is input, without waiting; it has no outcome. A blocked
// guest's input counts for nothing (403).
func (s *Server) input(w http.ResponseWriter, r *http.Request) {
	var req inputRequest
	a, ok := s.decode(w, r, classCaret, &req)
	if !ok || s.paused(w, r, a.By.Actor) || !s.connected(w, r, a.By.Actor, a.Tab) {
		return
	}
	if err := s.w.Send(people.Input{Person: a.By.Actor.ID, At: a.By.At}); err != nil {
		s.failAction(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PresenceJobs runs until ctx ends: every few seconds it tells the
// presence rows of the people who turned away since (TurnAway), and
// every hour it forgets input older than a day. cmd/tickit runs it.
func (s *Server) PresenceJobs(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	last, swept := time.Now(), time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			s.TurnAway(ctx, last, now)
			last = now
			if now.Sub(swept) >= time.Hour {
				swept = now
				if err := s.w.Do(ctx, people.SweepInput{At: now}); err != nil && ctx.Err() == nil {
					s.log.Warn("presence sweep", "err", err)
				}
			}
		}
	}
}

// TurnAway touches the away topics of the people who turned away after
// from and by to. Nothing in the database changes: time passed.
func (s *Server) TurnAway(ctx context.Context, from, to time.Time) {
	ids, err := people.TurnedAway(ctx, s.db.R, from, to)
	if err != nil {
		s.log.Warn("who turned away", "err", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	topics := make([]string, len(ids))
	for i, id := range ids {
		topics[i] = people.AwayTopic(id)
	}
	s.hub.Changed(topics)
}
