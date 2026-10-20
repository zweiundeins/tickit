//go:build race

package web_test

// raceDetector is true when the tests run with -race, which slows SQLite
// and every request several times over.
const raceDetector = true
