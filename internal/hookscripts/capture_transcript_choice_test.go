package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Which transcript does SessionEnd capture? The hook used to read only
// session_id, look under Claude Code's transcript directory, and — when that
// session's file was not there — capture the NEWEST transcript instead. Under
// Codex (whose transcripts live elsewhere) that was always the wrong session,
// and under Claude Code with two sessions open in one repo it could be. The
// payload names the file (transcript_path, sent by both Claude Code and Codex);
// a named session whose file cannot be found is captured as nothing, loudly.

type choiceRun struct {
	captured string // the transcript the binary was asked to capture; "" = not called
	stderr   string
	log      string // the capture log after the run
}

type choiceEnv struct {
	projectDir     string
	home           string
	transcriptsDir string
}

func newChoiceEnv(t *testing.T) choiceEnv {
	t.Helper()
	e := choiceEnv{projectDir: t.TempDir(), home: t.TempDir()}
	e.transcriptsDir = filepath.Join(e.home, ".claude", "projects",
		strings.ReplaceAll(e.projectDir, string(os.PathSeparator), "-"))
	require.NoError(t, os.MkdirAll(e.transcriptsDir, 0o750))
	return e
}

// transcript writes a Claude Code transcript for sid; later calls are newer.
func (e choiceEnv) transcript(t *testing.T, sid string, age time.Duration) string {
	t.Helper()
	p := filepath.Join(e.transcriptsDir, sid+".jsonl")
	require.NoError(t, os.WriteFile(p, []byte(`{"sessionId":"`+sid+`"}`+"\n"), 0o600))
	ts := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(p, ts, ts))
	return p
}

// run executes the real hook with payload on stdin (nil = no stdin payload)
// and a stub binary that records the transcript it is asked to capture.
func (e choiceEnv) run(t *testing.T, payload map[string]any) choiceRun {
	t.Helper()
	return e.runWith(t, payload, "/vault/episodes/episode-x.md")
}

// runWith is run with the stub binary's stdout set: the real binary prints the
// episode path it wrote, or a "(nothing new …)" line when there was nothing.
func (e choiceEnv) runWith(t *testing.T, payload map[string]any, binaryOut string) choiceRun {
	t.Helper()
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	script, err := filepath.Abs("capture-episode.sh")
	require.NoError(t, err)

	binDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "captured")
	stub := "#!/bin/bash\n[ \"$1 $2\" = \"episode capture\" ] && printf '%s' \"$3\" > '" + record + "'\necho '" + binaryOut + "'\nexit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "vaultmind"), []byte(stub), 0o700)) //nolint:gosec // G306: test stub must be executable

	cmd := exec.Command(bashPath, script)
	cmd.Env = []string{
		"PATH=" + binDir + ":/usr/bin:/bin:/opt/homebrew/bin:/usr/local/bin",
		"CLAUDE_PROJECT_DIR=" + e.projectDir,
		"HOME=" + e.home,
	}
	logFile := filepath.Join(e.home, ".vaultmind", "capture", "capture.log")
	if payload != nil {
		raw, mErr := json.Marshal(payload)
		require.NoError(t, mErr)
		cmd.Stdin = strings.NewReader(string(raw))
	}
	var errb strings.Builder
	cmd.Stderr = &errb
	require.NoErrorf(t, cmd.Run(), "SessionEnd must always exit 0 (stderr: %s)", errb.String())

	got, _ := os.ReadFile(record)     // #nosec G304 -- test-controlled path
	logged, _ := os.ReadFile(logFile) // #nosec G304 -- test-controlled path
	return choiceRun{captured: string(got), stderr: errb.String(), log: string(logged)}
}

