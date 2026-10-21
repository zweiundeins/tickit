package demo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/images"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// Uploaded images (FR-85.2), in one file as the wiki's part is: an upload
// counts against the demo's 200 images, and a moderator hides one in
// every page that shows it, or deletes it, which hides it for good.
func init() {
	CountCreations(func(cmd writer.Command) string {
		if _, ok := cmd.(images.Upload); ok {
			return CapImage
		}
		return ""
	})
	RegisterModerated(Moderated{
		Kind: "upload", Name: "Uploaded image",
		Hide: func(by tabs.By, id string, hidden bool, reason string) writer.Command {
			return wiki.HideImage{By: by, Image: id, Hidden: hidden, Reason: reason}
		},
		Delete: func(by tabs.By, id string) writer.Command {
			return writer.Func(func(tx *writer.Tx) error {
				if err := (wiki.HideImage{By: by, Image: id, Hidden: true, Reason: "deleted"}).Apply(tx); err != nil {
					return err
				}
				return images.Delete{By: by, ID: id}.Apply(tx)
			})
		},
		Label: func(tx *writer.Tx, id string) (string, error) {
			u, err := images.One(tx.Context(), tx, id)
			return u.Label(), err
		},
		Newest: newestUploads,
		Scope: func(tx *writer.Tx, id string) (access.Scope, error) {
			u, err := images.One(tx.Context(), tx, id)
			if err != nil {
				return access.Scope{}, err
			}
			if u.Subject == images.OnPage {
				return scopeOf(tx, `SELECT 'space', space_id FROM pages WHERE id = ?`, u.On)
			}
			return scopeOf(tx, `SELECT 'project', project_id FROM issues WHERE id = ?`, u.On)
		},
	})
}

// newestUploads are the images people uploaded, newest first, with the
// page or the issue each went into.
func newestUploads(ctx context.Context, q sqlite.Querier, limit int) ([]Target, error) {
	list, err := images.Newest(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(list))
	for i, u := range list {
		ids[i] = u.ID
	}
	hidden, err := wiki.HiddenImages(ctx, q, ids...)
	if err != nil {
		return nil, err
	}
	var out []Target
	for _, u := range list {
		t := Target{Kind: "upload", ID: u.ID, Label: u.Label(), Href: "/img/lib/" + u.ID, Author: u.By, At: u.At, Hidden: hidden[u.ID]}
		var key, title, scope string
		if u.Subject == images.OnPage {
			err = q.QueryRowContext(ctx, `SELECT s.key, p.title, p.space_id FROM pages p JOIN spaces s ON s.id = p.space_id WHERE p.id = ?`, u.On).Scan(&key, &title, &scope)
			t.Scope, t.Context = access.SpaceScope(scope), "in "+key+": "+title
		} else {
			err = q.QueryRowContext(ctx, `SELECT key, project_id FROM issues WHERE id = ?`, u.On).Scan(&key, &scope)
			t.Scope, t.Context = access.ProjectScope(scope), "in the description of "+key
		}
		if errors.Is(err, sql.ErrNoRows) {
			t.Scope, t.Context, err = access.Scope{}, "", nil
		}
		t.Text = t.Context
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}
