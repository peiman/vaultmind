package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Codex project should not look like a Claude Code project. The hooks used
// to run `export CLAUDE_PROJECT_DIR=…; bash "$CLAUDE_PROJECT_DIR"/.claude/scripts/…`
// and install a .claude/ folder into projects that never used Claude Code —
// exactly what a person reads when approving them in /hooks. Codex projects now
// get a neutral variable and their scripts under .vaultmind/scripts/.

func TestCodexHooks_NameNothingClaude(t *testing.T) {
	out, err := CodexHooksStanza("/Users/x/proj", "/Users/x/proj/vault", ProfileFull)
	require.NoError(t, err)
	assert.NotContains(t, out, "CLAUDE")
	assert.NotContains(t, out, ".claude")
}

func TestCodexHooks_RunScriptsFromTheVaultmindFolder(t *testing.T) {
	f := renderCodex(t, "/Users/x/proj")
	cmd := f.Hooks["SessionStart"][0].Hooks[0].Command
	assert.Contains(t, cmd, `bash '/Users/x/proj/.vaultmind/scripts/load-persona.sh'`)
}

func TestInstall_CodexWritesScriptsUnderVaultmindNotClaude(t *testing.T) {
	dir := t.TempDir()
	res, err := Install(InstallConfig{ProjectDir: dir, Agent: AgentCodex})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".vaultmind", "scripts"), res.ScriptsDir)
	_, err = os.Stat(filepath.Join(dir, ".vaultmind", "scripts", hookSessionStartScript))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, ".claude"))
	assert.True(t, os.IsNotExist(err), "a Codex install creates no .claude folder")

	p, err := DeclaredProfile(dir)
	require.NoError(t, err)
	assert.Equal(t, ProfileFull, p, "the declared profile is found in the Codex location")
}

func TestInstall_ClaudeLayoutUnchanged(t *testing.T) {
	dir := t.TempDir()
	res, err := Install(InstallConfig{ProjectDir: dir})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".claude", "scripts"), res.ScriptsDir)
}