func TestCaptureHook_UsesTheTranscriptPathThePayloadNames(t *testing.T) {
	e := newChoiceEnv(t)
	e.transcript(t, "someone-else", 0) // newer, and in the Claude directory
	codex := filepath.Join(t.TempDir(), "rollout-2026-09-24T09-00-00-01a0d100.jsonl")
	require.NoError(t, os.WriteFile(codex, []byte(`{"type":"session_meta","payload":{}}`+"\n"), 0o600))

	r := e.run(t, map[string]any{"session_id": "01a0d100", "transcript_path": codex})
	assert.Equal(t, codex, r.captured)
}

// The failure this exists for: the named session's file is not where the hook
// looks, and a NEIGHBOUR's is. Capturing the neighbour records another session
// as this one; capturing nothing and saying so loses one episode, honestly.
func TestCaptureHook_NeverCapturesANeighbourForANamedSession(t *testing.T) {
	e := newChoiceEnv(t)
	e.transcript(t, "someone-else", 0)

	r := e.run(t, map[string]any{"session_id": "mine-not-on-disk", "transcript_path": nil})
	assert.Empty(t, r.captured, "a named session is never replaced by the newest transcript")
	assert.Contains(t, r.stderr, "mine-not-on-disk")
}

func TestCaptureHook_ANamedPathThatDoesNotExistCapturesNothing(t *testing.T) {
	e := newChoiceEnv(t)
	e.transcript(t, "someone-else", 0)

	r := e.run(t, map[string]any{"session_id": "s1", "transcript_path": filepath.Join(t.TempDir(), "gone.jsonl")})
	assert.Empty(t, r.captured)
	assert.Contains(t, r.stderr, "gone.jsonl")
}

func TestCaptureHook_ClaudeSessionIDStillFindsItsTranscript(t *testing.T) {
	e := newChoiceEnv(t)
	mine := e.transcript(t, "mine", time.Hour)
	e.transcript(t, "someone-else", 0)

	r := e.run(t, map[string]any{"session_id": "mine"})
	assert.Equal(t, mine, r.captured, "its own file, not the newer neighbour")
}

// With no payload at all there is no session to be wrong about; the newest
// transcript is the only guess available, as before.
func TestCaptureHook_NoPayloadKeepsTheNewestFallback(t *testing.T) {
	e := newChoiceEnv(t)
	e.transcript(t, "older", time.Hour)
	newest := e.transcript(t, "newest", 0)

	r := e.run(t, nil)
	assert.Equal(t, newest, r.captured)
}

// SessionEnd output is shown nowhere — under Codex or Claude Code — so a
// capture that failed, or a hook that never ran, looked identical: no episode.
// Found on the first live Codex test (2026-09-24). Every run now leaves one
// line in ~/.vaultmind/capture/capture.log saying what happened.
func TestCaptureHook_LogsEveryRunWithItsOutcome(t *testing.T) {
	e := newChoiceEnv(t)
	mine := e.transcript(t, "mine", 0)

	r := e.run(t, map[string]any{"session_id": "mine"})
	assert.Equal(t, mine, r.captured)
	assert.Contains(t, r.log, "\tmine\tcaptured /vault/episodes/episode-x.md", "names the episode written")
	assert.Contains(t, r.log, e.projectDir)

	r = e.run(t, map[string]any{"session_id": "gone"})
	lines := strings.Split(strings.TrimSpace(r.log), "\n")
	require.Len(t, lines, 2, "one line per run, appended")
	assert.Contains(t, lines[1], "\tgone\tnot captured: no transcript for this session")
}

// A session with nothing said in it (opened, approved, quit) writes no episode.
// The log said "captured" for one on the first live Codex run; it now says what
// the binary said.
func TestCaptureHook_LogsWhenThereWasNothingToCapture(t *testing.T) {
	e := newChoiceEnv(t)
	e.transcript(t, "empty", 0)
	r := e.runWith(t, map[string]any{"session_id": "empty"}, "(nothing new to capture since the last incremental capture)")
	assert.Contains(t, r.log, "\tempty\tnothing new to capture")
	assert.NotContains(t, r.log, "captured")
}
