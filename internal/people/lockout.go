package people

import (
	"strings"
	"sync"
	"time"
)

// Lockout keeps an address from guessing one account's password: after
// Tries failed logins for one email from one address within Window, that
// address waits Wait before it may try that email again (NFR-09.2). It
// counts emails, not accounts, so an email without an account locks the
// same way (NFR-10). It lives in memory: a deploy forgets it, which only
// lets an attacker start over at the per-address rate limit.
type Lockout struct {
	Tries  int
	Window time.Duration
	Wait   time.Duration

	mu    sync.Mutex
	keys  map[string]*lockState
	swept time.Time // the last forget
}

type lockState struct {
	fails []time.Time // within Window, oldest first
	until time.Time   // locked until then
}

// NewLockout returns the lockout of NFR-09.2: 5 failures in 15 minutes
// lock for 15 minutes.
func NewLockout() *Lockout {
	return &Lockout{Tries: 5, Window: 15 * time.Minute, Wait: 15 * time.Minute}
}

// LockoutKey is the key of an email tried from a client address
// (edge.ClientKey). No email has more than 254 bytes.
func LockoutKey(email, addr string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > 254 {
		email = email[:254]
	}
	return email + "\x00" + addr
}

// MaxLockoutKeys bounds the keys a Lockout keeps: past it, the keys that
// are not locked go first.
const MaxLockoutKeys = 50_000

// Len is how many keys l keeps.
func (l *Lockout) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.keys)
}

// Locked reports whether key must wait.
func (l *Lockout) Locked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.keys[key]
	return s != nil && now.Before(s.until)
}

// Fail counts a failed login of key, and locks it at the Tries-th failure
// within Window.
func (l *Lockout) Fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.keys == nil {
		l.keys = map[string]*lockState{}
	}
	if len(l.keys) >= MaxLockoutKeys/5 && now.Sub(l.swept) > time.Minute {
		l.forget(now)
		l.swept = now
	}
	if len(l.keys) >= MaxLockoutKeys { // a flood: forget the failures, keep the locks
		for k, s := range l.keys {
			if !now.Before(s.until) {
				delete(l.keys, k)
			}
		}
	}
	s := l.keys[key]
	if s == nil {
		s = &lockState{}
		l.keys[key] = s
	}
	s.fails = append(recent(s.fails, now.Add(-l.Window)), now)
	if len(s.fails) >= l.Tries {
		s.until, s.fails = now.Add(l.Wait), nil
	}
}

// Succeed forgets key's failures after a login that worked.
func (l *Lockout) Succeed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.keys, key)
}

// forget drops the keys that neither wait nor have failures within Window.
func (l *Lockout) forget(now time.Time) {
	for k, s := range l.keys {
		if s.fails = recent(s.fails, now.Add(-l.Window)); len(s.fails) == 0 && !now.Before(s.until) {
			delete(l.keys, k)
		}
	}
}

func recent(fails []time.Time, since time.Time) []time.Time {
	i := 0
	for i < len(fails) && !fails[i].After(since) {
		i++
	}
	return fails[i:]
}
