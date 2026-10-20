package web

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// routesWiki registers spaces, pages, versions, page comments, templates
// and the trash (FR-75 to FR-105). Pages:
//
//	/spaces                              the spaces (FR-75)
//	/spaces/{key}                        a space: its home page beside its tree
//	/spaces/{key}/pages/{id}/{slug}      a page; the slug only reads well (titles change)
//	/spaces/{key}/pages/{id}/edit        the editor (routes_editor.go)
//	/spaces/{key}/pages/{id}/history     its versions (FR-87.2)
//	/spaces/{key}/pages/{id}/versions/n  a version (FR-87.3)
//	/spaces/{key}/pages/{id}/compare     two versions compared (FR-88)
//	/spaces/{key}/trash                  the trash (FR-94)
//	/pages/{id}                          a page link's address: redirects
//	/labels/{label}                      pages and issues with a label (FR-93)
//	/img/lib/{id}                        an image of the library (FR-84)
func (s *Server) routesWiki() {
	const probeID = "0192d000-0000-7000-8000-000000000001"
	s.get("GET /spaces", http.HandlerFunc(s.spacesPage))
	s.get("GET /spaces/{key}", http.HandlerFunc(s.spaceHome), "/spaces/ENG", "/spaces/nope")
	s.get("GET /spaces/{key}/pages/{id}", http.HandlerFunc(s.pagePage), "/spaces/ENG/pages/"+probeID)
	s.get("GET /spaces/{key}/pages/{id}/{slug}", http.HandlerFunc(s.pagePage), "/spaces/ENG/pages/"+probeID+"/a-page", "/spaces/ENG/pages/nope/x")
	s.get("GET /spaces/{key}/pages/{id}/history", http.HandlerFunc(s.historyPage), "/spaces/ENG/pages/"+probeID+"/history")
	s.get("GET /spaces/{key}/pages/{id}/versions/{n}", http.HandlerFunc(s.versionPage), "/spaces/ENG/pages/"+probeID+"/versions/1")
	s.get("GET /spaces/{key}/pages/{id}/compare", http.HandlerFunc(s.comparePage), "/spaces/ENG/pages/"+probeID+"/compare?from=1&to=2")
	s.get("GET /spaces/{key}/trash", http.HandlerFunc(s.trashPage), "/spaces/ENG/trash")
	s.get("GET /pages/{id}", http.HandlerFunc(s.pageRedirect), "/pages/"+probeID)
	s.get("GET /pages/{id}/edit", http.HandlerFunc(s.pageRedirect), "/pages/"+probeID+"/edit")
	s.get("GET /labels/{label}", http.HandlerFunc(s.labelPage), "/labels/release")
	s.get("GET /img/lib/{id}", http.HandlerFunc(s.libraryImage), "/img/lib/architecture-overview", "/img/lib/nope")

	s.post("POST /cmd/spaces/create", s.createSpace)
	s.post("POST /cmd/pages/create", s.createPage)
	s.post("POST /cmd/pages/rename", s.renamePage)
	s.post("POST /cmd/pages/move", s.movePage)
	s.post("POST /cmd/pages/labels", s.setLabels)
	s.post("POST /cmd/pages/watch", s.watchPage)
	s.post("POST /cmd/pages/trash", s.onPage(func(a action, page string) writer.Command { return wiki.TrashPage{By: a.By, Page: page} }))
	s.post("POST /cmd/pages/restore", s.onPage(func(a action, page string) writer.Command { return wiki.RestorePage{By: a.By, Page: page} }))
	s.post("POST /cmd/pages/delete", s.onPage(func(a action, page string) writer.Command { return wiki.DeletePage{By: a.By, Page: page} }))
	s.post("POST /cmd/pages/save-version", s.saveVersion)
	s.post("POST /cmd/pages/restore-version", s.restoreVersion)
	s.post("POST /cmd/pages/comment", s.addComment)
	s.post("POST /cmd/pages/comment/delete", s.deleteComment)
	s.post("POST /cmd/pages/comment/hide", s.hideComment)

	s.views.Register("spaces", s.spacesView)
	s.views.Register("page", s.pageHeadView)
	s.views.Register("pagedoc", s.readerView)
	s.views.Register("pagepeople", s.pagePeopleView)
	s.views.RegisterWithPresenceOf("tree", s.treeView, treePresence)
	s.views.Register("history", s.historyView)
	s.views.Register("trash", s.trashView)
}

// canRead answers access.ErrNotFound for a space the actor may not read.
func canRead(ctx context.Context, q sqlite.Querier, actor people.Person, space string) error {
	ok, err := access.Can(ctx, q, actor, access.Read, access.SpaceScope(space))
	switch {
	case err != nil:
		return err
	case !ok:
		return access.ErrNotFound
	}
	return nil
}

func canIn(ctx context.Context, q sqlite.Querier, actor people.Person, r access.Right, space string) bool {
	ok, _ := access.Can(ctx, q, actor, r, access.SpaceScope(space))
	return ok
}

