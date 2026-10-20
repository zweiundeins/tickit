package web

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/views"
)

// Page is what a page handler makes of the request: the title, the views
// its stream watches ("issue:<id>", registered in the view registry) and
// the content inside the document.
type Page struct {
	Title string
	Views []string
	Body  templ.Component
}

// errGone is what a page handler returns for something deleted: the page
// answers 410.
var errGone = errors.New("gone")

// render answers with a page that build makes in one read transaction,
// which the document renders in too, so the stream's first frames redraw
// what is there. It writes nothing: the tab is new, and its row appears
// with its first action. build answers access.ErrNotFound for 404 and
// errGone for 410.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, build func(ctx context.Context, q sqlite.Querier) (Page, error)) {
	ctx := r.Context()
	var buf bytes.Buffer
	err := s.db.View(ctx, func(tx *sql.Tx) error {
		p, err := build(ctx, tx)
		switch {
		case errors.Is(err, access.ErrNotFound):
			status, p, err = http.StatusNotFound, Page{Title: "Not found", Body: views.NotFound()}, nil
		case errors.Is(err, errGone):
			status, p, err = http.StatusGone, Page{Title: "Deleted", Body: views.Gone()}, nil
		case err != nil:
			return err
		}
		return views.Layout(s.shell(r, p)).Render(templ.WithChildren(ctx, p.Body), &buf)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")    // the tab id is this document's alone
	h.Set("Accept-CH", edge.NetworkHints) // Chromium's hints for edge.SlowLink
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

// shell is the document around a page, for the request's person.
func (s *Server) shell(r *http.Request, p Page) views.Shell {
	ctx := r.Context()
	me := people.Actor(ctx)
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	return views.Shell{
		Title:       p.Title,
		Tab:         newID(),
		Nonce:       edge.Nonce(ctx),
		CSS:         s.static.URL("style.css"),
		JS:          s.static.URL("vendor/datastar-rocket.js"),
		NewestFrame: s.static.URL("lith/" + live.NewestFrame),
		Views:       p.Views,
		Me:          me,
		Moderates:   access.SiteCan(me, access.Moderate),
		Administers: access.SiteCan(me, access.Administer),
		Notice:      demo.Notice(time.Now()),
		Path:        path,
	}
}

// notFound answers with the page of an address that names nothing.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusNotFound, func(context.Context, sqlite.Querier) (Page, error) {
		return Page{Title: "Not found", Body: views.NotFound()}, nil
	})
}

// gone answers with the page of something deleted.
func (s *Server) gone(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusGone, func(context.Context, sqlite.Querier) (Page, error) {
		return Page{Title: "Deleted", Body: views.Gone()}, nil
	})
}
