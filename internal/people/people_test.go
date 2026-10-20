package people_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/dbtest"
	"github.com/mbolli/tickit/internal/people"
)

var now = time.Unix(1_800_000_000, 0)

// FR-120.1, FR-120.2: a session is one guest, in every tab and on every
// page, named by a hash: the session id never shows.
func TestGuestIdentity(t *testing.T) {
	sid := "a-session-id-of-forty-three-characters-xyz"
	g := people.GuestOf(sid)
	if g != people.GuestOf(sid) || g.ID != people.GuestID(sid) || g.Kind != people.Guest {
		t.Fatalf("a guest: %+v", g)
	}
	if !people.ValidID(g.ID) || !strings.HasPrefix(g.ID, "g-") || len(g.ID) != 22 {
		t.Errorf("a guest id: %q", g.ID)
	}
	if words := strings.Fields(g.Name); len(words) != 2 || g.Face == "" || g.Initials() != words[0][:1]+words[1][:1] {
		t.Errorf("a guest's name and face: %+v, initials %q", g, g.Initials())
	}
	if other := people.GuestOf(sid + "2"); other.ID == g.ID {
		t.Error("two sessions are one guest") // FR-120.3
	}
	if s := fmt.Sprintf("%+v", g); strings.Contains(s, sid) || strings.Contains(s, sid[:12]) {
		t.Errorf("the session id shows: %s", s)
	}
}

// Faces are mirrored and never nearly empty, as in lith's board demo.
func TestFaces(t *testing.T) {
	names := map[string]bool{}
	for i := range 2000 {
		p := people.GuestOf(fmt.Sprint("session ", i))
		names[p.Name] = true
		cells := map[[2]int]bool{}
		for _, m := range strings.Split(strings.TrimPrefix(p.Face, "M"), "M") {
			var x, y, w int
			fmt.Sscanf(m, "%d %dh%d", &x, &y, &w)
			for i := range w {
				cells[[2]int{x + i, y}] = true
			}
		}
		if len(cells) < 6 {
			t.Fatalf("%s: a face of %d pixels", p.ID, len(cells))
		}
		for c := range cells {
			if !cells[[2]int{6 - c[0], c[1]}] {
				t.Fatalf("%s: pixel %v has no mirror", p.ID, c)
			}
		}
		if p.Hue < 0 || p.Hue >= 360 {
			t.Fatalf("%s: hue %d", p.ID, p.Hue)
		}
	}
	if len(names) < 600 {
		t.Errorf("2000 guests share %d names", len(names))
	}
}

func TestLookup(t *testing.T) {
	e := dbtest.Open(t)
	ada := people.GuestOf("session-ada")
	bob := people.GuestOf("session-bob")
	e.Do(t, writer.Func(func(tx *writer.Tx) error { return people.Ensure(tx, ada, now) }))
	e.Do(t, people.Rename{Person: ada.ID, Name: "Ada L."})
	e.View(t, func(tx *sql.Tx) error {
		got, err := people.Lookup(context.Background(), tx, ada.ID, bob.ID, "s-priya", "a-gone", "")
		if err != nil {
			return err
		}
		if got[ada.ID].Name != "Ada L." || got[ada.ID].Face != ada.Face {
			t.Errorf("a guest with a row: %+v", got[ada.ID])
		}
		if got[bob.ID] != bob {
			t.Errorf("a guest without a row: %+v, not %+v", got[bob.ID], bob)
		}
		if got["s-priya"].Name != "Priya Raman" || got["s-priya"].Kind != people.Seed {
			t.Errorf("a seeded person before the seed: %+v", got["s-priya"])
		}
		if got["a-gone"].Name != people.DeletedName || got["a-gone"].Kind != people.Deleted || got[""].Name != people.DeletedName {
			t.Errorf("an id of nobody: %+v %+v", got["a-gone"], got[""])
		}
		return nil
	})
}

// DM-02.3: six seeded people, who have no accounts.
func TestSeedPeople(t *testing.T) {
	e := dbtest.Open(t)
	e.Do(t, people.SeedPeople{At: now})
	e.Do(t, people.SeedPeople{At: now}) // again: the same six
	e.View(t, func(tx *sql.Tx) error {
		var n, accounts int
		if err := tx.QueryRow(`SELECT count(*), (SELECT count(*) FROM accounts) FROM people WHERE kind = 'seed'`).Scan(&n, &accounts); err != nil {
			return err
		}
		if n != 6 || len(people.Seeded) != 6 || accounts != 0 {
			t.Errorf("%d seeded people, %d accounts", n, accounts)
		}
		return nil
	})
}

