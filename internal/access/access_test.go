package access_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/dbtest"
	"github.com/zweiundeins/tickit/internal/people"
)

var now = time.Unix(1_800_000_000, 0)

// The cast: one person for each column of section 3.2, and strangers.
var (
	viewer    = people.GuestOf("session-viewer")
	member    = people.Person{ID: "a-0192a000-0000-7000-8000-000000000001", Kind: people.Account, Name: "Mia Member", Role: people.NoRole}
	admin     = people.Person{ID: "a-0192a000-0000-7000-8000-000000000002", Kind: people.Account, Name: "Adam Admin", Role: people.NoRole}
	moderator = people.Person{ID: "a-0192a000-0000-7000-8000-000000000003", Kind: people.Account, Name: "Mo Moderator", Role: people.Moderator}
	siteAdmin = people.Person{ID: "a-0192a000-0000-7000-8000-000000000004", Kind: people.Account, Name: "Sia Siteadmin", Role: people.Admin}
	stranger  = people.GuestOf("session-stranger")
)

var (
	closed   = access.ProjectScope("closed")
	open     = access.ProjectScope("open")
	archived = access.ProjectScope("archived")
	space    = access.SpaceScope("space")
)

// setup makes the scopes: closed has a viewer, a member and an admin;
// open and archived are open (Q14), archived is archived.
func setup(t *testing.T) *dbtest.Env {
	t.Helper()
	e := dbtest.Open(t)
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		for _, g := range []people.Person{viewer, stranger} {
			if err := people.Ensure(tx, g, now); err != nil {
				return err
			}
		}
		for _, p := range []people.Person{member, admin, moderator, siteAdmin} {
			if _, err := tx.Exec(`INSERT INTO people (id, kind, display_name, hue, created_at, last_seen_at) VALUES (?, 'account', ?, 0, 0, 0)`, p.ID, p.Name); err != nil {
				return err
			}
		}
		for _, s := range []struct {
			scope access.Scope
			key   string
			open  bool
		}{{closed, "CLO", false}, {open, "OPN", true}, {archived, "ARC", true}, {space, "DOCS", true}} {
			if err := access.AddScope(tx, s.scope.Kind, s.scope.ID, s.key, s.open, admin.ID, now); err != nil {
				return err
			}
		}
		if err := access.SetMember(tx, closed, viewer.ID, access.Viewer, admin.ID, now); err != nil {
			return err
		}
		if err := access.SetMember(tx, closed, member.ID, access.Member, admin.ID, now); err != nil {
			return err
		}
		return access.SetArchived(tx, archived, true)
	}))
	e.Touched()
	return e
}

// Section 3.2, row by row, in a closed project: Viewer, Member, Admin of
// the project, Moderator and Site admin without a membership.
func TestRights(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	cast := []people.Person{viewer, member, admin, moderator, siteAdmin}
	for _, row := range []struct {
		name  string
		right access.Right
		yes   [5]bool
	}{
		{"read issues, boards, pages, comments and history", access.Read, [5]bool{true, true, true, true, true}},
		{"comment on issues and pages", access.Comment, [5]bool{true, true, true, true, true}},
		{"create and edit issues, move cards, create and edit pages", access.EditIssues, [5]bool{false, true, true, false, true}},
		{"delete an issue", access.DeleteIssue, [5]bool{false, false, true, true, true}},
		{"move a page they created to the trash", access.TrashOwnPage, [5]bool{false, true, true, true, true}},
		{"move a page to the trash", access.TrashPage, [5]bool{false, false, true, true, true}},
		{"restore a page version", access.RestoreVersion, [5]bool{false, true, true, false, true}},
		{"edit or delete their own comment", access.EditOwnComment, [5]bool{true, true, true, true, true}},
		{"edit or delete any comment", access.EditAnyComment, [5]bool{false, false, false, true, true}},
		{"hide other people's comments", access.HideComment, [5]bool{false, false, true, true, true}},
		{"configure workflow, board columns, components, templates", access.Configure, [5]bool{false, false, true, false, true}},
		{"manage members and roles", access.ManageMembers, [5]bool{false, false, true, false, true}},
		{"archive a project or a space", access.ArchiveScope, [5]bool{false, false, true, false, true}},
		{"block a guest, hide any content", access.Moderate, [5]bool{false, false, false, true, true}},
		{"manage accounts and site roles, reset the demo", access.Administer, [5]bool{false, false, false, false, true}},
	} {
		for i, p := range cast {
			s := closed
			if row.right == access.Moderate || row.right == access.Administer {
				s = access.Scope{} // site-wide
			}
			got, err := access.Can(ctx, e.DB.R, p, row.right, s)
			if err != nil {
				t.Fatal(err)
			}
			if got != row.yes[i] {
				t.Errorf("%s may %s: %t, want %t", p.Name, row.name, got, row.yes[i])
			}
		}
	}
}

