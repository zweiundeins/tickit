package web

import (
	"context"
	"strings"
	"sync"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/mbolli/tickit/internal/tracker"
)

// projectsCache keeps the projects of the home page with their open
// counts (tracker.Projects), by the projects a person reads, as of a
// commit's number: good while no commit touched projects, nor the project
// or the board of one of them, since (lith's Hub.Version: every change to
// an issue touches its project's board). The home page's views render on
// every change to an issue, for every person, and the counts go over
// every open issue of each project.
type projectsCache struct {
	hub *live.Hub
	mu  sync.Mutex
	m   map[string]cachedProjects
}

type cachedProjects struct {
	list []tracker.Project // shared: never changed
	seq  uint64
}

// maxCachedProjects bounds the lists kept, one per set of readable projects.
const maxCachedProjects = 256

func newProjectsCache(hub *live.Hub) *projectsCache {
	return &projectsCache{hub: hub, m: map[string]cachedProjects{}}
}

// get returns tracker.Projects of readable, read in q or taken from the
// cache. seq is live.Seq(ctx) in a Render, and in a GET hub.Seq() read
// before its transaction began.
func (c *projectsCache) get(ctx context.Context, q sqlite.Querier, readable []string, seq uint64) ([]tracker.Project, error) {
	key := strings.Join(readable, ",")
	topics := make([]string, 0, 1+2*len(readable))
	topics = append(topics, tracker.ProjectsTopic)
	for _, id := range readable {
		topics = append(topics, tracker.ProjectTopic(id), tracker.BoardTopic(id))
	}
	c.mu.Lock()
	e, ok := c.m[key]
	c.mu.Unlock()
	if ok && c.hub.Version(topics...) <= e.seq {
		return e.list, nil
	}
	list, err := tracker.Projects(ctx, q, readable)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= maxCachedProjects {
		clear(c.m)
	}
	if old, ok := c.m[key]; !ok || old.seq < seq {
		c.m[key] = cachedProjects{list: list, seq: seq}
	}
	return list, nil
}
