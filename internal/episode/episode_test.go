package episode_test

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixturePath = "testdata/mini-session.jsonl"

func TestParseTranscript_ExtractsSessionMetadata(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	assert.Equal(t, "test-session-abc12345", ep.SessionID)
	assert.Equal(t, "2026-04-24T10:00:00.000Z", ep.StartedAt)
	assert.Equal(t, "2026-04-24T10:00:45.000Z", ep.EndedAt)
	assert.Equal(t, "/home/test", ep.CWD)
	assert.Equal(t, "main", ep.GitBranch)
}

func TestParseTranscript_FiltersRealUserMessagesFromNoise(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	// Two real Peiman-style messages; system-reminder and tool_result lists must be filtered.
	require.Len(t, ep.UserMessages, 2)
	assert.Equal(t, "hi there", ep.UserMessages[0].Text)
	assert.Equal(t, "run the tests please", ep.UserMessages[1].Text)
}

func TestParseTranscript_CapturesAssistantTextBlocksOnly(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	// Thinking blocks and tool_use blocks must be excluded; only text blocks kept.
	require.Len(t, ep.AssistantMessages, 2)
	assert.Equal(t, "Hello. Ready to work.", ep.AssistantMessages[0].Text)
	assert.Equal(t, "Done. PR opened.", ep.AssistantMessages[1].Text)
}

func TestParseTranscript_CountsToolUsesByName(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	assert.Equal(t, 2, ep.ToolCounts["Bash"])
	assert.Equal(t, 1, ep.ToolCounts["Edit"])
}

func TestParseTranscript_ExtractsCommitSubjectNotFullHeredoc(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	// Inline `-m` form: subject extracted cleanly.
	require.Len(t, ep.Commits, 1)
	assert.Equal(t, "fix: the bug", ep.Commits[0])
}

func TestParseTranscript_DedupesRepeatedPRLinks(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	// Claude Code emits the same pr-link multiple times; we collapse to one.
	require.Len(t, ep.PRs, 1)
	assert.Equal(t, 42, ep.PRs[0].Number)
	assert.Equal(t, "https://github.com/test/repo/pull/42", ep.PRs[0].URL)
}

func TestParseTranscript_ExtractsFilesTouched(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	assert.Contains(t, ep.FilesTouched, "/home/test/foo.go")
}

func TestRenderMarkdown_EmitsExpectedSections(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	md := episode.RenderMarkdown(ep)

	// Frontmatter
	assert.True(t, strings.HasPrefix(md, "---\n"))
	assert.Contains(t, md, "type: episode")
	assert.Contains(t, md, "session_id: test-session-abc12345")
	assert.Contains(t, md, "started_at:")

	// Section headers
	assert.Contains(t, md, "## Metadata")
	assert.Contains(t, md, "## Commits made")
	assert.Contains(t, md, "## PRs opened")
	assert.Contains(t, md, "## User messages (verbatim)")
	assert.Contains(t, md, "## Assistant responses (verbatim)")

	// Content preserved verbatim
	assert.Contains(t, md, "run the tests please")
	assert.Contains(t, md, "Done. PR opened.")
	assert.Contains(t, md, "#42")
}

func TestEpisodeID_DerivedFromSessionDateAndIDPrefix(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	// Deterministic: date from StartedAt + 8-char session prefix.
	assert.Equal(t, "episode-2026-04-24-test-ses", ep.ID)
}
