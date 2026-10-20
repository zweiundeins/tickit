// Package access decides who may do what (requirements 3.2): rights per
// role in a project or a space, what moderators and site admins add, and
// Q14's open scopes, in which everyone has a Member's rights. Projects and
// spaces are scopes here: guard rows in the table scopes, with their
// memberships beside them. Commands write both directly, never a
// projector, so a command can check a right inside its transaction
// (Require) and the keys stay unique (NFR-18). AddScope writes a scope
// with its first Admin.
package access

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Right is something a person may do.
type Right int

const (
	Read           Right = iota + 1 // read issues, boards, pages, comments and history
	Comment                         // comment on issues and pages
	EditIssues                      // create and edit issues, move cards, create and edit pages
	DeleteIssue                     // delete an issue
	TrashOwnPage                    // move a page one created to the trash: check the creator too
	TrashPage                       // move any page to the trash
	RestoreVersion                  // restore a page version
	EditOwnComment                  // edit or delete one's own comment: check the author too
	EditAnyComment                  // edit or delete anyone's comment
	HideComment                     // hide other people's comments
	Configure                       // workflow, board columns, components, templates
	ManageMembers                   // members and roles of a project or space
	CreateScope                     // create a project or a space (rate-limited, Q24)
	ArchiveScope                    // archive or unarchive a project or a space
	Moderate                        // block a guest, hide any content
	Administer                      // accounts, site roles, the demo's reset
)

// Role is a person's role in one project or space.
type Role string

const (
	Viewer Role = "viewer"
	Member Role = "member"
	Admin  Role = "admin"
)

// Valid reports whether r is one of the roles.
func (r Role) Valid() bool { return r == Viewer || r == Member || r == Admin }

// rank orders the roles, none below Viewer.
func (r Role) rank() int {
	switch r {
	case Viewer:
		return 1
	case Member:
		return 2
	case Admin:
		return 3
	}
	return 0
}

// table is section 3.2, row by row. Moderators and site admins hold the
// rights of their roles too; their columns say what the site role adds.
var table = map[Right]struct{ viewer, member, admin, moderator, siteAdmin bool }{
	Read:           {true, true, true, true, true},
	Comment:        {true, true, true, true, true},
	EditIssues:     {false, true, true, false, true},
	DeleteIssue:    {false, false, true, true, true},
	TrashOwnPage:   {false, true, true, true, true},
	TrashPage:      {false, false, true, true, true},
	RestoreVersion: {false, true, true, false, true},
	EditOwnComment: {true, true, true, true, true},
	EditAnyComment: {false, false, false, true, true},
	HideComment:    {false, false, true, true, true},
	Configure:      {false, false, true, false, true},
	ManageMembers:  {false, false, true, false, true},
	CreateScope:    {false, true, true, false, true},
	ArchiveScope:   {false, false, true, false, true},
	Moderate:       {false, false, false, true, true},
	Administer:     {false, false, false, false, true},
}

// siteWide are the rights that belong to no project or space.
func siteWide(r Right) bool { return r == CreateScope || r == Moderate || r == Administer }

// changes are the rights an archived scope takes from everyone: its
// content is read-only (FR-04.1, FR-77.1). Unarchiving, members and
// moderation stay.
func changes(r Right) bool {
	switch r {
	case Comment, EditIssues, DeleteIssue, TrashOwnPage, TrashPage, RestoreVersion, EditOwnComment, EditAnyComment, Configure:
		return true
	}
	return false
}

func roleHas(role Role, r Right) bool {
	row := table[r]
	switch role {
	case Viewer:
		return row.viewer
	case Member:
		return row.member
	case Admin:
		return row.admin
	}
	return false
}

func siteHas(role people.SiteRole, r Right) bool {
	row := table[r]
	switch role {
	case people.Moderator:
		return row.moderator
	case people.Admin:
		return row.siteAdmin
	}
	return false
}

// Kind is the kind of a scope.
type Kind string

const (
	Project Kind = "project"
	Space   Kind = "space"
)

// Scope names a project or a space. The zero Scope is the site, for the
// site-wide rights.
type Scope struct {
	Kind Kind
	ID   string
}

// ProjectScope and SpaceScope name a scope.
func ProjectScope(id string) Scope { return Scope{Kind: Project, ID: id} }
func SpaceScope(id string) Scope   { return Scope{Kind: Space, ID: id} }

// Topic is the scope's topic, "project:<id>" or "space:<id>": access
// touches it when the scope's members or state change.
func (s Scope) Topic() string { return string(s.Kind) + ":" + s.ID }

// ErrNotFound is the error of an action or a view that names something
// that does not exist, or that the actor may not know exists: both answer
// 404. Domain packages wrap it for their own ids too.
var ErrNotFound = errors.New("not found")

// Info is a scope's row.
type Info struct {
	Scope
	Key       string
	Open      bool
	Archived  bool
	CreatedBy string
}