// readable is a page the actor may read, with its space.
func readable(ctx context.Context, q sqlite.Querier, actor people.Person, id string) (wiki.Page, wiki.Space, error) {
	p, err := wiki.PageByID(ctx, q, id)
	if err != nil {
		return p, wiki.Space{}, err
	}
	sp, err := wiki.SpaceByID(ctx, q, p.Space)
	if err != nil {
		return p, sp, err
	}
	if p.State == wiki.Trashed && !access.SiteCan(actor, access.Moderate) {
		// A page a moderator hid shows to staff alone (DM-07.1).
		staff, err := people.Staff(ctx, q, p.TrashedBy)
		if err != nil {
			return p, sp, err
		}
		if staff {
			return p, sp, access.ErrNotFound
		}
	}
	return p, sp, canRead(ctx, q, actor, sp.ID)
}

// The spaces.

func (s *Server) spacesPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		list, err := readableSpaces(ctx, q, me)
		if err != nil {
			return Page{}, err
		}
		return Page{Title: "Spaces", Nav: "spaces", Views: []string{"spaces:"}, Body: views.Spaces(views.SpacesProps{
			Spaces: list, CanCreate: access.SiteCan(me, access.CreateScope), Now: time.Now(),
		})}, nil
	})
}

func readableSpaces(ctx context.Context, q sqlite.Querier, me people.Person) ([]wiki.Space, error) {
	all, err := wiki.Spaces(ctx, q)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(sp wiki.Space) bool { return canRead(ctx, q, me, sp.ID) != nil }), nil
}

// spacesView is the list of the open spaces, the same for everyone.
func (s *Server) spacesView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	return live.View{
		Key:    "spaces",
		Topics: []string{wiki.SpacesTopic, wiki.PagesTopic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			all, err := wiki.Spaces(ctx, querier(ctx, tx))
			if err != nil {
				return nil, err
			}
			all = slices.DeleteFunc(all, func(sp wiki.Space) bool { return !sp.Open })
			return patch(ctx, views.SpacesRegion(all, time.Now()))
		},
	}, nil
}

type createSpaceRequest struct {
	Payload
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// createSpace creates an open space (FR-76) and goes to it.
func (s *Server) createSpace(w http.ResponseWriter, r *http.Request) {
	var req createSpaceRequest
	a, ok := s.decode(w, r, classCreateScope, &req)
	if !ok {
		return
	}
	s.actResult(w, r, a, wiki.CreateSpace{By: a.By, Key: req.Key, Name: req.Name, Description: req.Description, Open: true}, func(res []byte) string {
		var m map[string]string
		json.Unmarshal(res, &m)
		return "/spaces/" + m["key"]
	})
}

// A space and its pages.

// errRedirect sends the browser elsewhere: a page whose address changed.
type errRedirect struct{ to string }

func (e errRedirect) Error() string { return "redirect to " + e.to }

// renderOrRedirect is render, with a redirect for a build that answers
// errRedirect.
func (s *Server) renderOrRedirect(w http.ResponseWriter, r *http.Request, build func(ctx context.Context, q sqlite.Querier) (Page, error)) {
	var to string
	s.db.View(r.Context(), func(tx *sql.Tx) error {
		var re errRedirect
		if _, err := build(r.Context(), tx); errors.As(err, &re) {
			to = re.to
		}
		return nil
	})
	if to != "" {
		http.Redirect(w, r, to, http.StatusFound)
		return
	}
	s.render(w, r, http.StatusOK, build)
}

func (s *Server) spaceHome(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.renderOrRedirect(w, r, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		sp, err := wiki.SpaceByKey(ctx, q, r.PathValue("key"))
		if err != nil {
			return Page{}, err
		}
		if sp.Key != r.PathValue("key") {
			return Page{}, errRedirect{"/spaces/" + sp.Key}
		}
		if err := canRead(ctx, q, me, sp.ID); err != nil {
			return Page{}, err
		}
		p, err := wiki.PageByID(ctx, q, sp.Home)
		if err != nil {
			return Page{}, err
		}
		return s.pageView(ctx, q, me, p, sp, "home")
	})
}

func (s *Server) pagePage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.renderOrRedirect(w, r, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		p, sp, err := readable(ctx, q, me, r.PathValue("id"))
		if err != nil {
			return Page{}, err
		}
		if url := wiki.PageURL(sp.Key, p.ID, p.Title); r.URL.Path != url {
			return Page{}, errRedirect{url}
		}
		return s.pageView(ctx, q, me, p, sp, "")
	})
}

// pageRedirect sends a page link (/pages/<id>) to the page's address.
func (s *Server) pageRedirect(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	var to string
	err := s.db.View(r.Context(), func(tx *sql.Tx) error {
		p, sp, err := readable(r.Context(), tx, me, r.PathValue("id"))
		if err != nil {
			return err
		}
		to = wiki.PageURL(sp.Key, p.ID, p.Title)
		if strings.HasSuffix(r.URL.Path, "/edit") {
			to = wiki.EditURL(sp.Key, p.ID)
		}
		return nil
	})
	if err != nil {
		s.notFound(w, r)
		return
	}
	http.Redirect(w, r, to, http.StatusFound)
}

