package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// routesInline registers inline comments on pages (FR-91): a comment on a
// range of the text, resolving and reopening one, and the view of a
// page's inline comments beside the reader and the editor. Replies, edits,
// deletes and hiding are the page comments' actions. The highlights come
// with the reader's view (pagedoc:<id>) and each editor's frame (its
// host's comments attribute), so they are at the version of the text they
// mark.
func (s *Server) routesInline() {
	s.post("POST /cmd/pages/comment/inline", s.addInlineComment)
	s.post("POST /cmd/pages/comment/resolve", s.resolveInline)
	s.views.Register("inline", s.inlineView)
}

// inlineScript is the page's module for inline comments: the selection,
// its Comment button and the highlights' clicks.
const inlineScript = "wiki/inline.js"

type inlineRequest struct {
	Payload
	Page  string `json:"page"`
	Epoch string `json:"epoch"`
	Base  int64  `json:"base"`
	From  int    `json:"from"`
	To    int    `json:"to"`
	Text  string `json:"text"`
	Body  string `json:"body"`
}

// addInlineComment comments on a range of a page's text (FR-91.1): plain
// text, a paragraph per line, as page comments. It answers the new
// comment's id as $_icMade, which the page opens beside its text.
func (s *Server) addInlineComment(w http.ResponseWriter, r *http.Request) {
	var req inlineRequest
	a, ok := s.decode(w, r, classComment, &req)
	if !ok {
		return
	}
	body := tracker.PlainDoc(req.Body)
	if body == nil {
		body = json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`)
	}
	cmd := wiki.AddInlineComment{By: a.By, Page: req.Page, Epoch: req.Epoch, Base: req.Base, From: req.From, To: req.To, Text: req.Text, Body: body}
	res, ok := s.run(w, r, a, cmd)
	switch {
	case !ok:
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
	case !res.OK:
		w.WriteHeader(http.StatusNoContent)
	default:
		var made struct{ ID string }
		json.Unmarshal(res.Value, &made)
		s.signals(w, map[string]any{"_icMade": made.ID})
	}
}

type resolveRequest struct {
	Payload
	Comment  string `json:"comment"`
	Resolved bool   `json:"resolved"`
}

// resolveInline resolves an inline comment or reopens it (FR-91.3).
func (s *Server) resolveInline(w http.ResponseWriter, r *http.Request) {
	var req resolveRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.act(w, r, a, wiki.ResolveInline{By: a.By, Comment: req.Comment, Resolved: req.Resolved})
}

// inlineView is a page's inline comments beside its text, shared by its
// readers and editors: on text in the order of the text, detached, and
// resolved (FR-91).
func (s *Server) inlineView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, _, err := readable(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "inline:" + p.ID,
		Topics: []string{wiki.InlineTopic(p.ID), people.Topic},
		Every:  time.Minute, // relative times
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			list, err := s.inlineItems(ctx, querier(ctx, tx), p.ID)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.InlineRegion(list, time.Now()))
		},
	}, nil
}

// inlineItems are a page's inline comments with their authors, bodies and
// who resolved them.
func (s *Server) inlineItems(ctx context.Context, q sqlite.Querier, page string) ([]views.InlineItem, error) {
	list, err := wiki.InlineComments(ctx, q, page)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	threads := make([]wiki.Comment, len(list))
	var resolvers []string
	for i, c := range list {
		threads[i] = c.Comment
		if c.ResolvedBy != "" {
			resolvers = append(resolvers, c.ResolvedBy)
		}
	}
	items, err := commentItems(ctx, q, threads)
	if err != nil {
		return nil, err
	}
	who, err := people.Lookup(ctx, q, resolvers...)
	if err != nil {
		return nil, err
	}
	out := make([]views.InlineItem, len(list))
	for i, c := range list {
		out[i] = views.InlineItem{CommentItem: items[i], Quote: c.Quote, Detached: c.Detached, Resolved: c.Resolved, ResolvedAt: c.ResolvedAt}
		if c.Resolved {
			out[i].ResolvedBy = who[c.ResolvedBy]
		}
	}
	return out, nil
}

// commentsJSON is an editor host's comments attribute: the highlights of
// the page's head document, at its version v (FR-91).
func commentsJSON(ctx context.Context, q sqlite.Querier, page string, v int64) (string, error) {
	hs, err := wiki.Highlights(ctx, q, page)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	type mark struct {
		ID   string `json:"id"`
		From int    `json:"from"`
		To   int    `json:"to"`
	}
	list := make([]mark, 0, len(hs))
	for _, h := range hs {
		list = append(list, mark{h.ID, h.From, h.To})
	}
	b, err := json.Marshal(struct {
		V    int64  `json:"v"`
		List []mark `json:"list"`
	}{v, list})
	return string(b), err
}
