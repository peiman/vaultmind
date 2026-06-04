package episode_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapture_WritesMarkdownToOutputDir(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")

	outPath, err := episode.Capture(fixturePath, outDir)
	require.NoError(t, err)

	// File exists and lives under outDir with the expected id.
	assert.True(t, strings.HasPrefix(outPath, outDir))
	assert.True(t, strings.HasSuffix(outPath, "episode-2026-04-24-test-ses.md"))

	content, err := os.ReadFile(outPath) // #nosec G304 -- test-controlled path.
	require.NoError(t, err)
	body := string(content)
	assert.Contains(t, body, "type: episode")
	assert.Contains(t, body, "run the tests please")
}

func TestCapture_IsIdempotent(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")

	first, err := episode.Capture(fixturePath, outDir)
	require.NoError(t, err)
	second, err := episode.Capture(fixturePath, outDir)
	require.NoError(t, err)

	// Same transcript → same path → overwrite, not duplicate.
	assert.Equal(t, first, second)
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestCapture_ErrorsOnBadTranscript(t *testing.T) {
	_, err := episode.Capture("/no/such/transcript.jsonl", t.TempDir())
	require.Error(t, err)
}

// CaptureDir batch-captures every *.jsonl transcript under a directory
// (recursively), skipping malformed ones instead of aborting — bootstrapping an
// identity vault from a large existing session history must survive noise files.
func TestCaptureDir_BatchCapturesRecursivelyAndSkipsMalformed(t *testing.T) {
	src, err := os.ReadFile(fixturePath) // #nosec G304 -- test fixture
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "good.jsonl"), src, 0o600))
	sub := filepath.Join(dir, "project-b")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "nested.jsonl"), src, 0o600)) // recursion
	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.jsonl"), []byte("not json\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# ignored"), 0o600))

	outDir := filepath.Join(t.TempDir(), "episodes")
	batch, err := episode.CaptureDir(dir, outDir)
	require.NoError(t, err, "a malformed transcript must not fail the whole batch")

	assert.GreaterOrEqual(t, len(batch.Captured), 1, "valid transcripts (incl. the nested one) are captured")
	assert.Contains(t, batch.Skipped, filepath.Join(dir, "junk.jsonl"), "malformed transcript is recorded, not fatal")
	assert.NotContains(t, batch.Skipped, filepath.Join(dir, "notes.md"), "non-.jsonl files are ignored, not skipped")

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "good.jsonl and the nested copy share a session id → one episode file")
}