// pageView is a page in view (FR-80), and the views its stream watches.
func (s *Server) pageView(ctx context.Context, q sqlite.Querier, me people.Person, p wiki.Page, sp wiki.Space, section string) (Page, error) {
	props, err := s.pageProps(ctx, q, me, p, sp)
	if err != nil {
		return Page{}, err
	}
	if props.Body, err = s.readerHTML(ctx, q, p.ID); err != nil {
		return Page{}, err
	}
	if props.Refs, err = pageRefs(ctx, q, p.ID); err != nil {
		return Page{}, err
	}
	if props.Comments, err = comments(ctx, q, p.ID); err != nil {
		return Page{}, err
	}
	sidebar, err := s.sidebar(ctx, q, sp, p.ID, section)
	if err != nil {
		return Page{}, err
	}
	pageViews := []string{"page:" + p.ID, "pagedoc:" + p.ID, "pagepeople:" + p.ID, "tree:" + p.ID}
	if section == "home" {
		feed, err := s.ScopeActivity(ctx, q, access.SpaceScope(sp.ID), time.Now())
		if err != nil {
			return Page{}, err
		}
		props.Activity = views.Activity(feed)
		pageViews = append(pageViews, "activity-space:"+sp.ID)
	}
	return Page{
		Title: p.Title, Nav: "spaces", Sidebar: sidebar, SidebarLabel: "Space",
		Views: pageViews,
		Body:  views.WikiPage(props),
	}, nil
}

// pageProps is what every view of a page shows: its head, the viewer's
// rights, the dialogs' choices.
func (s *Server) pageProps(ctx context.Context, q sqlite.Querier, me people.Person, p wiki.Page, sp wiki.Space) (views.PageProps, error) {
	head, err := pageHead(ctx, q, p, sp)
	if err != nil {
		return views.PageProps{}, err
	}
	watching, err := wiki.Watching(ctx, q, p.ID, me.ID)
	if err != nil {
		return views.PageProps{}, err
	}
	props := views.PageProps{
		Page: p, Space: sp, URL: wiki.PageURL(sp.Key, p.ID, p.Title), Me: me, Head: head, Watching: watching,
		CanEdit:   canIn(ctx, q, me, access.EditIssues, sp.ID),
		CanAdmin:  canIn(ctx, q, me, access.TrashPage, sp.ID),
		Templates: wiki.Templates(), Now: time.Now(),
	}
	props.CanTrash = props.CanAdmin || p.CreatedBy == me.ID && canIn(ctx, q, me, access.TrashOwnPage, sp.ID)
	if props.CanEdit {
		if props.Targets, err = moveTargets(ctx, q, me, p.ID); err != nil {
			return props, err
		}
	}
	return props, nil
}

// pageHead is the shared head of a page.
func pageHead(ctx context.Context, q sqlite.Querier, p wiki.Page, sp wiki.Space) (views.PageHead, error) {
	anc, err := wiki.Ancestors(ctx, q, p.ID)
	if err != nil {
		return views.PageHead{}, err
	}
	crumbs := []ui.Crumb{{Label: sp.Name, Href: "/spaces/" + sp.Key, Icon: "space"}}
	for _, a := range anc {
		if !a.Home {
			crumbs = append(crumbs, ui.Crumb{Label: a.Title, Href: wiki.PageURL(sp.Key, a.ID, a.Title)})
		}
	}
	if !p.Home {
		crumbs = append(crumbs, ui.Crumb{Label: p.Title})
	}
	who, err := people.Lookup(ctx, q, p.CreatedBy, p.EditedBy)
	if err != nil {
		return views.PageHead{}, err
	}
	return views.PageHead{
		Crumbs: crumbs, Title: p.Title, Labels: p.Labels, State: p.State,
		Creator: who[p.CreatedBy], Created: p.CreatedAt, Editor: who[p.EditedBy], Edited: p.EditedAt,
		Versions: p.Versions, History: wiki.HistoryURL(sp.Key, p.ID), Now: time.Now(),
	}, nil
}

// pageRefs are the chips of the issues a page shows, and the pages that
// link to it.
func pageRefs(ctx context.Context, q sqlite.Querier, page string) (views.PageRefs, error) {
	var refs views.PageRefs
	keys, err := wiki.LinkedIssues(ctx, q, page)
	if err != nil {
		return refs, err
	}
	res, err := wiki.Resolve(ctx, q, doc.Refs{Issues: keys})
	if err != nil {
		return refs, err
	}
	for _, k := range keys {
		refs.Issues = append(refs.Issues, chipProps(k, res))
	}
	refs.LinkedFrom, err = wiki.LinkedFrom(ctx, q, page)
	return refs, err
}

