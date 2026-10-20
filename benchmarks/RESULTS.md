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

## Measuring

`lith/loadtest` runs the writer and the render loop in process, without HTTP: N streams watch the views while M producers send commands, and it reports tick percentiles, renders per tick, bytes per frame and heap per stream. Run it before and after a change and record both runs here; `go doc github.com/zweiundeins/lith/loadtest` has its API. In production the writer's `Stats()` and the hub's overrun log (n, p50, p95, p99 and max of the ticks over 20 seconds) show the same quantities. For the whole path, drive the binary from `go tool task build` with a browser or a load tool on other cores (`taskset`) or another machine. The lith skill's `references/performance.md` has the numbers measured on the other projects and the settings they led to.
