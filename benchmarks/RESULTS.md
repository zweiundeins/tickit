# Benchmark results

Numbers measured on this project. Each one carries its date, the machine, the settings and the exact command that produced it, so a later run can be compared with it. Measure again before quoting a number for another machine.

## Machine

Date, CPU and cores, memory, disk and filesystem, OS, Go version, modernc.org/sqlite version.

## Writer

Commands per second and commands per transaction with 1, 50 and 200 producers, with `synchronous` FULL and NORMAL.

## Render loop

Tick duration (p50, p95, p99, max), renders per tick, overruns.

## Streams

Bytes per frame on the wire, heap per open stream, CPU per frame.

## End to end

Time from an action to its frame in the browser, with the number of tabs open.

## Measuring

`lith/loadtest` runs the writer and the render loop in process, without HTTP: N streams watch the views while M producers send commands, and it reports tick percentiles, renders per tick, bytes per frame and heap per stream. Run it before and after a change and record both runs here; `go doc github.com/zweiundeins/lith/loadtest` has its API. In production the writer's `Stats()` and the hub's overrun log (n, p50, p95, p99 and max of the ticks over 20 seconds) show the same quantities. For the whole path, drive the binary from `go tool task build` with a browser or a load tool on other cores (`taskset`) or another machine. The lith skill's `references/performance.md` has the numbers measured on the other projects and the settings they led to.

## The board (NFR-37, NFR-38)

2026-10-20, Intel i5-13500 (20 threads), 62 GB, Linux 6.18.38 (Unraid), Go 1.27.2, modernc.org/sqlite v1.60.1, the server in process (`webtest`: hub tick 10 ms, 2 render workers), streams read without compression.

- **Frame of 100 cards** (`go test ./internal/web -run TestBoardFrameSize -v`): 58,878 bytes of HTML in one patch, 588 bytes a card (`trackertest.Board`: every card with type, key, summary, priority and face or "Unassigned"; a quarter with an epic, a third with labels, a fifth with points, a sixth with a due date). Read and rendered in 1.8 ms on average over 50 runs. Before the cards referred to the page's glyphs (`ui.BoardCard`, `ui.Glyphs`), the same board was 316 KB with `ui.Card`, then 108 KB with SVG `<use>` references.
- **100 streams on one board, one move a second** (`TICKIT_LOAD_SECONDS=30 go test ./internal/web -run TestBoardLoad -v`): 30 moves, 30 renders of the board's one view (`RendersByKey`), 3,000 frames of 58,877 bytes, 30 to each stream. From a move's request to its frame on the slowest of the 100 streams: p50 3.6 ms, p95 7.3 ms, max 7.6 ms. The test process (server, 100 stream readers, a page load and a move a second) used 2.4 s of CPU in 29.2 s, 8.3% of one core. The 10-second run: p50 3.6 ms, p95 5.9 ms, 19% of a core while the 100 streams opened.
- **In browsers** (`CHROME=/usr/bin/chromium LITH_CHROME_NO_SANDBOX=1 go test ./e2e -run 'TestBoardMoveLive|TestBoardPresence' -v`, Chromium 154 headless, three browser contexts on loopback): a card dropped in one shows in the two others 22 to 74 ms after the drop over five runs (section 12 item 4 asks for 250 ms); a closed tab's face leaves the others' presence row after 50 to 55 ms (MP-02.3 asks for 1 s).