// chipProps is an issue chip as internal/ui draws it.
func chipProps(key string, res doc.StaticResolver) ui.IssueChipProps {
	c, ok := res.Issues[key]
	switch {
	case !ok:
		return ui.IssueChipProps{Key: key, Href: "/" + key, State: ui.ChipUnknown}
	case c.Deleted:
		return ui.IssueChipProps{Key: key, Href: "/" + key, State: ui.ChipDeleted}
	}
	return ui.IssueChipProps{Key: key, Summary: c.Summary, Type: ui.IssueType(c.Type), Status: c.Status, Category: ui.StatusCategory(c.Category), Href: "/" + key}
}

// comments are a page's comments with their authors and bodies.
func comments(ctx context.Context, q sqlite.Querier, page string) ([]views.CommentItem, error) {
	list, err := wiki.Comments(ctx, q, page)
	if err != nil {
		return nil, err
	}
	var ids []string
	var refs doc.Refs
	docs := map[string]doc.Doc{}
	var walk func([]wiki.Comment)
	walk = func(cs []wiki.Comment) {
		for _, c := range cs {
			ids = append(ids, c.Author)
			if d, err := doc.ParseDoc(c.Body); err == nil && !c.Deleted && !c.Hidden {
				docs[c.ID] = d
				r := doc.References(d)
				refs.Issues, refs.People, refs.Pages = append(refs.Issues, r.Issues...), append(refs.People, r.People...), append(refs.Pages, r.Pages...)
			}
			walk(c.Replies)
		}
	}
	walk(list)
	who, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return nil, err
	}
	res, err := wiki.Resolve(ctx, q, refs)
	if err != nil {
		return nil, err
	}
	var conv func([]wiki.Comment) []views.CommentItem
	conv = func(cs []wiki.Comment) []views.CommentItem {
		out := make([]views.CommentItem, 0, len(cs))
		for _, c := range cs {
			it := views.CommentItem{Comment: c, Author: who[c.Author], Replies: conv(c.Replies)}
			if d, ok := docs[c.ID]; ok {
				it.HTML = doc.RenderHTML(d, res)
			}
			out = append(out, it)
		}
		return out
	}
	return conv(list), nil
}

// moveTargets are the places a page may move under: the top of each
// space the actor may edit in, and every page there but the page itself
// and its own pages.
func moveTargets(ctx context.Context, q sqlite.Querier, me people.Person, page string) ([]views.MoveTarget, error) {
	spaces, err := wiki.Spaces(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []views.MoveTarget
	for _, sp := range spaces {
		if sp.Archived || !canIn(ctx, q, me, access.EditIssues, sp.ID) {
			continue
		}
		out = append(out, views.MoveTarget{Value: "space:" + sp.ID, Label: sp.Name + " (top)"})
		nodes, err := wiki.Tree(ctx, q, sp.ID)
		if err != nil {
			return nil, err
		}
		var walk func([]*wiki.Node, int)
		walk = func(ns []*wiki.Node, depth int) {
			for _, n := range ns {
				if n.ID == page {
					continue
				}
				out = append(out, views.MoveTarget{Value: "page:" + n.ID, Label: n.Title, Depth: depth})
				walk(n.Children, depth+1)
			}
		}
		walk(nodes, 1)
	}
	return out, nil
}

// sidebar is the space, its overview and trash, and the page tree, which
// the tree view patches.
func (s *Server) sidebar(ctx context.Context, q sqlite.Querier, sp wiki.Space, current, section string) (templ.Component, error) {
	items, err := treeItems(ctx, q, sp, current)
	if err != nil {
		return nil, err
	}
	presenceRow := "" // a page's tree has the space's presence row (MP-02.1)
	if current != "" {
		presenceRow = views.PresenceID(spacePresenceKey(sp.ID))
	}
	return ui.SideNav(ui.SideNavProps{
		Title: sp.Name, Key: sp.Key, Href: "/spaces/" + sp.Key,
		Sections: []ui.NavSection{{Items: []ui.NavItem{
			{Label: "Overview", Href: "/spaces/" + sp.Key, Icon: "home", Current: section == "home"},
			{Label: "Trash", Href: "/spaces/" + sp.Key + "/trash", Icon: "trash", Current: section == "trash"},
		}}},
		Extra:       views.SpaceTree("Pages in "+sp.Name, items, presenceRow),
		Collapsible: true,
	}), nil
}

// treeItems is a space's tree with the current page highlighted and its
// ancestors open (FR-78.1).
func treeItems(ctx context.Context, q sqlite.Querier, sp wiki.Space, current string) ([]ui.TreeItem, error) {
	nodes, err := wiki.Tree(ctx, q, sp.ID)
	if err != nil {
		return nil, err
	}
	var conv func([]*wiki.Node) ([]ui.TreeItem, bool)
	conv = func(ns []*wiki.Node) ([]ui.TreeItem, bool) {
		out := make([]ui.TreeItem, 0, len(ns))
		found := false
		for _, n := range ns {
			children, under := conv(n.Children)
			it := ui.TreeItem{ID: n.ID, Label: n.Title, Href: wiki.PageURL(sp.Key, n.ID, n.Title), Current: n.ID == current, Children: children}
			if n.Home {
				it.Icon = "home"
			}
			it.Expanded = under || n.Home
			found = found || under || it.Current
			out = append(out, it)
		}
		return out, found
	}
	items, _ := conv(nodes)
	return items, nil
}

// spacePresenceKey names the presence row of a space's page tree: the
// people on any page of the space (MP-02.1), whose tree views join it.
func spacePresenceKey(space string) string { return "pages-in:" + space }

// treePresence is the presence row a tree view's key joins.
func treePresence(key string) string {
	parts := strings.Split(key, ":")
	return spacePresenceKey(parts[1])
}

// treeView is a space's tree as one page shows it, shared by the tabs on
// that page; a page created, renamed, moved or trashed touches its space
// (FR-78.3).
// Its key names the space too, for its presence row (treePresence).
func (s *Server) treeView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, sp, err := readable(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "tree:" + sp.ID + ":" + p.ID,
		Topics: []string{wiki.SpaceTopic(sp.ID), wiki.PageTopic(p.ID)},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			cur, err := wiki.PageByID(ctx, q, p.ID)
			if err != nil {
				return nil, nil
			}
			space, err := wiki.SpaceByID(ctx, q, cur.Space)
			if err != nil {
				return nil, err
			}
			items, err := treeItems(ctx, q, space, p.ID)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.PageTree("Pages in "+space.Name, items))
		},
	}, nil
}

