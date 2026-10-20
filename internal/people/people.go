// Package people holds everyone who acts in tickit: guests, whose identity
// derives from their browser session (FR-120), staff accounts that log in
// (FR-122 to FR-124), and the seeded people of the demo's history
// (DM-02.3). It resolves the actor of a request (Middleware, Actor), draws
// names and faces from ids, and keeps the login sessions and the security
// log (FR-156).
//
// The session id never leaves this package: a guest is named by a hash
// of it, and a login session is keyed by another.
package people

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/demo/words"
)

// Kind is what kind of person someone is.
type Kind string

const (
	Guest   Kind = "guest"   // a browser session without a login
	Account Kind = "account" // staff, who log in
	Seed    Kind = "seed"    // a seeded person of the demo's history; cannot log in
	Deleted Kind = "deleted" // an id that names nobody (any more); never stored
)

// SiteRole is what an account may do beyond its roles in projects and
// spaces (requirements 3.1).
type SiteRole string

const (
	NoRole    SiteRole = "none"
	Moderator SiteRole = "moderator"
	Admin     SiteRole = "admin"
)

// Valid reports whether r is one of the site roles.
func (r SiteRole) Valid() bool { return r == NoRole || r == Moderator || r == Admin }

// Person is someone as pages show them.
type Person struct {
	ID   string
	Kind Kind
	Name string
	// Hue is one of sixteen hues, 0 to 15, of the face, the caret and the
	// name tag: data-hue in the page, whose stylesheet turns it into
	// degrees (times 22.5). Guests and accounts get the one their id
	// draws; seeded people have their own.
	Hue int
	// Face draws the face on a 7x7 grid: an SVG path for viewBox "0 0 7 7".
	Face string
	// Role is an account's site role, NoRole for everyone else.
	Role SiteRole
	// Inactive marks a deactivated account (FR-143.1).
	Inactive bool
}

// IsZero reports whether p is nobody: the actor outside Middleware.
func (p Person) IsZero() bool { return p.ID == "" }

// Initials are the first letters of the first two words of the name.
func (p Person) Initials() string {
	var b strings.Builder
	for i, w := range strings.Fields(p.Name) {
		if i == 2 {
			break
		}
		r, _ := utf8.DecodeRuneInString(w)
		b.WriteRune(unicode.ToUpper(r))
	}
	if b.Len() == 0 {
		return "?"
	}
	return b.String()
}

// Topic is touched by every change to how people are shown: names and
// faces. Views that show people list it.
const Topic = "people"

// PersonTopic is touched by every change that concerns one person alone:
// their login, their notifications. The person view (person:<id>) lists it.
func PersonTopic(id string) string { return "person:" + id }

// DeletedName is how pages name an id that names nobody (NFR-27).
const DeletedName = "Deleted person"

var lowerBase32 = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// GuestID is the person id of the guest of a session: "g-" and 20
// characters of base32 of the session id's SHA-256. Pages show it; the
// session id does not follow from it.
func GuestID(sid string) string {
	sum := sha256.Sum256([]byte(sid))
	return "g-" + lowerBase32.EncodeToString(sum[:])[:20]
}

// sidHash is the key of a session's login session row: the hex SHA-256 of
// the session id.
func sidHash(sid string) string {
	sum := sha256.Sum256([]byte(sid))
	return hex.EncodeToString(sum[:])
}

// NewAccountID returns a new account's person id: "a-" and a UUIDv7.
func NewAccountID() string { return "a-" + uuid.Must(uuid.NewV7()).String() }

// ValidID accepts the ids of persons: guests, accounts and seeded people.
func ValidID(id string) bool {
	switch {
	case strings.HasPrefix(id, "g-"):
		return len(id) == 22 && strings.Trim(id[2:], "abcdefghijklmnopqrstuvwxyz234567") == ""
	case strings.HasPrefix(id, "a-"):
		u, err := uuid.Parse(id[2:])
		return err == nil && u.String() == id[2:]
	case strings.HasPrefix(id, "s-"):
		return validSlug(id[2:])
	}
	return false
}

func validSlug(s string) bool {
	if len(s) == 0 || len(s) > 32 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	return strings.Trim(s, "abcdefghijklmnopqrstuvwxyz0123456789-") == ""
}

// GuestOf is the guest of a session as their id alone draws them: what a
// guest without a row in people looks like.
func GuestOf(sid string) Person { return derived(GuestID(sid)) }

// derived is the guest named id as the id alone draws them.
func derived(id string) Person {
	a := avatarOf(id)
	return Person{ID: id, Kind: Guest, Name: a.Name, Hue: a.Hue, Face: a.Face, Role: NoRole}
}

// unstored is the person an id names that has no row: a guest who has not
// acted yet, a seeded person before the seed ran, or nobody.
func unstored(id string) Person {
	if strings.HasPrefix(id, "g-") && ValidID(id) {
		return derived(id)
	}
	for _, p := range Seeded {
		if p.ID == id {
			return p
		}
	}
	return deleted(id)
}

// deleted is how pages show an id that names nobody.
func deleted(id string) Person {
	return Person{ID: id, Kind: Deleted, Name: DeletedName, Hue: 0, Face: avatarOf(id).Face, Role: NoRole}
}

