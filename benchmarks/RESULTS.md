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

## Capacity: the load of NFR-38 (NFR-35 to NFR-43)

20 October 2026, commit `ff4d49a` (`rw/load`, with `rewrite` merged). Intel Core i5-13500 (14 cores, 20 threads; `lscpu`: 6 P-cores with two threads each as CPUs 0 to 11, 8 E-cores as 12 to 19, up to 4.8 GHz), 62 GB, Linux 6.18.38 (Unraid) in a container, the databases in `/tmp` on btrfs on an SSD, Go 1.27.2, modernc.org/sqlite v1.60.1, lith v0.3.0. Other agents' jobs ran on the machine; the load average was 2 to 4 during the run recorded here (it was 19 during an earlier one, whose numbers were worse and are not used).

**How it runs.** `cmd/tickit-load` is a client process of the real binary (`go tool task load`):

- `tickit-load gen -db base.db` writes the seed and then the data of NFR-38.2 through the real commands, 1,000 to a writer transaction, with the events runner going as in the server, so every projection, the search index and the notifications' checkpoints are what a server would hold: 4 more projects with 9,861 issues (one, LOAD, has 100 cards on its board and 2,400 issues in its backlog), 30,000 comments, 10 more spaces with 1,958 pages of 200 to 600 words, up to 5 levels deep, and field changes (status, priority, assignee, points, summary, labels) up to 1,000,000 events, times spread over 30 days. It took 10 min 48 s and 13 min 43 s in two runs, with other jobs on the machine (the catch-up projectors take the last few minutes), and gave a 526 MiB database (`/tmp/tickit-load/base.db`, 10,000 issues, 2,000 pages, 1,000,000 events).
- `tickit-load run` copies it, starts `tickit serve -streams 200` with `GOMEMLIMIT=448MiB` (the unit's settings; no cgroup could be set in the container, so MemoryMax is checked against the measured RSS), pinned with `taskset -c 0-5` (so GOMAXPROCS 6), and itself runs on CPUs 12 to 19. It opens 200 tabs as browsers do: each its own cookie jar and its own client address (`X-Real-IP` from loopback, as Caddy sets it, so every simulated person counts against the limits of an address of its own, the per-address cap of 40 streams included), loads the tab's document, reads `_tab` and `_views` from it and opens `/stream` with them, half with `Accept-Encoding: identity`, half with `zstd`, decoding as a browser does; 10 of the board tabs and 5 of the readers send `Save-Data: on`. The tabs: 100 on LOAD's board, 20 editors of one page (each posts `open`, then commits and presence as `sb-rich-editor` does: one commit in flight, typed keystrokes in the next, on the newest version it saw, the server rebasing), 20 readers of that page, 30 on the 10 issues whose fields change (three each), 20 on the issue list of LOAD sorted by update (25 rows), 10 on the home page. Then a 201st stream.
- The load: a card moves a second (the mover reads the card's version from its frames), each of the 10 issues has a field changed once a second (priority, points, summary in turn, with the version its page showed), and 20 editors type 5 characters a second each. A document (home, board, issue, list, page in turn) loads every 200 ms. 5 s of warm-up, 120 s measured, then the server gets SIGTERM and starts again on the same database while the tabs reconnect as Datastar's retry does (at once, then after 1, 2, 4, 8 and 10 s), then 25 minutes more of the same load for the growth of the database.
- "Commit to frame" is measured from the moment the action's request goes out to the moment each watching tab has read the frame that shows it (the card at its new version, the issue's `$_is.max`, the commit's ref in an editor's `batches`): an upper bound of NFR-35's "commit to the start of the frame write", which includes the request, the tick's wait, the render, compression, loopback and the client's parsing. The server's side is `tickit_tick_seconds` and the overrun log.

| Requirement | Measured | Result |
| --- | --- | --- |
| NFR-38.1 | 200 streams open in 0.5 s, all fed for 120 s: 120 moves, 1,200 field changes, 12,000 commits (12,481 keystrokes), 18,830 presence posts, every one answered 204; every watcher got the frame of every change (12,000 of 12,000 move frames, 3,600 of 3,600 field frames, 228,000 of 228,000 commit frames) | pass, but see NFR-35 |
| NFR-38.2 | 10,000 issues, 2,000 pages, 1,000,000 events, 526 MiB | pass |
| NFR-35.1 | commit to frame: a move to the 90 board tabs without Save-Data p50 53.3 ms, p95 77.0 ms, max 98.9 ms (11,250 samples); a field p50 55.5 ms, p95 83.1 ms (3,723); a commit to the other editors p50 53.3 ms, p95 78.7 ms, max 160.5 ms (237,139). Ticks p50 about 25 ms (`tickit_tick_seconds`: 1,991 of 2,397 under 30 ms) | fail at the median by 3 to 6 ms of an upper bound; p95 passes. The tick is about half the wait |
| NFR-35.2 | 9 overruns in 2,397 ticks (0.38 %); the overrun log: p50 23 to 26 ms, p95 31 to 45 ms, max 59 to 71 ms per 20 s | fail (asks under 0.1 %) |
| NFR-36 | 32,150 actions answered 204: p50 0.6 ms, p95 3.6 ms, p99 8.5 ms, max 91.8 ms (moves p95 9.1 ms, fields 11.3 ms, commits 5.3 ms) | pass |
| NFR-37.1 | the list's 25 rows 36,313 bytes a frame (36,483 at most); LOAD's board of 100 cards 64,604 bytes | list pass; board fail: every generated card carries points, a label and a face (the board area's mix of 58,878 bytes passes, "The board" below) |
| NFR-37.2 | Save-Data tabs: board frames at most 4.0 a second, shortest gap 244 ms; page body 4.0 a second, 244 ms; the others got up to 10 board and 19.7 page frames a second | pass |
| NFR-39 | under the load, to the last byte: home p95 37.0 ms, board 25.5 ms, issue 31.0 ms, issue list 51.9 ms, page 90.1 ms (120 each); first frames of a stream from its request p50 52.4 ms, p95 72.0 ms, max 74.1 ms | documents pass; first frames come with the next tick plus its render, p95 22 ms past one tick: partly |
| NFR-41.1 | the 201st stream answered 503; RSS: anonymous at most 488.6 MiB in the measured run and 519.0 MiB over the 27 minutes, with the readers' mmap of the database up to 860.9 MiB more (VmHWM 1,107 MiB), which the kernel reclaims; Go heap 303 MiB, from the OS 438 MiB, the stream budget 181 MiB (100 identity streams, 100 at zstd 2^19, none refused); 1.98 cores | pass under MemoryMax 640M; the anonymous peak is above MemoryHigh (512M), where systemd throttles |
| NFR-42 | SIGTERM to exit 46 ms, back on `/healthz` 83 ms later; every one of the 200 streams had its first frame again 1.00 s (p50) to 1.03 s (max) after `/healthz`; the board's `data-rev` and the 10 issues' versions as before, the 20 editors `state="ok"` | pass |
| NFR-43.1 | the fold folded 72,315 commits in the last 12 minutes, as many as came; the page's document stream held 92,000 events throughout (15 minutes of commits) | pass |
| NFR-43.2 | the database without its WAL grew 0.43 MiB a minute once the fold ran (minutes 13 to 25), 620 MiB a day; of that, co-editing adds the versions (a 160 KB document every 10 minutes, twice: about 46 MB a day) and the 15 minutes before the fold (about 30 MB, steady); the rest is the 10 field changes and the move a second, 950,000 events a day that history keeps (about 650 bytes each with their indexes, projections and search index). The WAL stays at its high-water mark, 197 MiB, while commands never pause for a second | co-editing passes; the whole load of NFR-38 for 24 hours fails (about 800 MB with the WAL) |
| NFR-43.3 | every fold command held the writer 23.6 to 24.8 ms on average (1,444 commands, all over 5 ms); `tickit-load fold` on the database after the run: 17 to 25 ms whatever the batch (200, 100, 50, 25 or 10 events) | fail: see below |
| MP-05.1 | 12,481 keystrokes taken, 12,481 in the page | pass |

**What the load found, and what changed.** The first runs (30 s each, the same load, `-cpuprofile` on a binary built with `-tags pprof`):

- The catch-up projectors of notifications started 970,000 events behind on a database written without the runner, and held the writer 85 % busy: `gen` now runs the runner as the server does.
- Ticks took 250 to 300 ms (84 % overran), a commit took 480 ms to its frame, and actions answered at p95 65 ms. 51 % of the server's CPU was `activity.Mine` ("recently edited by you"), which read the newest 20,000 events back into Go for every home tab on every commit; 25 % was the home tab's "assigned to me", a scan of every issue. The home page's own view now lets SQLite filter the log by actor, skips a person without a row in `people` (who never acted), reads a person's issues by `issues_assignee` (`INDEXED BY`, as the planner preferred the projects by its statistics) and the open counts from the new covering index `issues_project_open` (migration 072). The editor's frame parsed the whole seed for its image library at every render (4 %): now once. After: ticks p50 25 ms, actions p95 3.6 ms, the server at 2 cores instead of 5.
- The issue list's frame of 25 rows was 72 KB: its icons and faces are now masks of the stylesheet, as the board's are (36 KB, visually the same in both themes).
- The fold folded at most 4,000 events a minute, against the editors' 6,000 to 7,000 commits: it now runs every 10 seconds, up to 20 commands.
- Every commit kept an op id with its result (`writer.OnceResult`), 165 bytes each, kept a day by `writer.PruneOnce(24h)`: 1.4 GB a day at 100 commits a second. A resent commit now applies once by its ref among the commits since its base, and no op id is kept.

**What is still open.**

- NFR-43.3: the fold's time is not its batch. lith's `events.Compact` deletes a page's folded events with `DELETE FROM lith_events WHERE stream = ? AND stream_id = ? AND position < ?`, which walks the stream's whole entry of the unique index `(stream, stream_id, version)` and looks up every row (84,000 rows for 0 deleted take 9 ms in the sqlite3 shell), and finds its cut with `WHERE stream = ? AND at >= ? ORDER BY position`, which scans the kind from its start. Both grow with the events of the page since the fold, 15 minutes of commits. A fix belongs in lith: delete by `version <` the cut event's version, which the unique index serves, and an index or a position bound for the cut. In tickit the stream could be kept small with a shorter `FoldAfter` and `CutInterval` (more versions, each a full document), which this work did not change.
- NFR-35: a tick costs about 25 ms on 6 cores; the biggest renders are the issue page's view (325 KB a frame, 415 KB at most, every field change: its history of about 100 entries renders with it), the home feed (81 KB) and ten home views, one per person, on every commit. Splitting the issue's history and activity into views of their own, and the home page's shared parts out of the per-person view, would cut the tick.
- NFR-37.1: the board of 100 fully decorated cards is 7 % over 60 KB.
- NFR-41: anonymous memory reached 519 MiB, above the unit's `MemoryHigh=512M`; `GOMEMLIMIT` of about 400 MiB, or `MemoryHigh` of 576M, would leave room.
- The WAL is truncated only after a second without commands (`sqlite.CheckpointWriter`), so under constant load it stays at its high-water mark (197 and 284 MiB in two runs).

Commands:

    go tool task load LOAD_SECONDS=120 LOAD_SUSTAIN=25m SERVER_CPUS=0-5 LOAD_CPUS=12-19
    # the same by hand:
    go build -o /tmp/tickit-load/tickit-load ./cmd/tickit-load && go tool task build
    /tmp/tickit-load/tickit-load gen -db /tmp/tickit-load/base.db
    taskset -c 12-19 /tmp/tickit-load/tickit-load run -bin ./tickit -base /tmp/tickit-load/base.db -seconds 120 -sustain 25m -server-cpus 0-5 -out results.json
    CGO_ENABLED=0 go build -tags pprof -o tickit-pprof ./cmd/tickit    # then run -bin ./tickit-pprof -cpuprofile cpu.pprof
    cp /tmp/tickit-load/run.db fold.db && /tmp/tickit-load/tickit-load fold -db fold.db

## Capacity on lith v0.4.1

20 October 2026, the same machine, settings and command as "Capacity" above (`go tool task load LOAD_SECONDS=120 LOAD_SUSTAIN=25m SERVER_CPUS=0-5 LOAD_CPUS=12-19`, the cached `base.db`, each build in a load directory of its own), Go 1.27.2, modernc.org/sqlite v1.60.1. Other agents' builds and browsers ran on the machine; the load average is given for each run, and it decides much: a v0.3.0 control run today, while this work's tests ran on CPUs 6 to 11, came out well below the recorded one. Migration 073 adds two indexes, which the server builds at its start on each run's copy of `base.db` (10,000 issues: milliseconds), so `base.db` was not generated again.

The builds: **v0.3.0** recorded (`ff4d49a`, above), the **control**, v0.3.0 again today (`0409d1b`); **v0.4.1**, the upgrade alone with what lith's changelog asks of an app (`6bf4926`: the stream cap per client, pacing, static files outside the session, the 409 of a tab without a stream); **tuned**, v0.4.1 with the fixes the profiles found (`e0928aa`, below).

| Requirement | v0.3.0 recorded (load 2 to 4) | control today (2.4 to 6.2) | v0.4.1 (1.0 to 3.4) | tuned (4.9 to 3.7) |
| --- | --- | --- | --- | --- |
| NFR-35.1 move to the 90 board tabs, p50 / p95 | 53.3 / 77.0 ms | 61.8 / 129.2 ms | 56.1 / 84.5 ms | **49.2** / 81.4 ms |
| NFR-35.1 field to its 3 tabs, p50 / p95 | 55.5 / 83.1 ms | 61.9 / 115.7 ms | 57.2 / 86.0 ms | 50.7 / 83.8 ms |
| NFR-35.1 commit to the other editors, p50 / p95 | 53.3 / 78.7 ms | 58.0 / 134.2 ms | 54.7 / 80.9 ms | **43.4** / 76.4 ms |
| Ticks: summed time, renders | | 70.8 s, 83,209 | 64.5 s, 86,936 | 32.0 s, 74,315 |
| NFR-35.2 overruns | 9 of 2,397 (0.38 %) | 141 of 2,336 (6.04 %) | 17 of 2,399 (0.71 %) | 13 of 2,386 (0.54 %) |
| NFR-36 actions, p50 / p95 / p99 | 0.6 / 3.6 / 8.5 ms | 0.6 / 7.3 / 23.1 ms | 0.6 / 3.7 / 8.1 ms | 0.8 / 5.5 / 25.4 ms |
| NFR-37.1 list of 25, board of 100 | 36,313 B, 64,604 B | 36,315 B, 64,605 B | 36,314 B, 64,604 B | 36,314 B, 64,604 B |
| issue view, home feed (average frame) | 325 KB, 81 KB | 323,540 B, 83,278 B | 322,979 B, 81,321 B | 324,152 B, 81,809 B |
| NFR-37.2 Save-Data board: rate, shortest gap | 4.0/s, 244 ms | 4.0/s, 224 ms | 4.0/s, 246 ms | 4.0/s, 227 ms |
| NFR-39 documents p95: home, board, issue, list, page | 37, 26, 31, 52, 90 ms | 61, 48, 85, 187, 286 ms | 40, 20, 26, 49, 89 ms | 31, 29, 33, 99, 160 ms |
| NFR-39 first frame of a stream, p50 / p95 | 52.4 / 72.0 ms | 53.8 / 80.7 ms | 52.5 / 66.3 ms | 55.9 / 70.2 ms |
| NFR-41 anonymous memory, measured run / whole run | 488.6 / 519.0 MiB | 500.1 / 509.7 MiB | 481.5 / 517.4 MiB | 497.0 / 530.3 MiB |
| NFR-41 Go heap at the end, server cores | 303 MiB, 1.98 | 348 MiB, 2.16 | 323 MiB, 2.19 | 314 MiB, **1.32** |
| NFR-42 stop, streams back after /healthz (p50 / max) | 46 ms, 1.00 / 1.03 s | 48 ms, 1.03 / 1.05 s | 46 ms, 1.03 / 1.04 s | 139 ms, 0.91 / 0.94 s |
| NFR-43.1 folded in the last 12 minutes | 72,315 | 72,327 | 72,102 | 72,315 |
| NFR-43.2 growth without the WAL (minutes 13 to 25); WAL | 0.43 MiB/min; 197 MiB | 0.43 MiB/min; 185 MiB | 0.43 MiB/min; 268 MiB | 0.43 MiB/min; 51 MiB |
| NFR-43.3 a fold command in the writer (mean per minute) | 23.6 to 24.8 ms | 29.0 to 30.3 ms | 27.9 to 31.4 ms | 21.0 to 21.9 ms |

Every run passed NFR-38.1 (200 streams, every frame of every change to every watcher), the 201st stream's 503, MP-05.1 and NFR-42's state after the restart. NFR-35's field changes still miss the median by 0.7 ms of an upper bound, and the overruns stay above 0.1 %.

**Short runs side by side.** Since the load average moves the numbers by more than the builds differ, the builds also ran 120 s each (no sustain), one after the other: **B** is v0.4.1 with the feeds' topics, **C** adds the home page's cache, **D** the feeds' reads (the tuned build adds the issue page's indexes to D).

| Run (load at start) | move p50 / p95 | field p50 / p95 | commit p50 / p95 | overruns | ticks summed, renders |
| --- | --- | --- | --- | --- | --- |
| v0.4.1 (10.6) | 58.8 / 88.3 ms | 57.0 / 90.0 ms | 55.4 / 83.7 ms | 3.01 % | 65.5 s, 86,173 |
| B (5.3) | 59.9 / 93.0 ms | 59.0 / 90.7 ms | 46.4 / 88.7 ms | 2.52 % | 43.1 s, 73,522 |
| v0.4.1 (4.0) | 58.3 / 85.0 ms | 57.9 / 88.2 ms | 55.2 / 82.4 ms | 1.04 % | 64.5 s, 86,526 |
| B (5.3) | 59.4 / 83.2 ms | 61.4 / 89.8 ms | 47.8 / 85.6 ms | 1.55 % | 44.5 s, 73,169 |
| v0.4.1 (2.1) | 54.0 / 78.7 ms | 57.6 / 86.5 ms | 55.1 / 81.0 ms | 0.83 % | 65.4 s, 86,594 |
| C (4.1) | 53.2 / 78.7 ms | 57.0 / 85.5 ms | 45.6 / 79.0 ms | 0.92 % | 39.9 s, 74,126 |
| v0.4.1 (5.8) | 56.0 / 86.2 ms | 57.4 / 87.8 ms | 55.4 / 82.5 ms | 0.38 % | 64.6 s, 86,200 |
| C (2.9) | 56.3 / 77.6 ms | 58.4 / 87.3 ms | 45.9 / 80.1 ms | 1.09 % | 40.5 s, 73,967 |
| D (2.5) | 48.0 / 70.1 ms | 51.6 / 80.8 ms | 43.3 / 71.1 ms | 0.04 % | 33.5 s, 74,887 |
| v0.4.1 (2.4) | 55.9 / 82.7 ms | 56.4 / 85.8 ms | 54.3 / 80.3 ms | 0.75 % | 63.6 s, 86,928 |

**What v0.4.1 gives by itself.** Next to v0.3.0 at a similar load it is about the same: commit to frame within a few ms either way, overruns 0.4 to 1 %, ticks summed 64 to 65 s; the hub's fewer allocations do not show at this load, whose ticks are reads in SQLite. Compared with the control under the heavier load of the same afternoon it is clearly better (p95 81 to 86 ms against 116 to 134 ms, overruns 0.7 % against 6 %), but that control ran beside this work's tests.

**What the profiles found** (`-tags pprof`, `run -cpuprofile`, 60 s), and changed in tickit:

- The home feeds and the ten home views listened to `lith:events`, which each of the editors' 100 commits a second touches, though a feed shows a page's versions and never its commits: they rendered on every tick. They listen to the topics of the events they show (`perf: feeds and the home page's view...`). Renders fell 15 % and the editors' frames came 9 ms sooner.
- The home view then still took 27 % of the server's CPU, on every change to an issue: `tracker.Projects` counted the open issues of every project for each of the ten people, and `wiki.SpaceByID` counted every space's pages, which the page never shows. The open counts are kept against the hub's version of the projects' topics (lith v0.4.0's `Hub.Version` and `Seq(ctx)`), the views that miss the cache in one tick share one read, and the spaces give their names only.
- The shared home feed took about 16 ms a render: it read the log's last windows whole, up to 20,000 events with their data, nearly all of them a co-edited page's commits. It reads each source's kind by the log's index of positions and decodes only the types it shows. With this the overruns fell to 0.04 % in their run and moves passed NFR-35.
- The issue view read the project's epics by walking its issues in rank order and its 25 issues changed last through `tracker.Search`, which counts and sorts the whole project; migration 073 indexes both and `tracker.Recent` reads the 25.

**What is still open.**

- NFR-35: field changes 0.7 ms over the median, and overruns at 0.5 %. The largest renders left are the editors' own views (20, each on every commit, 16 % of the CPU in the last profile), whose reads of a 20-row table cost what a cold page cache costs: with 100 commits a second each read transaction starts without cached pages and looks them up in the WAL (`walFindFrame`, `btreeInitPage`: 13 % of the samples), and the issue view, 324 KB a frame with its history of about 100 entries. Splitting the issue's history and comments into views of their own would spread its render over the workers.
- NFR-42: the stop took 139 ms in the tuned run (46 ms before). An action still waiting for its tab's stream when the hub stopped would hold the shutdown for up to 2 s; it now answers 503 at once (`fix: an action waiting...`). Not measured again.
- NFR-41: anonymous memory reached 530 MiB over the 27 minutes of the tuned run, above the unit's `MemoryHigh=512M`, as before.
- NFR-43.3: lith v0.4.1 did not change `events.Compact`, and a fold command still holds the writer about 21 ms (16 to 20 ms with `tickit-load fold` on the run's database whatever the batch, 1 to 200 events). Its profile: `events.Compact` 82 %, of it `StateAt` loading the stream 39 %, the DELETE and the snapshot's UPDATE 34 %, the cut 10 %; applying the steps 2 %. All three walk the page's whole entry of the unique index `(stream, stream_id, version)`, about 92,000 rows, because they bound it by position: `DELETE ... WHERE stream = ? AND stream_id = ? AND position < ?` deleting nothing takes 6.1 ms in the sqlite3 shell, and the same bounded by `version <` 0.017 ms; `LoadUntil`'s `version > ? AND position <= ? ORDER BY version` walks the same way. tickit cannot avoid it without lith: a larger batch keeps the time per command, a smaller stream needs versions more often than every 10 minutes (each a full document, against NFR-43.2), and deleting from `lith_events` itself would bypass Compact's checks of projectors and upcasters. lith would need Compact to read the cut event's version once (a lookup by position) and bound the DELETE and `StateAt` by version, and to find its cut from the kind's compacted position on rather than from the kind's start.

Commands, besides the one above:

    cp -r --reflink=auto /tmp/tickit-load/base.db* /tmp/tickit-load-041/    # each build in a directory of its own
    taskset -c 12-19 /tmp/tickit-load-041/tickit-load run -bin ./tickit -base /tmp/tickit-load-041/base.db -seconds 120 -sustain 0 -server-cpus 0-5 -out results.json
    CGO_ENABLED=0 go build -tags pprof -o tickit-pprof ./cmd/tickit    # then run -bin ./tickit-pprof -seconds 60 -cpuprofile cpu.pprof
    cp --reflink=always run.db fold.db && tickit-load fold -db fold.db -n 20 -cpuprofile fold.pprof

## The race detector

20 October 2026, same machine: `go test -race -count=1 -timeout 40m ./...` found no data race. The first run, before the merge of `rewrite`, timed out in `internal/web` after 10 minutes, in `TestSearchSpeed` (10,000 issues), and `internal/search`'s `TestRebuild` missed its 20 s wait for the rebuild; the run after the merge passed every package in 4 min 3 s but `internal/people`'s `TestHashesAtOnce`, a heap measurement that read 171 MiB under `-race` with the suite running (it passes alone, with and without `-race`). Timing and heap measurements now skip under `-race` (each package's `raceDetector`, true in a test file built with the `race` tag), and the rebuild test waits 2 minutes there; `internal/web` takes 3 min 46 s and `internal/search` 1 min 17 s under the detector.

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

The fold (NFR-43.3): `go test ./internal/wiki -run TestFoldTime -v`, a page of 116 KB with 500 commits since its creation, then a version: one `wiki.Fold` command folds 200 events (`FoldBatch`) in 3.6 ms through the writer, 8.1 ms the first time (cold statements), and the last 102 in 2.1 ms. With 500 events a command it took 9.2 ms, so `FoldBatch` is 200. A commit on that page takes 0.07 to 0.18 ms in the writer. That page's stream held 500 events; at NFR-38's load it holds 15 minutes of commits, about 92,000, and a fold command takes 24 ms whatever its batch ("Capacity" above).

## Measuring

`cmd/tickit-load` (`go tool task load`) drives the built binary as a separate process with 200 streams, as recorded in "Capacity" above. `lith/loadtest` runs the writer and the render loop in process, without HTTP: N streams watch the views while M producers send commands, and it reports tick percentiles, renders per tick, bytes per frame and heap per stream. Run it before and after a change and record both runs here; `go doc github.com/zweiundeins/lith/loadtest` has its API. In production the writer's `Stats()` and the hub's overrun log (n, p50, p95, p99 and max of the ticks over 20 seconds) show the same quantities. For the whole path, drive the binary from `go tool task build` with a browser or a load tool on other cores (`taskset`) or another machine. The lith skill's `references/performance.md` has the numbers measured on the other projects and the settings they led to.

## The board (NFR-37, NFR-38)

2026-10-20, Intel i5-13500 (20 threads), 62 GB, Linux 6.18.38 (Unraid), Go 1.27.2, modernc.org/sqlite v1.60.1, the server in process (`webtest`: hub tick 10 ms, 2 render workers), streams read without compression.

- **Frame of 100 cards** (`go test ./internal/web -run TestBoardFrameSize -v`): 58,878 bytes of HTML in one patch, 588 bytes a card (`trackertest.Board`: every card with type, key, summary, priority and face or "Unassigned"; a quarter with an epic, a third with labels, a fifth with points, a sixth with a due date). Read and rendered in 1.8 ms on average over 50 runs. Before the cards referred to the page's glyphs (`ui.BoardCard`, `ui.Glyphs`), the same board was 316 KB with `ui.Card`, then 108 KB with SVG `<use>` references.
- **100 streams on one board, one move a second** (`TICKIT_LOAD_SECONDS=30 go test ./internal/web -run TestBoardLoad -v`): 30 moves, 30 renders of the board's one view (`RendersByKey`), 3,000 frames of 58,877 bytes, 30 to each stream. From a move's request to its frame on the slowest of the 100 streams: p50 3.6 ms, p95 7.3 ms, max 7.6 ms. The test process (server, 100 stream readers, a page load and a move a second) used 2.4 s of CPU in 29.2 s, 8.3% of one core. The 10-second run: p50 3.6 ms, p95 5.9 ms, 19% of a core while the 100 streams opened.
- **In browsers** (`CHROME=/usr/bin/chromium LITH_CHROME_NO_SANDBOX=1 go test ./e2e -run 'TestBoardMoveLive|TestBoardPresence' -v`, Chromium 154 headless, three browser contexts on loopback): a card dropped in one shows in the two others 22 to 74 ms after the drop over five runs (section 12 item 4 asks for 250 ms); a closed tab's face leaves the others' presence row after 50 to 55 ms (MP-02.3 asks for 1 s).

## The seed and the reset

2026-10-20, Intel Core i5-13500 (20 threads), Linux 6.18 (Unraid), Go 1.27.2, a test database in the test's temporary directory, `synchronous` NORMAL, without the wiki's pages (no PageSeeder yet).

| What | Took | Command |
| --- | --- | --- |
| Seed: 3 projects, 139 issues, 199 changes, 16 links, 99 comments, 83 board moves (593 steps, 1,475 events) | 91 to 110 ms | `go test ./internal/demo -run TestSeed -v` (the `seeded` log line) |
| Wipe after 1,000 guest issues and 1,000 comments (4,778 events): the one command that holds the writer | 9 ms | a throwaway test that ran `demo.Wipe` in `writer.Func` after `tracker.CreateIssue` and `AddComment` by 1,000 guests |
| Reset end to end: flush, wipe, seed, log | 138 ms | the same test, `Demo.Reset` |

DM-02.4 wants the seed under 30 s; the reset holds the writer at most for the wipe and then for one batch of 64 seed steps at a time.
