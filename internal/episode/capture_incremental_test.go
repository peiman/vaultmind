package episode_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// growingTranscript simulates a long-lived session's transcript file: each
// call to grow() appends a new JSONL line in place, as if the session kept
// producing new records between SessionEnd calls.
type growingTranscript struct {
	path string
}

func newGrowingTranscript(t *testing.T, seedPath string) *growingTranscript {
	t.Helper()
	seed, err := os.ReadFile(seedPath) // #nosec G304 -- test fixture path
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "growing.jsonl")
	require.NoError(t, os.WriteFile(path, seed, 0o600))
	return &growingTranscript{path: path}
}

func (g *growingTranscript) grow(t *testing.T, line string) {
	t.Helper()
	f, err := os.OpenFile(g.path, os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(line + "\n")
	require.NoError(t, err)
}

func TestCaptureIncremental_FirstCallCapturesFromStart(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()

	path, err := episode.CaptureIncremental(fixturePath, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, path)

	content, err := os.ReadFile(path) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(content), "run the tests please")
}

func TestCaptureIncremental_SecondCallOnUnchangedTranscriptIsANoOp(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()

	first, err := episode.CaptureIncremental(fixturePath, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	second, err := episode.CaptureIncremental(fixturePath, outDir, cursorDir)
	require.NoError(t, err)
	assert.Empty(t, second, "nothing new since the cursor advanced — must not write an empty segment")

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the no-op call wrote nothing new")
}

func TestCaptureIncremental_GrowingTranscriptProducesASecondBoundedSegment(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()
	txn := newGrowingTranscript(t, fixturePath)

	first, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	second, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err, "the transcript hasn't grown again yet after the first capture")
	assert.Empty(t, second)

	txn.grow(t, `{"type":"user","message":{"role":"user","content":"one more thing"},"timestamp":"2026-04-24T10:01:00.000Z","sessionId":"test-session-abc12345"}`)

	third, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, third)

	assert.NotEqual(t, first, third, "the second segment must not collide with or overwrite the first")

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "two bounded segments, not one ever-growing file")

	firstBody, err := os.ReadFile(first) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	thirdBody, err := os.ReadFile(third) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(firstBody), "run the tests please", "first segment keeps the original content")
	assert.NotContains(t, string(firstBody), "one more thing", "the delta must not leak into the earlier segment")
	assert.Contains(t, string(thirdBody), "one more thing", "second segment carries only the new tail")
	assert.NotContains(t, string(thirdBody), "run the tests please", "and none of the already-captured content")
}

func TestCaptureIncremental_DifferentSessionsDoNotShareACursor(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()

	other := filepath.Join(t.TempDir(), "other.jsonl")
	require.NoError(t, os.WriteFile(other, []byte(
		`{"type":"user","message":{"role":"user","content":"different session"},"timestamp":"2026-05-01T00:00:00.000Z","sessionId":"other-session-99999999"}`+"\n",
	), 0o600))

	a, err := episode.CaptureIncremental(fixturePath, outDir, cursorDir)
	require.NoError(t, err)
	b, err := episode.CaptureIncremental(other, outDir, cursorDir)
	require.NoError(t, err)

	assert.NotEmpty(t, a)
	assert.NotEmpty(t, b)
	assert.NotEqual(t, a, b)
}

func TestCaptureIncremental_ErrorsOnBadTranscript(t *testing.T) {
	_, err := episode.CaptureIncremental("/no/such/transcript.jsonl", t.TempDir(), t.TempDir())
	require.Error(t, err)
}
