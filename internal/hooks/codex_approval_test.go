package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Codex runs a project's hooks only after the person approves them in /hooks,
// and skips an unapproved hook WITHOUT SAYING SO: the agent just starts with no
// memory. A written warning is not enough — people miss warnings — so status
// checks the approvals Codex records in its config.toml:
//
//	[hooks.state."<project>/.codex/hooks.json:<event>:<group>:<handler>"]
//	trusted_hash = "sha256:…"
//
// (key format: codex-rs/hooks/src/lib.rs hook_key, rust-v0.156.1). Present
// means approved at least once; absent means never approved. A hook changed
// after approval shows as "modified" in /hooks — this check cannot see that,
// because the hash is over Codex's internal normalization of the hook.

// A hand-written hooks.json with known positions, including a hook that is
// the user's own: it must be ignored, and it shifts nothing.
const approvalHooksJSON = `{"hooks":{
  "SessionStart":[{"hooks":[
    {"type":"command","command":"export CLAUDE_PROJECT_DIR='/p'; bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/load-persona.sh"},
    {"type":"command","command":"export CLAUDE_PROJECT_DIR='/p'; bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/vaultmind-health.sh"}]}],
  "Stop":[{"hooks":[{"type":"command","command":"echo the user's own hook"}]}],
  "SessionEnd":[{"hooks":[
    {"type":"command","command":"export CLAUDE_PROJECT_DIR='/p'; bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/capture-episode.sh"}]}]
}}`

func approvalProject(t *testing.T) (project, hooksFile string) {
	t.Helper()
	project = t.TempDir()
	hooksFile = filepath.Join(project, ".codex", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(hooksFile), 0o750))
	require.NoError(t, os.WriteFile(hooksFile, []byte(approvalHooksJSON), 0o600))
	return project, hooksFile
}

func codexHomeWith(t *testing.T, toml string) string {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(toml), 0o600))
	return home
}

func states(r CodexApproval) map[string]string {
	out := map[string]string{}
	for _, h := range r.Hooks {
		out[h.Event+" "+h.Script] = string(h.State)
	}
	return out
}

func TestCodexApprovals_NothingApproved(t *testing.T) {
	project, _ := approvalProject(t)
	r, err := codexApprovals(project, codexHomeWith(t, "model = \"x\"\n"))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"SessionStart load-persona.sh":     "not_approved",
		"SessionStart vaultmind-health.sh": "not_approved",
		"SessionEnd capture-episode.sh":    "not_approved",
	}, states(r), "the user's own Stop hook is not ours to report")
}

func TestCodexApprovals_ReadsCodexsKeys(t *testing.T) {
	project, hooksFile := approvalProject(t)
	home := codexHomeWith(t, `
[hooks.state."`+hooksFile+`:session_start:0:0"]
trusted_hash = "sha256:aa"

[hooks.state."`+hooksFile+`:session_start:0:1"]
trusted_hash = "sha256:bb"
enabled = false

[hooks.state."`+hooksFile+`:session_end:0:0"]
trusted_hash = "sha256:cc"
`)
	r, err := codexApprovals(project, home)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"SessionStart load-persona.sh":     "approved",
		"SessionStart vaultmind-health.sh": "disabled",
		"SessionEnd capture-episode.sh":    "approved",
	}, states(r))
	assert.Equal(t, 2, r.Approved())
}

// Codex keys the path as it resolved the project. A project reached through a
// symlink (macOS /tmp -> /private/tmp) must still match.
func TestCodexApprovals_MatchesThroughASymlink(t *testing.T) {
	real, hooksFile := approvalProject(t)
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))
	resolved, err := filepath.EvalSymlinks(hooksFile)
	require.NoError(t, err)
	home := codexHomeWith(t, `[hooks.state."`+resolved+`:session_end:0:0"]
trusted_hash = "sha256:cc"
`)
	r, err := codexApprovals(link, home)
	require.NoError(t, err)
	assert.Equal(t, "approved", states(r)["SessionEnd capture-episode.sh"])
}

func TestCodexApprovals_NoCodexHooksMeansNoReport(t *testing.T) {
	r, err := codexApprovals(t.TempDir(), codexHomeWith(t, ""))
	require.NoError(t, err)
	assert.Empty(t, r.Hooks)
}

// No Codex config at all is the most common unapproved state: Codex has never
// recorded an approval on this machine.
func TestCodexApprovals_MissingCodexConfigMeansNothingApproved(t *testing.T) {
	project, _ := approvalProject(t)
	r, err := codexApprovals(project, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, 0, r.Approved())
	assert.Len(t, r.Hooks, 3)
}

func TestStatus_ReportsCodexApprovalsFromCODEXHOME(t *testing.T) {
	project, _ := approvalProject(t)
	t.Setenv("CODEX_HOME", codexHomeWith(t, ""))
	report, err := Status(project)
	require.NoError(t, err)
	require.NotNil(t, report.Codex)
	assert.Equal(t, 3, report.Codex.Unapproved())
}

// A project whose Codex hooks are all its own (focalc runs its own python
// hooks) has nothing of ours to approve; "0 of 0 approved" is noise.
func TestCodexApprovals_NoVaultMindHooksMeansNoReport(t *testing.T) {
	project := t.TempDir()
	f := filepath.Join(project, ".codex", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o750))
	require.NoError(t, os.WriteFile(f, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"python3 scripts/codex/own.py"}]}]}}`), 0o600))
	r, err := codexApprovals(project, codexHomeWith(t, ""))
	require.NoError(t, err)
	assert.Empty(t, r.Hooks)

	t.Setenv("CODEX_HOME", codexHomeWith(t, ""))
	report, err := Status(project)
	require.NoError(t, err)
	assert.Nil(t, report.Codex, "no Codex section when nothing is ours")
}
