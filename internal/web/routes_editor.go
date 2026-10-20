package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/doc"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/views"
	"github.com/mbolli/tickit/internal/wiki"
)

// routesEditor registers the rich-text editor of pages: its page, the
// commits and presence of sb-rich-editor, the views that bring each
// editing tab its batches and every editor the others' carets (FR-82,
// FR-86, MP-05 to MP-07, docs/editor.md).
//
// The wire format is docs/editor.md's. A commit runs outside tabs.Act: it
// is confirmed by its ref in the next frame's batches, not by an op id,
// and a refusal by a rule goes into the editing tab's row in
// page_editors, which its view renders as the host's refusal; so ten
// commits a second rewrite no tab row and send no outcome.
func (s *Server) routesEditor() {
	s.get("GET /spaces/{key}/pages/{id}/edit", http.HandlerFunc(s.editorPage), "/spaces/ENG/pages/0192d000-0000-7000-8000-000000000001/edit")
	s.get("GET /wiki/pick", http.HandlerFunc(s.pick), "/wiki/pick?kind=page&q=dep", "/wiki/pick?kind=person&q=a", "/wiki/pick?kind=issue&q=web")
	s.post("POST /cmd/pages/{id}/open", s.openEditor)
	s.post("POST /cmd/pages/{id}/commit", s.commit)
	s.post("POST /cmd/pages/{id}/presence", s.presence)
	s.views.Register("editor", s.editorView)
	s.views.Register("carets", s.caretsView)
	s.views.Register("pagetitle", s.titleView)
}

// caretsKey is the key of a page's carets view, which every editing tab
// of the page watches: its watchers are the page's editors.
func caretsKey(page string) string { return "carets:" + page }

// Editing reports whether someone has the page open in the editor.
func (s *Server) Editing(page string) bool { return len(s.hub.Watchers(caretsKey(page))) > 0 }

func (s *Server) editorPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.renderOrRedirect(w, r, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		p, sp, err := readable(ctx, q, me, r.PathValue("id"))
		if err != nil {
			return Page{}, err
		}
		if sp.Key != r.PathValue("key") || p.State != wiki.Current || !canIn(ctx, q, me, access.EditIssues, sp.ID) {
			return Page{}, errRedirect{wiki.PageURL(sp.Key, p.ID, p.Title)}
		}
		props, err := s.pageProps(ctx, q, me, p, sp)
		if err != nil {
			return Page{}, err
		}
		h, err := wiki.HeadOf(ctx, q, p.ID)
		if err != nil {
			return Page{}, err
		}
		raw, err := doc.MarshalDoc(h.Doc)
		if err != nil {
			return Page{}, err
		}
		chips, err := chipsJSON(ctx, q, h.Doc)
		if err != nil {
			return Page{}, err
		}
		host := views.EditorHost{
			Page: p.ID, Epoch: h.Epoch, Client: newClient(), State: "ok", Base: h.Version, Head: h.Version,
			Batches: "[]", Chips: chips, Images: imagesJSON(),
			// The script must not hold "</script": < goes out as <.
			Doc: strings.ReplaceAll(string(raw), "<", `<`),
		}
		sidebar, err := s.sidebar(ctx, q, sp, p.ID, "")
		if err != nil {
			return Page{}, err
		}
		return Page{
			Title: "Editing " + p.Title, Nav: "spaces", Sidebar: sidebar, SidebarLabel: "Space",
			Views:   []string{"pagepeople:" + p.ID, "carets:" + p.ID, "pagetitle:" + p.ID, "tree:" + p.ID},
			Scripts: []string{"editor/sb-rich-editor.min.js"},
			Body:    views.EditorPage(views.EditorProps{PageProps: props, Host: host}),
		}, nil
	})
}

// imagesJSON is the image library as the editor's Image button offers it.
// The library is fixed in the binary; it is read once, not at every
// render of an editor frame (seed.Load parses the whole seed).
var imagesJSON = sync.OnceValue(func() string {
	type image struct {
		ID  string `json:"id"`
		Alt string `json:"alt"`
	}
	var list []image
	for _, im := range wiki.Images() {
		list = append(list, image{im.ID, im.Alt})
	}
	b, _ := json.Marshal(list)
	return string(b)
})

