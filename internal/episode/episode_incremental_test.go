package episode_test

import (
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
