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

// A delta consisting only of tool-use records (no text block) still has
// real, worth-keeping signal — which files were touched, which tools ran —
// and must not be classified as empty just because it produced no prose.
func TestCaptureIncremental_ToolOnlyDeltaWithNoTextIsStillCapturable(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()
	txn := newGrowingTranscript(t, fixturePath)

	first, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	txn.grow(t, `{"type":"assistant","message":{"model":"claude-opus-4-7","role":"assistant","content":[{"type":"tool_use","id":"t9","name":"Read","input":{"file_path":"/tmp/foo.go"}}]},"timestamp":"2026-04-24T10:01:00.000Z","sessionId":"test-session-abc12345"}`)

	second, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, second, "a tool-only delta (file touched, no prose) must still be captured, not silently dropped")

	body, err := os.ReadFile(second) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(body), "/tmp/foo.go", "the file-touch signal must survive into the written segment")
}

// A corrupt cursor file must fail the whole capture loudly, not silently
// fall back to zero and re-render the transcript — that fallback would
// recreate the exact ever-growing-blob failure this mechanism exists to fix.
func TestCaptureIncremental_PropagatesACorruptCursorAsAnError(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()

	// The cursor key is a vault-scoped internal detail (sessionID combined
	// with a hash of outputDir) — discover the real file CaptureIncremental
	// wrote rather than assuming its name, so this test doesn't silently
	// stop exercising the real ReadCursor path the moment that scheme changes.
	_, err := episode.CaptureIncremental(fixturePath, outDir, cursorDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(cursorDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one cursor file should exist after one capture")
	require.NoError(t, os.WriteFile(filepath.Join(cursorDir, entries[0].Name()), []byte("not-a-number"), 0o600))

	_, err = episode.CaptureIncremental(fixturePath, outDir, cursorDir)
	require.Error(t, err)
}

// A delta that grew the transcript with only filtered noise (no real content)
// must still advance the cursor — so the noise is never re-scanned — but must
// not write an episode file for it.
func TestCaptureIncremental_NoiseOnlyDeltaAdvancesCursorWithoutWritingAFile(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()
	txn := newGrowingTranscript(t, fixturePath)

	first, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	txn.grow(t, `{"type":"user","message":{"role":"user","content":"<system-reminder>ignore me</system-reminder>"},"timestamp":"2026-04-24T10:02:00.000Z","sessionId":"test-session-abc12345"}`)

	second, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	assert.Empty(t, second, "filtered noise alone is not capturable content")

	third, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	assert.Empty(t, third, "the cursor must have advanced past the noise, or this would re-detect it forever")

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the original real content produced a file")
}

// The window this whole cursor design exists to make safe: the segment file
// write succeeds, then the cursor write fails (disk fills, permissions
// change mid-run, whatever). The segment is now on disk with content the
// cursor doesn't know about. This must self-heal: a later successful call
// re-reads the stale (unadvanced) cursor, re-derives the SAME segment id —
// since startLine didn't move — and overwrites that same file rather than
// losing the content or leaving a stray duplicate behind.
func TestCaptureIncremental_SelfHealsWhenCursorWriteFailsAfterSegmentIsWritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions; cannot exercise the cursor-write-failure path")
	}
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()
	txn := newGrowingTranscript(t, fixturePath)

	first, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	txn.grow(t, `{"type":"user","message":{"role":"user","content":"partial-failure content"},"timestamp":"2026-04-24T10:03:00.000Z","sessionId":"test-session-abc12345"}`)

	require.NoError(t, os.Chmod(cursorDir, 0o500)) // readable+listable, not writable
	t.Cleanup(func() { _ = os.Chmod(cursorDir, 0o700) })

	_, err = episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.Error(t, err, "the cursor write must fail and surface, not be swallowed")

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	require.Len(t, entries, 2, "the segment write already committed before the cursor write failed")
	second := filepath.Join(outDir, entries[1].Name())
	body, err := os.ReadFile(second) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(body), "partial-failure content", "the segment genuinely has the new content, despite the cursor not knowing it yet")

	require.NoError(t, os.Chmod(cursorDir, 0o700))
	third, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err, "the stale cursor lets this retry the same delta cleanly")
	assert.Equal(t, second, third, "re-derives the SAME segment id — the cursor never advanced — and overwrites it")

	entriesAfter, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entriesAfter, 2, "no stray duplicate left behind by the retry")
}

