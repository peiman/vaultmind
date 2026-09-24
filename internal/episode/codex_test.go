package episode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Codex CLI keeps a session as a "rollout" JSONL (~/.codex/sessions/…/rollout-
// <ts>-<session-id>.jsonl). The fixture mirrors the record shapes of real
// 0.153–0.156 rollouts, with invented content. Measured on 200 real rollouts
// (2026-09-24): the conversation lives in response_item messages, and the
// "user" role also carries text Codex injects itself — environment context,
// AGENTS.md, IDE file lists, plugin lists. An episode must record what the
// person said, not what Codex told the model.

const codexFixture = "testdata/codex-session.jsonl"

func TestParseTranscript_CodexSessionMetadata(t *testing.T) {
	ep, err := ParseTranscript(codexFixture)
	require.NoError(t, err)
	assert.Equal(t, "01a0d100-aaaa-7bbb-8ccc-000000000001", ep.SessionID)
	assert.Equal(t, "/home/test/studio", ep.CWD)
	assert.Equal(t, "main", ep.GitBranch)
	assert.Equal(t, "2026-09-24T09:00:00.000Z", ep.StartedAt)
	assert.Equal(t, "2026-09-24T09:00:13.000Z", ep.EndedAt)
	assert.Equal(t, "episode-2026-09-24-01a0d100", ep.ID)
}

func TestParseTranscript_CodexKeepsOnlyWhatThePersonSaid(t *testing.T) {
	ep, err := ParseTranscript(codexFixture)
	require.NoError(t, err)
	var said []string
	for _, m := range ep.UserMessages {
		said = append(said, m.Text)
	}
	assert.Equal(t, []string{
		"Add a retry to the asset fetcher",
		"Answer to \"Should the retry back off between attempts?\": Yes, exponential, capped at 2 seconds.",
	}, said, "injected context (AGENTS.md, environment, file lists), shell-command echoes and developer text are not speech")
}

func TestParseTranscript_CodexAssistantMessages(t *testing.T) {
	ep, err := ParseTranscript(codexFixture)
	require.NoError(t, err)
	require.Len(t, ep.AssistantMessages, 2)
	assert.Equal(t, "Should the retry back off between attempts?", ep.AssistantMessages[0].Text)
	assert.Contains(t, ep.AssistantMessages[1].Text, "capped exponential backoff")
}

// Codex tool calls are JavaScript that calls tools.<name>(…); the tool is the
// name inside, not the wrapper ("exec"). Files come from apply_patch's fixed
// headers. Commits are NOT extracted: a subject pulled out of a JS string
// would be a guess, and an episode must not claim a commit it guessed.
func TestParseTranscript_CodexToolsAndFiles(t *testing.T) {
	ep, err := ParseTranscript(codexFixture)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"exec_command": 1, "apply_patch": 1, "update_plan": 1}, ep.ToolCounts)
	assert.Equal(t, []string{"/home/test/studio/fetch/fetch.go", "/home/test/studio/fetch/retry.go"}, ep.FilesTouched)
	assert.Empty(t, ep.Commits)
}

// Incremental capture skips the session_meta line on later calls; the delta
// must still know whose session it is.
func TestParseTranscriptFrom_CodexDeltaKeepsTheSessionID(t *testing.T) {
	ep, _, err := ParseTranscriptFrom(codexFixture, 10)
	require.NoError(t, err)
	assert.Equal(t, "01a0d100-aaaa-7bbb-8ccc-000000000001", ep.SessionID)
	require.Len(t, ep.AssistantMessages, 2, "only lines after the cursor")
}

// Sub-agent threads (Codex's guardian reviews, spawned threads) outnumbered
// real sessions 191 to 9 on one machine, and carry the PARENT's session_id —
// capturing them would overwrite the real session's episode.
func codexSubagent(t *testing.T, source string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rollout-sub.jsonl")
	body := `{"timestamp":"2026-09-24T10:00:00.000Z","type":"session_meta","payload":{"session_id":"01a0d100-aaaa-7bbb-8ccc-000000000001","id":"01a0d1ff-0000-7000-8000-000000000009","parent_thread_id":"01a0d100-aaaa-7bbb-8ccc-000000000001","cwd":"/home/test/studio","source":` + source + `}}
{"timestamp":"2026-09-24T10:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Review this command for risk."}]}}
`
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestIsSidechainTranscript_CodexSubagents(t *testing.T) {
	assert.True(t, isSidechainTranscript(codexSubagent(t, `{"subagent":{"other":"guardian"}}`)))
	assert.True(t, isSidechainTranscript(codexSubagent(t, `{"subagent":{"thread_spawn":{"parent_thread_id":"x"}}}`)))
	assert.False(t, isSidechainTranscript(codexFixture), "a main session is not a sidechain")
}

func TestCaptureDir_SkipsCodexSubagents(t *testing.T) {
	in := t.TempDir()
	raw, err := os.ReadFile(codexFixture)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(in, "rollout-main.jsonl"), raw, 0o600))
	sub, err := os.ReadFile(codexSubagent(t, `{"subagent":{"other":"guardian"}}`))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(in, "rollout-sub.jsonl"), sub, 0o600))

	out := t.TempDir()
	batch, err := CaptureDir(in, out)
	require.NoError(t, err)
	assert.Len(t, batch.Captured, 1, "the main session only")
	assert.Equal(t, 1, batch.Sidechains)
}

// Commits and PRs are not extracted from Codex sessions. "(none)" would claim
// the session made none — a statement nothing checked. The episode says what
// it did not record, and is tagged so the two sources can be told apart.
func TestRenderMarkdown_CodexSaysWhatItDidNotRecord(t *testing.T) {
	ep, err := ParseTranscript(codexFixture)
	require.NoError(t, err)
	md := RenderMarkdown(ep)
	assert.Contains(t, md, "tags:\n  - episode\n  - codex\n")
	assert.Contains(t, md, "## Commits made\n\n"+codexNotRecorded)
	assert.Contains(t, md, "## PRs opened\n\n"+codexNotRecorded)
	assert.NotContains(t, md, "## Commits made\n\n_(none)_")
}

func TestRenderMarkdown_ClaudeEpisodeUnchanged(t *testing.T) {
	ep, err := ParseTranscript("testdata/mini-session.jsonl")
	require.NoError(t, err)
	md := RenderMarkdown(ep)
	assert.Contains(t, md, "tags:\n  - episode\ncreated:")
	assert.NotContains(t, md, codexNotRecorded)
}