// chipsJSON is the editor's chips attribute: what the document's issue
// chips, mentions and page links show now (editor/README.md).
func chipsJSON(ctx context.Context, q sqlite.Querier, d doc.Doc) (string, error) {
	refs := doc.References(d)
	res, err := wiki.Resolve(ctx, q, refs)
	if err != nil {
		return "", err
	}
	out := map[string]any{}
	for _, k := range refs.Issues {
		c, ok := res.Issues[k]
		switch {
		case !ok:
			out[k] = map[string]string{"state": "unknown"}
		case c.Deleted:
			out[k] = map[string]string{"state": "deleted"}
		default:
			out[k] = map[string]string{"summary": c.Summary, "status": c.Status, "category": c.Category, "type": c.Type}
		}
	}
	ppl := map[string]any{}
	for id, name := range res.People {
		ppl[id] = map[string]string{"name": name}
	}
	pages := map[string]any{}
	for _, id := range refs.Pages {
		if t, ok := res.Pages[id]; ok {
			pages[id] = map[string]string{"title": t}
		} else {
			pages[id] = map[string]string{"state": "gone"}
		}
	}
	out["people"], out["pages"] = ppl, pages
	b, err := json.Marshal(out)
	return string(b), err
}

type openRequest struct {
	Payload
	Base   int64  `json:"base"`
	Client string `json:"client"`
}

