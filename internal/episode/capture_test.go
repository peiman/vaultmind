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

// On failure the path must be empty. Returning the path it WOULD have written
// hands the caller a filename for a file that does not exist — an invitation to
// print or store it if the error is ever checked second.
func TestCapture_ReturnsNoPathWhenTheWriteFails(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))

	path, err := episode.Capture(fixturePath, filepath.Join(blocked, "episodes"))
	require.Error(t, err)
	assert.Empty(t, path, "no file was written, so there is no path to hand back")
}

// A write failure is OUR fault, not the transcript's, and it is systemic: the
// output directory is the same for every transcript in the batch, so the first
// failure will repeat for all of them. Reporting them one by one as skipped
// files told the user their entire session history was junk — with exit 0 —
// when the real cause was an unusable --output-dir. Abort and say so.
func TestCaptureDir_WriteFailureAbortsInsteadOfBlamingTheTranscripts(t *testing.T) {
	src, err := os.ReadFile(fixturePath) // #nosec G304 -- test fixture
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jsonl"), src, 0o600))

	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))

	batch, err := episode.CaptureDir(dir, filepath.Join(blocked, "episodes"))
	require.Error(t, err, "an unusable output dir must fail the run, not be absorbed")
	assert.Contains(t, err.Error(), "not a directory", "and must name the real cause")
	assert.Empty(t, batch.Skipped, "a good transcript is never recorded as a bad one")
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
	assert.Contains(t, batch.Collisions, filepath.Join(dir, "b.jsonl"),
		"the colliding transcript is reported, not silently dropped")
	assert.Contains(t, batch.Collisions[filepath.Join(dir, "b.jsonl")], "episode-2026-04-24-test-ses",
		"the reason names the id it collided on, so the user can tell which two files clashed")
	assert.Empty(t, batch.Skipped, "a collision is not a malformed file — different cause, different fix")
}

// A collision is the one outcome where the user may have lost something: two
// transcripts existed and only one is representable. It is separated from
// Skipped so the CLI can NAME the pair instead of folding it into a count —
// "Skipped 1 file(s)" tells a reader nothing they can act on, and the reason
// string this code carefully builds was being computed and then discarded.
func TestCaptureDir_CollisionsAreDistinctFromFaults(t *testing.T) {
	src, err := os.ReadFile(fixturePath) // #nosec G304 -- test fixture
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jsonl"), src, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.jsonl"), src, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.jsonl"), []byte("not json\n"), 0o600))

	batch, err := episode.CaptureDir(dir, filepath.Join(t.TempDir(), "episodes"))
	require.NoError(t, err)

	assert.Len(t, batch.Collisions, 1, "b.jsonl collided")
	assert.Len(t, batch.Skipped, 1, "junk.jsonl is a fault")
	assert.Contains(t, batch.Skipped, filepath.Join(dir, "junk.jsonl"))
}

// Partial defence in depth: if the sidechain detector misses one that ran on the
// SAME DATE as its session, the collision check still keeps the real session,
// because a session's own transcript sorts before its subdirectory ("." is 0x2e,
// "/" is 0x2f) and so claims the episode id first. That sort-order property is
// load-bearing and would otherwise vanish silently if the walk ever changed.
//
// The cover is only partial: a missed sidechain from a DIFFERENT date derives a
// different id and collides with nothing — see
// TestCaptureDir_SidechainOnALaterDateIsCaughtByTheDetectorAlone, which is the
// case with no second line of defence.
func TestCaptureDir_RealSessionWinsACollisionAgainstItsOwnSidechain(t *testing.T) {
	src, err := os.ReadFile(fixturePath) // #nosec G304 -- test fixture
	require.NoError(t, err)
	const sessionID = "test-session-abc12345"

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), src, 0o600))
	nested := filepath.Join(dir, sessionID, "subagents")
	require.NoError(t, os.MkdirAll(nested, 0o750))
	// Deliberately UNMARKED: stands in for a sidechain the detector fails to
	// recognize (a future transcript format, a marker moved past the scan limit).
	unmarked := []byte(`{"type":"user","message":{"role":"user","content":"Review this change for security vulnerabilities."},"timestamp":"2026-04-24T11:00:00.000Z","sessionId":"` + sessionID + `","cwd":"/home/test","gitBranch":"main"}` + "\n")
	require.NoError(t, os.WriteFile(filepath.Join(nested, "agent-a1.jsonl"), unmarked, 0o600))

	outDir := filepath.Join(t.TempDir(), "episodes")
	batch, err := episode.CaptureDir(dir, outDir)
	require.NoError(t, err)

	require.Len(t, batch.Captured, 1)
	assert.Zero(t, batch.Sidechains, "this one is unmarked by construction — the detector does NOT catch it")
	assert.Len(t, batch.Collisions, 1, "so the collision check is what protects the session")

	body, err := os.ReadFile(batch.Captured[0]) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(body), "run the tests please", "the real session still wins")
	assert.NotContains(t, string(body), "Review this change for security vulnerabilities")
}

