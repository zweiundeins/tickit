package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/collab"
	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
)

// routesDescription registers the co-edited issue description (FR-15,
// docs/editor.md, "Issue descriptions"): the editor's host for the issue's
// page, its open, close, commits and presence, the views that bring each
// editing tab its commits and the editors each other's carets, the
// readers' live description, and the comparison of a description's
// version with the one before (FR-68).
//
// The wire format is the pages': a commit runs outside tabs.Act and is
// confirmed by its ref in the next frame (commitWith); open and close run
// through tabs.Act, and record the two editing views in the tab's row, so
// a stream that reconnects watches them again.
func (s *Server) routesDescription() {
	s.get("GET /issues/{id}/description/editor", http.HandlerFunc(s.descEditorHost), "/issues/"+ProbeIssue+"/description/editor", "/issues/nope/description/editor")
	s.get("GET /issues/{id}/description", http.HandlerFunc(s.descVersion), "/issues/"+ProbeIssue+"/description?v=1", "/issues/"+ProbeIssue+"/description?v=x")
	s.post("POST /cmd/issues/{id}/description/open", s.openDesc)
	s.post("POST /cmd/issues/{id}/description/close", s.closeDesc)
	s.post("POST /cmd/issues/{id}/description/commit", s.commitDesc)
	s.post("POST /cmd/issues/{id}/description/presence", s.presenceDesc)
	s.views.Register("desc-editor", s.descEditorView)
	s.views.Register("desc-carets", s.descCaretsView)
	s.views.Register("issue-desc", s.descReaderView)
}

// EditingDescription reports whether someone has the issue's description
// open in the editor: the tabs that watch its carets view.
func (s *Server) EditingDescription(issue string) bool {
	return len(s.hub.Watchers(tracker.DescCaretsView(issue))) > 0
}

// descCmd is where the description's editor posts.
func descCmd(issue string) string { return "/cmd/issues/" + issue + "/description" }

// editableIssue is an issue the actor may read, and whether they may edit
// its description now: a Member of its open project, the issue neither
// deleted nor hidden.
func editableIssue(ctx context.Context, q sqlite.Querier, actor people.Person, id string) (tracker.Detail, bool, error) {
	if _, err := readableIssue(ctx, q, actor, id); err != nil {
		return tracker.Detail{}, false, err
	}
	d, err := tracker.IssueDetail(ctx, q, id)
	if err != nil {
		return d, false, err
	}
	ok, err := access.Can(ctx, q, actor, access.EditIssues, access.ProjectScope(d.ProjectID))
	return d, err == nil && ok && d.Deleted.IsZero() && !d.Hidden, err
}

// descHTML renders an issue's description as it is now, from its
// document; an issue created before descriptions had one shows the
// description its row holds.
func descHTML(ctx context.Context, q sqlite.Querier, issue string, fallback json.RawMessage, r *docResolver) (string, error) {
	h, err := tracker.DescriptionHead(ctx, q, issue)
	if errors.Is(err, sql.ErrNoRows) {
		html, _, err := renderDoc(fallback, r)
		return html, err
	}
	if err != nil {
		return "", err
	}
	html := doc.RenderHTML(h.Doc, r)
	if strings.TrimSpace(doc.Text(h.Doc)) == "" && !strings.Contains(html, "<img") && len(doc.References(h.Doc).Issues) == 0 {
		return "", nil
	}
	return html, nil
}

