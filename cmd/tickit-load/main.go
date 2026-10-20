// Command tickit-load measures tickit at the capacity of NFR-38, as a
// client of the real binary in its own process.
//
//	tickit-load gen -db PATH       write the data of NFR-38.2 into a new database
//	tickit-load run [flags]        start the binary on a copy of it, drive the load, report
//
// gen goes through the real commands and the writer, so the projections and
// the search index are what the server would hold. run starts `tickit
// serve` as a separate process with GOMEMLIMIT and the stream cap of the
// unit, opens 200 streams as browsers do (Go HTTP clients reading SSE with
// Accept-Encoding identity or zstd), sends the actions on their schedule,
// measures, restarts the server and checks the reconnect, then prints the
// numbers per requirement. `go tool task load` runs both; benchmarks/RESULTS.md
// has the results.
package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tickit-load gen|run [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "gen":
		err = gen(os.Args[2:])
	case "run":
		err = run(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q: gen or run", os.Args[1])
	}
	if err != nil {
		slog.Error(os.Args[1], "err", err)
		os.Exit(1)
	}
}
