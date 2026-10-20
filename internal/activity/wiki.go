package activity

import (
	"context"
	"encoding/json"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/internal/wiki"
)

// The wiki's part of the feeds: pages created, edited and commented on.
// An edit is a version (page.version_cut), not a commit: a minute of
// typing is one entry, and the log's commits never fill a feed's window.

func init() {
	addSource(wiki.PageStream.Kind(), []string{wiki.PageCreatedType.Name(), wiki.VersionCutType.Name()}, pageEntries)
	commentSubjects[tracker.SubjectPage] = pageSubjects
	subjectOfs[wiki.PageStream.Kind()] = func(ev events.Raw) (string, string) {
		id, _ := ev.ID.(string)
		return id, tracker.SubjectPage
	}
	mineSubjectsBy[tracker.SubjectPage] = pageSubjects
}

func pageEntries(ctx context.Context, q sqlite.Querier, evs []events.Raw) ([]Entry, error) {
	var out []Entry
	var ids []string
	for _, ev := range evs {
		e, err := events.Decode(wiki.Store(), wiki.PageStream, ev.Type, ev.Data)
		if err != nil {
			return nil, err
		}
		id, _ := ev.ID.(string)
		entry := Entry{Position: ev.Position, At: ev.Meta.At, Since: ev.Meta.At, Actor: ev.Meta.Actor, Subject: Subject{Kind: tracker.SubjectPage, ID: id}, Edits: 1}
		switch e := e.(type) {
		case wiki.PageCreated:
			entry.Action = PageCreated
		case wiki.VersionCut:
			if e.Reason == "created" {
				continue
			}
			entry.Action = PageEdited
		default:
			continue
		}
		out = append(out, entry)
		ids = append(ids, id)
	}
	subjects, err := pageSubjects(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Subject = subjects[out[i].Subject.ID] // the zero Subject for a deleted page
	}
	return out, nil
}

// pageSubjects names the current pages, as they are now.
func pageSubjects(ctx context.Context, q sqlite.Querier, ids []string) (map[string]Subject, error) {
	out := map[string]Subject{}
	if len(ids) == 0 {
		return out, nil
	}
	list, _ := json.Marshal(ids)
	rows, err := q.QueryContext(ctx, `SELECT p.id, p.space_id, p.title, s.key FROM pages p JOIN spaces s ON s.id = p.space_id
		WHERE p.id IN (SELECT value FROM json_each(?)) AND p.state = 'current'`, string(list))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s Subject
		var space, key string
		if err := rows.Scan(&s.ID, &space, &s.Label, &key); err != nil {
			return nil, err
		}
		s.Kind, s.Href, s.Scope = tracker.SubjectPage, wiki.PageURL(key, s.ID, s.Label), access.SpaceScope(space)
		out[s.ID] = s
	}
	return out, rows.Err()
}
