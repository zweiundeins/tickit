package people

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"
)

// What the login page says (FR-122, NFR-10). An unknown email and a wrong
// password get the same words; a pending or deactivated account learns
// its state only after the right password.
const (
	MsgWrongLogin = "Email or password is wrong."
	MsgPending    = "This account is not active yet. A site admin activates it."
	MsgInactive   = "This account is deactivated."
	MsgLocked     = "Too many failed logins. Wait 15 minutes, then try again."
)

var (
	ErrWrongLogin = errors.New("people: wrong email or password")
	ErrPending    = errors.New("people: the account is pending")
	ErrInactive   = errors.New("people: the account is deactivated")
)

// LoginMessage is what the login page says for an error of Verify or Login.
func LoginMessage(err error) string {
	switch {
	case errors.Is(err, ErrPending):
		return MsgPending
	case errors.Is(err, ErrInactive):
		return MsgInactive
	}
	return MsgWrongLogin
}

// DefaultSessionTTL is how long a login session lasts (FR-124.1).
const DefaultSessionTTL = 24 * time.Hour

// Credentials is an account as Verify found it, for Login.
type Credentials struct {
	Person Person
	hash   string // the password hash Verify checked; Login refuses if it changed since
}

// Verify finds the account of email and checks password against it. It
// does the same work for an email without an account, a wrong password,
// and the right one (NFR-10): one read and one argon2id hash. It returns
// ErrWrongLogin, or for the right password of an account that may not log
// in, ErrPending or ErrInactive. It writes nothing: the hash is too slow
// for the writer, so Login, the command, checks again that nothing changed.
func Verify(ctx context.Context, q sqlite.Querier, email, password string) (Credentials, error) {
	var c Credentials
	var status string
	err := q.QueryRowContext(ctx, `SELECT a.person_id, a.password_hash, a.status, a.site_role, p.display_name, p.hue
		FROM accounts a JOIN people p ON p.id = a.person_id WHERE a.email = ?`, strings.TrimSpace(email)).
		Scan(&c.Person.ID, &c.hash, &status, &c.Person.Role, &c.Person.Name, &c.Person.Hue)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		CheckPassword(dummyHash, password)
		return Credentials{}, ErrWrongLogin
	case err != nil:
		return Credentials{}, err
	}
	if !CheckPassword(c.hash, password) {
		return Credentials{Person: Person{ID: c.Person.ID}}, ErrWrongLogin
	}
	c.Person.Kind, c.Person.Face = Account, avatarOf(c.Person.ID).Face
	switch status {
	case "pending":
		return c, ErrPending
	case "inactive":
		return c, ErrInactive
	}
	return c, nil
}

// Login starts a login session for credentials that Verify accepted: the
// row of Session (the id edge.Sessions.Renew returned, not the request's),
// the time of the login, and the security log's entry. It refuses with
// MsgWrongLogin when the password or the status changed since Verify.
type Login struct {
	Credentials Credentials
	Session     string
	UserAgent   string
	TTL         time.Duration
	At          time.Time
}

func (c Login) Validate() error {
	if c.Credentials.Person.ID == "" || c.Credentials.hash == "" || c.Session == "" || c.TTL <= 0 {
		return errors.New("a login needs credentials, a session and a lifetime")
	}
	return nil
}

func (c Login) Apply(tx *writer.Tx) error {
	id := c.Credentials.Person.ID
	var hash, status string
	err := tx.QueryRow(`SELECT password_hash, status FROM accounts WHERE person_id = ?`, id).Scan(&hash, &status)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (hash != c.Credentials.hash || status != "active") {
		return &writer.Refusal{Msg: MsgWrongLogin}
	}
	if err != nil {
		return err
	}
	at := c.At.Unix()
	if _, err := tx.Exec(`INSERT INTO login_sessions (sid_hash, person_id, created_at, expires_at, last_seen_at, user_agent) VALUES (?, ?, ?, ?, ?, ?)`,
		sidHash(c.Session), id, at, c.At.Add(c.TTL).Unix(), at, clip(c.UserAgent, 200)); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE accounts SET last_login_at = ? WHERE person_id = ?`, at, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE people SET last_seen_at = ? WHERE id = ?`, at, id); err != nil {
		return err
	}
	tx.Touch(PersonTopic(id))
	return LogSecurity{Kind: EventLogin, Outcome: OutcomeOK, Person: id, At: c.At}.Apply(tx)
}

// Logout ends the login session of a session (FR-123): its row goes, and
// the browser continues as a guest. The account's other sessions stay.
type Logout struct {
	Session string
	At      time.Time
}

func (c Logout) Apply(tx *writer.Tx) error {
	var id string
	err := tx.QueryRow(`DELETE FROM login_sessions WHERE sid_hash = ? RETURNING person_id`, sidHash(c.Session)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // not logged in: nothing to end
	}
	if err != nil {
		return err
	}
	tx.Touch(PersonTopic(id))
	return LogSecurity{Kind: EventLogout, Outcome: OutcomeOK, Person: id, At: c.At}.Apply(tx)
}

// SeenSession records that a session's login session was used at at, for
// the list of an account's sessions (FR-129). It does nothing for a
// session without one.
func SeenSession(tx *writer.Tx, sid string, at time.Time) error {
	_, err := tx.Exec(`UPDATE login_sessions SET last_seen_at = ? WHERE sid_hash = ? AND last_seen_at < ?`, at.Unix(), sidHash(sid), at.Unix())
	return err
}

