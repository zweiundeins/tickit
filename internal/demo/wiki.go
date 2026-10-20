package demo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo/seed"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// The wiki's part of the demo, in one file as notify's and activity's
// are: the seed's spaces and pages, pages as something moderators hide
// and delete, the daily cap of new pages.
func init() {
	RegisterPageSeeder(wikiSeeder{})
	CountCreations(func(cmd writer.Command) string {
		if _, ok := cmd.(wiki.CreatePage); ok {
			return CapPage
		}
		return ""
	})
	RegisterModerated(Moderated{
		Kind: "page", Name: "Page",
		// A hidden page is in its space's trash: it leaves every open page
		// and tree at once, and its editors stop ("gone").
		Hide: func(by tabs.By, id string, hidden bool, reason string) writer.Command {
			if hidden {
				return wiki.TrashPage{By: by, Page: id}
			}
			return wiki.RestorePage{By: by, Page: id}
		},
		Delete: func(by tabs.By, id string) writer.Command {
			return writer.Func(func(tx *writer.Tx) error {
				if err := (wiki.TrashPage{By: by, Page: id}).Apply(tx); err != nil {
					return err
				}
				return wiki.DeletePage{By: by, Page: id}.Apply(tx)
			})
		},
		Label: func(tx *writer.Tx, id string) (string, error) {
			var title string
			err := tx.QueryRow(`SELECT title FROM pages WHERE id = ?`, id).Scan(&title)
			if errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("page %s: %w", id, access.ErrNotFound)
			}
			return title, err
		},
		Newest: newestPages,
	})
}

// newestPages are the pages visitors created, newest first.
func newestPages(ctx context.Context, q sqlite.Querier, limit int) ([]Target, error) {
	rows, err := q.QueryContext(ctx, `SELECT p.id, p.title, s.key, p.space_id, p.created_by, p.created_at, p.state = 'trashed'
		FROM pages p JOIN spaces s ON s.id = p.space_id
		WHERE p.state != 'deleted' AND p.created_by NOT LIKE 's-%' ORDER BY p.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		var t Target
		var key, space string
		var at int64
		if err := rows.Scan(&t.ID, &t.Label, &key, &space, &t.Author, &at, &t.Hidden); err != nil {
			return nil, err
		}
		t.Kind, t.Href, t.At, t.Scope, t.Context = "page", wiki.PageURL(key, t.ID, t.Label), time.UnixMilli(at), access.SpaceScope(space), "in "+key
		out = append(out, t)
	}
	return out, rows.Err()
}

type wikiSeeder struct{}

// PageSteps are the seed's spaces, their pages with every version (oldest
// first, by their authors, at their times), the pages' labels and
// comments (DM-02.2). Templates need no steps: they are the seed's files
// (Templates).
func (wikiSeeder) PageSteps(r *Run) ([]Step, error) {
	c := r.Content
	var steps []Step
	add := func(at time.Time, phase int, name string, cmd writer.Command) {
		steps = append(steps, Step{At: at, Phase: phase, Name: name, Cmd: cmd})
	}
	for _, sp := range c.Spaces {
		pages := slices.DeleteFunc(slices.Clone(c.Pages), func(p seed.Page) bool { return p.Space != sp.Key })
		hi := slices.IndexFunc(pages, func(p seed.Page) bool { return p.Home })
		if hi < 0 {
			return nil, fmt.Errorf("wiki: space %s has no home page", sp.Key)
		}
		h := pages[hi]
		homeID := r.PageID(sp.Key, h.Title)
		first, err := r.Doc(h.Versions[0].Body, sp.Key)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", h.File, err)
		}
		at := r.Time(h.Versions[0].At)
		add(at, PhaseScope, "space "+sp.Key, wiki.CreateSpace{By: r.By(h.Versions[0].By, at), ID: r.SpaceID(sp.Key), Home: homeID,
			Key: sp.Key, Name: sp.Name, Description: sp.Description, Open: true, HomeDoc: first})
		if h.Title != sp.Name {
			add(at, PhaseChange, "home "+sp.Key, wiki.RenamePage{By: r.By(h.Versions[0].By, at), Page: homeID, Title: h.Title})
		}
		for _, p := range pages {
			id := r.PageID(sp.Key, p.Title)
			if !p.Home {
				parent := homeID
				if p.Parent != "" {
					parent = r.PageID(sp.Key, p.Parent)
				}
				d, err := r.Doc(p.Versions[0].Body, sp.Key)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", p.File, err)
				}
				at := r.Time(p.Versions[0].At)
				// Siblings of one minute keep their positions: a later
				// position a moment later.
				at = at.Add(time.Duration(p.Position) * time.Millisecond)
				add(at, PhaseCreate, "page "+p.File, wiki.CreatePage{By: r.By(p.Versions[0].By, at), ID: id, Space: r.SpaceID(sp.Key), Parent: parent,
					Title: p.Title, Doc: d, Message: p.Versions[0].Message})
			}
			for _, v := range p.Versions[1:] {
				d, err := r.Doc(v.Body, sp.Key)
				if err != nil {
					return nil, fmt.Errorf("%s version %d: %w", p.File, v.Number, err)
				}
				at := r.Time(v.At)
				add(at, PhaseChange, fmt.Sprintf("version %d of %s", v.Number, p.File), wiki.SetDocument{By: r.By(v.By, at), Page: id, Doc: d, Message: v.Message})
			}
			if len(p.Labels) > 0 {
				last := p.Versions[len(p.Versions)-1]
				at := r.Time(last.At).Add(time.Duration(p.Position) * time.Millisecond)
				add(at, PhaseChange, "labels of "+p.File, wiki.SetPageLabels{By: r.By(last.By, at), Page: id, Add: p.Labels})
			}
		}
	}
	n := 0
	for _, cm := range c.PageComments {
		page := r.PageID(cm.Space, cm.Page)
		body, err := r.Doc(string(cm.Body), cm.Space)
		if err != nil {
			return nil, err
		}
		id := r.PageCommentID(n)
		n++
		at := r.Time(cm.At)
		add(at, PhaseChange, "comment on "+cm.Page, wiki.AddComment{By: r.By(cm.By, at), ID: id, Page: page, Body: body})
		for _, rp := range cm.Replies {
			rb, err := r.Doc(string(rp.Body), cm.Space)
			if err != nil {
				return nil, err
			}
			at := r.Time(rp.At)
			add(at, PhaseChange, "reply on "+cm.Page, wiki.AddComment{By: r.By(rp.By, at), ID: r.PageCommentID(n), Page: page, Parent: id, Body: rb})
			n++
		}
	}
	return steps, nil
}