// Existing Codex installs carry the old commands. The merge used to skip any
// script already wired, so they would have kept .claude/scripts forever. Our
// own entries are updated IN PLACE: same position (Codex keys approvals by
// position), and the project's own hooks untouched.
func TestMergeIntoCodexHooks_UpdatesAnOldVaultMindEntryInPlace(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o750))
	old := `{"hooks":{"SessionStart":[
	  {"matcher":"startup","hooks":[{"type":"command","command":"export CLAUDE_PROJECT_DIR='` + dir + `'; bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/load-persona.sh","additionalContextLimit":0}]},
	  {"hooks":[{"type":"command","command":"echo mine"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".codex", "hooks.json"), []byte(old), 0o600))

	res, err := MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	assert.True(t, res.Changed)

	var f codexFile
	require.NoError(t, json.Unmarshal([]byte(res.Merged), &f))
	ss := f.Hooks["SessionStart"]
	require.GreaterOrEqual(t, len(ss), 2)
	assert.Contains(t, ss[0].Hooks[0].Command, ".vaultmind/scripts/load-persona.sh", "updated where it was")
	assert.Equal(t, "echo mine", ss[1].Hooks[0].Command, "the project's own hook keeps its place")
	assert.NotContains(t, res.Merged, "CLAUDE_PROJECT_DIR")
	assert.Equal(t, 1, strings.Count(res.Merged, "load-persona.sh"), "updated, not duplicated")

	again, err := MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	assert.False(t, again.Changed)
}

// A Codex-only project is judged as a Codex project. Grading it on Claude
// Code wiring it never had made status fail forever ("7 unwired"), found on the
// first real Codex install.
func TestStatus_CodexOnlyProjectIsNotJudgedOnClaudeWiring(t *testing.T) {
	dir := t.TempDir()
	_, err := Install(InstallConfig{ProjectDir: dir, Agent: AgentCodex})
	require.NoError(t, err)
	_, err = MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	t.Setenv("CODEX_HOME", t.TempDir())

	report, err := Status(dir)
	require.NoError(t, err)
	_, unwired := report.EventCounts()
	assert.Equal(t, 0, unwired, "no Claude Code wiring to judge")
	require.NotNil(t, report.Codex)
	assert.Equal(t, filepath.Join(dir, ".vaultmind", "scripts"), report.Codex.ScriptsDir)
	_, drifted, missing := report.Codex.ScriptCounts()
	assert.Equal(t, 0, drifted+missing, "Codex scripts checked where Codex runs them")
}

// An old-layout Codex install (scripts under .claude/scripts) shows its Codex
// scripts as missing, so status says to reinstall.
func TestStatus_OldLayoutCodexInstallAsksForAReinstall(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".codex", "hooks.json"), []byte(
		`{"hooks":{"SessionEnd":[{"hooks":[{"type":"command","command":"export CLAUDE_PROJECT_DIR='/p'; bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/capture-episode.sh"}]}]}}`), 0o600))
	t.Setenv("CODEX_HOME", t.TempDir())

	report, err := Status(dir)
	require.NoError(t, err)
	require.NotNil(t, report.Codex)
	_, _, missing := report.Codex.ScriptCounts()
	assert.Positive(t, missing)
}

// Codex gives a SessionEnd hook 1s by default and 3s at most (codex-rs
// session_end.rs, 0.156.1), then kills it. Capture takes ~0.06s warm; the
// maximum is margin for a cold start. additionalContextLimit is only for
// events that can add context — Codex ignores it on SessionEnd, with a warning.
func TestCodexHooks_SessionEndGetsTheMaximumTimeoutAndNoContextLimit(t *testing.T) {
	out, err := CodexHooksStanza("/p", "", ProfileFull)
	require.NoError(t, err)
	var f struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &f))
	h := f.Hooks["SessionEnd"][0].Hooks[0]
	assert.EqualValues(t, codexSessionEndMaxTimeout, h["timeout"])
	assert.NotContains(t, h, "additionalContextLimit")
	assert.NotContains(t, f.Hooks["SessionStart"][0].Hooks[0], "timeout", "other events keep Codex's default")
}

// Upgrading from the old layout leaves .claude/scripts behind in a Codex-only
// project. Without Claude Code wiring those files are leftovers: status must
// not judge Claude Code events or script drift from them (found on the first
// live upgrade, 2026-09-24), and says they can go. It does not delete them.
func TestStatus_LeftoverClaudeScriptsInACodexProjectAreNotJudged(t *testing.T) {
	dir := t.TempDir()
	_, err := Install(InstallConfig{ProjectDir: dir}) // the old layout's leftovers
	require.NoError(t, err)
	_, err = Install(InstallConfig{ProjectDir: dir, Agent: AgentCodex})
	require.NoError(t, err)
	_, err = MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	t.Setenv("CODEX_HOME", t.TempDir())

	report, err := Status(dir)
	require.NoError(t, err)
	_, unwired := report.EventCounts()
	assert.Equal(t, 0, unwired)
	assert.Empty(t, report.Scripts, "leftover Claude scripts are not graded")
	assert.Equal(t, filepath.Join(dir, ".claude", "scripts"), report.LeftoverClaudeScripts)
	_, err = os.Stat(filepath.Join(dir, ".claude", "scripts"))
	require.NoError(t, err, "status reports; it never deletes")
}

// Uninstall used to know only Claude Code: a Codex project kept its
// .codex/hooks.json entries and .vaultmind/scripts after `hooks uninstall`.
func TestRemoveFromCodexHooks_StripsOnlyOurEntries(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".codex", "hooks.json"),
		[]byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo mine"}]}]}}`), 0o600))
	_, err := Install(InstallConfig{ProjectDir: dir, Agent: AgentCodex})
	require.NoError(t, err)
	_, err = MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)

	res, err := RemoveFromCodexHooks(dir, false)
	require.NoError(t, err)
	assert.Len(t, res.Removed, 5)
	raw, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "echo mine", "the project's own hook survives")
	assert.NotContains(t, string(raw), ".vaultmind/scripts")
	_, err = os.Stat(filepath.Join(dir, ".vaultmind", "scripts", hookSessionStartScript))
	require.NoError(t, err, "scripts stay unless asked to remove them")
}

func TestRemoveFromCodexHooks_RemovesScriptsButNeverTheVaultmindFolder(t *testing.T) {
	dir := t.TempDir()
	_, err := Install(InstallConfig{ProjectDir: dir, Agent: AgentCodex})
	require.NoError(t, err)
	_, err = MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	// .vaultmind/ can also be a vault's own data folder.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "index.db"), []byte("vault data"), 0o600))

	res, err := RemoveFromCodexHooks(dir, true)
	require.NoError(t, err)
	assert.NotEmpty(t, res.ScriptsDeleted)
	_, err = os.Stat(filepath.Join(dir, ".vaultmind", "scripts", hookSessionStartScript))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err, "nothing but our scripts is deleted")
}
