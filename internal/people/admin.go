package people

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"
)

// Account administration (FR-140 to FR-143) and an account's own password
// and sessions (FR-125, FR-129). Each command checks inside the writer
// that its actor may: a site admin who is still active, or the account
// itself. A refusal for want of a right is Denied (403, NFR-01.1).

// denial is a refusal for want of a right, as access.Denial is (people
// cannot import access).
type denial struct{ writer.Refusal }

func (d *denial) Error() string { return d.Msg }
func (d *denial) Unwrap() error { return &d.Refusal }
func (d *denial) Denied() bool  { return true }

var errNotAdmin = &denial{writer.Refusal{Msg: "Only site admins manage accounts."}}

// ErrNoAccount is the error of a command that names no account: 404.
var ErrNoAccount = errors.New("people: no such account")

// requireAdmin checks that by is an active site admin now.
func requireAdmin(tx *writer.Tx, by Person) error {
	if by.Kind != Account || by.Role != Admin {
		return errNotAdmin
	}
	var role, status string
	err := tx.QueryRow(`SELECT site_role, status FROM accounts WHERE person_id = ?`, by.ID).Scan(&role, &status)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (role != string(Admin) || status != "active") {
		return errNotAdmin
	}
	return err
}

// accountOf reads an account's role and status.
func accountOf(tx *writer.Tx, id string) (SiteRole, string, error) {
	var role, status string
	err := tx.QueryRow(`SELECT site_role, status FROM accounts WHERE person_id = ?`, id).Scan(&role, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("%w: %s", ErrNoAccount, id)
	}
	return SiteRole(role), status, err
}

// NewAccount is CreateAccount by a site admin (FR-141): the first password
// is set by the admin and must be changed at the first login.
type NewAccount struct {
	CreateAccount
	Admin Person
}