// A transcript that now has FEWER lines than the persisted cursor already
// consumed (truncated, rotated, or replaced underneath the hook) must not
// get stuck reporting "nothing new" forever, silently losing every future
// real message. CaptureIncremental is the only layer that can safely detect
// this — its startLine came from a real cursor it wrote for THIS session,
// unlike the low-level ParseTranscriptFrom, which can't tell "shrank" apart
// from "never grew that far" (see TestParseTranscriptFrom_
// StartLineNeverReachedStaysAtStartLineNotBelow for that boundary).
func TestCaptureIncremental_ShrunkenTranscriptResetsAndRescans(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")
	cursorDir := t.TempDir()
	txn := newGrowingTranscript(t, fixturePath)

	first, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	// Replace the transcript with a much shorter one — same session,
	// simulating rotation/truncation rather than ordinary append growth.
	require.NoError(t, os.WriteFile(txn.path, []byte(
		`{"type":"user","message":{"role":"user","content":"replaced after truncation"},"timestamp":"2026-05-01T00:00:00.000Z","sessionId":"test-session-abc12345"}`+"\n",
	), 0o600))

	second, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, second, "must re-capture the shrunken file, not silently report nothing-new forever")

	body, err := os.ReadFile(second) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(body), "replaced after truncation")

	// And the cursor genuinely reset, rather than getting stuck: a further
	// no-op call now correctly reports nothing new against the SHORT file.
	third, err := episode.CaptureIncremental(txn.path, outDir, cursorDir)
	require.NoError(t, err)
	assert.Empty(t, third, "cursor is caught up with the (short) file again, not stuck mid-reset")
}

func TestCaptureIncremental_ErrorsWhenOutputDirCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	_, err := episode.CaptureIncremental(fixturePath, filepath.Join(blocker, "episodes"), t.TempDir())
	require.Error(t, err)
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

// Routing the SAME session's capture at two different --output-dirs (e.g. two
// separate identity vaults sharing one --cursor-dir) must not let the first
// vault's cursor silently truncate the second vault's capture — each vault
// needs its own full view of the session, not whatever line the OTHER vault
// happened to already consume.
func TestCaptureIncremental_DifferentOutputDirsDoNotShareACursor(t *testing.T) {
	cursorDir := t.TempDir()
	vaultA := filepath.Join(t.TempDir(), "vault-a", "episodes")
	vaultB := filepath.Join(t.TempDir(), "vault-b", "episodes")

	a, err := episode.CaptureIncremental(fixturePath, vaultA, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, a, "first vault must capture the full session")

	b, err := episode.CaptureIncremental(fixturePath, vaultB, cursorDir)
	require.NoError(t, err)
	require.NotEmpty(t, b, "second vault must ALSO capture the full session, not see it as already-consumed by vault A's cursor")

	aEntries, err := os.ReadDir(vaultA)
	require.NoError(t, err)
	assert.Len(t, aEntries, 1)
	bEntries, err := os.ReadDir(vaultB)
	require.NoError(t, err)
	assert.Len(t, bEntries, 1)

	cursorEntries, err := os.ReadDir(cursorDir)
	require.NoError(t, err)
	assert.Len(t, cursorEntries, 2, "each vault gets its own cursor file, not a shared one")
}

func TestCaptureIncremental_ErrorsOnBadTranscript(t *testing.T) {
	_, err := episode.CaptureIncremental("/no/such/transcript.jsonl", t.TempDir(), t.TempDir())
	require.Error(t, err)
}
