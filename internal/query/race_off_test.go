//go:build !race

package query_test

// raceEnabled is true when the test binary was built with `-race`.
// Used by tests that exercise third-party code with known races we
// can't fix (e.g. go-huggingface's DownloadFilesCtx as of v0.3.5).
// Such tests skip under race rather than fail CI on noise.
const raceEnabled = false
