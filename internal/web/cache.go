package web

import (
	"context"
	"strings"
	"sync"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/tracker"
)

// projectsCache keeps the projects of the home page with their open
// counts (tracker.Projects), by the projects a person reads, as of a
// commit's number: good while no commit touched projects, nor the project
// or the board of one of them, since (lith's Hub.Version: every change to
// an issue touches its project's board). The home page's views render on
// every change to an issue, for every person, all in the same tick: one
// of them reads the list, and the others wait for it.
type projectsCache struct {
	hub *live.Hub
	mu  sync.Mutex
	m   map[string]*cachedProjects
}

type cachedProjects struct {
	seq  uint64
	done chan struct{}     // closed once list and err are set
	list []tracker.Project // shared: never changed
	err  error
}

// maxCachedProjects bounds the lists kept, one per set of readable projects.
const maxCachedProjects = 256

func newProjectsCache(hub *live.Hub) *projectsCache {
	return &projectsCache{hub: hub, m: map[string]*cachedProjects{}}
}

// get returns tracker.Projects of readable, read in q, or taken from the
// cache or from the read under way. seq is live.Seq(ctx) in a Render, and
// in a GET hub.Seq() read before its transaction began.
func (c *projectsCache) get(ctx context.Context, q sqlite.Querier, readable []string, seq uint64) ([]tracker.Project, error) {
	key := strings.Join(readable, ",")
	topics := make([]string, 0, 1+2*len(readable))
	topics = append(topics, tracker.ProjectsTopic)
	for _, id := range readable {
		topics = append(topics, tracker.ProjectTopic(id), tracker.BoardTopic(id))
	}
	c.mu.Lock()
	if e, ok := c.m[key]; ok && c.hub.Version(topics...) <= e.seq {
		c.mu.Unlock()
		select {
		case <-e.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if e.err == nil {
			return e.list, nil
		}
		return tracker.Projects(ctx, q, readable)
	}
	if len(c.m) >= maxCachedProjects {
		clear(c.m)
	}
	e := &cachedProjects{seq: seq, done: make(chan struct{})}
	c.m[key] = e
	c.mu.Unlock()
	e.list, e.err = tracker.Projects(ctx, q, readable)
	close(e.done)
	if e.err != nil {
		c.mu.Lock()
		if c.m[key] == e {
			delete(c.m, key)
		}
		c.mu.Unlock()
	}
	return e.list, e.err
}