// A long session spans midnight (one real session in the corpus ran 2026-07-26
// to 2026-08-05), so its subagents routinely carry a LATER date than the session
// itself and derive a different episode id. Nothing collides, so the detector is
// the only thing standing between the user and a spurious standalone episode
// built from a subagent's prompt. This is the case the sort-order property does
// not cover.
func TestCaptureDir_SidechainOnALaterDateIsCaughtByTheDetectorAlone(t *testing.T) {
	src, err := os.ReadFile(fixturePath) // #nosec G304 -- test fixture
	require.NoError(t, err)
	const sessionID = "test-session-abc12345" // fixture session starts 2026-04-24

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), src, 0o600))
	nested := filepath.Join(dir, sessionID, "subagents")
	require.NoError(t, os.MkdirAll(nested, 0o750))
	late := []byte(`{"type":"user","isSidechain":true,"agentId":"a9","message":{"role":"user","content":"Review this change for security vulnerabilities."},"timestamp":"2026-04-25T09:00:00.000Z","sessionId":"` + sessionID + `","cwd":"/home/test","gitBranch":"main"}
{"type":"assistant","isSidechain":true,"agentId":"a9","message":{"role":"assistant","content":[{"type":"text","text":"No vulnerabilities found."}]},"timestamp":"2026-04-25T09:00:09.000Z","sessionId":"` + sessionID + `"}
`)
	require.NoError(t, os.WriteFile(filepath.Join(nested, "agent-a9.jsonl"), late, 0o600))

	outDir := filepath.Join(t.TempDir(), "episodes")
	batch, err := episode.CaptureDir(dir, outDir)
	require.NoError(t, err)

	assert.Equal(t, 1, batch.Sidechains)
	assert.Empty(t, batch.Collisions, "different date → different id → nothing collides")
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the subagent must not become a second, standalone episode")
}

// A workflow journal carries agentId but NOT isSidechain, and no sessionId at
// all. It is the case that makes agentId alone sufficient — and, because it has
// no session id, the case that would otherwise be reported to the user as a
// malformed transcript when it is simply not a session.
func TestCaptureDir_WorkflowJournalIsASidechainByAgentIDAlone(t *testing.T) {
	src, err := os.ReadFile(fixturePath) // #nosec G304 -- test fixture
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "session.jsonl"), src, 0o600))
	wf := filepath.Join(dir, "session", "subagents", "workflows", "wf_1")
	require.NoError(t, os.MkdirAll(wf, 0o750))
	journal := []byte(`{"type":"started","key":"step-1","agentId":"a1"}
{"type":"result","key":"step-1","agentId":"a1"}
`)
	require.NoError(t, os.WriteFile(filepath.Join(wf, "journal.jsonl"), journal, 0o600))

	batch, err := episode.CaptureDir(dir, filepath.Join(t.TempDir(), "episodes"))
	require.NoError(t, err)

	assert.Equal(t, 1, batch.Sidechains, "a journal is a sidechain via agentId alone")
	assert.Empty(t, batch.Skipped, "and must not be reported as a malformed transcript")
	assert.Len(t, batch.Captured, 1)
}

// The marker need not be on record 1. Measured across 1,765 real nested
// transcripts it always was, but the scan limit exists precisely because that is
// an observation rather than a guarantee — so the walk past record 1 must work.
func TestCaptureDir_MarkerFoundPastTheFirstRecord(t *testing.T) {
	dir := t.TempDir()
	body := `{"type":"summary","summary":"prior conversation","leafUuid":"x"}` + "\n"
	for i := 0; i < 5; i++ {
		body += `{"type":"user","isSidechain":true,"agentId":"a1","message":{"role":"user","content":"go"},"timestamp":"2026-04-24T11:00:00.000Z","sessionId":"test-session-abc12345"}` + "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agent-a1.jsonl"), []byte(body), 0o600))

	batch, err := episode.CaptureDir(dir, filepath.Join(t.TempDir(), "episodes"))
	require.NoError(t, err)
	assert.Equal(t, 1, batch.Sidechains, "the marker is found past the first record")
	assert.Empty(t, batch.Captured)
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
	assert.Contains(t, batch.Collisions, filepath.Join(sub, "nested.jsonl"), "recursion reaches the nested transcript")

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "good.jsonl and the nested copy share a session id → one episode file")
	assert.Len(t, batch.Captured, len(entries), "and the reported count says one, not two")
}
