//go:build race

package search_test

// raceDetector is set under -race, which runs 5 to 20 times slower: the
// timing tests skip, and waits take longer.
const raceDetector = true
