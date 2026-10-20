package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/migrations"
)

// NFR-71: the deploy script asks the new binary whether it migrates the
// database, and takes a copy first if it does.
func TestPending(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tickit.db")
	db, err := sqlite.Open(ctx, path, sqlite.Options{Readers: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fsys := fstest.MapFS{"001_a.sql": {}, "002_b.sql": {}, "010_c.sql": {}, "migrations.go": {}}
	for version, want := range map[int]int{0: 3, 1: 2, 2: 1, 9: 1, 10: 0} {
		if _, err := db.W.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(version)); err != nil {
			t.Fatal(err)
		}
		if n, err := pending(ctx, path, fsys); err != nil || n != want {
			t.Errorf("at version %d: %d %v, want %d", version, n, err, want)
		}
	}
	// NFR-65: an older binary refuses a newer schema, here before the restart.
	if _, err := db.W.ExecContext(ctx, "PRAGMA user_version = 11"); err != nil {
		t.Fatal(err)
	}
	if _, err := pending(ctx, path, fsys); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Errorf("a newer database: %v", err)
	}
	if _, err := pending(ctx, filepath.Join(t.TempDir(), "none.db"), fsys); err == nil {
		t.Error("a missing database")
	}
}

func TestMigrationsPendingCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tickit.db")
	db, err := sqlite.Open(context.Background(), path, sqlite.Options{Migrations: migrations.FS, Readers: 1})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	var out bytes.Buffer
	if err := migrationsPending([]string{"-db", path}, &out); err != nil || out.String() != "0\n" {
		t.Errorf("a migrated database: %q %v", out.String(), err)
	}
}

// NFR-70: each copy passes PRAGMA integrity_check.
func TestBackupChecksCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tickit.db")
	db, err := sqlite.Open(context.Background(), path, sqlite.Options{Migrations: migrations.FS, Readers: 1})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	backups := filepath.Join(dir, "backups")
	if err := backup([]string{"-db", path, "-backups", backups, "-keep", "2"}); err != nil {
		t.Fatal(err)
	}
	copies, _ := filepath.Glob(filepath.Join(backups, "tickit-*.db"))
	if len(copies) != 1 {
		t.Fatalf("copies: %v", copies)
	}
	if err := checkCopy(context.Background(), copies[0]); err != nil {
		t.Error(err)
	}
	junk := filepath.Join(dir, "junk.db")
	if err := os.WriteFile(junk, bytes.Repeat([]byte("not a database "), 512), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkCopy(context.Background(), junk); err == nil {
		t.Error("a file that is no database passes the check")
	}
}