// descEditorHost is GET /issues/{id}/description/editor, a read that
// answers a patch of #issue-desc-editor with the editor's host and the
// document as it is now (docs/editor.md, "Editor frame"), with a client id
// of its own; the host's data-init then opens the editor. 403 for someone
// who may not edit it now.
func (s *Server) descEditorHost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me, id := people.Actor(ctx), r.PathValue("id")
	host, err := s.db.ViewValue(ctx, func(tx *sql.Tx) (views.EditorHost, error) {
		_, ok, err := editableIssue(ctx, tx, me, id)
		if err != nil {
			return views.EditorHost{}, err
		}
		if !ok {
			return views.EditorHost{}, &access.Denial{}
		}
		host := views.EditorHost{Page: id, Cmd: descCmd(id), Label: "Description", Client: newClient(), State: "ok", Batches: "[]", Images: imagesJSON()}
		h, err := tracker.DescriptionHead(ctx, tx, id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// An issue from before: open starts its document at version
			// 1 from the description it holds, in this epoch.
			d, _ := tracker.IssueDetail(ctx, tx, id)
			if h.Doc, err = doc.ParseDoc(d.Description); err != nil || collab.IsBlank(d.Description) {
				h.Doc, _ = doc.ParseDoc([]byte(collab.Blank))
			}
			n, err := demo.Epoch(ctx, tx)
			if err != nil {
				return host, err
			}
			h.Epoch, h.Version = "e"+strconv.FormatInt(n, 10), 1
		case err != nil:
			return host, err
		}
		host.Epoch, host.Base, host.Head = h.Epoch, h.Version, h.Version
		raw, err := doc.MarshalDoc(h.Doc)
		if err != nil {
			return host, err
		}
		// The script must not hold "</script": < goes out as \u003c.
		host.Doc = strings.ReplaceAll(string(raw), "<", `\u003c`)
		host.Chips, err = chipsJSON(ctx, tx, h.Doc)
		return host, err
	})
	var denial *access.Denial
	switch {
	case errors.As(err, &denial):
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	case errors.Is(err, access.ErrNotFound):
		s.notFound(w, r)
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	frame, err := patch(ctx, views.DescEditorMount(host))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	w.Write(frame)
}

// descViews are the two views a tab that edits a description watches:
// its own editor view and the shared carets of the description's editors.
func (s *Server) descViews(ctx context.Context, actor people.Person, issue, tab string) ([]live.View, error) {
	return s.db.ViewValue(ctx, func(tx *sql.Tx) ([]live.View, error) {
		var out []live.View
		for _, spec := range []string{tracker.DescEditorView(issue, tab), tracker.DescCaretsView(issue)} {
			vs, err := s.resolve(ctx, tx, actor, spec)
			if err != nil {
				return nil, err
			}
			out = append(out, vs...)
		}
		return out, nil
	})
}

// openDesc starts the tab's editing of the description: its row, then
// its two views on its stream (hub.Watch), which its row's watched views
// restore after a reconnect.
func (s *Server) openDesc(w http.ResponseWriter, r *http.Request) {
	var req openRequest
	a, ok := s.decode(w, r, classView, &req)
	if !ok {
		return
	}
	issue := r.PathValue("id")
	if !tracker.ValidID(issue) || !collab.ValidClient(req.Client) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res, ok := s.run(w, r, a, tracker.OpenDescription{By: a.By, Session: a.Session, Tab: a.Tab, Issue: issue, Base: req.Base, Client: req.Client})
	switch {
	case !ok:
		return
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	case !res.OK:
		w.WriteHeader(http.StatusNoContent) // the refusal is the tab's outcome
		return
	}
	ctx := r.Context()
	vs, err := s.descViews(ctx, a.By.Actor, issue, a.Tab)
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	// After tabs.Act, which saves the row it read before its command.
	err = s.w.Do(ctx, writer.Func(func(tx *writer.Tx) error {
		for i, spec := range []string{tracker.DescEditorView(issue, a.Tab), tracker.DescCaretsView(issue)} {
			if _, err := tabs.AddWatched(tx, a.Session, a.Tab, tabs.Watched{View: spec, Key: vs[i].Key}, time.Now()); err != nil {
				return err
			}
		}
		return nil
	}))
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	for _, v := range vs {
		s.hub.Watch(streamID(a.By.Actor, a.Tab), v)
	}
	w.WriteHeader(http.StatusNoContent)
}