func (c NewAccount) Apply(tx *writer.Tx) error {
	if err := requireAdmin(tx, c.Admin); err != nil {
		return err
	}
	c.By = c.Admin.ID
	if err := c.CreateAccount.Apply(tx); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE accounts SET must_change_password = 1 WHERE person_id = ?`, c.ID)
	return err
}

// SetRole makes an account a moderator, an admin or neither (FR-142). The
// last active site admin keeps their role. The account's open pages follow
// in their next frame: its person view renders again.
type SetRole struct {
	Admin   Person
	Account string
	Role    SiteRole
	At      time.Time
}

func (c SetRole) Validate() error {
	if !c.Role.Valid() || !strings.HasPrefix(c.Account, "a-") {
		return errors.New("people: a role is none, moderator or admin, of an account")
	}
	return nil
}

func (c SetRole) Apply(tx *writer.Tx) error {
	if err := requireAdmin(tx, c.Admin); err != nil {
		return err
	}
	old, status, err := accountOf(tx, c.Account)
	if err != nil {
		return err
	}
	if old == c.Role {
		return nil
	}
	if old == Admin && status == "active" {
		var others int
		if err := tx.QueryRow(`SELECT count(*) FROM accounts WHERE site_role = 'admin' AND status = 'active' AND person_id != ?`, c.Account).Scan(&others); err != nil {
			return err
		}
		if others == 0 {
			return &writer.Refusal{Msg: "This is the last site admin. Make another account an admin first."}
		}
	}
	if _, err := tx.Exec(`UPDATE accounts SET site_role = ? WHERE person_id = ?`, string(c.Role), c.Account); err != nil {
		return err
	}
	tx.Touch(Topic, PersonTopic(c.Account))
	return LogSecurity{Kind: EventRoleChanged, Outcome: OutcomeOK, Person: c.Account, Actor: c.Admin.ID,
		Detail: string(old) + " to " + string(c.Role), At: c.At}.Apply(tx)
}

// SetActive deactivates an account or activates it again (FR-143): a
// deactivated account cannot log in, its sessions end at once (NFR-04.2),
// and what it wrote stays under its name, marked deactivated.
type SetActive struct {
	Admin   Person
	Account string
	Active  bool
	At      time.Time
}

func (c SetActive) Apply(tx *writer.Tx) error {
	if err := requireAdmin(tx, c.Admin); err != nil {
		return err
	}
	if !c.Active && c.Account == c.Admin.ID {
		return &writer.Refusal{Msg: "You cannot deactivate your own account."}
	}
	_, status, err := accountOf(tx, c.Account)
	if err != nil {
		return err
	}
	want := "inactive"
	if c.Active {
		want = "active"
	}
	if status == want {
		return nil
	}
	if _, err := tx.Exec(`UPDATE accounts SET status = ? WHERE person_id = ?`, want, c.Account); err != nil {
		return err
	}
	if !c.Active {
		if _, err := tx.Exec(`DELETE FROM login_sessions WHERE person_id = ?`, c.Account); err != nil {
			return err
		}
	}
	tx.Touch(Topic, PersonTopic(c.Account))
	detail := "deactivated"
	if c.Active {
		detail = "reactivated"
	}
	return LogSecurity{Kind: EventDeactivated, Outcome: OutcomeOK, Person: c.Account, Actor: c.Admin.ID, Detail: detail, At: c.At}.Apply(tx)
}

// Current is an account's password as CheckCurrent found it right, for
// ChangePassword.
type Current struct {
	person string
	hash   string
}

// CheckCurrent checks an account's current password, outside the writer
// (argon2id is slow). It answers ErrWrongLogin for a wrong one.
func CheckCurrent(ctx context.Context, q sqlite.Querier, person, password string) (Current, error) {
	var hash string
	err := q.QueryRowContext(ctx, `SELECT password_hash FROM accounts WHERE person_id = ? AND status = 'active'`, person).Scan(&hash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		CheckPassword(dummyHash, password)
		return Current{}, ErrWrongLogin
	case err != nil:
		return Current{}, err
	}
	if !CheckPassword(hash, password) {
		return Current{}, ErrWrongLogin
	}
	return Current{person: person, hash: hash}, nil
}

// ChangePassword sets an account's new password (FR-125) after
// CheckCurrent: every other session of the account ends, the current one
// (Session) stays, and a first password set by an admin counts as changed.
type ChangePassword struct {
	Current Current
	Session string
	NewHash string // from HashPassword
	At      time.Time
}

func (c ChangePassword) Validate() error {
	if c.Current.person == "" || c.Session == "" || !strings.HasPrefix(c.NewHash, "$argon2id$") {
		return errors.New("people: a password change needs the current password, a session and a new hash")
	}
	return nil
}

func (c ChangePassword) Apply(tx *writer.Tx) error {
	var hash string
	err := tx.QueryRow(`SELECT password_hash FROM accounts WHERE person_id = ? AND status = 'active'`, c.Current.person).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) || err == nil && hash != c.Current.hash {
		return &writer.Refusal{Msg: "The password changed meanwhile. Log in again."}
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE accounts SET password_hash = ?, must_change_password = 0 WHERE person_id = ?`, c.NewHash, c.Current.person); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM login_sessions WHERE person_id = ? AND sid_hash != ?`, c.Current.person, sidHash(c.Session)); err != nil {
		return err
	}
	tx.Touch(PersonTopic(c.Current.person))
	return LogSecurity{Kind: EventPassword, Outcome: OutcomeOK, Person: c.Current.person, At: c.At}.Apply(tx)
}

// SessionInfo is one login session of an account (FR-129).
type SessionInfo struct {
	ID        string // the row's key: a hash, never the session id
	Current   bool
	UserAgent string
	Created   time.Time
	Seen      time.Time
	Expires   time.Time
}

// Sessions are an account's login sessions that have not expired, the
// newest first; sid is the asking session's id, marked Current.
func Sessions(ctx context.Context, q sqlite.Querier, person, sid string, now time.Time) ([]SessionInfo, error) {
	rows, err := q.QueryContext(ctx, `SELECT sid_hash, user_agent, created_at, last_seen_at, expires_at FROM login_sessions
		WHERE person_id = ? AND expires_at > ? ORDER BY created_at DESC`, person, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cur := sidHash(sid)
	var out []SessionInfo
	for rows.Next() {
		var s SessionInfo
		var c, seen, exp int64
		if err := rows.Scan(&s.ID, &s.UserAgent, &c, &seen, &exp); err != nil {
			return nil, err
		}
		s.Current, s.Created, s.Seen, s.Expires = s.ID == cur, time.Unix(c, 0), time.Unix(seen, 0), time.Unix(exp, 0)
		out = append(out, s)
	}
	return out, rows.Err()
}

// EndSession ends one of the account's own login sessions (FR-129.1): a
// row of another account is not found.
type EndSession struct {
	Person string
	ID     string // SessionInfo.ID
	At     time.Time
}

func (c EndSession) Apply(tx *writer.Tx) error {
	res, err := tx.Exec(`DELETE FROM login_sessions WHERE sid_hash = ? AND person_id = ?`, c.ID, c.Person)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoAccount
	}
	tx.Touch(PersonTopic(c.Person))
	return LogSecurity{Kind: EventLogout, Outcome: OutcomeOK, Person: c.Person, Detail: "a session ended from the account page", At: c.At}.Apply(tx)
}

// AccountRow is an account as the admin list shows it (FR-140.1).
type AccountRow struct {
	Person     Person
	Email      string
	Username   string
	FullName   string
	Status     string
	MustChange bool
	LastLogin  time.Time // zero: never
}

// AccountFilter narrows the account list.
type AccountFilter struct {
	Search string   // in name, username or email, without case
	Role   SiteRole // "" for any
	Status string   // "", "active", "inactive" or "pending"
	Page   int      // from 1
}

// AccountsPage is how many accounts a page of the list shows.
const AccountsPage = 50

// Accounts lists the accounts the filter keeps, by name, AccountsPage a
// page, and how many it keeps in all.
func Accounts(ctx context.Context, q sqlite.Querier, f AccountFilter) ([]AccountRow, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	where := ` WHERE (? = '' OR instr(lower(p.display_name), lower(?)) > 0 OR instr(lower(a.email), lower(?)) > 0 OR instr(lower(a.username), lower(?)) > 0)
		AND (? = '' OR a.site_role = ?) AND (? = '' OR a.status = ?)`
	args := []any{f.Search, f.Search, f.Search, f.Search, string(f.Role), string(f.Role), f.Status, f.Status}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM accounts a JOIN people p ON p.id = a.person_id`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.QueryContext(ctx, `SELECT p.id, p.display_name, p.hue, a.site_role, a.status, a.email, a.username, a.full_name, a.must_change_password, coalesce(a.last_login_at, 0)
		FROM accounts a JOIN people p ON p.id = a.person_id`+where+` ORDER BY lower(p.display_name), p.id LIMIT ? OFFSET ?`,
		append(args, AccountsPage, (f.Page-1)*AccountsPage)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AccountRow
	for rows.Next() {
		var a AccountRow
		var last int64
		if err := rows.Scan(&a.Person.ID, &a.Person.Name, &a.Person.Hue, &a.Person.Role, &a.Status, &a.Email, &a.Username, &a.FullName, &a.MustChange, &last); err != nil {
			return nil, 0, err
		}
		a.Person.Kind, a.Person.Face, a.Person.Inactive = Account, avatarOf(a.Person.ID).Face, a.Status == "inactive"
		if last > 0 {
			a.LastLogin = time.Unix(last, 0)
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

// SecurityEntry is one entry of the security log.
type SecurityEntry struct {
	ID                    int64
	At                    time.Time
	Kind, Outcome, Detail string
	Person, Actor         string
}

// SecurityFilter narrows the security log (FR-156.2).
type SecurityFilter struct {
	Person string // an account's person id, "" for all
	Kind   string // one of the Event kinds, "" for all
	Before int64  // entries with a smaller id, 0 for the newest
}

// SecurityPage is how many entries a page of the security log shows.
const SecurityPage = 100

// SecurityLog reads the security log, newest first.
func SecurityLog(ctx context.Context, q sqlite.Querier, f SecurityFilter) ([]SecurityEntry, error) {
	before := f.Before
	if before <= 0 {
		before = 1 << 62
	}
	rows, err := q.QueryContext(ctx, `SELECT id, at, kind, outcome, detail, coalesce(person_id, ''), coalesce(actor_id, '') FROM security_log
		WHERE id < ? AND (? = '' OR person_id = ? OR actor_id = ?) AND (? = '' OR kind = ?) ORDER BY id DESC LIMIT ?`,
		before, f.Person, f.Person, f.Person, f.Kind, f.Kind, SecurityPage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SecurityEntry
	for rows.Next() {
		var e SecurityEntry
		var at int64
		if err := rows.Scan(&e.ID, &at, &e.Kind, &e.Outcome, &e.Detail, &e.Person, &e.Actor); err != nil {
			return nil, err
		}
		e.At = time.Unix(at, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// SecurityKinds are the kinds of the security log, for its filter.
var SecurityKinds = []string{EventLogin, EventLogout, EventAccountCreated, EventRoleChanged, EventPassword, EventDeactivated, EventModeration}

// GuestRow is a guest who acted lately (FR-140.2).
type GuestRow struct {
	Person      Person
	First, Last time.Time
	Changes     int
}

// ActiveGuests lists the guests seen since since, the last seen first;
// changes counts their events in the log since since.
func ActiveGuests(ctx context.Context, q sqlite.Querier, since time.Time, limit int) ([]GuestRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT p.id, p.display_name, p.hue, p.created_at, p.last_seen_at, coalesce(c.n, 0)
		FROM people p LEFT JOIN (SELECT actor, count(*) AS n FROM lith_events WHERE at >= ? AND actor LIKE 'g-%' GROUP BY actor) c ON c.actor = p.id
		WHERE p.kind = 'guest' AND p.last_seen_at >= ? ORDER BY p.last_seen_at DESC LIMIT ?`, since.UnixMilli(), since.Unix(), limit)
	if err != nil && strings.Contains(err.Error(), "no such table") { // no event yet
		rows, err = q.QueryContext(ctx, `SELECT p.id, p.display_name, p.hue, p.created_at, p.last_seen_at, 0
			FROM people p WHERE p.kind = 'guest' AND p.last_seen_at >= ? ORDER BY p.last_seen_at DESC LIMIT ?`, since.Unix(), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GuestRow
	for rows.Next() {
		var g GuestRow
		var first, last int64
		if err := rows.Scan(&g.Person.ID, &g.Person.Name, &g.Person.Hue, &first, &last, &g.Changes); err != nil {
			return nil, err
		}
		g.Person.Kind, g.Person.Face = Guest, avatarOf(g.Person.ID).Face
		g.First, g.Last = time.Unix(first, 0), time.Unix(last, 0)
		out = append(out, g)
	}
	return out, rows.Err()
}

// Erasers delete what other packages keep of a person who is erased:
// their notifications, their saved filters. Each registers from an init
// function (RegisterEraser).
var (
	erasersMu sync.Mutex
	erasers   []func(tx *writer.Tx, person string) error
)

// RegisterEraser adds what EraseAccount deletes besides the account.
func RegisterEraser(f func(tx *writer.Tx, person string) error) {
	erasersMu.Lock()
	defer erasersMu.Unlock()
	erasers = append(erasers, f)
}

// EraseAccount erases an account by a site admin (NFR-27.1): its row of
// people and of accounts goes, and with them email, username, full name
// and password hash; its login sessions and memberships are deleted, and
// what the erasers keep of it. The events keep the person id alone
// (NFR-26), so its changes show as by DeletedName from then on. The
// security log records it by the id. An admin does not erase themselves.
type EraseAccount struct {
	Admin   Person
	Account string
	At      time.Time
}

func (c EraseAccount) Apply(tx *writer.Tx) error {
	if err := requireAdmin(tx, c.Admin); err != nil {
		return err
	}
	if c.Account == c.Admin.ID {
		return &writer.Refusal{Msg: "You cannot erase your own account."}
	}
	if _, _, err := accountOf(tx, c.Account); err != nil {
		return err
	}
	erasersMu.Lock()
	list := slices.Clone(erasers)
	erasersMu.Unlock()
	for _, f := range list {
		if err := f(tx, c.Account); err != nil {
			return err
		}
	}
	for _, q := range []string{
		`DELETE FROM login_sessions WHERE person_id = ?`,
		`DELETE FROM memberships WHERE person_id = ?`,
		`DELETE FROM accounts WHERE person_id = ?`,
		`DELETE FROM people WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, c.Account); err != nil {
			return err
		}
	}
	tx.Touch(Topic, PersonTopic(c.Account), "projects", "spaces") // the members lists
	return LogSecurity{Kind: EventErased, Outcome: OutcomeOK, Person: c.Account, Actor: c.Admin.ID, At: c.At}.Apply(tx)
}
