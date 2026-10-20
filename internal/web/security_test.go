package web_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/internal/web/webtest"
	"github.com/mbolli/tickit/internal/wiki"
)

// The security suite (docs/security.md, requirements 12.3): every action
// without the right answers 403 and changes nothing, from every kind of
// person (NFR-01.1); no cross-site POST gets through (NFR-03); the actor
// comes from the session (NFR-02.2). The routes come from the server's
// registration, and a route without a case fails the suite.

// need is the right an action wants, which says who lacks it.
type need int

const (
	public     need = iota // anyone: login, logout
	self                   // the person's own session or tab: nobody lacks it
	read                   // reading the scope (Viewer): a guest of a closed scope lacks it
	edit                   // a Member's: Viewers lack it too
	admin                  // the scope's Admin: Members lack it too
	ownComment             // the comment's author (member): Viewers and the scope's Admin lack it
	ownThing               // a filter or a notification of member's: viewer and the guest lack it
	moderator              // a moderator: the scope's Admin and members lack it
	siteAdmin              // a site admin: moderators lack it
	account                // an account's own: a guest lacks it
)

// secCase is one action, sent as each person who lacks its right.
type secCase struct {
	name    string // the action, and the field for /cmd/issues/{id}/{field}
	pattern string
	need    need
	guard   bool // through the demo's guard: a blocked guest is refused
	mustOK  bool // allowed with a first password to change
	outcome bool // through tabs.Act: the tab's row records it
	noop    bool // the rightful request changes no table but the bookkeeping
	reseeds bool // writes the seed, whose events the seeded people author
	by      who  // who may: the positive control
	open    bool // the positive control in the open scope
	req     func(f *fixture, open bool) (string, map[string]any)
	// fields of the body that name a person as the target, not the actor
	targets []string
	// setup prepares the positive control's world
	setup func(f *fixture)
}

func at(path string, kv ...any) func(*fixture, bool) (string, map[string]any) {
	return func(f *fixture, open bool) (string, map[string]any) {
		body := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			v := kv[i+1]
			if s, ok := v.(string); ok {
				v = idRef.ReplaceAllStringFunc(s, func(m string) string {
					pre, name, _ := strings.Cut(m, "@")
					return pre + f.id(name, open)
				})
			}
			if s, ok := v.(string); ok && strings.HasPrefix(s, "#") {
				n, _ := strconv.ParseInt(f.id(s[1:], open), 10, 64)
				v = n
			}
			body[kv[i].(string)] = v
		}
		p := path
		for strings.Contains(p, "{") {
			i, j := strings.Index(p, "{"), strings.Index(p, "}")
			p = p[:i] + f.id(p[i+1:j], open) + p[j+1:]
		}
		return p, body
	}
}

// idRef is a fixture's id in a case's body: "@issue", "issue:@issue";
// "#notification" is a number.
var idRef = regexp.MustCompile(`(?:^|:)@[a-zA-Z0-9.]+`)

const never = 1 << 40 // a version no field has yet: no conflict (MP-08)

func issueCase(field string, n need, by who, kv ...any) secCase {
	c := secCase{name: "issues/" + field, pattern: "POST /cmd/issues/{id}/{field}", need: n, guard: true, outcome: true, by: by,
		req: at("/cmd/issues/{issue}/"+field, append([]any{"seen", never}, kv...)...)}
	return c
}

