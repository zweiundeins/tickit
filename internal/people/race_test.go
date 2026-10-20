//go:build race

package people_test

// raceDetector is set under -race, whose bookkeeping makes heap
// measurements mean little.
const raceDetector = true