// closeDesc ends the tab's editing of the description (Done): its row
// goes, with its caret, and its two views come off its stream and its
// row. A tab that edits nothing changes nothing.
func (s *Server) closeDesc(w http.ResponseWriter, r *http.Request) {
	var req Payload
	a, ok := s.decode(w, r, classView, &req)
	if !ok {
		return
	}
	issue := r.PathValue("id")
	if !tracker.ValidID(issue) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res, ok := s.run(w, r, a, &tracker.CloseDescription{Session: a.Session, Tab: a.Tab})
	switch {
	case !ok:
		return
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	keys := []string{tracker.DescEditorView(issue, a.Tab) + "." + a.By.Actor.ID, tracker.DescCaretsView(issue)}
	err := s.w.Do(r.Context(), writer.Func(func(tx *writer.Tx) error {
		for _, k := range keys {
			if err := tabs.RemoveWatched(tx, a.Session, a.Tab, k, time.Now()); err != nil {
				return err
			}
		}
		return nil
	}))
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	for _, k := range keys {
		s.hub.Unwatch(streamID(a.By.Actor, a.Tab), k)
	}
	w.WriteHeader(http.StatusNoContent)
}

// commitDesc applies a commit to the description, as commit does to a
// page (docs/editor.md, "Wire format").
func (s *Server) commitDesc(w http.ResponseWriter, r *http.Request) {
	s.commitWith(w, r, func(by tabs.By, sid, id string, req editorRequest) (writer.ResultCommand, func() time.Duration) {
		c := &tracker.CommitDescription{By: by, Session: sid, Tab: req.Tab, Issue: id, Epoch: req.Epoch, Base: req.Base, Ref: req.Ref, Client: req.Client, Steps: req.Steps}
		return c, func() time.Duration { return c.Took }
	})
}

// presenceDesc is an editing tab's selection in the description, sent
// without waiting (MP-06.2).
func (s *Server) presenceDesc(w http.ResponseWriter, r *http.Request) {
	req, actor, ok := s.decodeEditor(w, r, classCaret)
	if !ok || s.paused(w, r, actor) {
		return
	}
	err := s.w.Send(tracker.DescriptionPresence(edge.SessionID(r.Context()), req.Tab, r.PathValue("id"), req.Epoch,
		req.Ack, req.V, req.Anchor, req.Head, req.Seq, time.Now()))
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// descEditorView is one editing tab's view of a description, as
// editorView is of a page: it patches the host with every commit after
// the tab's acknowledged version, the chips and the tab's state. Its arg
// is "<issue>.<tab>"; only the tab's person may watch it.
func (s *Server) descEditorView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	issue, tab, _ := strings.Cut(arg, ".")
	if !tracker.ValidID(issue) || !validID(tab) {
		return live.View{}, access.ErrNotFound
	}
	if _, err := readableIssue(ctx, q, actor, issue); err != nil {
		return live.View{}, err
	}
	return live.View{
		// The person too: another who names the tab id gets a view of
		// their own, never the tab's (NFR-02.3).
		Key:    tracker.DescEditorView(issue, tab) + "." + actor.ID,
		Topics: []string{tracker.DescTopic(issue), tracker.IssueTopic(issue), tracker.DescEditorTopic(tab), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			host, err := descHost(ctx, querier(ctx, tx), actor, issue, tab)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.EditorHostEl(host))
		},
	}, nil
}

// descHost is the host element of a tab's frame (docs/editor.md): reset
// when the tab's row is gone or of another epoch, gone for a deleted or
// hidden issue, gap when the commits after its ack were folded, readonly
// without the right to edit.
func descHost(ctx context.Context, q sqlite.Querier, actor people.Person, issue, tab string) (views.EditorHost, error) {
	host := views.EditorHost{Page: issue, Cmd: descCmd(issue), Label: "Description", Batches: "[]", Chips: "{}", Images: imagesJSON(), State: "ok"}
	row, err := tracker.DescriptionEditor(ctx, q, tab, actor.ID)
	if err != nil || row.ID != issue {
		host.State = "reset" // the tab's row is gone: a reset, or a sweep
		return host, nil
	}
	host.Client, host.Epoch, host.Base, host.Refusal = row.Client, row.Epoch, row.Ack, row.Refusal
	d, err := tracker.IssueDetail(ctx, q, issue)
	if err != nil || !d.Deleted.IsZero() || d.Hidden {
		host.State = "gone"
		return host, nil
	}
	h, err := tracker.DescriptionHead(ctx, q, issue)
	if err != nil {
		return host, err
	}
	host.Head = h.Version
	if h.Epoch != row.Epoch {
		host.State = "reset"
		return host, nil
	}
	list, ok, err := tracker.DescriptionBatches(ctx, q, issue, h.Epoch, row.Ack, h.Version)
	if err != nil {
		return host, err
	}
	if !ok {
		host.State = "gap"
		return host, nil
	}
	b, err := json.Marshal(list)
	if err != nil {
		return host, err
	}
	host.Batches = string(b)
	if host.Chips, err = chipsJSON(ctx, q, h.Doc); err != nil {
		return host, err
	}
	if ok, _ := access.Can(ctx, q, actor, access.EditIssues, access.ProjectScope(d.ProjectID)); !ok {
		host.State, host.Readonly = "readonly", true
	}
	return host, nil
}

