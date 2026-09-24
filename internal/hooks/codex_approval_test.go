package hooks

import (
	"encoding/json"
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
// (key format: codex-rs/hooks/src/lib.rs hook_key, rust-v0.156.1). Absent means
// never approved; present with a different hash means changed since approval
// ("modified" in /hooks), which Codex skips just the same — see
// TestCodexHookHash_MatchesCodex for how the hash is reproduced.

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
trusted_hash = "`+hashOf(t, hooksFile, "SessionStart", 0, 0)+`"

[hooks.state."`+hooksFile+`:session_start:0:1"]
trusted_hash = "`+hashOf(t, hooksFile, "SessionStart", 0, 1)+`"
enabled = false

[hooks.state."`+hooksFile+`:session_end:0:0"]
trusted_hash = "`+hashOf(t, hooksFile, "SessionEnd", 0, 0)+`"
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
trusted_hash = "`+hashOf(t, hooksFile, "SessionEnd", 0, 0)+`"
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

// Codex's approval hash (codex-rs discovery.rs hook_hash + fingerprint.rs
// version_for_toml, 0.156.1): sha256 over canonical JSON of the NORMALIZED hook
// — default timeouts filled in (600s; SessionEnd 1s, clamped to 3), async
// always present, additionalContextLimit kept only where it applies and not
// the 2500 default. An independent Python implementation of this reproduced
// all 10 real trusted_hash values on a real machine (2026-09-24); the expected
// values below come from it, not from this code.
func TestCodexHookHash_MatchesCodex(t *testing.T) {
	zero := 0
	three := 3
	startup := "startup"
	cases := []struct {
		name    string
		event   string
		matcher *string
		hook    codexHookJSON
		want    string
	}{
		{"session start with a context limit", "SessionStart", &startup,
			codexHookJSON{Type: "command", Command: "export VAULTMIND_PROJECT_DIR='/p'; bash '/p/.vaultmind/scripts/load-persona.sh'", AdditionalContextLimit: &zero},
			"sha256:0e6a08e6c1db49a0b1ff5e6665fd2a4c68a76fe8ab1ab316a7463b58b9295190"},
		{"session end with the max timeout", "SessionEnd", nil,
			codexHookJSON{Type: "command", Command: "bash '/p/.vaultmind/scripts/capture-episode.sh'", Timeout: &three},
			"sha256:1120f8ddabdfeba46d813a17ac22becbe1583ccd40c020e57f87e8290444d5e9"},
		{"shell operators are not HTML-escaped", "UserPromptSubmit", nil,
			codexHookJSON{Type: "command", Command: "a && b < c > d"},
			"sha256:e18109f30b0aa0ca441913320a668df91c124030e8ec3fc0d1609a91c16c3d60"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, codexHookHash(c.event, c.matcher, c.hook))
		})
	}
}

// Approved once, then changed (an upgrade rewrote the command): Codex shows it
// as "modified" and SKIPS it. Present-in-config is not approved.
func TestCodexApprovals_AChangedHookIsNotApproved(t *testing.T) {
	project, hooksFile := approvalProject(t)
	home := codexHomeWith(t, `[hooks.state."`+hooksFile+`:session_end:0:0"]
trusted_hash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
`)
	r, err := codexApprovals(project, home)
	require.NoError(t, err)
	assert.Equal(t, "modified", states(r)["SessionEnd capture-episode.sh"])
	assert.Equal(t, 0, r.Approved())
}

// hashOf is the current hash of one hook in a hooks.json, for fixtures that
// need a genuinely approved state.
func hashOf(t *testing.T, hooksFile, event string, gi, hi int) string {
	t.Helper()
	raw, err := os.ReadFile(hooksFile) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	var f codexHooksFileJSON
	require.NoError(t, json.Unmarshal(raw, &f))
	g := f.Hooks[event][gi]
	return codexHookHash(event, g.Matcher, g.Hooks[hi])
}
