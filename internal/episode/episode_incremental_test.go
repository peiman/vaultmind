package episode_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTranscriptFrom_ZeroStartMatchesFullParse(t *testing.T) {
	full, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	delta, endLine, err := episode.ParseTranscriptFrom(fixturePath, 0)
	require.NoError(t, err)

	assert.Equal(t, full.UserMessages, delta.UserMessages)
	assert.Equal(t, full.AssistantMessages, delta.AssistantMessages)
	assert.Equal(t, full.SessionID, delta.SessionID)
	assert.Positive(t, endLine, "must report how many lines it consumed")
}

func TestParseTranscriptFrom_SkipsAlreadyCapturedLines(t *testing.T) {
	full, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)
	require.Len(t, full.UserMessages, 2, "fixture sanity check")

	// The fixture's first user message ("hi there") is on line 2; the second
	// ("run the tests please") is on line 4. Starting from line 2 must skip
	// the first but still include the second and everything after it.
	delta, _, err := episode.ParseTranscriptFrom(fixturePath, 2)
	require.NoError(t, err)

	require.Len(t, delta.UserMessages, 1, "only the second user message is after line 2")
	assert.Equal(t, "run the tests please", delta.UserMessages[0].Text)
}

func TestParseTranscriptFrom_ReturnsEmptyEpisodeWhenNothingNew(t *testing.T) {
	full, endLine, err := episode.ParseTranscriptFrom(fixturePath, 0)
	require.NoError(t, err)

	delta, newEndLine, err := episode.ParseTranscriptFrom(fixturePath, endLine)
	require.NoError(t, err)

	assert.Empty(t, delta.UserMessages)
	assert.Empty(t, delta.AssistantMessages)
	assert.Empty(t, delta.PRs)
	assert.Equal(t, endLine, newEndLine, "no new lines consumed")
	assert.NotEmpty(t, full.UserMessages, "sanity: the first parse actually saw content")
}

func TestParseTranscriptFrom_StartLineBeyondEOFReturnsEmptyNotError(t *testing.T) {
	delta, endLine, err := episode.ParseTranscriptFrom(fixturePath, 10_000)
	require.NoError(t, err)
	assert.Empty(t, delta.UserMessages)
	assert.Equal(t, 10_000, endLine, "cursor doesn't regress past a file that hasn't grown")
}

func TestParseTranscriptFrom_PreservesSessionIDEvenWhenDeltaHasNoRecords(t *testing.T) {
	// A delta with zero new messages still needs a SessionID for the caller
	// (CaptureIncremental) to know which session's cursor to advance.
	_, endLine, err := episode.ParseTranscriptFrom(fixturePath, 0)
	require.NoError(t, err)

	delta, _, err := episode.ParseTranscriptFrom(fixturePath, endLine)
	require.NoError(t, err)
	assert.Equal(t, "test-session-abc12345", delta.SessionID)
}

func TestParseTranscriptFrom_ErrorsOnMissingFile(t *testing.T) {
	_, _, err := episode.ParseTranscriptFrom("/no/such/transcript.jsonl", 0)
	require.Error(t, err)
}

// The bug this guards against: a session's transcript is read mid-flush, so
// its last line is truncated JSON. Without this guard, the truncated line
// fails to unmarshal, gets silently skipped as "noise", but the cursor still
// advances past its line number — so once the write finishes and the line
// becomes valid, it's never re-read: the message is gone, permanently,
// with no error anywhere. The fix: a failed-to-parse TAIL line must not
// advance the cursor past itself.
func TestParseTranscriptFrom_DoesNotAdvanceCursorPastATruncatedTailLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mid-flush.jsonl")
	// A complete, valid first line, then a truncated second line — as if
	// caught exactly while Claude Code was still writing it.
	require.NoError(t, os.WriteFile(path, []byte(
		`{"type":"user","message":{"role":"user","content":"first"},"timestamp":"2026-04-24T10:00:00.000Z","sessionId":"s1"}`+"\n"+
			`{"type":"user","message":{"role":"user","content":"seco`, // deliberately unterminated, no trailing newline
	), 0o600))

	delta, endLine, err := episode.ParseTranscriptFrom(path, 0)
	require.NoError(t, err)
	require.Len(t, delta.UserMessages, 1, "only the complete first line parses")
	assert.Equal(t, "first", delta.UserMessages[0].Text)
	assert.Equal(t, 1, endLine, "the cursor must stop BEFORE the truncated tail line, not past it")

	// Simulate the write finishing: the same line, now complete.
	require.NoError(t, os.WriteFile(path, []byte(
		`{"type":"user","message":{"role":"user","content":"first"},"timestamp":"2026-04-24T10:00:00.000Z","sessionId":"s1"}`+"\n"+
			`{"type":"user","message":{"role":"user","content":"second"},"timestamp":"2026-04-24T10:00:05.000Z","sessionId":"s1"}`+"\n",
	), 0o600))

	// Resuming from the PREVIOUS call's endLine must pick the now-complete
	// line up whole — this is the property that was broken before the fix.
	delta2, _, err := episode.ParseTranscriptFrom(path, endLine)
	require.NoError(t, err)
	require.Len(t, delta2.UserMessages, 1, "the completed line is captured, not permanently lost")
	assert.Equal(t, "second", delta2.UserMessages[0].Text)
}

// A malformed line that is NOT the last line in the file (i.e. real content
// exists after it) is not a write-in-progress candidate — it's tolerated as
// noise exactly as before, and the cursor still advances past it and
// everything after it.
func TestParseTranscriptFrom_MidStreamMalformedLineIsStillTreatedAsNoise(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "noisy.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(
		`{"type":"user","message":{"role":"user","content":"first"},"timestamp":"2026-04-24T10:00:00.000Z","sessionId":"s1"}`+"\n"+
			`not json at all`+"\n"+
			`{"type":"user","message":{"role":"user","content":"third"},"timestamp":"2026-04-24T10:00:10.000Z","sessionId":"s1"}`+"\n",
	), 0o600))

	delta, endLine, err := episode.ParseTranscriptFrom(path, 0)
	require.NoError(t, err)
	require.Len(t, delta.UserMessages, 2, "the malformed middle line is skipped as noise, both real lines captured")
	assert.Equal(t, "first", delta.UserMessages[0].Text)
	assert.Equal(t, "third", delta.UserMessages[1].Text)
	assert.Equal(t, 3, endLine, "cursor advances past the noise once something valid follows it")
}

// ParseTranscriptFrom itself cannot distinguish "the file genuinely shrank
// since a real prior cursor was recorded" from "the caller simply asked for
// a startLine this file never reached" — both look identical from inside
// this function (fewer lines exist than startLine asks for). So it stays
// conservative here: never regress below what was asked for. Shrinkage
// detection lives one layer up, in CaptureIncremental, which alone knows
// whether startLine came from a real persisted cursor (see
// TestCaptureIncremental_ShrunkenTranscriptResetsAndRescans).
func TestParseTranscriptFrom_StartLineNeverReachedStaysAtStartLineNotBelow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "short.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(
		`{"type":"user","message":{"role":"user","content":"only line"},"timestamp":"2026-04-24T10:00:00.000Z","sessionId":"s1"}`+"\n",
	), 0o600))

	delta, endLine, err := episode.ParseTranscriptFrom(path, 500)
	require.NoError(t, err)
	assert.Empty(t, delta.UserMessages)
	assert.Equal(t, 500, endLine, "stays at the requested startLine, never regresses")
}
