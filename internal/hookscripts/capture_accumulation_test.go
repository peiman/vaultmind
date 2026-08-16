package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureFixture builds a project whose SessionEnd capture is stubbed (the Go
// binary is not under test here) with one already-written episode, so the only
// behaviour exercised is the accumulation addendum.
type captureFixture struct {
	projectDir string
	vaultRoot  string
	deskDir    string
	episode    string
}

func newCaptureFixture(t *testing.T, withDesk bool) *captureFixture {
	t.Helper()
	projectDir := t.TempDir()
	vaultRoot := filepath.Join(projectDir, "vaultmind-identity")
	episodes := filepath.Join(vaultRoot, "episodes")
	require.NoError(t, os.MkdirAll(episodes, 0o750))

	ep := filepath.Join(episodes, "episode-2026-08-16-sess1234.md")
	require.NoError(t, os.WriteFile(ep,
		[]byte("---\nid: episode-2026-08-16-sess1234\nsession_id: sess1234\n---\n\n# Episode\n"), 0o600))

	f := &captureFixture{projectDir: projectDir, vaultRoot: vaultRoot, episode: ep}
	if withDesk {
		f.deskDir = filepath.Join(vaultRoot, "journal")
		require.NoError(t, os.MkdirAll(f.deskDir, 0o750))
	}
	return f
}

func (f *captureFixture) run(t *testing.T, extraEnv ...string) string {
	t.Helper()
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	script, err := filepath.Abs("capture-episode.sh")
	require.NoError(t, err)

	// The hook DERIVES the transcript directory from $HOME plus the project path
	// with "/" replaced by "-" — it does not read a path from the payload. The
	// fixture has to mirror that layout or the hook exits before it gets near the
	// accumulation record.
	home := t.TempDir()
	transcriptsDir := filepath.Join(home, ".claude", "projects",
		strings.ReplaceAll(f.projectDir, string(os.PathSeparator), "-"))
	require.NoError(t, os.MkdirAll(transcriptsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(transcriptsDir, "sess1234.jsonl"),
		[]byte(`{"sessionId":"sess1234"}`+"\n"), 0o600))

	// Stub binary so `episode capture` succeeds without doing anything — the
	// episode already exists on disk, and the Go side is not under test here.
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "vaultmind"),
		[]byte("#!/bin/bash\nexit 0\n"), 0o700))

	payload, err := json.Marshal(map[string]string{"session_id": "sess1234"})
	require.NoError(t, err)

	cmd := exec.Command(bashPath, script)
	cmd.Env = append([]string{
		"PATH=" + binDir + ":/usr/bin:/bin",
		"CLAUDE_PROJECT_DIR=" + f.projectDir,
		"HOME=" + home,
	}, extraEnv...)
	cmd.Stdin = strings.NewReader(string(payload))
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	require.NoErrorf(t, cmd.Run(), "SessionEnd hook must always exit 0 (stderr: %s)", errb.String())

	body, err := os.ReadFile(f.episode) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	return string(body)
}

// An episode records what the session DID. Whether anything was KEPT is a
// different question, and the only place it can be answered is here — a run of
// sessions that captured perfectly and distilled nothing otherwise looks
// identical to a run that grew the vault.
func TestCaptureHook_AccumulationNamesTheDeskEntryWhenOneExists(t *testing.T) {
	f := newCaptureFixture(t, true)
	require.NoError(t, os.WriteFile(filepath.Join(f.deskDir, "2026-08-16-something-landed.md"),
		[]byte("---\ntype: journal\n---\nsess1234\n"), 0o600))

	body := f.run(t)
	assert.Contains(t, body, "## Accumulation")
	assert.Contains(t, body, "Desk entry: YES")
	assert.Contains(t, body, "something-landed.md")
}

func TestCaptureHook_AccumulationSaysNoWhenTheSessionKeptNothing(t *testing.T) {
	f := newCaptureFixture(t, true)

	body := f.run(t)
	assert.Contains(t, body, "## Accumulation")
	assert.Contains(t, body, "Desk entry: **NO")
	assert.Contains(t, body, "Last desk entry: never")
}

// The section must not claim an absence it never looked for. With no desk
// configured and none at the default path, "Desk entry: NO" would be a
// fabricated finding rather than a measurement.
func TestCaptureHook_NoDeskMeansNoAccumulationSection(t *testing.T) {
	f := newCaptureFixture(t, false)

	body := f.run(t)
	assert.NotContains(t, body, "## Accumulation",
		"without a desk to check, report nothing rather than report an absence")
}

// The desk is not always inside the vault — an agent's desk is often its own
// vault, which is the layout the arc-distillation docs describe.
func TestCaptureHook_DeskDirIsConfigurable(t *testing.T) {
	f := newCaptureFixture(t, false)
	elsewhere := filepath.Join(t.TempDir(), "my-desk")
	require.NoError(t, os.MkdirAll(elsewhere, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(elsewhere, "2026-08-16-kept.md"),
		[]byte("---\ntype: journal\n---\nsess1234\n"), 0o600))

	body := f.run(t, "VAULTMIND_DESK_DIR="+elsewhere)
	assert.Contains(t, body, "Desk entry: YES")
	assert.Contains(t, body, "kept.md")
}

// Appending twice must not produce two sections — SessionEnd can fire more than
// once for a session.
func TestCaptureHook_AccumulationIsIdempotent(t *testing.T) {
	f := newCaptureFixture(t, true)

	f.run(t)
	body := f.run(t)
	assert.Equal(t, 1, strings.Count(body, "## Accumulation"),
		"a second SessionEnd must not append a second record")
}
