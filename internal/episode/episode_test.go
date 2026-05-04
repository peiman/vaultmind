package episode_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestExtractCommitSubject_HandlesKnownQuoteShapes(t *testing.T) {
	// Pin the canonical shapes the heuristic must handle. If a future refactor
	// breaks one, this table names exactly which form regressed.
	cases := map[string]string{
		`git commit -m "feat: add frobnicator"`:                                    "feat: add frobnicator",
		`git commit -m 'fix: off-by-one'`:                                          "fix: off-by-one",
		"git commit -m \"$(cat <<'EOF'\nfeat: multiline subject\n\nBody\nEOF\n)\"": "feat: multiline subject",
	}
	for shellCmd, want := range cases {
		ep := parseOneBashCall(t, shellCmd)
		require.Len(t, ep.Commits, 1, "for cmd %q", shellCmd)
		assert.Equal(t, want, ep.Commits[0], "for cmd %q", shellCmd)
	}
}

// parseOneBashCall builds a minimal one-record JSONL containing a single Bash
// tool_use and runs it through ParseTranscript.
func parseOneBashCall(t *testing.T, shellCmd string) *episode.Episode {
	t.Helper()
	inputJSON, err := json.Marshal(map[string]string{"command": shellCmd, "description": "x"})
	require.NoError(t, err)
	rec := map[string]any{
		"type":      "assistant",
		"sessionId": "test-cases",
		"timestamp": "2026-04-24T00:00:00.000Z",
		"message": map[string]any{
			"role":    "assistant",
			"content": []any{map[string]any{"type": "tool_use", "id": "t", "name": "Bash", "input": json.RawMessage(inputJSON)}},
		},
	}
	recJSON, err := json.Marshal(rec)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "one.jsonl")
	require.NoError(t, os.WriteFile(path, recJSON, 0o600))
	ep, err := episode.ParseTranscript(path)
	require.NoError(t, err)
	return ep
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

// TestRenderMarkdown_HonorsSchemaOwnershipContract — every emitted
// episode MUST carry vaultmind-owned fields (created, vm_updated)
// per the four-tier taxonomy in schema/registry.go. The episode-
// capture path was bypassing this contract before slice 8: it wrote
// id/type/session_id/started_at/ended_at/cwd/git_branch/tags but
// not created or vm_updated. The 2026-05-04 dogfood pass surfaced
// 8/8 captured episodes flagged by `vaultmind frontmatter fix`,
// which is the contract violation this test pins shut.
//
// `created` for an episode is the started_at date (when the
// session occurred — the episode's semantic "birthday"), NOT today.
// vm_updated is the canonical SSOT format applied at render time.
func TestRenderMarkdown_HonorsSchemaOwnershipContract(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	md := episode.RenderMarkdown(ep)

	// `created` is the started_at date portion (YYYY-MM-DD). Episodes
	// describe a session; the session is when they happened. This
	// matches the semantic of `created` for other note types
	// (when this thing was born), and avoids the
	// git-commit-vs-filename surprise the 2026-05-04 dogfood
	// surfaced for legacy episodes.
	assert.Contains(t, md, "created: 2026-04-24",
		"created MUST be the started_at date (YYYY-MM-DD), not today")

	// vm_updated MUST be present and in canonical RFC3339 second-
	// precision UTC form (schema.VMUpdatedFormat). Doctor's drift
	// detector parses with the SSOT constant; an unquoted or wrongly-
	// formatted value here would silently fail every drift check.
	assert.Regexp(t, `vm_updated: "?20\d{2}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z"?`, md,
		"vm_updated MUST be present in canonical RFC3339 second-precision UTC")
}

// TestRenderMarkdown_CreatedFallsBackToTodayOnDegenerateStartedAt —
// when started_at is empty or unparseable, `created` defaults to
// today's UTC date so the schema-ownership contract holds even on
// torn or malformed transcripts. Without this fallback, a degenerate
// transcript could produce a note with empty `created`, which the
// fix command would then re-flag as missing.
func TestRenderMarkdown_CreatedFallsBackToTodayOnDegenerateStartedAt(t *testing.T) {
	ep := &episode.Episode{
		ID:        "episode-degenerate",
		SessionID: "test",
		StartedAt: "", // degenerate
		EndedAt:   "",
	}
	md := episode.RenderMarkdown(ep)

	// Some date matching today's format must be present; we don't
	// pin the exact value because t.Now varies by run.
	assert.Regexp(t, `created: 20\d{2}-\d{2}-\d{2}`, md,
		"empty started_at must fall back to today (never empty created)")
}

func TestEpisodeID_DerivedFromSessionDateAndIDPrefix(t *testing.T) {
	ep, err := episode.ParseTranscript(fixturePath)
	require.NoError(t, err)

	// Deterministic: date from StartedAt + 8-char session prefix.
	assert.Equal(t, "episode-2026-04-24-test-ses", ep.ID)
}
