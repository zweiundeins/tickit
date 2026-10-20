package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/zweiundeins/lith/sqlite"

	"github.com/mbolli/tickit/migrations"
)

// migrationsPending prints how many of the binary's migrations the
// database has not had yet, which is what the binary applies when it
// opens it: tickit-deploy takes a copy first when that is more than 0
// (NFR-71). A database newer than the binary is an error, as it is for
// sqlite.Open (NFR-65).
func migrationsPending(args []string, out io.Writer) error {
	f := flags("migrations-pending")
	dbPath := f.String("db", "data/tickit.db", "the SQLite database")
	if err := parse(f, args); err != nil {
		return err
	}
	n, err := pending(context.Background(), *dbPath, migrations.FS)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, n)
	return err
}

// pending counts the NNN_name.sql files in fsys numbered above the
// database's user_version. It opens the database read-only, so it never
// creates or changes it.
func pending(ctx context.Context, path string, fsys fs.FS) (int, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return 0, err
	}
	var nums []int
	for _, e := range entries {
		num, _, ok := strings.Cut(e.Name(), "_")
		n, err := strconv.Atoi(num)
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") || !ok || err != nil {
			continue
		}
		nums = append(nums, n)
	}
	if _, err := os.Stat(path); err != nil {
		return 0, err
	}
	db, err := sqlite.Open(ctx, path, sqlite.Options{ReadOnly: true, Readers: 1})
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var current int
	if err := db.R.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return 0, err
	}
	count, newest := 0, 0
	for _, n := range nums {
		newest = max(newest, n)
		if n > current {
			count++
		}
	}
	if current > newest {
		return 0, fmt.Errorf("the database is at version %d, newer than this binary's newest migration (%d)", current, newest)
	}
	return count, nil
}

// checkCopy runs SQLite's integrity check on a backup copy (NFR-70).
func checkCopy(ctx context.Context, path string) error {
	db, err := sqlite.Open(ctx, path, sqlite.Options{ReadOnly: true, Readers: 1})
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err := db.R.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("integrity_check: " + result)
	}
	return nil
}