// Q14, Q24: in an open scope everyone has a Member's rights; in a closed
// one a stranger has none; everyone may create a scope.
func TestOpenScopes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	can := func(p people.Person, r access.Right, s access.Scope) bool {
		t.Helper()
		ok, err := access.Can(ctx, e.DB.R, p, r, s)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !can(stranger, access.EditIssues, open) || !can(stranger, access.EditIssues, space) || !can(stranger, access.RestoreVersion, space) || can(stranger, access.DeleteIssue, open) {
		t.Error("a guest in an open scope is not a Member")
	}
	if !can(viewer, access.EditIssues, open) {
		t.Error("a Viewer elsewhere is no Member in an open scope")
	}
	if can(stranger, access.Read, closed) || can(stranger, access.Comment, closed) {
		t.Error("a stranger reads a closed scope")
	}
	if !can(admin, access.DeleteIssue, open) {
		t.Error("an open scope's Admin is only a Member")
	}
	for _, p := range []people.Person{stranger, member, moderator} {
		if !can(p, access.CreateScope, access.Scope{}) || !access.SiteCan(p, access.CreateScope) {
			t.Errorf("%s may not create a project", p.Name)
		}
	}
	for _, nobody := range []people.Person{{}, people.Seeded[0], {ID: "a-gone", Kind: people.Deleted}, {ID: member.ID, Kind: people.Account, Inactive: true}} {
		if can(nobody, access.Read, open) || access.SiteCan(nobody, access.CreateScope) {
			t.Errorf("%+v has rights", nobody)
		}
	}
	if _, err := access.Can(ctx, e.DB.R, stranger, access.Read, access.ProjectScope("nope")); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("an unknown scope: %v", err)
	}
	if access.SiteCan(siteAdmin, access.Read) {
		t.Error("SiteCan answers a scope's right")
	}
}

// FR-04.1: an archived scope is read-only, but its Admin may unarchive it.
func TestArchived(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for r, want := range map[access.Right]bool{access.Read: true, access.EditIssues: false, access.Comment: false, access.ArchiveScope: true, access.ManageMembers: true} {
		if got, _ := access.Can(ctx, e.DB.R, admin, r, archived); got != want {
			t.Errorf("right %d in an archived project: %t", r, got)
		}
	}
}

// require runs access.Require in a command and returns its error.
func require(e *dbtest.Env, p people.Person, r access.Right, s access.Scope) error {
	return e.W.Do(context.Background(), writer.Func(func(tx *writer.Tx) error { return access.Require(tx, p, r, s) }))
}

// NFR-01: a missing right is refused with a sentence that says why; a
// scope the actor may not read is not found.
func TestRequire(t *testing.T) {
	e := setup(t)
	for _, c := range []struct {
		p    people.Person
		r    access.Right
		s    access.Scope
		want string
	}{
		{member, access.DeleteIssue, closed, "Only admins of this project can delete issues."},
		{viewer, access.EditIssues, closed, "Only members of this project can change its issues."},
		{stranger, access.Configure, space, "Only admins of this space can change its settings."},
		{admin, access.EditIssues, archived, "This project is archived."},
		{member, access.Administer, access.Scope{}, "Only site admins can do this."},
	} {
		err := require(e, c.p, c.r, c.s)
		var refusal *writer.Refusal
		var denial interface{ Denied() bool }
		if !errors.As(err, &refusal) || refusal.Msg != c.want || !errors.As(err, &denial) || !denial.Denied() {
			t.Errorf("%s, right %d in %v: %v", c.p.Name, c.r, c.s, err)
		}
	}
	if err := require(e, stranger, access.Comment, closed); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("a stranger in a closed project: %v", err)
	}
	if err := require(e, member, access.Read, access.SpaceScope("nope")); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("an unknown space: %v", err)
	}
	if err := require(e, member, access.EditIssues, closed); err != nil {
		t.Errorf("a member edits: %v", err)
	}
}

// FR-02.2, FR-02.3, FR-05.2: a key is taken once per kind; the creator
// is Admin; the last Admin stays.
func TestScopes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		return access.AddScope(tx, access.Project, "web", "WEB", true, stranger.ID, now)
	}))
	if got := e.Touched(); !slices.Equal(got, []string{"person:" + stranger.ID, "project:web"}) {
		t.Errorf("a new project touched %v", got)
	}
	if r, err := access.RoleOf(ctx, e.DB.R, stranger.ID, access.ProjectScope("web")); err != nil || r != access.Admin {
		t.Errorf("the creator's role: %q %v", r, err)
	}
	for _, key := range []string{"WEB", "ARC"} {
		err := e.W.Do(ctx, writer.Func(func(tx *writer.Tx) error {
			return access.AddScope(tx, access.Project, "web2", key, true, stranger.ID, now)
		}))
		var refusal *writer.Refusal
		var field interface{ OnField() string }
		if !errors.As(err, &refusal) || refusal.Msg != "The key "+key+" is taken." || !errors.As(err, &field) || field.OnField() != "key" {
			t.Errorf("key %s again: %v", key, err)
		}
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error { // another kind may have the key
		return access.AddScope(tx, access.Space, "web", "WEB", true, stranger.ID, now)
	}))
	err := e.W.Do(ctx, writer.Func(func(tx *writer.Tx) error {
		return access.SetMember(tx, access.ProjectScope("web"), stranger.ID, access.Member, stranger.ID, now)
	}))
	var refusal *writer.Refusal
	if !errors.As(err, &refusal) {
		t.Errorf("the last admin steps down: %v", err)
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		if err := access.SetMember(tx, access.ProjectScope("web"), member.ID, access.Admin, stranger.ID, now); err != nil {
			return err
		}
		return access.RemoveMember(tx, access.ProjectScope("web"), stranger.ID)
	}))
	i, err := access.Get(ctx, e.DB.R, access.ProjectScope("web"))
	if err != nil || i.Key != "WEB" || !i.Open || i.Archived || i.CreatedBy != stranger.ID {
		t.Errorf("the project: %+v %v", i, err)
	}
}
