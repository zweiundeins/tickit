package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/pprof"
	"slices"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
	"github.com/zweiundeins/tickit/migrations"
)

// fold times wiki.Fold on a database (a copy: it folds), n commands of
// each limit, for NFR-43.3: how long one fold command holds the writer.
func fold(args []string) error {
	f := flag.NewFlagSet("fold", flag.ExitOnError)
	dbPath := f.String("db", "", "a copy of a database after a sustained run (it is changed)")
	n := f.Int("n", 10, "fold commands per limit")
	prof := f.String("cpuprofile", "", "write a CPU profile here")
	f.Parse(args)
	if *prof != "" {
		pf, err := os.Create(*prof)
		if err != nil {
			return err
		}
		pprof.StartCPUProfile(pf)
		defer pprof.StopCPUProfile()
	}
	if *dbPath == "" {
		return errors.New("fold: -db is required")
	}
	store, err := events.New(parts()...)
	if err != nil {
		return err
	}
	tracker.Bind(store)
	wiki.Bind(store)
	ctx := context.Background()
	db, err := sqlite.Open(ctx, *dbPath, sqlite.Options{Migrations: migrations.FS, Readers: 2})
	if err != nil {
		return err
	}
	defer db.Close()
	w, err := writer.New(db, writer.Options{})
	if err != nil {
		return err
	}
	go w.Run(context.Background())
	defer w.Close()
	for _, limit := range []int{wiki.FoldBatch, 100, 50, 25, 10} {
		var took []time.Duration
		deleted := int64(0)
		for range *n {
			c := &wiki.Fold{At: time.Now(), Limit: limit}
			if err := w.Do(ctx, c); err != nil {
				return err
			}
			if c.Deleted == 0 {
				break
			}
			deleted += c.Deleted
			took = append(took, c.Took)
		}
		slices.Sort(took)
		if len(took) == 0 {
			fmt.Println("nothing left to fold")
			return nil
		}
		fmt.Printf("limit %3d: %d commands, %d events, in the writer p50 %v max %v, %.3f ms an event\n", limit, len(took), deleted,
			took[len(took)/2].Round(10*time.Microsecond), took[len(took)-1].Round(10*time.Microsecond), float64(sum(took).Microseconds())/1000/float64(deleted))
	}
	return nil
}

func sum(ds []time.Duration) (t time.Duration) {
	for _, d := range ds {
		t += d
	}
	return t
}
