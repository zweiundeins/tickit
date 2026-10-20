# internal/doc benchmarks

Measured 2026-10-20 on an Intel Core i5-13500 (14 cores, 20 threads, Linux 6.18), Go 1.27.2, with other jobs running (load average 15 to 85 during the run), so the spread between the three runs is wide:

    go test ./internal/doc -run '^$' -bench . -benchmem -count 3

The fixtures are those of the differential tests: a 20.6 KB document (35 top-level blocks) and a 204 KB one (364 blocks), 200 batches of 5 steps each, and 300 commits made 5 or 20 versions behind the head. Times are per operation, the range over the three runs.

| Benchmark | 20 KB | 200 KB | Memory (20 KB / 200 KB) |
| --- | --- | --- | --- |
| Apply a batch of 5 steps | 12 to 32 µs | 20 to 21 µs | 9.9 KB, 107 allocs / 24.7 KB, 111 allocs |
| Rebase a commit, k = 5 (map over 25 steps, apply 5) | 15 to 29 µs | 22 to 38 µs | 13.4 KB / 27.6 KB |
| Rebase a commit, k = 20 | 31 to 56 µs | | 18.8 KB, 323 allocs |
| Parse and check a document (`ParseDoc`) | 142 to 150 µs | 1.71 to 2.08 ms | 99 KB, 3,788 allocs / 981 KB, 37,296 allocs |
| Write a document (`MarshalDoc`) | 46 to 49 µs | 0.43 to 0.49 ms | 85 KB / 908 KB |
| Render HTML (`RenderHTML`) | 38 to 52 µs | 0.66 to 0.89 ms | 35 KB / 331 KB |
| Text for search (`Text`) | 13 to 23 µs | 158 to 243 µs | 25 KB / 287 KB |
| Server rules of a commit (`Rules.Check`, changed blocks only) | | 18 to 22 µs | 2 KB, 57 allocs |
| Compare two versions and render (`Diff(...).HTML`) | 2.8 to 3.2 ms | | 4.1 MB |

For comparison, the notes of the spike measured prosemirror-transform in goja at 0.56 to 1.03 ms for a warm batch of 5 steps on 20 KB and 34 to 44 ms to load a 200 KB document; this package applies the batch about 40 times faster and loads the document in about a twentieth of the time.