// pageHeadView is a page's shared head, its references and comments
// (FR-80, FR-90), and its title as $_wkTitle for the editor (FR-86.2).
func (s *Server) pageHeadView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, sp, err := readable(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "page:" + p.ID,
		Topics: []string{wiki.PageTopic(p.ID), wiki.SpaceTopic(sp.ID), wiki.ChipsTopic(p.ID), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			cur, err := wiki.PageByID(ctx, q, p.ID)
			if err != nil {
				return nil, nil
			}
			space, err := wiki.SpaceByID(ctx, q, cur.Space)
			if err != nil {
				return nil, err
			}
			head, err := pageHead(ctx, q, cur, space)
			if err != nil {
				return nil, err
			}
			refs, err := pageRefs(ctx, q, cur.ID)
			if err != nil {
				return nil, err
			}
			list, err := comments(ctx, q, cur.ID)
			if err != nil {
				return nil, err
			}
			var out []byte
			for _, c := range []templ.Component{views.PageHeadRegion(head), views.PageRefsRegion(refs), views.CommentsRegion(list, time.Now())} {
				b, err := patch(ctx, c)
				if err != nil {
					return nil, err
				}
				out = append(out, b...)
			}
			return out, nil
		},
	}, nil
}

// The reader view (FR-80.3, Q18): the head document rendered once per
// tick for every reader, from the document cache, re-rendering only the
// blocks a commit changed (doc.BlockCache), and all of them when a chip,
// a name or a linked title changes.
type reader struct {
	mu    sync.Mutex
	cache doc.BlockCache
	key   string // the resolver's answers the cache rendered with
}

var readers = struct {
	sync.Mutex
	m map[string]*reader
}{m: map[string]*reader{}}

func readerOf(page string) *reader {
	readers.Lock()
	defer readers.Unlock()
	r := readers.m[page]
	if r == nil {
		if len(readers.m) >= 2000 {
			readers.m = map[string]*reader{}
		}
		r = &reader{}
		readers.m[page] = r
	}
	return r
}

// readerHTML renders a page's head document.
func (s *Server) readerHTML(ctx context.Context, q sqlite.Querier, page string) (string, error) {
	h, err := wiki.HeadOf(ctx, q, page)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	res, err := wiki.Resolve(ctx, q, doc.References(h.Doc))
	if err != nil {
		return "", err
	}
	key, _ := json.Marshal(res)
	rd := readerOf(page)
	rd.mu.Lock()
	defer rd.mu.Unlock()
	if string(key) != rd.key {
		rd.cache.Reset()
		rd.key = string(key)
	}
	return rd.cache.Render(h.Doc, res), nil
}

func (s *Server) readerView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, _, err := readable(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "pagedoc:" + p.ID,
		Topics: []string{wiki.DocTopic(p.ID), wiki.ChipsTopic(p.ID), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			html, err := s.readerHTML(ctx, querier(ctx, tx), p.ID)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.PageBodyRegion(html))
		},
	}, nil
}

// pagePeopleView is a page's presence row (MP-02), with the people who
// have it open in the editor marked "editing" (MP-07.3, FR-86.3): those
// whose tabs watch the page's carets view.
func (s *Server) pagePeopleView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, _, err := readable(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	key := "pagepeople:" + p.ID
	return live.View{
		Key:      key,
		Topics:   []string{people.Topic, wiki.CaretsTopic(p.ID)},
		Presence: true,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			editing := map[string]bool{}
			for _, id := range s.hub.Watchers(caretsKey(p.ID)) {
				editing[personOf(id)] = true
			}
			present, err := s.present(ctx, querier(ctx, tx), key, time.Now())
			if err != nil {
				return nil, err
			}
			list := make([]ui.PresenceEntry, 0, len(present))
			for _, pr := range present {
				e := views.PresenceEntry(pr)
				e.Editing = editing[pr.ID]
				list = append(list, e)
			}
			return patch(ctx, ui.PresenceRow(ui.PresenceProps{ID: views.PagePresenceID(p.ID), Label: "People on this page", People: list, Max: views.Shown, Shared: true}))
		},
	}, nil
}