// Get reads a scope, or answers ErrNotFound.
func Get(ctx context.Context, q sqlite.Querier, s Scope) (Info, error) {
	i := Info{Scope: s}
	err := q.QueryRowContext(ctx, `SELECT key, open, archived, created_by FROM scopes WHERE kind = ? AND id = ?`, string(s.Kind), s.ID).
		Scan(&i.Key, &i.Open, &i.Archived, &i.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Info{}, fmt.Errorf("%s %s: %w", s.Kind, s.ID, ErrNotFound)
	}
	return i, err
}

// RoleOf is a person's role in a scope as its memberships say, "" for none.
func RoleOf(ctx context.Context, q sqlite.Querier, person string, s Scope) (Role, error) {
	var r Role
	err := q.QueryRowContext(ctx, `SELECT role FROM memberships WHERE person_id = ? AND scope_kind = ? AND scope_id = ?`, person, string(s.Kind), s.ID).Scan(&r)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return r, err
}

// Effective is the role a person acts with in a scope: their membership's,
// and at least Member in an open scope (Q14). Guests and accounts alike.
func Effective(ctx context.Context, q sqlite.Querier, actor people.Person, i Info) (Role, error) {
	r, err := RoleOf(ctx, q, actor.ID, i.Scope)
	if err != nil {
		return "", err
	}
	if i.Open && r.rank() < Member.rank() {
		r = Member
	}
	return r, nil
}

// actsHere reports whether p is someone who acts: a guest or an account.
// Seeded people and deleted ones never do.
func actsHere(p people.Person) bool {
	return !p.IsZero() && (p.Kind == people.Guest || p.Kind == people.Account && !p.Inactive)
}

// SiteCan answers a site-wide right (CreateScope, Moderate, Administer)
// without reading anything: the header's links use it. It is false for the
// rights of a scope.
func SiteCan(actor people.Person, r Right) bool {
	if !actsHere(actor) || !siteWide(r) {
		return false
	}
	if r == CreateScope {
		return true // everyone is a Member somewhere under Q14 (Q24)
	}
	return siteHas(actor.Role, r)
}

// Can reports whether actor has right in scope. A site-wide right takes
// the zero Scope. A scope that does not exist answers ErrNotFound.
// TrashOwnPage and EditOwnComment say only that the role allows it for
// one's own: the caller checks who made the page or the comment.
func Can(ctx context.Context, q sqlite.Querier, actor people.Person, r Right, s Scope) (bool, error) {
	if _, ok := table[r]; !ok {
		return false, fmt.Errorf("access: no right %d", r)
	}
	if siteWide(r) {
		return SiteCan(actor, r), nil
	}
	i, err := Get(ctx, q, s)
	if err != nil {
		return false, err
	}
	return can(ctx, q, actor, r, i)
}

func can(ctx context.Context, q sqlite.Querier, actor people.Person, r Right, i Info) (bool, error) {
	if !actsHere(actor) || i.Archived && changes(r) {
		return false, nil
	}
	role, err := Effective(ctx, q, actor, i)
	if err != nil {
		return false, err
	}
	return roleHas(role, r) || siteHas(actor.Role, r), nil
}

// Denial is the refusal of an action for want of a right. It is a
// *writer.Refusal (errors.As), so tabs.Act records its message, and it
// is Denied, so the action answers 403 (NFR-01.1).
type Denial struct {
	writer.Refusal
}

func (d *Denial) Error() string { return d.Msg }
func (d *Denial) Unwrap() error { return &d.Refusal }
func (d *Denial) Denied() bool  { return true }

// Require refuses, inside a command, an actor without right in scope,
// with a sentence that says why. A scope that does not exist, or that the
// actor may not read, answers ErrNotFound: it does not tell that it exists.
func Require(tx *writer.Tx, actor people.Person, r Right, s Scope) error {
	if _, ok := table[r]; !ok {
		return fmt.Errorf("access: no right %d", r)
	}
	ctx := tx.Context()
	if siteWide(r) {
		if SiteCan(actor, r) {
			return nil
		}
		return &Denial{writer.Refusal{Msg: why(r, "")}}
	}
	i, err := Get(ctx, tx, s)
	if err != nil {
		return err
	}
	if ok, err := can(ctx, tx, actor, Read, i); err != nil || !ok {
		return errors.Join(err, fmt.Errorf("%s %s: %w", s.Kind, s.ID, ErrNotFound))
	}
	ok, err := can(ctx, tx, actor, r, i)
	switch {
	case err != nil:
		return err
	case ok:
		return nil
	case i.Archived && changes(r):
		return &Denial{writer.Refusal{Msg: fmt.Sprintf("This %s is archived.", s.Kind)}}
	}
	return &Denial{writer.Refusal{Msg: why(r, s.Kind)}}
}

