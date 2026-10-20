package people_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
)

// NFR-41: many sign-ins at once hash a few at a time, so their 19 MiB
// each stay within the process's memory.
func TestHashesAtOnce(t *testing.T) {
	h, err := people.HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	people.ResetHashingPeak()
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() { people.CheckPassword(h, "wrong horse battery") })
	}
	wg.Wait()
	if peak := people.HashingPeak(); peak > people.MaxHashing {
		t.Errorf("%d hashes ran at once, more than %d", peak, people.MaxHashing)
	}
}

// NFR-09.2, NFR-41: a key holds at most an email's length, and a flood
// of emails keeps the lockout bounded while locked keys stay locked.
func TestLockoutBounded(t *testing.T) {
	if k := people.LockoutKey(strings.Repeat("a", 4000)+"@example.com", "203.0.113.1"); len(k) > 300 {
		t.Errorf("a key of %d bytes", len(k))
	}
	l := people.NewLockout()
	now := time.Now()
	locked := people.LockoutKey("ada@example.com", "203.0.113.1")
	for range l.Tries {
		l.Fail(locked, now)
	}
	for i := range people.MaxLockoutKeys + 10 {
		l.Fail(people.LockoutKey(strings.Repeat("x", 200)+string(rune('a'+i%26))+time.Duration(i).String(), "198.51.100.1"), now)
	}
	if n := l.Len(); n > people.MaxLockoutKeys {
		t.Errorf("%d keys", n)
	}
	if !l.Locked(locked, now) {
		t.Error("a flood of emails unlocked an address")
	}
}