// Versions.

func (s *Server) historyPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		p, sp, err := readable(ctx, q, me, r.PathValue("id"))
		if err != nil {
			return Page{}, err
		}
		props, err := s.pageProps(ctx, q, me, p, sp)
		if err != nil {
			return Page{}, err
		}
		vs, err := versionItems(ctx, q, sp, p)
		if err != nil {
			return Page{}, err
		}
		sidebar, err := s.sidebar(ctx, q, sp, p.ID, "")
		if err != nil {
			return Page{}, err
		}
		return Page{Title: "History of " + p.Title, Nav: "spaces", Sidebar: sidebar, SidebarLabel: "Space",
			Views: []string{"history:" + p.ID, "tree:" + p.ID}, Body: views.History(views.VersionsProps{PageProps: props, Versions: vs})}, nil
	})
}

func versionItems(ctx context.Context, q sqlite.Querier, sp wiki.Space, p wiki.Page) ([]views.VersionItem, error) {
	vs, err := wiki.Versions(ctx, q, p.ID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, v := range vs {
		ids = append(ids, v.Editors...)
	}
	who, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return nil, err
	}
	out := make([]views.VersionItem, 0, len(vs))
	for _, v := range vs {
		it := views.VersionItem{VersionInfo: v, URL: wiki.VersionURL(sp.Key, p.ID, v.Number)}
		for _, e := range v.Editors {
			it.Editors = append(it.Editors, who[e])
		}
		out = append(out, it)
	}
	return out, nil
}

func (s *Server) historyView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, _, err := readable(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "history:" + p.ID,
		Topics: []string{wiki.PageTopic(p.ID), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			cur, err := wiki.PageByID(ctx, q, p.ID)
			if err != nil {
				return nil, nil
			}
			sp, err := wiki.SpaceByID(ctx, q, cur.Space)
			if err != nil {
				return nil, err
			}
			vs, err := versionItems(ctx, q, sp, cur)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.HistoryRegion(views.VersionsProps{PageProps: views.PageProps{Page: cur, Space: sp, Now: time.Now()}, Versions: vs}))
		},
	}, nil
}

func (s *Server) versionPage(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 {
		s.notFound(w, r)
		return
	}
	s.oldVersion(w, r, 0, n)
}

func (s *Server) comparePage(w http.ResponseWriter, r *http.Request) {
	from, err1 := strconv.Atoi(r.URL.Query().Get("from"))
	to, err2 := strconv.Atoi(r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil || from < 1 || to < 1 {
		s.notFound(w, r)
		return
	}
	if from > to {
		from, to = to, from
	}
	s.oldVersion(w, r, from, to)
}

// oldVersion shows version n read-only (FR-87.3), or from compared with
// n (FR-88).
func (s *Server) oldVersion(w http.ResponseWriter, r *http.Request, from, n int) {
	me := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		p, sp, err := readable(ctx, q, me, r.PathValue("id"))
		if err != nil {
			return Page{}, err
		}
		props, err := s.pageProps(ctx, q, me, p, sp)
		if err != nil {
			return Page{}, err
		}
		v, d, err := wiki.Version(ctx, q, p.ID, n)
		if err != nil {
			return Page{}, err
		}
		vp := views.VersionProps{PageProps: props, Newest: p.Versions, To: n}
		items, err := versionItems(ctx, q, sp, p)
		if err != nil {
			return Page{}, err
		}
		for _, it := range items {
			if it.Number == v.Number {
				vp.Version = it
			}
		}
		refs := doc.References(d)
		var old doc.Doc
		if from > 0 {
			if _, old, err = wiki.Version(ctx, q, p.ID, from); err != nil {
				return Page{}, err
			}
			o := doc.References(old)
			refs = doc.Refs{Issues: append(refs.Issues, o.Issues...), People: append(refs.People, o.People...), Pages: append(refs.Pages, o.Pages...)}
		}
		res, err := wiki.Resolve(ctx, q, refs)
		if err != nil {
			return Page{}, err
		}
		if old != nil {
			diff := doc.Diff(old, d)
			vp.Compare, vp.From, vp.HTML, vp.Inserted, vp.Deleted = true, from, diff.HTML(res), diff.Inserted, diff.Deleted
		} else {
			vp.HTML = doc.RenderHTML(d, res)
		}
		sidebar, err := s.sidebar(ctx, q, sp, p.ID, "")
		if err != nil {
			return Page{}, err
		}
		title := "Version " + strconv.Itoa(n) + " of " + p.Title
		if from > 0 {
			title = "Versions " + strconv.Itoa(from) + " and " + strconv.Itoa(n) + " of " + p.Title
		}
		return Page{Title: title, Nav: "spaces", Sidebar: sidebar, SidebarLabel: "Space",
			Views: []string{"tree:" + p.ID}, Body: views.OldVersion(vp)}, nil
	})
}

