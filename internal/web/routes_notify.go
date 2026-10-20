package web

import (
	"context"
	"database/sql"

	"github.com/zweiundeins/lith/live"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/views"
)

// routesNotify registers notifications: the panel, marking read and
// dismissing (FR-110 to FR-117).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesNotify() {}

// personView is what concerns one person in every tab of theirs: for now
// their name in the header; the bell and its unread count belong here too
// (FR-110). Commands that change it touch people.PersonTopic(id).
func (s *Server) personView(p people.Person) live.View {
	key := people.PersonTopic(p.ID)
	return live.View{
		Key:    key,
		Topics: []string{key},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			me, err := people.Get(ctx, querier(ctx, tx), p.ID)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.PersonName(me))
		},
	}
}
