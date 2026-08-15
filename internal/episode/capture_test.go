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

// sidechainTranscript renders a subagent transcript: Claude Code marks these
// with agentId + isSidechain and — critically — stamps them with the PARENT
// session's id, which is what makes them collide with the real session.
func sidechainTranscript(sessionID, agentID string) []byte {
	return []byte(`{"type":"user","isSidechain":true,"agentId":"` + agentID + `","message":{"role":"user","content":"Review this change for security vulnerabilities."},"timestamp":"2026-04-24T11:00:00.000Z","sessionId":"` + sessionID + `","cwd":"/home/test","gitBranch":"main"}
{"type":"assistant","isSidechain":true,"agentId":"` + agentID + `","message":{"model":"claude-opus-4-7","role":"assistant","content":[{"type":"text","text":"No vulnerabilities found."}]},"timestamp":"2026-04-24T11:00:09.000Z","sessionId":"` + sessionID + `"}
`)
}

// A directory sweep must capture the user's SESSIONS, not the subagent and
// workflow transcripts Claude Code nests beneath them. Those carry the parent's
// session id, so every one of them derives the SAME episode filename as the real
// session and overwrites it — last writer wins, and the winner is a sidechain.
// Measured on four real project histories: 1,759 nested transcripts, every one
// carrying agentId; 141 top-level session transcripts, none carrying it.
//
// The damage is not merely a lost file. What survives is a subagent's prompt
// ("Review this change for security vulnerabilities…"), so the vault an agent
// later reconstructs itself from is furnished with tool-chatter in place of the
// collaboration that actually happened.
func TestCaptureDir_SkipsSubagentTranscripts(t *testing.T) {
	src, err := os.ReadFile(fixturePath) // #nosec G304 -- test fixture
	require.NoError(t, err)
	const sessionID = "test-session-abc12345" // the fixture's own session

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), src, 0o600))
	nested := filepath.Join(dir, sessionID, "subagents", "workflows", "wf_1")
	require.NoError(t, os.MkdirAll(nested, 0o750))
	for _, agent := range []string{"a1111", "a2222", "a3333"} {
		require.NoError(t, os.WriteFile(filepath.Join(nested, "agent-"+agent+".jsonl"),
			sidechainTranscript(sessionID, agent), 0o600))
	}

	outDir := filepath.Join(t.TempDir(), "episodes")
	batch, err := episode.CaptureDir(dir, outDir)
	require.NoError(t, err)

	require.Len(t, batch.Captured, 1, "one real session in the tree → one episode")
	assert.Equal(t, 3, batch.Sidechains, "the three subagent transcripts are counted as deliberately skipped")
	assert.Empty(t, batch.Skipped, "a sidechain is not a malformed transcript — it must not be reported as one")

	// The surviving episode is the session, not the last subagent to be written.
	body, err := os.ReadFile(batch.Captured[0]) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(body), "run the tests please", "the real session's content survives")
	assert.NotContains(t, string(body), "Review this change for security vulnerabilities",
		"a subagent's prompt must never stand in for the session")
}

// Captured is the number the CLI reports to the user. If two transcripts derive
// the same episode id, the second silently overwrote the first — reporting both
// tells the user they have more history than exists on disk. The count must be
// answerable against `ls`.
func TestCaptureDir_CountMatchesFilesOnDisk(t *testing.T) {
	src, err := os.ReadFile(fixturePath) // #nosec G304 -- test fixture
	require.NoError(t, err)
	dir := t.TempDir()
	// Two distinct transcripts, same session id and date → one derived id.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jsonl"), src, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.jsonl"), src, 0o600))

	outDir := filepath.Join(t.TempDir(), "episodes")
	batch, err := episode.CaptureDir(dir, outDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, batch.Captured, len(entries),
		"reported captures must equal the episode files actually written")
	assert.Contains(t, batch.Skipped, filepath.Join(dir, "b.jsonl"),
		"the colliding transcript is reported, not silently dropped")
	assert.Contains(t, batch.Skipped[filepath.Join(dir, "b.jsonl")], "episode-2026-04-24-test-ses",
		"the reason names the id it collided on, so the user can tell which two files clashed")
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

	assert.Contains(t, batch.Skipped, filepath.Join(dir, "junk.jsonl"), "malformed transcript is recorded, not fatal")
	assert.NotContains(t, batch.Skipped, filepath.Join(dir, "notes.md"), "non-.jsonl files are ignored, not skipped")
	// The nested copy is a real (non-sidechain) transcript in a subdirectory, so
	// recursion still reaches it — it is passed over for colliding on the derived
	// id, not for being nested.
	assert.Contains(t, batch.Skipped, filepath.Join(sub, "nested.jsonl"), "recursion reaches the nested transcript")

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "good.jsonl and the nested copy share a session id → one episode file")
	assert.Len(t, batch.Captured, len(entries), "and the reported count says one, not two")
}