// The trash.

func (s *Server) trashPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		sp, err := wiki.SpaceByKey(ctx, q, r.PathValue("key"))
		if err != nil {
			return Page{}, err
		}
		if err := canRead(ctx, q, me, sp.ID); err != nil {
			return Page{}, err
		}
		props, err := trashProps(ctx, q, sp)
		if err != nil {
			return Page{}, err
		}
		props.CanAdmin = canIn(ctx, q, me, access.TrashPage, sp.ID)
		sidebar, err := s.sidebar(ctx, q, sp, "", "trash")
		if err != nil {
			return Page{}, err
		}
		return Page{Title: "Trash of " + sp.Name, Nav: "spaces", Sidebar: sidebar, SidebarLabel: "Space",
			Views: []string{"trash:" + sp.ID}, Body: views.Trash(props)}, nil
	})
}

func trashProps(ctx context.Context, q sqlite.Querier, sp wiki.Space) (views.TrashProps, error) {
	list, err := wiki.Trash(ctx, q, sp.ID)
	if err != nil {
		return views.TrashProps{}, err
	}
	var ids []string
	for _, p := range list {
		ids = append(ids, p.TrashedBy)
	}
	who, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return views.TrashProps{}, err
	}
	props := views.TrashProps{Space: sp, Now: time.Now()}
	for _, p := range list {
		props.Pages = append(props.Pages, views.TrashItem{Page: p, By: who[p.TrashedBy]})
	}
	return props, nil
}

// trashView is a space's trash, one for its admins and one for the rest.
func (s *Server) trashView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	sp, err := wiki.SpaceByID(ctx, q, arg)
	if err != nil {
		return live.View{}, err
	}
	if err := canRead(ctx, q, actor, sp.ID); err != nil {
		return live.View{}, err
	}
	admin := canIn(ctx, q, actor, access.TrashPage, sp.ID)
	key := "trash:" + sp.ID
	if admin {
		key += ".admin"
	}
	return live.View{
		Key:    key,
		Topics: []string{wiki.SpaceTopic(sp.ID), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			props, err := trashProps(ctx, querier(ctx, tx), sp)
			if err != nil {
				return nil, err
			}
			props.CanAdmin = admin
			return patch(ctx, views.TrashRegion(props))
		},
	}, nil
}

// Labels (FR-93).

func (s *Server) labelPage(w http.ResponseWriter, r *http.Request) {
	label, ok := tracker.NormalizeLabel(r.PathValue("label"))
	if !ok {
		s.notFound(w, r)
		return
	}
	me := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		list, err := wiki.Labelled(ctx, q, label)
		if err != nil {
			return Page{}, err
		}
		list = slices.DeleteFunc(list, func(p wiki.Page) bool { return canRead(ctx, q, me, p.Space) != nil })
		rows, err := q.QueryContext(ctx, `SELECT i.key FROM issue_labels l JOIN issues i ON i.id = l.issue_id
			WHERE l.label = ? AND i.deleted_at IS NULL ORDER BY i.key LIMIT 200`, label)
		if err != nil {
			return Page{}, err
		}
		var keys []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return Page{}, err
			}
			keys = append(keys, k)
		}
		rows.Close()
		res, err := wiki.Resolve(ctx, q, doc.Refs{Issues: keys})
		if err != nil {
			return Page{}, err
		}
		var chips []ui.IssueChipProps
		for _, k := range keys {
			chips = append(chips, chipProps(k, res))
		}
		return Page{Title: "Label " + label, Nav: "spaces", Body: views.LabelPage(label, list, chips)}, nil
	})
}

// libraryImage serves an image of the library (FR-84), from the seed's
// images.json: SVG files under static/img/lib.
func (s *Server) libraryImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !wiki.ImageExists(id) {
		s.notFound(w, r)
		return
	}
	b, err := staticFiles.ReadFile("static/img/lib/" + id + ".svg")
	if err != nil {
		s.notFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml")
	h.Set("Cache-Control", "public, max-age=86400")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Write(b)
}

// Page actions: every one names its page in the payload.

type pageRequest struct {
	Payload
	Page string `json:"page"`
}

// onPage is the handler of an action on a page that takes nothing else.
func (s *Server) onPage(cmd func(a action, page string) writer.Command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req pageRequest
		a, ok := s.decode(w, r, classField, &req)
		if !ok {
			return
		}
		s.act(w, r, a, cmd(a, req.Page))
	}
}

type createPageRequest struct {
	Payload
	Space    string `json:"space"`
	Parent   string `json:"parent"`
	Title    string `json:"title"`
	Template string `json:"template"`
}