// secCases is every action. The issue fields are issueCommand's.
var secCases = []secCase{
	{name: "watch", pattern: "POST /cmd/watch", need: read, by: wViewer, noop: true, req: at("/cmd/watch", "view", "issue:@issue")},
	{name: "unwatch", pattern: "POST /cmd/unwatch", need: self, by: wViewer, noop: true, req: at("/cmd/unwatch", "view", "issue:@issue")},
	{name: "login", pattern: "POST /cmd/login", need: public, by: wGuest, mustOK: true, req: at("/cmd/login", "email", "nobody@example.com", "password", "not the password")},
	{name: "logout", pattern: "POST /cmd/logout", need: public, by: wViewer, mustOK: true, req: at("/cmd/logout")},
	{name: "rename", pattern: "POST /cmd/people/rename", need: self, guard: true, outcome: true, by: wGuest, req: at("/cmd/people/rename", "name", "Renamed")},
	{name: "prefs", pattern: "POST /cmd/prefs", need: self, guard: true, outcome: true, by: wGuest, req: at("/cmd/prefs", "key", "theme", "value", "dark")},

	{name: "projects/create", pattern: "POST /cmd/projects/create", need: self, guard: true, outcome: true, by: wGuest, req: at("/cmd/projects/create", "key", "NEWP", "name", "New project")},
	{name: "projects/edit", pattern: "POST /cmd/projects/edit", need: admin, guard: true, outcome: true, by: wOwner, req: at("/cmd/projects/edit", "project", "@projKey", "name", "Renamed", "description", "", "lead", "")},
	{name: "projects/archive", pattern: "POST /cmd/projects/archive", need: admin, guard: true, outcome: true, by: wOwner, req: at("/cmd/projects/archive", "project", "@projKey", "archived", true)},
	{name: "projects/component", pattern: "POST /cmd/projects/component", need: admin, guard: true, outcome: true, by: wOwner, req: at("/cmd/projects/component", "project", "@projKey", "name", "ui")},
	{name: "projects/component/remove", pattern: "POST /cmd/projects/component/remove", need: admin, guard: true, outcome: true, by: wOwner, req: at("/cmd/projects/component/remove", "project", "@projKey", "name", "core")},
	{name: "projects/wip", pattern: "POST /cmd/projects/wip", need: admin, guard: true, outcome: true, by: wOwner, req: at("/cmd/projects/wip", "project", "@projKey", "status", "inprogress", "limit", 3)},
	{name: "projects/member", pattern: "POST /cmd/projects/member", need: admin, guard: true, outcome: true, by: wOwner, targets: []string{"person"}, req: at("/cmd/projects/member", "project", "@projKey", "person", "@victim", "role", "member")},
	{name: "projects/member/remove", pattern: "POST /cmd/projects/member/remove", need: admin, guard: true, outcome: true, by: wOwner, targets: []string{"person"}, req: at("/cmd/projects/member/remove", "project", "@projKey", "person", "@member")},
	{name: "board/move", pattern: "POST /cmd/board/move", need: edit, guard: true, outcome: true, by: wMember, req: func(f *fixture, open bool) (string, map[string]any) {
		return "/cmd/board/move", map[string]any{"issue": f.id("projKey", open) + "-1", "status": "inprogress", "before": "", "after": "", "seen": never}
	}},
	{name: "backlog/plan", pattern: "POST /cmd/backlog/plan", need: edit, guard: true, outcome: true, by: wMember, req: func(f *fixture, open bool) (string, map[string]any) {
		return "/cmd/backlog/plan", map[string]any{"project": f.id("projKey", open), "issues": []any{map[string]any{"issue": f.id("projKey", open) + "-1", "seen": never}}, "sprint": f.id("sprint", open)}
	}},
	{name: "sprints/create", pattern: "POST /cmd/sprints/create", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/sprints/create", "project", "@projKey", "name", "Next")},
	{name: "sprints/start", pattern: "POST /cmd/sprints/start", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/sprints/start", "project", "@projKey", "sprint", "@sprint"),
		setup: func(f *fixture) {
			f.e.Do(f.t, tracker.CompleteSprint{By: f.by(wOwner), Project: f.id("proj", false), Sprint: f.id("sprint2", false)})
		}},
	{name: "sprints/complete", pattern: "POST /cmd/sprints/complete", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/sprints/complete", "project", "@projKey", "sprint", "@sprint2", "move_to", "")},

	{name: "notify/read", pattern: "POST /cmd/notify/read", need: ownThing, guard: true, outcome: true, by: wMember, req: at("/cmd/notify/read", "id", "#notification")},
	{name: "notify/dismiss", pattern: "POST /cmd/notify/dismiss", need: ownThing, guard: true, outcome: true, by: wMember, req: at("/cmd/notify/dismiss", "id", "#notification")},
	{name: "notify/read-all", pattern: "POST /cmd/notify/read-all", need: self, guard: true, outcome: true, by: wMember, req: at("/cmd/notify/read-all")},
	{name: "notify/dismiss-all", pattern: "POST /cmd/notify/dismiss-all", need: self, guard: true, outcome: true, by: wMember, req: at("/cmd/notify/dismiss-all")},
	{name: "notify/open", pattern: "POST /cmd/notify/open", need: ownThing, guard: true, outcome: true, by: wMember, req: at("/cmd/notify/open", "id", "#notification")},
	{name: "notify/seen", pattern: "POST /cmd/notify/seen", need: self, guard: true, outcome: true, by: wMember, req: at("/cmd/notify/seen", "subject", "@issue")},

	{name: "moderation/act", pattern: "POST /cmd/moderation/act", need: moderator, guard: true, outcome: true, by: wMod, open: true, req: at("/cmd/moderation/act", "action", "hide", "kind", "issue", "id", "@issue", "reason", "spam")},
	{name: "moderation/block", pattern: "POST /cmd/moderation/block", need: moderator, guard: true, outcome: true, by: wMod, open: true, targets: []string{"person"}, req: func(f *fixture, open bool) (string, map[string]any) {
		f.client(wGuest)
		return "/cmd/moderation/block", map[string]any{"person": f.p[wGuest].ID, "reason": "spam"}
	}},
	{name: "moderation/unblock", pattern: "POST /cmd/moderation/unblock", need: moderator, guard: true, outcome: true, by: wMod, open: true, targets: []string{"person"}, req: func(f *fixture, open bool) (string, map[string]any) {
		f.client(wBlocked)
		return "/cmd/moderation/unblock", map[string]any{"person": f.p[wBlocked].ID}
	}},

	{name: "spaces/create", pattern: "POST /cmd/spaces/create", need: self, guard: true, outcome: true, by: wGuest, req: at("/cmd/spaces/create", "key", "NEWS", "name", "News")},
	{name: "pages/create", pattern: "POST /cmd/pages/create", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/pages/create", "space", "@space", "parent", "@home", "title", "A new page")},
	{name: "pages/rename", pattern: "POST /cmd/pages/rename", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/pages/rename", "page", "@page", "title", "Renamed")},
	{name: "pages/move", pattern: "POST /cmd/pages/move", need: edit, guard: true, outcome: true, by: wMember, req: func(f *fixture, open bool) (string, map[string]any) {
		return "/cmd/pages/move", map[string]any{"page": f.id("mpage", open), "target": "page:" + f.id("page", open), "place": "last"}
	}},
	{name: "pages/labels", pattern: "POST /cmd/pages/labels", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/pages/labels", "page", "@page", "add", []string{"release"})},
	{name: "pages/watch", pattern: "POST /cmd/pages/watch", need: read, guard: true, outcome: true, by: wViewer, req: at("/cmd/pages/watch", "page", "@page", "watch", true)},
	{name: "pages/trash", pattern: "POST /cmd/pages/trash", need: admin, guard: true, outcome: true, by: wOwner, req: at("/cmd/pages/trash", "page", "@page")},
	{name: "pages/restore", pattern: "POST /cmd/pages/restore", need: admin, guard: true, outcome: true, by: wOwner, req: at("/cmd/pages/restore", "page", "@tpage")},
	{name: "pages/delete", pattern: "POST /cmd/pages/delete", need: admin, guard: true, outcome: true, by: wOwner, req: at("/cmd/pages/delete", "page", "@tpage")},
	{name: "pages/save-version", pattern: "POST /cmd/pages/save-version", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/pages/save-version", "page", "@page", "message", "Saved"),
		setup: func(f *fixture) {
			f.e.Do(f.t, wiki.RenamePage{By: f.by(wOwner), Page: f.id("page", false), Title: "Another title"})
		}},
	{name: "pages/restore-version", pattern: "POST /cmd/pages/restore-version", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/pages/restore-version", "page", "@page", "number", 1),
		setup: func(f *fixture) {
			f.e.Do(f.t, wiki.SetDocument{By: f.by(wOwner), Page: f.id("page", false), Doc: tracker.PlainDoc("Another text"), Message: "Two"})
		}},
	{name: "pages/comment", pattern: "POST /cmd/pages/comment", need: read, guard: true, outcome: true, by: wViewer, req: at("/cmd/pages/comment", "page", "@page", "body", "A reply")},
	{name: "pages/comment/delete", pattern: "POST /cmd/pages/comment/delete", need: ownComment, guard: true, outcome: true, by: wMember, req: at("/cmd/pages/comment/delete", "comment", "@pcomment")},
	{name: "pages/comment/hide", pattern: "POST /cmd/pages/comment/hide", need: admin, guard: true, outcome: true, by: wOwner, req: at("/cmd/pages/comment/hide", "comment", "@pcomment", "hidden", true)},
	{name: "pages/open", pattern: "POST /cmd/pages/{id}/open", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/pages/{page}/open", "base", 0, "client", "0123456789ab")},
	{name: "pages/commit", pattern: "POST /cmd/pages/{id}/commit", need: edit, guard: true, by: wMember, req: commitReq},
	{name: "pages/presence", pattern: "POST /cmd/pages/{id}/presence", need: self, guard: true, by: wMember, noop: true, req: func(f *fixture, open bool) (string, map[string]any) {
		return "/cmd/pages/" + f.id("page", open) + "/presence", map[string]any{"epoch": epochOf(f, f.id("page", open)), "ack": 0, "v": 0, "anchor": 1, "head": 1, "seq": 1}
	}},

	{name: "issues/create", pattern: "POST /cmd/issues/create", need: edit, guard: true, outcome: true, by: wMember, req: at("/cmd/issues/create", "project", "@proj", "type", "task", "summary", "A new one")},
	issueCase("summary", edit, wMember, "value", "Changed"),
	issueCase("description", edit, wMember, "value", "Changed *text*"),
	issueCase("type", edit, wMember, "value", "story"),
	issueCase("status", edit, wMember, "value", "inprogress"),
	issueCase("priority", edit, wMember, "value", "high"),
	issueCase("assignee", edit, wMember, "value", "me"),
	issueCase("parent", edit, wMember, "value", "@epic"),
	issueCase("due", edit, wMember, "value", "2030-01-31"),
	issueCase("points", edit, wMember, "value", "3"),
	issueCase("estimate", edit, wMember, "value", "2h"),
	issueCase("remaining", edit, wMember, "value", "1h 30m"),
	issueCase("sprint", edit, wMember, "value", "@sprint"),
	issueCase("labels", edit, wMember, "value", "urgent", "ref", "add"),
	issueCase("components", edit, wMember, "value", "core", "ref", "add"),
	issueCase("watch", read, wViewer),
	{name: "issues/link", pattern: "POST /cmd/issues/{id}/{field}", need: edit, guard: true, outcome: true, by: wMember, req: func(f *fixture, open bool) (string, map[string]any) {
		return "/cmd/issues/" + f.id("issue", open) + "/link", map[string]any{"value": f.id("projKey", open) + "-3", "ref": "blocks"}
	}},
	issueCase("unlink", edit, wMember, "value", "@issue2", "ref", "relates"),
	issueCase("subtask", edit, wMember, "value", "A part"),
	issueCase("comment", read, wViewer, "value", "A comment"),
	issueCase("cedit", ownComment, wMember, "value", "Edited", "ref", "@comment"),
	issueCase("cdelete", ownComment, wMember, "ref", "@comment"),
	issueCase("chide", admin, wOwner, "value", "hide", "ref", "@comment"),
	issueCase("delete", admin, wOwner),
	{name: "issues/typing", pattern: "POST /cmd/issues/{id}/{field}", need: read, guard: true, by: wViewer, req: at("/cmd/issues/{issue}/typing", "field", "comment")},

	{name: "list/view", pattern: "POST /cmd/list/view", need: self, by: wViewer, noop: true, req: at("/cmd/list/view", "view", "", "query", "project=SEC")},
	{name: "filters/save", pattern: "POST /cmd/filters/save", need: self, guard: true, outcome: true, by: wViewer, req: at("/cmd/filters/save", "name", "Mine too", "query", "assignee=me")},
	{name: "filters/rename", pattern: "POST /cmd/filters/rename", need: ownThing, guard: true, outcome: true, by: wMember, req: at("/cmd/filters/rename", "filter", "@filter", "name", "Renamed")},
	{name: "filters/delete", pattern: "POST /cmd/filters/delete", need: ownThing, guard: true, outcome: true, by: wMember, req: at("/cmd/filters/delete", "filter", "@filter")},
	{name: "filters/star", pattern: "POST /cmd/filters/star", need: self, guard: true, outcome: true, by: wViewer, req: at("/cmd/filters/star", "filter", "@filter", "star", true)},

	{name: "admin/create", pattern: "POST /cmd/admin/create", need: siteAdmin, guard: true, outcome: true, by: wRoot, req: at("/cmd/admin/create", "email", "new@example.com", "username", "newbie", "name", "New", "role", "none", "password", "a first password")},
	{name: "admin/role", pattern: "POST /cmd/admin/role", need: siteAdmin, guard: true, outcome: true, by: wRoot, targets: []string{"account"}, req: at("/cmd/admin/role", "account", "@victim", "role", "moderator")},
	{name: "admin/active", pattern: "POST /cmd/admin/active", need: siteAdmin, guard: true, outcome: true, by: wRoot, targets: []string{"account"}, req: at("/cmd/admin/active", "account", "@victim", "active", false)},
	{name: "admin/reset", pattern: "POST /cmd/admin/reset", need: siteAdmin, guard: true, outcome: true, reseeds: true, by: wRoot, req: at("/cmd/admin/reset")},
	{name: "account/password", pattern: "POST /cmd/account/password", need: account, guard: true, outcome: true, mustOK: true, by: wViewer, req: at("/cmd/account/password", "current", fixturePassword, "password", "a new password here")},
	{name: "account/end-session", pattern: "POST /cmd/account/end-session", need: account, guard: true, outcome: true, by: wViewer, req: at("/cmd/account/end-session", "session", "@session"),
		setup: func(f *fixture) {
			c := webtest.Client(f.t)
			f.e.Login(f.t, c, "viewer@example.com", fixturePassword)
			list, err := people.Sessions(context.Background(), f.e.DB.R, f.p[wViewer].ID, f.sid(wViewer), time.Now())
			if err != nil {
				f.t.Fatal(err)
			}
			for _, s := range list {
				if !s.Current {
					f.ids["session"] = s.ID
				}
			}
		}},
}

// commitReq is a commit of one character on the page as it is.
func commitReq(f *fixture, open bool) (string, map[string]any) {
	page := f.id("page", open)
	var h wiki.Head
	f.e.DB.View(context.Background(), func(tx *sql.Tx) (err error) { h, err = wiki.HeadOf(context.Background(), tx, page); return err })
	return "/cmd/pages/" + page + "/commit", map[string]any{"epoch": h.Epoch, "base": h.Version, "ref": fmt.Sprintf("r%d", f.n.Add(1)), "client": "0123456789ab",
		"steps": []any{map[string]any{"stepType": "replace", "from": 1, "to": 1, "slice": map[string]any{"content": []any{map[string]any{"type": "text", "text": "x"}}}}}}
}

func epochOf(f *fixture, page string) string {
	var h wiki.Head
	f.e.DB.View(context.Background(), func(tx *sql.Tx) (err error) { h, err = wiki.HeadOf(context.Background(), tx, page); return err })
	return h.Epoch
}

// issueFields are the fields of POST /cmd/issues/{id}/{field}
// (issueCommand and issueTyping): each needs a case.
var issueFields = []string{"summary", "description", "type", "status", "priority", "assignee", "parent", "due", "points", "estimate",
	"remaining", "sprint", "labels", "components", "watch", "link", "unlink", "subtask", "comment", "cedit", "cdelete", "chide", "delete", "typing"}

// lacking is who lacks c's right, and whether they act in the open scope.
func (c secCase) lacking() []struct {
	w    who
	open bool
} {
	type x = struct {
		w    who
		open bool
	}
	var out []x
	switch c.need {
	case read:
		out = append(out, x{wGuest, false})
	case edit:
		out = append(out, x{wGuest, false}, x{wViewer, false})
	case admin:
		out = append(out, x{wGuest, false}, x{wViewer, false}, x{wMember, false})
	case ownComment:
		out = append(out, x{wGuest, false}, x{wViewer, false}, x{wOwner, false})
	case ownThing:
		out = append(out, x{wGuest, false}, x{wViewer, false})
	case moderator:
		out = append(out, x{wGuest, true}, x{wMember, true}, x{wOwner, true})
	case siteAdmin:
		out = append(out, x{wGuest, true}, x{wOwner, true}, x{wMod, true})
	case account:
		out = append(out, x{wGuest, true})
	}
	if c.guard && c.need != moderator && c.need != siteAdmin && c.need != account && c.need != ownThing {
		out = append(out, x{wBlocked, true})
	}
	if !c.mustOK && c.need != public {
		out = append(out, x{wMust, true})
	}
	return out
}

// TestEveryActionCovered: every registered action has a case, and every
// field of an issue too.
func TestEveryActionCovered(t *testing.T) {
	e := startWiki(t)
	for _, p := range e.Server.Actions() {
		if !slices.ContainsFunc(secCases, func(c secCase) bool { return c.pattern == p }) {
			t.Errorf("NFR-01.1: the action %s has no case in the security suite", p)
		}
	}
	for _, f := range issueFields {
		if !slices.ContainsFunc(secCases, func(c secCase) bool { return c.name == "issues/"+f }) {
			t.Errorf("NFR-01.1: the issue field %s has no case in the security suite", f)
		}
	}
	if len(e.Server.Actions()) < 50 {
		t.Errorf("only %d actions registered", len(e.Server.Actions()))
	}
}

// NFR-01.1, requirements 12.3: every action, sent by each kind of person
// without its right, answers 403 (404 for a scope the person may not
// read, which does not tell that it exists) and changes nothing.
func TestEveryActionRefused(t *testing.T) {
	f := newFixture(t)
	for _, c := range secCases {
		for _, l := range c.lacking() {
			path, body := c.req(f, l.open)
			body["tab"], body["op"] = "sec", fmt.Sprintf("op%d", f.n.Add(1))
			f.client(l.w)
			before := f.state()
			code, answer := f.post(l.w, path, body)
			want := []int{http.StatusForbidden}
			if l.w == wGuest && !l.open {
				want = append(want, http.StatusNotFound)
				if c.name == "pages/commit" {
					want = append(want, http.StatusConflict) // as for a removed page (docs/editor.md)
				}
			}
			if !slices.Contains(want, code) {
				t.Errorf("%s as %s: %d %.80s, want %v", c.name, l.w, code, strings.TrimSpace(answer), want)
			}
			if diff := changed(before, f.state()); len(diff) > 0 {
				t.Errorf("%s as %s changed %v", c.name, l.w, diff)
			}
		}
	}
}

// NFR-01.1: the cases are right: whoever has the right gets through, and
// the request changes something. NFR-02.2: fields that name an actor, a
// reporter, an author or a session are ignored: every event is by the
// session's person, and none names the person the fields forge.
func TestEveryActionAllowed(t *testing.T) {
	forged := []string{"actor", "reporter", "author", "person", "by", "sid", "session", "user", "owner", "recipient", "creator", "account"}
	for _, c := range secCases {
		if c.need == public {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			c := c
			f.client(c.by)
			if c.setup != nil {
				c.setup(f)
			}
			path, body := c.req(f, c.open)
			body["tab"], body["op"] = "sec", "allowed"
			victim, victimSid := f.p[wOther].ID, f.sid(wOther)
			for _, k := range forged {
				if _, ok := body[k]; !ok && !slices.Contains(c.targets, k) {
					body[k] = map[bool]string{true: victimSid, false: victim}[k == "sid" || k == "session"]
				}
			}
			f.client(c.by)
			var last int64
			f.e.DB.R.QueryRow(`SELECT coalesce(max(position), 0) FROM lith_events`).Scan(&last)
			before := f.state()
			code, answer := f.post(c.by, path, body)
			if code != http.StatusOK && code != http.StatusNoContent {
				t.Fatalf("%s as %s: %d %.200s", c.name, c.by, code, answer)
			}
			if c.outcome {
				if o, ok := f.outcomeOf(c.by, "sec", "allowed"); !ok || !o.OK {
					t.Fatalf("%s as %s: the outcome %+v (%v)", c.name, c.by, o, ok)
				}
			}
			if diff := changed(before, f.state()); len(diff) == 0 && !c.noop {
				t.Errorf("%s as %s changed nothing", c.name, c.by)
			}
			// NFR-02.2: the actor is the session's.
			rows, err := f.e.DB.R.Query(`SELECT type, coalesce(actor, ''), data FROM lith_events WHERE position > ?`, last)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			for rows.Next() {
				var typ, actor, data string
				rows.Scan(&typ, &actor, &data)
				if actor != f.p[c.by].ID && !(c.reseeds && strings.HasPrefix(actor, "s-")) {
					t.Errorf("NFR-02.2: %s by %s: the event %s names %s as its actor", c.name, c.by, typ, actor)
				}
				if strings.Contains(data, victim) && len(c.targets) == 0 {
					t.Errorf("NFR-02.2: %s: the event %s names the forged person: %s", c.name, typ, data)
				}
			}
			var forgedRows int
			f.e.DB.R.QueryRow(`SELECT count(*) FROM tab_state WHERE sid = ?`, victimSid).Scan(&forgedRows)
			if forgedRows > 0 {
				t.Errorf("NFR-02.2: %s wrote the tab of the forged session", c.name)
			}
			var logged int
			f.e.DB.R.QueryRow(`SELECT count(*) FROM security_log WHERE actor_id = ? AND at >= ?`, victim, time.Now().Add(-time.Minute).Unix()).Scan(&logged)
			if logged > 0 {
				t.Errorf("NFR-02.2: %s logged the forged person as its actor", c.name)
			}
		})
	}
}

// NFR-03, NFR-55.3: a POST from another site, from another origin, or
// with neither header answers 403 for every action and changes nothing.
func TestEveryActionCrossSite(t *testing.T) {
	f := newFixture(t)
	for _, w := range []who{wGuest, wMember, wRoot, wBlocked} {
		f.client(w)
	}
	before := f.state()
	for _, c := range secCases {
		for name, h := range map[string]http.Header{
			"cross-site":     {"Sec-Fetch-Site": {"cross-site"}},
			"same-site":      {"Sec-Fetch-Site": {"same-site"}},
			"another origin": {"Origin": {"https://evil.example"}},
			"neither":        {},
			"no Datastar":    {"Sec-Fetch-Site": {"same-origin"}, "No-Datastar": {"1"}},
		} {
			for _, w := range []who{wGuest, wMember, wRoot} {
				path, body := c.req(f, c.open)
				body["tab"], body["op"] = "sec", fmt.Sprintf("x%d", f.n.Add(1))
				req, _ := http.NewRequest("POST", f.e.URL+path, strings.NewReader(mustJSON(body)))
				req.Header = h.Clone()
				req.Header.Set("Content-Type", "application/json")
				if req.Header.Get("No-Datastar") == "" {
					req.Header.Set("Datastar-Request", "true")
				}
				res, err := f.client(w).Do(req)
				if err != nil {
					t.Fatal(err)
				}
				res.Body.Close()
				if res.StatusCode != http.StatusForbidden {
					t.Errorf("%s POST %s as %s: %d", name, c.name, w, res.StatusCode)
				}
			}
		}
	}
	if diff := changed(before, f.state()); len(diff) > 0 {
		t.Errorf("cross-site requests changed %v", diff)
	}
	var tabs int
	f.e.DB.R.QueryRow(`SELECT count(*) FROM tab_state WHERE tab_id = 'sec'`).Scan(&tabs)
	if tabs > 0 {
		t.Error("a cross-site request wrote a tab's row")
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