// Lookup returns the persons the ids name, for rendering: from their rows,
// or for a guest without one from the id alone, or as "Deleted person"
// for an id that names nobody. One query, whatever the number of ids.
func Lookup(ctx context.Context, q sqlite.Querier, ids ...string) (map[string]Person, error) {
	found := make(map[string]Person, len(ids))
	if len(ids) == 0 {
		return found, nil
	}
	list, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT p.id, p.kind, p.display_name, p.hue, coalesce(a.site_role, 'none'), coalesce(a.status, '')
		FROM people p LEFT JOIN accounts a ON a.person_id = p.id
		WHERE p.id IN (SELECT value FROM json_each(?))`, string(list))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Person
		var status string
		if err := rows.Scan(&p.ID, &p.Kind, &p.Name, &p.Hue, &p.Role, &status); err != nil {
			return nil, err
		}
		p.Face = avatarOf(p.ID).Face
		p.Inactive = status == "inactive"
		found[p.ID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, ok := found[id]; ok {
			continue
		}
		found[id] = unstored(id)
	}
	return found, nil
}

// Get returns one person, as Lookup does.
func Get(ctx context.Context, q sqlite.Querier, id string) (Person, error) {
	m, err := Lookup(ctx, q, id)
	if err != nil {
		return Person{}, err
	}
	return m[id], nil
}

// Ensure gives the actor of a command a row in people, when a guest acts
// for the first time, and records when they were last seen. tabs.Act calls
// it for every action, so a command may refer to its actor's row.
func Ensure(tx *writer.Tx, p Person, at time.Time) error {
	switch p.Kind {
	case Guest:
		if !ValidID(p.ID) {
			return errors.New("people: a guest needs a guest id")
		}
		a := avatarOf(p.ID)
		_, err := tx.Exec(`INSERT INTO people (id, kind, display_name, hue, created_at, last_seen_at) VALUES (?, 'guest', ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET last_seen_at = excluded.last_seen_at`, p.ID, a.Name, a.Hue, at.Unix(), at.Unix())
		return err
	case Account:
		_, err := tx.Exec(`UPDATE people SET last_seen_at = ? WHERE id = ?`, at.Unix(), p.ID)
		return err
	}
	return errors.New("people: only guests and accounts act")
}

// Name lengths (FR-121, DM-05).
const (
	MinName = 2
	MaxName = 30
)

// Rename changes a person's display name (FR-121). A name that does not
// fit is refused next to the name field.
type Rename struct {
	Person string
	Name   string
}

func (c Rename) Validate() error {
	if !ValidID(c.Person) {
		return errors.New("rename: no such person")
	}
	return nil
}

func (c Rename) Apply(tx *writer.Tx) error {
	name := strings.Join(strings.Fields(c.Name), " ")
	if n := utf8.RuneCountInString(name); n < MinName || n > MaxName || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return &fieldRefusal{field: "name", Refusal: writer.Refusal{Msg: "A name has 2 to 30 characters."}}
	}
	if words.Blocked(name) {
		return &fieldRefusal{field: "name", Refusal: writer.Refusal{Msg: words.Refusal}}
	}
	res, err := tx.Exec(`UPDATE people SET display_name = ? WHERE id = ? AND display_name != ?`, name, c.Person, name)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err // unchanged: nothing to show
	}
	tx.Touch(Topic, PersonTopic(c.Person))
	return nil
}

// fieldRefusal is a refusal that belongs to one field of a page; tabs.Act
// records the field with the outcome (tabs.Refuse makes them elsewhere).
type fieldRefusal struct {
	field string
	writer.Refusal
}

func (r *fieldRefusal) Error() string   { return r.Msg }
func (r *fieldRefusal) Unwrap() error   { return &r.Refusal }
func (r *fieldRefusal) OnField() string { return r.field }

// actorKey carries the actor of a request in its context.
type actorKey struct{}

// WithActor returns ctx with p as its actor: Middleware does it for every
// request; tests and tools may too.
func WithActor(ctx context.Context, p Person) context.Context {
	return context.WithValue(ctx, actorKey{}, p)
}

// Actor is the person who makes the request: an account when the session
// has a login session that has not expired, else the session's guest. It
// is the zero Person outside Middleware.
func Actor(ctx context.Context) Person {
	p, _ := ctx.Value(actorKey{}).(Person)
	return p
}

// Resolve finds the actor of a session at now: the account of its login
// session if one is valid, else its guest, with the name the guest chose
// if they did.
func Resolve(ctx context.Context, q sqlite.Querier, sid string, now time.Time) (Person, error) {
	if sid == "" {
		return Person{}, errors.New("people: no session")
	}
	var p Person
	err := q.QueryRowContext(ctx, `SELECT p.id, p.display_name, p.hue, a.site_role
		FROM login_sessions s JOIN accounts a ON a.person_id = s.person_id JOIN people p ON p.id = s.person_id
		WHERE s.sid_hash = ? AND s.expires_at > ? AND a.status = 'active'`, sidHash(sid), now.Unix()).Scan(&p.ID, &p.Name, &p.Hue, &p.Role)
	switch {
	case err == nil:
		p.Kind, p.Face = Account, avatarOf(p.ID).Face
		return p, nil
	case !errors.Is(err, sql.ErrNoRows):
		return Person{}, err
	}
	p = GuestOf(sid)
	err = q.QueryRowContext(ctx, `SELECT display_name, hue FROM people WHERE id = ? AND kind = 'guest'`, p.ID).Scan(&p.Name, &p.Hue)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Person{}, err
	}
	return p, nil
}
