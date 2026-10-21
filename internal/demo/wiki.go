package demo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo/seed"
	"github.com/zweiundeins/tickit/internal/doc"
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
		Scope: func(tx *writer.Tx, id string) (access.Scope, error) {
			return scopeOf(tx, `SELECT 'space', space_id FROM pages WHERE id = ?`, id)
		},
	})
	// DM-07.1: a version a visitor's edits cut; its id is "<page>.<number>".
	RegisterModerated(Moderated{
		Kind: "version", Name: "Page version",
		Hide: func(by tabs.By, id string, hidden bool, reason string) writer.Command {
			page, n := versionID(id)
			return wiki.HideVersion{By: by, Page: page, Number: n, Hidden: hidden, Reason: reason}
		},
		Label: func(tx *writer.Tx, id string) (string, error) {
			page, n := versionID(id)
			if n < 1 {
				return "", fmt.Errorf("version %s: %w", id, access.ErrNotFound)
			}
			return wiki.VersionLabel(tx, page, n)
		},
		Newest: newestVersions,
		Scope: func(tx *writer.Tx, id string) (access.Scope, error) {
			page, _ := versionID(id)
			return scopeOf(tx, `SELECT 'space', space_id FROM pages WHERE id = ?`, page)
		},
	})
	// DM-07.1: an image of the library, hidden in every page that shows it.
	RegisterModerated(Moderated{
		Kind: "image", Name: "Image",
		Hide: func(by tabs.By, id string, hidden bool, reason string) writer.Command {
			return wiki.HideImage{By: by, Image: id, Hidden: hidden, Reason: reason}
		},
		Label: func(tx *writer.Tx, id string) (string, error) {
			for _, im := range wiki.Images() {
				if im.ID == id {
					return im.Title, nil
				}
			}
			return "", fmt.Errorf("image %s: %w", id, access.ErrNotFound)
		},
		Newest: newestImages,
	})
}

// versionID splits a version's moderation id, "<page>.<number>".
func versionID(id string) (string, int) {
	page, num, ok := strings.Cut(id, ".")
	n, err := strconv.Atoi(num)
	if !ok || err != nil || n < 1 {
		return page, 0
	}
	return page, n
}

// newestVersions are the versions visitors' edits cut after a page's
// first, newest first.
func newestVersions(ctx context.Context, q sqlite.Querier, limit int) ([]Target, error) {
	rows, err := q.QueryContext(ctx, `SELECT v.page_id, v.number, v.title, v.created_by, v.created_at, v.hidden_at IS NOT NULL, s.key, p.space_id
		FROM page_versions v JOIN pages p ON p.id = v.page_id JOIN spaces s ON s.id = p.space_id
		WHERE v.number > 1 AND v.created_by NOT LIKE 's-%' AND p.state != 'deleted' ORDER BY v.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		var t Target
		var page, key, space string
		var n int
		var at int64
		if err := rows.Scan(&page, &n, &t.Label, &t.Author, &at, &t.Hidden, &key, &space); err != nil {
			return nil, err
		}
		t.Kind, t.ID, t.At, t.Scope = "version", page+"."+strconv.Itoa(n), time.UnixMilli(at), access.SpaceScope(space)
		t.Href, t.Text, t.Context = wiki.VersionURL(key, page, n), "Version "+strconv.Itoa(n), "in "+key
		out = append(out, t)
	}
	return out, rows.Err()
}

// newestImages are the library's images in the versions visitors cut
// lately, and every hidden one, so that a moderator may show it again.
// Uploaded images are a kind of their own (images.go).
func newestImages(ctx context.Context, q sqlite.Querier, limit int) ([]Target, error) {
	titles := map[string]string{}
	for _, im := range wiki.Images() {
		titles[im.ID] = im.Title
	}
	seen := map[string]bool{}
	var out []Target
	hidden, err := wiki.HiddenImageList(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	for _, h := range hidden {
		if titles[h.ID] == "" {
			continue // an uploaded one
		}
		seen[h.ID] = true
		out = append(out, Target{Kind: "image", ID: h.ID, Label: titles[h.ID], Href: "/img/lib/" + h.ID, Author: h.By, At: h.At, Hidden: true, Context: "hidden in every page"})
	}
	rows, err := q.QueryContext(ctx, `SELECT v.doc, v.title, v.created_by, v.created_at, s.key, p.space_id
		FROM page_versions v JOIN pages p ON p.id = v.page_id JOIN spaces s ON s.id = p.space_id
		WHERE v.created_by NOT LIKE 's-%' AND p.state != 'deleted' AND v.doc LIKE '%"imageId"%' ORDER BY v.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw, title, by, key, space string
		var at int64
		if err := rows.Scan(&raw, &title, &by, &at, &key, &space); err != nil {
			return nil, err
		}
		d, err := doc.ParseDoc([]byte(raw))
		if err != nil {
			continue
		}
		for _, id := range doc.References(d).Images {
			if seen[id] || titles[id] == "" {
				continue
			}
			seen[id] = true
			out = append(out, Target{Kind: "image", ID: id, Label: titles[id], Href: "/img/lib/" + id, Author: by, At: time.UnixMilli(at),
				Scope: access.SpaceScope(space), Context: "in " + key + ": " + title})
		}
	}
	return out, rows.Err()
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