// createPage creates a page, blank or from a template (FR-81, FR-92),
// and goes to its editor, the cursor in the title.
func (s *Server) createPage(w http.ResponseWriter, r *http.Request) {
	var req createPageRequest
	a, ok := s.decode(w, r, classCreate, &req)
	if !ok {
		return
	}
	cmd := wiki.CreatePage{By: a.By, Space: req.Space, Parent: req.Parent, Title: req.Title}
	if req.Template != "" {
		for _, t := range wiki.Templates() {
			if t.Name == req.Template {
				cmd.Doc, cmd.Template = t.Doc, t.Name
			}
		}
	}
	s.actResult(w, r, a, cmd, func(res []byte) string {
		var m map[string]string
		json.Unmarshal(res, &m)
		return "/pages/" + m["id"] + "/edit"
	})
}

type renameRequest struct {
	Payload
	Page  string `json:"page"`
	Title string `json:"title"`
}

// renamePage changes a page's title (FR-86.2); a taken title is refused
// next to the field, which keeps the person's text.
func (s *Server) renamePage(w http.ResponseWriter, r *http.Request) {
	var req renameRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.act(w, r, a, wiki.RenamePage{By: a.By, Page: req.Page, Title: req.Title})
}

type movePageRequest struct {
	Payload
	Page   string `json:"page"`
	Target string `json:"target"` // "space:<id>" for its top, or "page:<id>"
	Place  string `json:"place"`  // "first" or "last"
}

// movePage moves a page under another, or to the top of a space (FR-79).
func (s *Server) movePage(w http.ResponseWriter, r *http.Request) {
	var req movePageRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	cmd := wiki.MovePage{By: a.By, Page: req.Page, First: req.Place == "first"}
	kind, id, _ := strings.Cut(req.Target, ":")
	switch kind {
	case "space":
		cmd.Space = id
	case "page":
		parent, err := s.db.ViewValue(r.Context(), func(tx *sql.Tx) (wiki.Page, error) { return wiki.PageByID(r.Context(), tx, id) })
		if err != nil {
			http.Error(w, "bad target", http.StatusBadRequest)
			return
		}
		cmd.Space, cmd.Parent = parent.Space, parent.ID
	}
	if !wiki.ValidID(cmd.Space) {
		http.Error(w, "bad target", http.StatusBadRequest)
		return
	}
	s.act(w, r, a, cmd)
}

type labelsRequest struct {
	Payload
	Page   string   `json:"page"`
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}

// setLabels adds and removes a page's labels (FR-93).
func (s *Server) setLabels(w http.ResponseWriter, r *http.Request) {
	var req labelsRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if len(req.Add)+len(req.Remove) > wiki.MaxLabels {
		http.Error(w, "too many labels", http.StatusBadRequest)
		return
	}
	s.act(w, r, a, wiki.SetPageLabels{By: a.By, Page: req.Page, Add: req.Add, Remove: req.Remove})
}

type watchPageRequest struct {
	Payload
	Page  string `json:"page"`
	Watch bool   `json:"watch"`
}

// watchPage starts or ends the person's watch of a page (FR-95).
func (s *Server) watchPage(w http.ResponseWriter, r *http.Request) {
	var req watchPageRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.act(w, r, a, wiki.WatchPage{By: a.By, Page: req.Page, Watch: req.Watch})
}

type versionRequest struct {
	Payload
	Page    string `json:"page"`
	Message string `json:"message"`
	Number  int    `json:"number"`
}

// saveVersion is "Save version" with an optional message (FR-87.1).
func (s *Server) saveVersion(w http.ResponseWriter, r *http.Request) {
	var req versionRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.act(w, r, a, wiki.CutVersion{By: a.By, Page: req.Page, Message: req.Message})
}

// restoreVersion makes an old version the current one (FR-89).
func (s *Server) restoreVersion(w http.ResponseWriter, r *http.Request) {
	var req versionRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if req.Number < 1 {
		http.Error(w, "bad version", http.StatusBadRequest)
		return
	}
	s.act(w, r, a, wiki.RestoreVersion{By: a.By, Page: req.Page, Number: req.Number})
}

type commentRequest struct {
	Payload
	Page    string `json:"page"`
	Parent  string `json:"parent"`
	Comment string `json:"comment"`
	Body    string `json:"body"`
	Hidden  bool   `json:"hidden"`
}

// addComment comments on a page or replies (FR-90): plain text, a
// paragraph per line.
func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	var req commentRequest
	a, ok := s.decode(w, r, classComment, &req)
	if !ok {
		return
	}
	body := tracker.PlainDoc(req.Body)
	if body == nil {
		body = json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`)
	}
	s.act(w, r, a, wiki.AddComment{By: a.By, Page: req.Page, Parent: req.Parent, Body: body})
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	var req commentRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.act(w, r, a, tracker.DeleteComment{By: a.By, Comment: req.Comment})
}

func (s *Server) hideComment(w http.ResponseWriter, r *http.Request) {
	var req commentRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.act(w, r, a, tracker.HideComment{By: a.By, Comment: req.Comment, Hidden: req.Hidden})
}

// newClient is the id an editing tab's caret shows to others: random,
// so tab ids stay private (docs/editor.md).
func newClient() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

var _ = sse.PatchSignals
