package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zweiundeins/lith/sqlite"

	"github.com/mbolli/tickit/internal/people"
)

// NFR-07.3: a staff account's password comes from the environment or
// standard input, never from a flag.
func TestAccountCreate(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tickit.db")
	t.Setenv("TICKIT_PASSWORD", "correct horse battery")
	if err := account([]string{"create", "-db", db, "-email", "ada@example.com", "-username", "ada", "-name", "Ada Admin", "-role", "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := account([]string{"create", "-db", db, "-email", "ada@example.com", "-username", "ada2"}); err == nil || !strings.Contains(err.Error(), "email exists") {
		t.Errorf("the same email again: %v", err)
	}
	if err := account([]string{"create", "-db", db, "-email", "bob@example.com", "-username", "bob", "-role", "root"}); err == nil {
		t.Error("a role that is none")
	}
	t.Setenv("TICKIT_PASSWORD", "short")
	if err := account([]string{"create", "-db", db, "-email", "bob@example.com", "-username", "bob"}); err == nil {
		t.Error("a short password")
	}
	d, err := sqlite.Open(context.Background(), db, sqlite.Options{ReadOnly: true, Readers: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c, err := people.Verify(context.Background(), d.R, "ada@example.com", "correct horse battery")
	if err != nil || c.Person.Role != people.Admin || c.Person.Name != "Ada Admin" {
		t.Errorf("the account: %+v %v", c.Person, err)
	}
}

func TestReadPassword(t *testing.T) {
	t.Setenv("TICKIT_PASSWORD", "")
	os.Unsetenv("TICKIT_PASSWORD") // restored after the test
	if pw, err := readPassword(strings.NewReader("secret words\nmore")); err != nil || pw != "secret words" {
		t.Errorf("from stdin: %q %v", pw, err)
	}
	if _, err := readPassword(strings.NewReader("")); err == nil {
		t.Error("an empty stdin")
	}
}

// NFR-76: metrics only on loopback.
func TestLoopback(t *testing.T) {
	for addr, ok := range map[string]bool{"": true, "127.0.0.1:7205": true, "[::1]:7205": true, "localhost:7205": true, "0.0.0.0:7205": false, ":7205": false, "192.0.2.1:7205": false, "nope": false} {
		if err := loopback(addr); (err == nil) != ok {
			t.Errorf("%q: %v", addr, err)
		}
	}
}
