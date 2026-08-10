package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const episodeFixture = "../internal/episode/testdata/mini-session.jsonl"

// `episode capture <file>` writes one episode and prints its path. Covers the
// single-transcript RunE path.
func TestEpisodeCapture_SingleFile(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	out, _, err := runRootCmd(t, "episode", "capture", episodeFixture, "--output-dir", outDir)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "episode-", "prints the written episode path")
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// `episode capture <file> --incremental` writes a bounded segment and prints
// its path, same as a normal capture, on the first call for a session.
func TestEpisodeCapture_Incremental_FirstCallWritesASegment(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()

	out, _, err := runRootCmd(t, "episode", "capture", episodeFixture,
		"--output-dir", outDir, "--incremental", "--cursor-dir", cursorDir)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "episode-", "prints the written segment's path")

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// Omitting --cursor-dir must resolve a real, working default (the XDG state
// dir) rather than silently doing nothing — the flag is documented as
// optional, so the default path is a real code path, not a formality.
//
// Isolation: xdg.StateDir() ignores XDG_STATE_HOME entirely on darwin
// (internal/xdg's stateBase() hardcodes ~/Library/Application Support there —
// consistent with every other StateDir/DataDir consumer in this codebase).
// $HOME is what actually redirects it on every platform this package
// supports, so that's what must be overridden — not XDG_STATE_HOME, which
// would silently no-op here and let the test write into the real user's
// Application Support directory. Confirmed by hand: an earlier version of
// this test using XDG_STATE_HOME did exactly that.
func TestEpisodeCapture_Incremental_DefaultsCursorDirToXDGStateWhenOmitted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	outDir := filepath.Join(t.TempDir(), "episodes")

	out, _, err := runRootCmd(t, "episode", "capture", episodeFixture, "--output-dir", outDir, "--incremental")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "episode-", "captures via the default cursor dir, no --cursor-dir needed")
}

// A second `--incremental` call on the same, unchanged transcript must not
// print a blank line or write a duplicate/empty file — it must say plainly
// that there was nothing new, so a human running it manually (or reading
// hook logs) never has to guess whether "no output" meant success or a
// silent failure.
func TestEpisodeCapture_Incremental_SecondCallOnUnchangedTranscriptSaysSo(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()

	_, _, err := runRootCmd(t, "episode", "capture", episodeFixture,
		"--output-dir", outDir, "--incremental", "--cursor-dir", cursorDir)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "episode", "capture", episodeFixture,
		"--output-dir", outDir, "--incremental", "--cursor-dir", cursorDir)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "nothing new to capture")

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the no-op second call wrote nothing new")
}

// `episode capture <dir>` batch-captures every transcript under the directory,
// skipping non-transcripts and pointing at the next step. Covers the RunE
// directory-detection branch + runEpisodeCaptureDir.
func TestEpisodeCapture_Directory_BatchAndSkips(t *testing.T) {
	src, err := os.ReadFile(episodeFixture) // #nosec G304 -- test fixture
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.jsonl"), src, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.jsonl"), []byte("garbage\n"), 0o600))

	outDir := filepath.Join(t.TempDir(), "episodes")
	out, _, err := runRootCmd(t, "episode", "capture", dir, "--output-dir", outDir)
	require.NoError(t, err)
	body := out.String()
	assert.Contains(t, body, "Captured 1 episode(s)")
	assert.Contains(t, body, "Skipped 1 file(s)")
	assert.Contains(t, body, "arc candidates", "points at the next step")
}
