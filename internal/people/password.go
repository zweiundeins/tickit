package people

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Password lengths in characters (NFR-07.1, Q23). Every character counts:
// argon2id hashes the whole password, whatever its length.
const (
	MinPassword = 8
	MaxPassword = 128
)

// ErrPasswordPolicy is the error of a password that does not fit.
var ErrPasswordPolicy = errors.New("a password has 8 to 128 characters")

// CheckPolicy accepts a password of 8 to 128 characters, of any kind
// (NFR-07, no composition rule).
func CheckPolicy(password string) error {
	if n := utf8.RuneCountInString(password); n < MinPassword || n > MaxPassword || !utf8.ValidString(password) {
		return ErrPasswordPolicy
	}
	return nil
}

// argon2id's parameters: OWASP's 19 MiB, two passes, one lane.
const (
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB
	argonThreads = 1
	argonKey     = 32
	argonSalt    = 16
)

// MaxHashing is how many argon2id hashes run at once: each holds 19 MiB,
// and many sign-ins from many addresses must not take the memory of the
// process (NFR-41). The others wait their turn.
const MaxHashing = 4

var (
	hashing = make(chan struct{}, MaxHashing)
	// hashingNow and hashingPeak count the hashes running, for the test of MaxHashing.
	hashingNow, hashingPeak atomic.Int32
)

func idKey(password, salt []byte, time, memory uint32, threads uint8, n uint32) []byte {
	hashing <- struct{}{}
	defer func() { <-hashing }()
	now := hashingNow.Add(1)
	defer hashingNow.Add(-1)
	for p := hashingPeak.Load(); now > p && !hashingPeak.CompareAndSwap(p, now); p = hashingPeak.Load() {
	}
	return argon2.IDKey(password, salt, time, memory, threads, n)
}

// HashPassword returns password's argon2id hash as a PHC string:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSalt)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := idKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKey)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// CheckPassword reports whether password matches the encoded hash, which
// HashPassword made. The keys are compared in constant time.
func CheckPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil || memory == 0 || time == 0 || threads == 0 {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := idKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is checked for an email without an account, so that its
// login does the same work as a wrong password (NFR-10).
var dummyHash = func() string {
	h, err := HashPassword("not the password of anyone")
	if err != nil {
		panic(err)
	}
	return h
}()