// openEditor starts the tab's editing of the page: its row, and its
// editor view on its stream (hub.Watch), which its row's watched views
// restore after a reconnect.
func (s *Server) openEditor(w http.ResponseWriter, r *http.Request) {
	var req openRequest
	a, ok := s.decode(w, r, classView, &req)
	if !ok {
		return
	}
	page := r.PathValue("id")
	if !wiki.ValidID(page) || !wiki.ValidClient(req.Client) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res, ok := s.run(w, r, a, wiki.OpenEditor{By: a.By, Session: a.Session, Tab: a.Tab, Page: page, Base: req.Base, Client: req.Client})
	switch {
	case !ok:
		return
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	if res.OK {
		if err := s.w.Do(ctx, writer.Func(func(tx *writer.Tx) error { return wiki.WatchEditor(tx, a.Session, a.Tab, page, time.Now()) })); err != nil {
			s.failAction(w, r, err)
			return
		}
	}
	vs, err := s.db.ViewValue(ctx, func(tx *sql.Tx) ([]live.View, error) {
		return s.resolve(ctx, tx, a.By.Actor, wiki.EditorView(page, a.Tab))
	})
	if err == nil {
		for _, v := range vs {
			s.hub.Watch(streamID(a.By.Actor, a.Tab), v)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// editorRequest is a commit's or a presence post's payload.
type editorRequest struct {
	Tab    string          `json:"tab"`
	Epoch  string          `json:"epoch"`
	Base   int64           `json:"base"`
	Ref    string          `json:"ref"`
	Client string          `json:"client"`
	Steps  json.RawMessage `json:"steps"`
	Ack    int64           `json:"ack"`
	V      int64           `json:"v"`
	Anchor int             `json:"anchor"`
	Head   int             `json:"head"`
	Seq    int64           `json:"seq"`
}

// decodeEditor reads a commit or a presence post: the payload of
// sb-commit or sb-presence, with the tab and no op id.
func (s *Server) decodeEditor(w http.ResponseWriter, r *http.Request, c class) (editorRequest, people.Person, bool) {
	var req editorRequest
	if !s.allow(w, r, c) {
		return req, people.Person{}, false
	}
	actor := people.Actor(r.Context())
	if err := sse.ReadSignalsLimit(w, r, &req, c.body); err != nil || !validID(req.Tab) || actor.IsZero() || !wiki.ValidID(r.PathValue("id")) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return req, actor, false
	}
	return req, actor, true
}

// commit applies a commit (docs/editor.md, "Wire format"): 204 applied or
// applied before, 409 reset, removed or folded away (the next frame says
// which), 422 refused by a rule (the tab's editor row says why), 403 for
// a reader, 429 over the limit.
func (s *Server) commit(w http.ResponseWriter, r *http.Request) {
	req, actor, ok := s.decodeEditor(w, r, classEditBatch)
	if !ok {
		return
	}
	page := r.PathValue("id")
	if req.Ref == "" || !validID(req.Ref) || !wiki.ValidClient(req.Client) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	c := &wiki.Commit{
		By: tabs.By{Actor: actor, Op: req.Ref, At: time.Now()}, Session: edge.SessionID(ctx), Tab: req.Tab, Page: page,
		Epoch: req.Epoch, Base: req.Base, Ref: req.Ref, Client: req.Client, Steps: req.Steps,
	}
	// The demo's guard: no edits during a reset (503), none from a blocked
	// guest (403, MP-14.2: the text stays in the browser).
	op := writer.OnceResult("commit:"+page+":"+req.Ref, s.demo.Guard(actor, edge.ClientKey(r), c).(writer.ResultCommand))
	if err := s.w.Do(ctx, op); err != nil {
		var denial interface{ Denied() bool }
		if errors.As(err, &denial) && denial.Denied() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.failAction(w, r, err)
		return
	}
	if !op.Replayed() {
		CommitTimes.Observe(c.Took)
	}
	res := op.Result()
	switch {
	case len(res) == 0:
		w.WriteHeader(http.StatusNoContent)
	case wiki.CommitAnswer(res[0]) == wiki.Stale:
		http.Error(w, "stale", http.StatusConflict)
	case wiki.CommitAnswer(res[0]) == wiki.Refused:
		http.Error(w, "refused", http.StatusUnprocessableEntity)
	case wiki.CommitAnswer(res[0]) == wiki.Readonly:
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// presence is an editing tab's selection, sent without waiting: the next
// one replaces it (MP-06.2).
func (s *Server) presence(w http.ResponseWriter, r *http.Request) {
	req, _, ok := s.decodeEditor(w, r, classCaret)
	if !ok {
		return
	}
	err := s.w.Send(wiki.Presence{
		Session: edge.SessionID(r.Context()), Tab: req.Tab, Page: r.PathValue("id"), Epoch: req.Epoch,
		Ack: req.Ack, V: req.V, Anchor: req.Anchor, Head: req.Head, Seq: req.Seq, At: time.Now(),
	})
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// editorView is one editing tab's view (docs/editor.md): it patches the
// host with every commit after the tab's acknowledged version, the chips
// and the tab's state, so a skipped frame loses nothing (MP-05.9). Its
// arg is "<page>.<tab>"; only the tab's person may watch it.
func (s *Server) editorView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	page, tab, _ := strings.Cut(arg, ".")
	if !wiki.ValidID(page) || !validID(tab) {
		return live.View{}, access.ErrNotFound
	}
	if _, _, err := readable(ctx, q, actor, page); err != nil {
		return live.View{}, err
	}
	key := wiki.EditorView(page, tab)
	return live.View{
		Key:    key,
		Topics: []string{wiki.DocTopic(page), wiki.ChipsTopic(page), wiki.PageTopic(page), wiki.EditorTopic(tab), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			host, err := editorHost(ctx, querier(ctx, tx), actor, page, tab)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.EditorHostEl(host))
		},
	}, nil
}

// editorHost is the host element of a tab's frame.
func editorHost(ctx context.Context, q sqlite.Querier, actor people.Person, page, tab string) (views.EditorHost, error) {
	host := views.EditorHost{Page: page, Batches: "[]", Chips: "{}", Images: imagesJSON(), State: "ok"}
	row, err := wiki.Editor(ctx, q, tab, actor.ID)
	if err != nil || row.Page != page {
		host.State = "reset" // the tab's row is gone: a reset, or a sweep
		return host, nil
	}
	host.Client, host.Epoch, host.Base, host.Refusal = row.Client, row.Epoch, row.Ack, row.Refusal
	p, err := wiki.PageByID(ctx, q, page)
	if err != nil || p.State != wiki.Current {
		host.State = "gone"
		return host, nil
	}
	h, err := wiki.HeadOf(ctx, q, page)
	if err != nil {
		return host, err
	}
	host.Head = h.Version
	if h.Epoch != row.Epoch {
		host.State = "reset"
		return host, nil
	}
	list, ok, err := wiki.Batches(ctx, q, page, h.Epoch, row.Ack, h.Version)
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
	if !canIn(ctx, q, actor, access.EditIssues, p.Space) {
		host.State, host.Readonly = "readonly", true
	}
	return host, nil
}

// caretsView is the carets of a page's editors (MP-06), shared by them
// all as the signal $_carets, which each host's carets attribute follows
// (each editor leaves out its own). Its watchers are the page's editing
// tabs: a tab that goes takes its caret with it (Presence).
func (s *Server) caretsView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, _, err := readable(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	key := caretsKey(p.ID)
	return live.View{
		Key:      key,
		Topics:   []string{wiki.CaretsTopic(p.ID), people.Topic},
		Presence: true,
		Every:    10 * time.Second, // idle carets hide their names
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			here := map[string]bool{}
			for _, id := range s.hub.Watchers(key) {
				_, tab, _ := strings.Cut(id, ".")
				here[tab] = true
			}
			rows, err := wiki.Editors(ctx, q, p.ID)
			if err != nil {
				return nil, err
			}
			var ids []string
			for _, r := range rows {
				ids = append(ids, r.Person)
			}
			who, err := people.Lookup(ctx, q, ids...)
			if err != nil {
				return nil, err
			}
			type caret struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Hue    int    `json:"hue"`
				V      int64  `json:"v"`
				Anchor int    `json:"anchor"`
				Head   int    `json:"head"`
				Idle   bool   `json:"idle"`
			}
			list := []caret{}
			now := time.Now()
			for _, r := range rows {
				if !here[r.Tab] {
					continue
				}
				pe := who[r.Person]
				list = append(list, caret{ID: r.Client, Name: pe.Name, Hue: pe.Hue, V: r.V, Anchor: r.Anchor, Head: r.Head, Idle: now.Sub(r.MovedAt) > 30*time.Second})
			}
			b, err := json.Marshal(list)
			if err != nil {
				return nil, err
			}
			sig, _ := json.Marshal(map[string]string{"_carets": string(b)})
			return sse.PatchSignals(sig, false), nil
		},
	}, nil
}

// titleView brings the editors the page's title as $_wkTitle, which the
// title field takes while the person is not typing in it (FR-86.2).
func (s *Server) titleView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, _, err := readable(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "pagetitle:" + p.ID,
		Topics: []string{wiki.PageTopic(p.ID)},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			cur, err := wiki.PageByID(ctx, querier(ctx, tx), p.ID)
			if err != nil {
				return nil, nil
			}
			b, _ := json.Marshal(map[string]string{"_wkTitle": cur.Title})
			return sse.PatchSignals(b, false), nil
		},
	}, nil
}

