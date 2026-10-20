package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/migrations"
)

// account makes a staff account on the host, the only way there is to
// make one besides an admin's page (FR-141, Q14):
//
//	TICKIT_PASSWORD=... tickit account create -email ada@example.com -username ada -role admin
//
// The password comes from TICKIT_PASSWORD, or else the first line of
// standard input, never from a flag, which other users can see in the
// process list (NFR-07.3).
func account(args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return errors.New("usage: tickit account create -email ... -username ... [-name ...] [-role none|moderator|admin]")
	}
	f := flags("account create")
	dbPath := f.String("db", "data/tickit.db", "the SQLite database")
	email := f.String("email", "", "the account's email address")
	username := f.String("username", "", "the account's username, 2 to 50 letters, digits, '.', '-' or '_'")
	name := f.String("name", "", "the account's full name, shown beside what it does (default: the username)")
	role := f.String("role", "none", "the site role: none, moderator or admin")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	password, err := readPassword(os.Stdin)
	if err != nil {
		return err
	}
	if err := people.CheckPolicy(password); err != nil {
		return err
	}
	hash, err := people.HashPassword(password)
	if err != nil {
		return err
	}
	cmd := people.CreateAccount{
		ID: people.NewAccountID(), Email: *email, Username: *username, FullName: *name,
		Role: people.SiteRole(*role), PasswordHash: hash, At: time.Now(),
	}
	if err := cmd.Validate(); err != nil {
		return err
	}

	ctx := context.Background()
	db, err := sqlite.Open(ctx, *dbPath, sqlite.Options{Migrations: migrations.FS, Readers: 1})
	if err != nil {
		return err
	}
	w, err := writer.New(db, writer.Options{})
	if err != nil {
		return errors.Join(err, db.Close())
	}
	go w.Run(context.Background())
	err = w.Do(ctx, cmd)
	if err := errors.Join(w.Close(), db.Close()); err != nil {
		return err
	}
	var refusal *writer.Refusal
	if errors.As(err, &refusal) {
		return errors.New(refusal.Msg)
	}
	if err != nil {
		return err
	}
	slog.Info("account created", "id", cmd.ID, "username", cmd.Username, "role", cmd.Role)
	return nil
}

// readPassword takes the password from TICKIT_PASSWORD, or else from the
// first line of r.
func readPassword(r io.Reader) (string, error) {
	if pw, ok := os.LookupEnv("TICKIT_PASSWORD"); ok {
		return pw, nil
	}
	fmt.Fprint(os.Stderr, "Password (8 to 128 characters), then Enter: ")
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || line == "") {
		return "", fmt.Errorf("no password on standard input: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