// why says which right was missing, in the scope's words.
func why(r Right, k Kind) string {
	things := "issues"
	if k == Space {
		things = "pages"
	}
	switch r {
	case Comment:
		return fmt.Sprintf("You cannot comment in this %s.", k)
	case EditIssues:
		return fmt.Sprintf("Only members of this %s can change its %s.", k, things)
	case DeleteIssue:
		return fmt.Sprintf("Only admins of this %s can delete issues.", k)
	case TrashOwnPage:
		return fmt.Sprintf("Only members of this %s can move their pages to the trash.", k)
	case TrashPage:
		return fmt.Sprintf("Only admins of this %s can move other people's pages to the trash.", k)
	case RestoreVersion:
		return fmt.Sprintf("Only members of this %s can restore a version.", k)
	case EditOwnComment:
		return fmt.Sprintf("You cannot change comments in this %s.", k)
	case EditAnyComment:
		return "Only moderators can change other people's comments."
	case HideComment:
		return fmt.Sprintf("Only admins of this %s can hide comments.", k)
	case Configure:
		return fmt.Sprintf("Only admins of this %s can change its settings.", k)
	case ManageMembers:
		return fmt.Sprintf("Only admins of this %s can manage its members.", k)
	case ArchiveScope:
		return fmt.Sprintf("Only admins of this %s can archive it.", k)
	case CreateScope:
		return "You cannot create projects or spaces."
	case Moderate:
		return "Only moderators can do this."
	case Administer:
		return "Only site admins can do this."
	}
	return "You cannot do this."
}

// AddScope adds a project or a space with its key, and makes its
// creator its Admin (FR-02.3), in the command that creates it. A key that
// any scope of the kind has, archived ones included, is refused next to
// the key field (FR-02.2). The domain checks the key's format.
func AddScope(tx *writer.Tx, k Kind, id, key string, open bool, createdBy string, at time.Time) error {
	if k != Project && k != Space || id == "" || key == "" || createdBy == "" {
		return errors.New("access: a scope needs a kind, an id, a key and its creator")
	}
	var one int
	err := tx.QueryRow(`SELECT 1 FROM scopes WHERE kind = ? AND key = ?`, string(k), key).Scan(&one)
	switch {
	case err == nil:
		return tabs.Refuse("key", fmt.Sprintf("The key %s is taken.", key))
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	if _, err := tx.Exec(`INSERT INTO scopes (kind, id, key, open, archived, created_by, created_at) VALUES (?, ?, ?, ?, 0, ?, ?)`,
		string(k), id, key, open, createdBy, at.Unix()); err != nil {
		return err
	}
	return SetMember(tx, Scope{Kind: k, ID: id}, createdBy, Admin, createdBy, at)
}

// SetMember gives a person a role in a scope, or changes it. The last
// Admin of a scope keeps the role (FR-05.2).
func SetMember(tx *writer.Tx, s Scope, person string, r Role, by string, at time.Time) error {
	if !r.Valid() || person == "" {
		return errors.New("access: a membership needs a person and a role")
	}
	if r != Admin {
		if err := keepAnAdmin(tx, s, person); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO memberships (person_id, scope_kind, scope_id, role, added_at, added_by) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (person_id, scope_kind, scope_id) DO UPDATE SET role = excluded.role`,
		person, string(s.Kind), s.ID, string(r), at.Unix(), by); err != nil {
		return err
	}
	tx.Touch(s.Topic(), people.PersonTopic(person))
	return nil
}

// RemoveMember ends a person's membership of a scope. The last Admin
// stays (FR-05.2).
func RemoveMember(tx *writer.Tx, s Scope, person string) error {
	if err := keepAnAdmin(tx, s, person); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM memberships WHERE person_id = ? AND scope_kind = ? AND scope_id = ?`, person, string(s.Kind), s.ID); err != nil {
		return err
	}
	tx.Touch(s.Topic(), people.PersonTopic(person))
	return nil
}

// keepAnAdmin refuses to take the role of Admin from person when they are
// the scope's last.
func keepAnAdmin(tx *writer.Tx, s Scope, person string) error {
	var others, self int
	err := tx.QueryRow(`SELECT count(*) FILTER (WHERE person_id != ?), count(*) FILTER (WHERE person_id = ?)
		FROM memberships WHERE scope_kind = ? AND scope_id = ? AND role = 'admin'`, person, person, string(s.Kind), s.ID).Scan(&others, &self)
	if err != nil {
		return err
	}
	if self > 0 && others == 0 {
		return tabs.Refuse("role", fmt.Sprintf("Every %s needs an admin: make someone else admin first.", s.Kind))
	}
	return nil
}

// SetArchived archives a scope or takes it out of the archive (FR-04).
func SetArchived(tx *writer.Tx, s Scope, archived bool) error {
	res, err := tx.Exec(`UPDATE scopes SET archived = ? WHERE kind = ? AND id = ? AND archived != ?`, archived, string(s.Kind), s.ID, archived)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err
	}
	tx.Touch(s.Topic())
	return nil
}