// pick answers an editor's picker (FR-100.1, FR-67, FR-103.1): pages by
// title, people by name, issues by key or summary. It reads, never
// writes: simple matches for now; search brings ranked ones.
func (s *Server) pick(w http.ResponseWriter, r *http.Request) {
	kind, text := r.URL.Query().Get("kind"), strings.TrimSpace(r.URL.Query().Get("q"))
	if len(text) > 100 {
		text = text[:100]
	}
	me := people.Actor(r.Context())
	ctx := r.Context()
	var items []views.PickItem
	err := s.db.View(ctx, func(tx *sql.Tx) error {
		if text == "" {
			return nil
		}
		low := strings.ToLower(text)
		switch kind {
		case "page":
			list, err := wiki.SearchPages(ctx, tx, text, 20)
			if err != nil {
				return err
			}
			for _, p := range list {
				if len(items) < 8 && canRead(ctx, tx, me, spaceOf(ctx, tx, p.ID)) == nil {
					items = append(items, views.PickItem{Kind: "page", ID: p.ID, Label: p.Title, Meta: p.SpaceName})
				}
			}
		case "person":
			rows, err := tx.QueryContext(ctx, `SELECT id FROM people WHERE instr(lower(display_name), ?) > 0 ORDER BY instr(lower(display_name), ?) != 1, display_name LIMIT 8`, low, low)
			if err != nil {
				return err
			}
			var ids []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
			}
			rows.Close()
			who, err := people.Lookup(ctx, tx, ids...)
			if err != nil {
				return err
			}
			for _, id := range ids {
				items = append(items, views.PickItem{Kind: "person", ID: id, Label: who[id].Name})
			}
		case "issue":
			rows, err := tx.QueryContext(ctx, `SELECT key, summary FROM issues WHERE deleted_at IS NULL AND (instr(lower(key), ?) > 0 OR instr(search, ?) > 0)
				ORDER BY instr(lower(key), ?) != 1, updated_at DESC LIMIT 8`, low, low, low)
			if err != nil {
				return err
			}
			for rows.Next() {
				var it views.PickItem
				if err := rows.Scan(&it.ID, &it.Meta); err != nil {
					rows.Close()
					return err
				}
				it.Kind, it.Label = "issue", it.ID
				items = append(items, it)
			}
			rows.Close()
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var buf bytes.Buffer
	if err := views.PickResults(items).Render(ctx, &buf); err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	w.Write(sse.PatchElements(buf.Bytes()))
}

func spaceOf(ctx context.Context, q sqlite.Querier, page string) string {
	p, err := wiki.PageByID(ctx, q, page)
	if err != nil {
		return ""
	}
	return p.Space
}

// WikiJobs runs the wiki's background work until ctx ends: every few
// seconds the versions that are due (CutDue: idle, the last editor gone,
// every 10 minutes), every minute the fold (NFR-43), every hour the
// sweep of editing tabs gone for a day. cmd/tickit runs it beside the
// sweep.
func (s *Server) WikiJobs(ctx context.Context) {
	cut := time.NewTicker(3 * time.Second)
	defer cut.Stop()
	// 20 editors at 5 characters a second commit about 7,000 times a
	// minute (benchmarks/RESULTS.md); 20 folds of FoldBatch every 10 s
	// take 24,000, each holding the writer a few milliseconds (NFR-43.3).
	fold := time.NewTicker(10 * time.Second)
	defer fold.Stop()
	sweep := time.NewTicker(time.Hour)
	defer sweep.Stop()
	do := func(cmd writer.Command) {
		if err := s.w.Do(ctx, cmd); err != nil && ctx.Err() == nil {
			s.log.Warn("wiki job", "err", err)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-cut.C:
			do(wiki.CutDue{At: time.Now(), Editing: s.Editing})
		case <-fold.C:
			for range 20 { // at most FoldBatch events a command
				f := &wiki.Fold{At: time.Now()}
				do(f)
				if f.Deleted == 0 {
					break
				}
				s.metrics.folded.Add(float64(f.Deleted))
				s.metrics.foldSeconds.Observe(f.Took.Seconds())
			}
		case <-sweep.C:
			do(wiki.SweepEditors{At: time.Now()})
		}
	}
}

var _ = slog.Default

// Times keeps the newest durations of something, for its percentiles:
// the writer's time per commit (benchmarks/RESULTS.md).
type Times struct {
	mu   sync.Mutex
	list []time.Duration
	next int
	n    int64
}

// CommitTimes are the writer's times of the newest commits.
var CommitTimes = &Times{}

// Observe records a duration.
func (t *Times) Observe(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.list) < 100000 {
		t.list = append(t.list, d)
	} else {
		t.list[t.next] = d
		t.next = (t.next + 1) % len(t.list)
	}
	t.n++
}

// Percentiles returns how many were observed and the p50, p95 and max of
// the newest.
func (t *Times) Percentiles() (n int64, p50, p95, max time.Duration) {
	t.mu.Lock()
	list := slices.Clone(t.list)
	n = t.n
	t.mu.Unlock()
	if len(list) == 0 {
		return n, 0, 0, 0
	}
	slices.Sort(list)
	return n, list[len(list)/2], list[len(list)*95/100], list[len(list)-1]
}

// Reset forgets every duration.
func (t *Times) Reset() {
	t.mu.Lock()
	t.list, t.next, t.n = nil, 0, 0
	t.mu.Unlock()
}
