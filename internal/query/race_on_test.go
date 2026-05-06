//go:build race

package query_test

// raceEnabled is true when the test binary was built with `-race`.
// See race_off_test.go for the docstring.
const raceEnabled = true