func TestEnsure(t *testing.T) {
	e := dbtest.Open(t)
	g := people.GuestOf("session-ada")
	e.Do(t, writer.Func(func(tx *writer.Tx) error { return people.Ensure(tx, g, now) }))
	e.Do(t, writer.Func(func(tx *writer.Tx) error { return people.Ensure(tx, g, now.Add(time.Minute)) }))
	e.View(t, func(tx *sql.Tx) error {
		var name string
		var created, seen int64
		if err := tx.QueryRow(`SELECT display_name, created_at, last_seen_at FROM people WHERE id = ?`, g.ID).Scan(&name, &created, &seen); err != nil {
			return err
		}
		if name != g.Name || created != now.Unix() || seen != now.Unix()+60 {
			t.Errorf("the guest's row: %q, %d, %d", name, created, seen)
		}
		return nil
	})
	if err := e.W.Do(context.Background(), writer.Func(func(tx *writer.Tx) error { return people.Ensure(tx, people.Person{}, now) })); err == nil {
		t.Error("nobody got a row")
	}
}

// FR-121: a name of 2 to 30 characters; others are refused at the field.
func TestRename(t *testing.T) {
	e := dbtest.Open(t)
	g := people.GuestOf("session-ada")
	e.Do(t, writer.Func(func(tx *writer.Tx) error { return people.Ensure(tx, g, now) }))
	e.Touched()
	e.Do(t, people.Rename{Person: g.ID, Name: "  Ada   Lovelace "})
	if got := e.Touched(); !slices.Equal(got, []string{"people", "person:" + g.ID}) {
		t.Errorf("a rename touched %v", got)
	}
	err := e.W.Do(context.Background(), people.Rename{Person: g.ID, Name: "A"})
	var refusal *writer.Refusal
	var field interface{ OnField() string }
	if !errors.As(err, &refusal) || !errors.As(err, &field) || field.OnField() != "name" || refusal.Msg != "A name has 2 to 30 characters." {
		t.Errorf("a name of one letter: %v", err)
	}
	e.View(t, func(tx *sql.Tx) error {
		p, err := people.Get(context.Background(), tx, g.ID)
		if p.Name != "Ada Lovelace" {
			t.Errorf("renamed: %+v", p)
		}
		return err
	})
}

// NFR-07: 8 to 128 characters, every one significant, argon2id.
func TestPasswords(t *testing.T) {
	for pw, ok := range map[string]bool{
		"seven77":                     false,
		"eight888":                    true,
		strings.Repeat("ü", 128):      true,
		strings.Repeat("x", 129):      false,
		"no rules: lowercase is fine": true,
	} {
		if err := people.CheckPolicy(pw); (err == nil) != ok {
			t.Errorf("%.20q: %v", pw, err)
		}
	}
	long := strings.Repeat("a", 100)
	h, err := people.HashPassword(long + "1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("a hash: %s", h)
	}
	if !people.CheckPassword(h, long+"1") || people.CheckPassword(h, long+"2") || people.CheckPassword(h, long) {
		t.Error("the 101st character does not count")
	}
	if people.CheckPassword("$argon2id$v=19$garbage", long+"1") || people.CheckPassword("", "") {
		t.Error("a malformed hash matches")
	}
}

// account makes an active staff account with password.
func account(t *testing.T, e *dbtest.Env, email, password string, role people.SiteRole) string {
	t.Helper()
	h, err := people.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	id := people.NewAccountID()
	e.Do(t, people.CreateAccount{ID: id, Email: email, Username: strings.Split(email, "@")[0], FullName: "Staff " + email[:3], Role: role, PasswordHash: h, At: now})
	return id
}

