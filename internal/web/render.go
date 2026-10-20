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

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/views"
)

// Page is what a page handler makes of the request: the title, the views
// its stream watches ("issue:<id>", registered in the view registry), the
// content inside the document, and how the document frames it.
type Page struct {
	Title string
	Views []string
	Body  templ.Component
	// Nav is the header link the page belongs to: "projects", "spaces",
	// "filters", "dashboards" (ui.MainNav), "moderation", "admin".
	Nav string
	// Sidebar is the project's or space's ui.SideNav; SidebarLabel names
	// it ("Project" by default).
	Sidebar      templ.Component
	SidebarLabel string
	// Width of main: "" (80rem), "prose" (46rem) or "full" (boards).
	Width string
	// Query fills the header's search box (the search page).
	Query string
	// Components are the Starbase components the page uses ("select",
	// "tabs"), loaded with the modules they render (StarbaseScripts).
	Components []string
	// Scripts are further modules by their static name
	// ("editor/sb-rich-editor.min.js").
	Scripts []string
	// Create replaces the header's create dialog (views.CreateID); nil
	// gives the issue create dialog (FR-10.1), with Project preselected.
	Create templ.Component
	// Project is the id of the project the page belongs to, if any: the
	// create dialog preselects it.
	Project string
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
	if people.Actor(ctx).MustChange && r.URL.Path != PasswordPath { // FR-141.1
		http.Redirect(w, r, PasswordPath, http.StatusSeeOther)
		return
	}
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
		if p.Create == nil {
			if p.Create, err = s.createDialog(ctx, tx, people.Actor(ctx), p.Project); err != nil {
				return err
			}
		}
		shell, err := s.shell(r, tx, p)
		if err != nil {
			return err
		}
		return views.Layout(shell).Render(templ.WithChildren(ctx, p.Body), &buf)
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

// shell is the document around a page, for the request's person, read in
// q (the session's preferences).
func (s *Server) shell(r *http.Request, q sqlite.Querier, p Page) (views.Shell, error) {
	ctx := r.Context()
	me := people.Actor(ctx)
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	prefs := tabs.Prefs{}.Normal()
	if sid := edge.SessionID(ctx); sid != "" {
		var err error
		if prefs, err = tabs.GetPrefs(ctx, q, sid); err != nil {
			return views.Shell{}, err
		}
	}
	scripts, err := s.scripts(p)
	if err != nil {
		return views.Shell{}, err
	}
	bell, err := s.bell(ctx, q, me) // routes_notify.go
	if err != nil {
		return views.Shell{}, err
	}
	return views.Shell{
		Title:        p.Title,
		Nav:          p.Nav,
		Sidebar:      p.Sidebar,
		SidebarLabel: p.SidebarLabel,
		Width:        p.Width,
		Query:        p.Query,
		Views:        p.Views,
		Create:       p.Create,
		Tab:          newID(),
		Nonce:        edge.Nonce(ctx),
		CSS:          s.static.URL(Bundle),
		JS:           s.static.URL("vendor/datastar-rocket.js"),
		NewestFrame:  s.static.URL("lith/" + live.NewestFrame),
		Mark:         s.static.URL(Mark),
		Fonts:        s.fonts,
		Scripts:      scripts,
		Me:           me,
		Moderates:    access.SiteCan(me, access.Moderate),
		Administers:  access.SiteCan(me, access.Administer),
		NextReset:    demo.Remaining(time.Now()),
		DemoAlert:    demoAlert(s.demo.Status(time.Now())),
		Prefs:        prefs,
		Bell:         bell,
		Path:         path,
	}, nil
}

// scripts are the URLs of the modules page p loads after Datastar.
func (s *Server) scripts(p Page) ([]string, error) {
	var names []string
	if len(p.Components) > 0 {
		var err error
		if names, err = StarbaseScripts(s.files, p.Components...); err != nil {
			return nil, err
		}
	}
	urls := make([]string, 0, len(names)+len(p.Scripts))
	for _, n := range append(names, p.Scripts...) {
		urls = append(urls, s.static.URL(n))
	}
	return urls, nil
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