// descCaretsView is the carets of a description's editors (MP-06), shared
// by them all as the signal $_carets, as caretsView is a page's.
func (s *Server) descCaretsView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if _, err := readableIssue(ctx, q, actor, arg); err != nil {
		return live.View{}, err
	}
	key := tracker.DescCaretsView(arg)
	return live.View{
		Key:      key,
		Topics:   []string{tracker.DescCaretsTopic(arg), people.Topic},
		Presence: true,
		Every:    10 * time.Second, // idle carets hide their names
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			rows, err := tracker.DescriptionEditors(ctx, q, arg)
			if err != nil {
				return nil, err
			}
			return s.caretsSignal(ctx, q, key, rows)
		},
	}, nil
}

// descReaderView is issue-desc:<id>, the description as it is now for
// everyone who has the issue open (MP-03): every commit renders it again.
func (s *Server) descReaderView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if _, err := readableIssue(ctx, q, actor, arg); err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "issue-desc:" + arg,
		Topics: []string{tracker.DescTopic(arg), tracker.IssueTopic(arg), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			d, err := tracker.IssueDetail(ctx, q, arg)
			if err != nil {
				return nil, err
			}
			html, err := descHTML(ctx, q, arg, d.Description, &docResolver{ctx: ctx, q: q, people: map[string]people.Person{}})
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.IssueDescBody(html))
		},
	}, nil
}

// descVersion is GET /issues/{id}/description?v=<version>: the
// description as the issue's version v made it, compared with the one
// before (FR-68: "changed the description", Compare); v of the issue's
// creation shows its first description. A version that changed no
// description answers 404.
func (s *Server) descVersion(w http.ResponseWriter, r *http.Request) {
	v, err := strconv.ParseInt(r.URL.Query().Get("v"), 10, 64)
	if err != nil || v < 1 {
		s.notFound(w, r)
		return
	}
	me, id := people.Actor(r.Context()), r.PathValue("id")
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		if _, err := readableIssue(ctx, q, me, id); err != nil {
			return Page{}, err
		}
		ver, err := tracker.DescriptionVersion(ctx, q, id, v)
		if err != nil {
			return Page{}, err
		}
		d, err := tracker.IssueDetail(ctx, q, id)
		if err != nil {
			return Page{}, err
		}
		who, err := people.Lookup(ctx, q, append([]string{ver.Actor}, ver.Editors...)...)
		if err != nil {
			return Page{}, err
		}
		res := &docResolver{ctx: ctx, q: q, people: who}
		p := views.DescVersionProps{Key: d.Key, Summary: d.Summary, Version: v, At: ver.At, Actor: who[ver.Actor], Now: time.Now(), First: ver.Before == nil}
		for _, e := range ver.Editors {
			if e != ver.Actor {
				p.Others = append(p.Others, who[e])
			}
		}
		if ver.Before == nil {
			p.HTML = doc.RenderHTML(ver.After, res)
		} else {
			diff := doc.Diff(ver.Before, ver.After)
			p.HTML, p.Inserted, p.Deleted = diff.HTML(res), diff.Inserted, diff.Deleted
		}
		return Page{Title: d.Key + " description, version " + strconv.FormatInt(v, 10), Nav: "projects", Project: d.ProjectID, Width: "prose",
			Body: views.DescVersion(p)}, nil
	})
}