// FR-122, FR-123, FR-124, NFR-04: a login session is a row keyed by the
// renewed session; logout ends it; it expires.
func TestLoginLogout(t *testing.T) {
	e := dbtest.Open(t)
	ctx := context.Background()
	id := account(t, e, "mod@example.com", "correct horse", people.Moderator)

	c, err := people.Verify(ctx, e.DB.R, "MOD@example.com ", "correct horse")
	if err != nil || c.Person.ID != id || c.Person.Role != people.Moderator {
		t.Fatalf("verify: %+v %v", c, err)
	}
	e.Touched()
	e.Do(t, people.Login{Credentials: c, Session: "renewed", UserAgent: "Firefox", TTL: people.DefaultSessionTTL, At: now})
	if got := e.Touched(); !slices.Equal(got, []string{"person:" + id}) {
		t.Errorf("a login touched %v", got)
	}
	resolve := func(sid string, at time.Time) people.Person {
		t.Helper()
		p, err := people.Resolve(ctx, e.DB.R, sid, at)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := resolve("renewed", now.Add(time.Hour)); p.ID != id || p.Kind != people.Account || p.Role != people.Moderator || p.Name != "Staff mod" {
		t.Errorf("logged in: %+v", p)
	}
	if p := resolve("the old one", now.Add(time.Hour)); p.Kind != people.Guest {
		t.Errorf("the session before the login: %+v", p)
	}
	// FR-124.1: after 24 hours, a guest's.
	if p := resolve("renewed", now.Add(people.DefaultSessionTTL)); p.Kind != people.Guest || p.ID != people.GuestID("renewed") {
		t.Errorf("expired: %+v", p)
	}
	// A second session of the account stays when the first logs out (FR-123.2).
	e.Do(t, people.Login{Credentials: c, Session: "second", TTL: time.Hour, At: now})
	e.Do(t, people.Logout{Session: "renewed", At: now.Add(time.Minute)})
	if p := resolve("renewed", now.Add(2*time.Minute)); p.Kind != people.Guest {
		t.Errorf("logged out: %+v", p)
	}
	if p := resolve("second", now.Add(2*time.Minute)); p.ID != id {
		t.Errorf("the other session: %+v", p)
	}
	e.Do(t, people.Logout{Session: "not logged in", At: now})

	// The sweep deletes expired sessions; the security log has no session id.
	e.Do(t, people.Sweep{At: now.Add(2 * time.Hour)})
	e.View(t, func(tx *sql.Tx) error {
		var sessions int
		if err := tx.QueryRow(`SELECT count(*) FROM login_sessions`).Scan(&sessions); err != nil {
			return err
		}
		if sessions != 0 {
			t.Errorf("%d sessions after the sweep", sessions)
		}
		rows, err := tx.Query(`SELECT kind || ' ' || outcome || ' ' || coalesce(person_id, '') || ' ' || detail FROM security_log ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		var log []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return err
			}
			if strings.Contains(s, "renewed") || strings.Contains(s, "second") || strings.Contains(s, "correct horse") {
				t.Errorf("the security log holds a secret: %s", s)
			}
			log = append(log, strings.Fields(s)[0])
		}
		if !slices.Equal(log, []string{"account_created", "login", "login", "logout"}) {
			t.Errorf("security log: %v", log)
		}
		return rows.Err()
	})
}

// FR-122.2, NFR-10: an unknown email and a wrong password get the same
// answer after the same work.
func TestLoginEnumeration(t *testing.T) {
	e := dbtest.Open(t)
	ctx := context.Background()
	account(t, e, "ada@example.com", "correct horse", people.NoRole)
	measure := func(email string) (time.Duration, error) {
		var times []time.Duration
		var last error
		for range 5 {
			start := time.Now()
			_, last = people.Verify(ctx, e.DB.R, email, "wrong horse")
			times = append(times, time.Since(start))
		}
		slices.Sort(times)
		return times[2], last
	}
	known, errKnown := measure("ada@example.com")
	unknown, errUnknown := measure("bob@example.com")
	if !errors.Is(errKnown, people.ErrWrongLogin) || !errors.Is(errUnknown, people.ErrWrongLogin) ||
		people.LoginMessage(errKnown) != "Email or password is wrong." || people.LoginMessage(errUnknown) != people.LoginMessage(errKnown) {
		t.Errorf("wrong password: %v; unknown email: %v", errKnown, errUnknown)
	}
	if d := known - unknown; d > 50*time.Millisecond || d < -50*time.Millisecond || unknown < known/2 {
		t.Errorf("a wrong password takes %v, an unknown email %v", known, unknown)
	}
}

// FR-122.3: a pending or deactivated account learns it only after the
// right password; Login checks again in the writer.
func TestLoginStates(t *testing.T) {
	e := dbtest.Open(t)
	ctx := context.Background()
	id := account(t, e, "ada@example.com", "correct horse", people.NoRole)
	c, err := people.Verify(ctx, e.DB.R, "ada@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"pending", "inactive"} {
		e.Do(t, writer.Func(func(tx *writer.Tx) error {
			_, err := tx.Exec(`UPDATE accounts SET status = ? WHERE person_id = ?`, status, id)
			return err
		}))
		if _, err := people.Verify(ctx, e.DB.R, "ada@example.com", "wrong horse"); !errors.Is(err, people.ErrWrongLogin) {
			t.Errorf("%s, a wrong password: %v", status, err)
		}
		_, err := people.Verify(ctx, e.DB.R, "ada@example.com", "correct horse")
		if want := map[string]error{"pending": people.ErrPending, "inactive": people.ErrInactive}[status]; !errors.Is(err, want) {
			t.Errorf("%s, the right password: %v", status, err)
		}
		// Verified while active, logged in after the change: refused.
		var refusal *writer.Refusal
		if err := e.W.Do(ctx, people.Login{Credentials: c, Session: "s", TTL: time.Hour, At: now}); !errors.As(err, &refusal) || refusal.Msg != people.MsgWrongLogin {
			t.Errorf("%s, login: %v", status, err)
		}
	}
}

// NFR-09.2: five failures for one email from one address lock it for 15
// minutes, an email without an account too.
func TestLockout(t *testing.T) {
	l := people.NewLockout()
	key := people.LockoutKey("Ada@example.com ", "192.0.2.1")
	for i := range 4 {
		l.Fail(key, now.Add(time.Duration(i)*time.Minute))
	}
	if l.Locked(key, now.Add(4*time.Minute)) {
		t.Fatal("locked after 4 failures")
	}
	l.Fail(key, now.Add(4*time.Minute))
	switch {
	case !l.Locked(people.LockoutKey("ada@example.com", "192.0.2.1"), now.Add(5*time.Minute)):
		t.Error("not locked after 5 failures")
	case l.Locked(people.LockoutKey("ada@example.com", "192.0.2.2"), now.Add(5*time.Minute)):
		t.Error("another address is locked")
	case l.Locked(key, now.Add(19*time.Minute+time.Second)):
		t.Error("still locked after 15 minutes")
	}
	// Failures spread over more than 15 minutes do not lock.
	other := people.LockoutKey("nobody@example.com", "192.0.2.1")
	for i := range 6 {
		l.Fail(other, now.Add(time.Duration(i)*4*time.Minute))
	}
	if l.Locked(other, now.Add(21*time.Minute)) {
		t.Error("locked by failures over 20 minutes")
	}
	l.Succeed(key)
}

func TestCreateAccount(t *testing.T) {
	e := dbtest.Open(t)
	ctx := context.Background()
	account(t, e, "ada@example.com", "correct horse", people.Admin)
	h, _ := people.HashPassword("correct horse")
	var refusal *writer.Refusal
	err := e.W.Do(ctx, people.CreateAccount{ID: people.NewAccountID(), Email: "ADA@example.com", Username: "other", Role: people.NoRole, PasswordHash: h, At: now})
	if !errors.As(err, &refusal) || refusal.Msg != "An account with this email exists already." {
		t.Errorf("a taken email: %v", err)
	}
	err = e.W.Do(ctx, people.CreateAccount{ID: people.NewAccountID(), Email: "bob@example.com", Username: "ADA", Role: people.NoRole, PasswordHash: h, At: now})
	if !errors.As(err, &refusal) || refusal.Msg != "An account with this username exists already." {
		t.Errorf("a taken username: %v", err)
	}
	for _, bad := range []people.CreateAccount{
		{ID: people.NewAccountID(), Email: "not an email", Username: "bob", Role: people.NoRole, PasswordHash: h},
		{ID: people.NewAccountID(), Email: "bob@example.com", Username: "b", Role: people.NoRole, PasswordHash: h},
		{ID: people.NewAccountID(), Email: "bob@example.com", Username: "bob", Role: "root", PasswordHash: h},
		{ID: "a-1", Email: "bob@example.com", Username: "bob", Role: people.NoRole, PasswordHash: h},
		{ID: people.NewAccountID(), Email: "bob@example.com", Username: "bob", Role: people.NoRole, PasswordHash: "plain"},
	} {
		if err := e.W.Do(ctx, bad); !errors.Is(err, writer.ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}
