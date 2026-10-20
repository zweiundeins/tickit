package wiki

// ResetCaches empties the caches of documents and commits, as a new
// process starts: for tests that restart the server within one process.
// A commit the fold deleted then comes only from the log, as after a
// deploy (MP-05.6's gap).
func ResetCaches() {
	docs.reset()
	commits.reset()
}
