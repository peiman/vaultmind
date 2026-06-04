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
