package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/demo/seed"
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/migrations"
)

// seedCommand writes the demo's seed (DM-02) into a database for
// development: a new one, or with -force one whose content it deletes
// first, as the nightly reset does. The server seeds an empty database by
// itself with -dev; in production the content comes from the nightly
// reset or "Reset now" (docs/deploy.md).
func seedCommand(args []string) error {
	f := flags("seed")
	dbPath := f.String("db", "data/tickit.db", "the SQLite database")
	force := f.Bool("force", false, "delete the demo's content first, as the nightly reset does; staff accounts stay")
	if err := parse(f, args); err != nil {
		return err
	}
	ctx := context.Background()
	store, err := events.New(parts()...)
	if err != nil {
		return err
	}
	tracker.Bind(store)
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		return err
	}
	db, err := sqlite.Open(ctx, *dbPath, sqlite.Options{Migrations: migrations.FS, Readers: 2})
	if err != nil {
		return err
	}
	w, err := writer.New(db, writer.Options{})
	if err != nil {
		return errors.Join(err, db.Close())
	}
	go w.Run(context.Background())
	defer func() { w.Close(); db.Close() }()
	empty, err := demo.Empty(ctx, db.R)
	if err != nil {
		return err
	}
	if !empty && !*force {
		return fmt.Errorf("%s holds content already: -force deletes it first, as the nightly reset does", *dbPath)
	}
	c, err := seed.Load()
	if err != nil {
		return err
	}
	epoch, err := demo.Epoch(ctx, db.R)
	if err != nil {
		return err
	}
	if !empty {
		if err := w.Do(ctx, writer.Func(func(tx *writer.Tx) (err error) { epoch, err = demo.Wipe(tx); return err })); err != nil {
			return err
		}
	}
	n, err := demo.Seed(ctx, w, c, time.Now(), epoch)
	if err != nil {
		return err
	}
	slog.Info("seeded", "db", *dbPath, "projects", n.Projects, "issues", n.Issues, "changes", n.Changes, "comments", n.Comments, "page steps", n.PageSteps, "took", n.Took.Round(time.Millisecond))
	return nil
}

// seedIfEmpty seeds a database without content, for -dev.
func seedIfEmpty(ctx context.Context, db *sqlite.DB, w *writer.Writer, d *demo.Demo, log *slog.Logger) error {
	empty, err := demo.Empty(ctx, db.R)
	if err != nil || !empty {
		return err
	}
	epoch, err := demo.Epoch(ctx, db.R)
	if err != nil {
		return err
	}
	n, err := demo.Seed(ctx, w, d.Content(), time.Now(), epoch)
	if err != nil {
		return err
	}
	log.Info("seeded an empty database", "issues", n.Issues, "took", n.Took.Round(time.Millisecond))
	return nil
}
