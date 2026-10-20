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

## Search and the issue list (NFR-40)

20 October 2026, Intel i5-13500 (14 cores, 20 threads), 62 GB, Linux 6.18 (Unraid), the test database in the test's temporary directory, Go 1.27.2, modernc.org/sqlite v1.60.1, other jobs running. `go test ./internal/web -run TestSearchSpeed -v` (skipped with `-short`): 10,000 issues in two projects made through `tracker.CreateIssue`, 500 to a transaction (4.7 s with the tracker's projections and the search index), and 2,000 pages of 200 to 600 words put in the index through its hook, as the wiki's projector would. Texts of 3 to 7 words for summaries and 20 to 80 for descriptions, from a list of 100 words, so common words match thousands of documents.

| What | n | p50 | p95 | max |
| --- | --- | --- | --- | --- |
| `search.Search`, 20 hits of eight texts (a common word, two words, a key, a prefix, three words, none, another key, two words) | 200 | 28.4 ms | 50.7 ms | 64.3 ms |
| `GET /search?q=`, the whole page behind the edge | 100 | 61.7 ms | 80.0 ms | 89.1 ms |
| `GET /search/suggest?q=`, the typeahead (two searches, 5 issues and 5 pages) | 100 | 55.5 ms | 87.9 ms | 97.3 ms |
| `GET /issues?<filters>`, six filtered and sorted lists, the whole page | 100 | 40.3 ms | 59.5 ms | 64.1 ms |
| `tracker.Search`, a filtered page of 25 | 200 | 17.7 ms | 37.8 ms | 51.0 ms |

Every one is under the 200 ms of NFR-40 at the 95th percentile; the test fails above it. A common word costs most: the index ranks every match with bm25 before it cuts the page.

## Co-editing a page (MP-05.3, MP-05.4, NFR-43.3)

20 October 2026, Intel i5-13500 (14 cores, 20 threads), 62 GB, Linux 6.18 (Unraid), Go 1.27.2, headless Chromium 154, other jobs running. The real server (`e2e/wiki_server_test.go`: the database file, writer, hub with its 50 ms tick, events runner, the wiki's jobs) on loopback, so with no network delay: MP-05.3 asks for a 50 ms round trip, which these runs do not add. Every editor types its own letter into the first paragraph of one page, 5 characters a second, from a client address of its own (`X-Real-IP` from loopback, as Caddy reports it: on one address, 20 editors exceed the address's limit of 40 commits a second and wait for 429s). Keystroke to the other editors: from the keystroke to the moment each other editor's document holds it (browsers: a `MutationObserver` counting the letter in `editor.text()`; Go clients: the first frame whose `batches` carries the commit). Writer per commit: `wiki.Commit`'s time inside the writer (the rebase, the rules, the append and the projections).

| Run | Keystrokes | p50 | p95 | max | Settled after the last | Writer per commit p50 / p95 / max |
| --- | --- | --- | --- | --- | --- | --- |
| 5 browser editors, 60 s | 1,500 | 34 ms | 59 ms | 82 ms | 48 ms | 0.20 / 0.63 / 2.5 ms (1,500 commits) |
| 20 browser editors, 30 s | 3,000 | 77 ms | 152 ms | 282 ms | 110 ms | 0.25 / 0.40 / 14.5 ms (3,000 commits) |
| 5 Go clients, 30 s | 745 | 5 ms | 19 ms | 55 ms | | 0.13 / 0.72 / 2.6 ms (745 commits) |
| 20 Go clients, 60 s | 5,980 | 60 ms | 75 ms | 111 ms | | 0.23 / 1.3 / 20 ms (5,980 commits) |

Every run ended with identical documents in every editor and on the server, holding every typed character, and the page loaded again showed the same text. Commands:

    CHROME=/usr/bin/chromium LITH_CHROME_NO_SANDBOX=1 go test ./e2e -run 'TestCoEditing$' -v                                   # 5 editors, 60 s
    CHROME=/usr/bin/chromium LITH_CHROME_NO_SANDBOX=1 COEDIT_EDITORS=20 COEDIT_SECONDS=30 go test ./e2e -run 'TestCoEditing$' -v
    LOAD_EDITORS=20 LOAD_SECONDS=60 go test ./e2e -run TestCoEditingLoad -v                                                     # Go clients

The fold (NFR-43.3): `go test ./internal/wiki -run TestFoldTime -v`, a page of 116 KB with 500 commits since its creation, then a version: one `wiki.Fold` command folds 200 events (`FoldBatch`) in 3.6 ms through the writer, 8.1 ms the first time (cold statements), and the last 102 in 2.1 ms. With 500 events a command it took 9.2 ms, so `FoldBatch` is 200. A commit on that page takes 0.07 to 0.18 ms in the writer.

## Measuring

`lith/loadtest` runs the writer and the render loop in process, without HTTP: N streams watch the views while M producers send commands, and it reports tick percentiles, renders per tick, bytes per frame and heap per stream. Run it before and after a change and record both runs here; `go doc github.com/zweiundeins/lith/loadtest` has its API. In production the writer's `Stats()` and the hub's overrun log (n, p50, p95, p99 and max of the ticks over 20 seconds) show the same quantities. For the whole path, drive the binary from `go tool task build` with a browser or a load tool on other cores (`taskset`) or another machine. The lith skill's `references/performance.md` has the numbers measured on the other projects and the settings they led to.