// What the security log records (FR-156).
const (
	EventLogin          = "login"
	EventLogout         = "logout"
	EventAccountCreated = "account_created"
	EventRoleChanged    = "role_changed"
	EventPassword       = "password_changed"
	EventDeactivated    = "deactivated"
	EventModeration     = "moderation"
	EventErased         = "erased"

	OutcomeOK       = "ok"
	OutcomeWrong    = "wrong_password" // or an unknown email: then Person is empty
	OutcomePending  = "pending"
	OutcomeInactive = "inactive"
	OutcomeLocked   = "locked"
)

// SecurityRetention is how long the security log keeps an entry (FR-156.2).
const SecurityRetention = 90 * 24 * time.Hour

// LogSecurity writes an entry of the security log. Detail must hold no
// password, token, session id or address. Other commands call its Apply
// to log in the same transaction.
type LogSecurity struct {
	Kind, Outcome string
	Person        string // the account it concerns, "" for an unknown email
	Actor         string // who did it, when someone else did
	Detail        string
	At            time.Time
}

func (c LogSecurity) Validate() error {
	if c.Kind == "" || c.Outcome == "" {
		return errors.New("a security log entry needs a kind and an outcome")
	}
	return nil
}

func (c LogSecurity) Apply(tx *writer.Tx) error {
	_, err := tx.Exec(`INSERT INTO security_log (at, kind, person_id, actor_id, outcome, detail) VALUES (?, ?, ?, ?, ?, ?)`,
		c.At.Unix(), c.Kind, nullable(c.Person), nullable(c.Actor), c.Outcome, clip(c.Detail, 500))
	return err
}

// CreateAccount makes a staff account (FR-141): a person of kind account
// with a password hash from HashPassword, active and verified. A taken
// email or username is refused.
type CreateAccount struct {
	ID           string // NewAccountID
	Email        string
	Username     string
	FullName     string
	Role         SiteRole
	PasswordHash string
	By           string // the admin who made it, "" from the command line
	At           time.Time
}

// Username lengths (requirements 4.1).
const (
	MinUsername = 2
	MaxUsername = 50
)

func (c CreateAccount) Validate() error {
	addr, err := mail.ParseAddress(c.Email)
	switch {
	case !strings.HasPrefix(c.ID, "a-") || !ValidID(c.ID):
		return errors.New("an account needs an id from NewAccountID")
	case err != nil || addr.Address != c.Email || len(c.Email) > 254:
		return fmt.Errorf("%q is not an email address", c.Email)
	case !validUsername(c.Username):
		return fmt.Errorf("a username has %d to %d letters, digits, '.', '-' or '_'", MinUsername, MaxUsername)
	case utf8.RuneCountInString(c.FullName) > 100:
		return errors.New("a full name has at most 100 characters")
	case !c.Role.Valid():
		return fmt.Errorf("%q is not a site role: none, moderator or admin", c.Role)
	case !strings.HasPrefix(c.PasswordHash, "$argon2id$"):
		return errors.New("an account needs a password hash from HashPassword")
	}
	return nil
}

func validUsername(s string) bool {
	if len(s) < MinUsername || len(s) > MaxUsername {
		return false
	}
	return strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-_") == ""
}

func (c CreateAccount) Apply(tx *writer.Tx) error {
	var taken string
	err := tx.QueryRow(`SELECT CASE WHEN email = ? THEN 'email' ELSE 'username' END FROM accounts WHERE email = ? OR username = ? LIMIT 1`,
		c.Email, c.Email, c.Username).Scan(&taken)
	switch {
	case err == nil:
		return &writer.Refusal{Msg: "An account with this " + taken + " exists already."}
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	name := strings.TrimSpace(c.FullName)
	if name == "" {
		name = c.Username
	}
	at := c.At.Unix()
	if _, err := tx.Exec(`INSERT INTO people (id, kind, display_name, hue, created_at, last_seen_at) VALUES (?, 'account', ?, ?, ?, ?)`,
		c.ID, name, avatarOf(c.ID).Hue, at, at); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO accounts (person_id, email, username, full_name, password_hash, site_role, status, email_verified)
		VALUES (?, ?, ?, ?, ?, ?, 'active', 1)`, c.ID, c.Email, c.Username, strings.TrimSpace(c.FullName), c.PasswordHash, string(c.Role)); err != nil {
		return err
	}
	tx.Touch(Topic)
	return LogSecurity{Kind: EventAccountCreated, Outcome: OutcomeOK, Person: c.ID, Actor: c.By, Detail: "role " + string(c.Role), At: c.At}.Apply(tx)
}

// Sweep deletes expired login sessions and security log entries older than
// SecurityRetention. Run it every hour.
type Sweep struct{ At time.Time }

func (c Sweep) Apply(tx *writer.Tx) error {
	if _, err := tx.Exec(`DELETE FROM login_sessions WHERE expires_at <= ?`, c.At.Unix()); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM security_log WHERE at < ?`, c.At.Add(-SecurityRetention).Unix())
	return err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// clip cuts s to at most n bytes, on a character boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
