//go:build !race

package index_test

// raceEnabled is true when the test binary was built with `-race`.
// Used to skip wall-clock performance assertions under the race detector,
// whose instrumentation adds heavy, variable overhead that makes timing
// budgets meaningless (and flaky) — the correctness assertions still run.
const raceEnabled = false
