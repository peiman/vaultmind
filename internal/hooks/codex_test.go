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

// LIVE-OBSERVED (2026-09-22, Codex CLI 0.156 against the real identity vault):
// Codex runs the same hook events with the same stdin shape as Claude Code, but
// sets no CLAUDE_PROJECT_DIR. Every script path became /.claude/scripts/... and
// every hook reported "Failed" — an agent with no memory and no visible error.

type codexFile struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command                string `json:"command"`
			AdditionalContextLimit *int   `json:"additionalContextLimit"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func renderCodex(t *testing.T, projectDir string) codexFile {
	t.Helper()
	out, err := CodexHooksStanza(projectDir, "", ProfileFull)
	require.NoError(t, err)
	var f codexFile
	require.NoError(t, json.Unmarshal([]byte(out), &f), "must be valid JSON under a top-level \"hooks\" key")
	return f
}

// THE ONE THAT BIT. The project dir must be EXPORTED before the command, not
// written as a prefix assignment: `X=/p bash "$X"/s.sh` expands "$X" before the
// assignment applies, which is exactly the failure observed live.
func TestCodexHooks_ExportProjectDirBeforeTheCommand(t *testing.T) {
	f := renderCodex(t, "/Users/x/proj")
	require.NotEmpty(t, f.Hooks)
	var n int
	for _, groups := range f.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				n++
				assert.True(t, strings.HasPrefix(h.Command, "export CLAUDE_PROJECT_DIR='/Users/x/proj'; "), h.Command)
			}
		}
	}
	assert.Positive(t, n)
}

// Only the hooks that WORK under Codex are wired. Episode capture is left out
// deliberately: it looks for the transcript under Claude Code's layout, misses
// the Codex session, and falls back to the newest Claude transcript — recording
// the WRONG session as this one, silently. Read-tracking has no Read tool to
// match; PreCompact cannot emit context in Codex.
func TestCodexHooks_WiresOnlyWhatWorksUnderCodex(t *testing.T) {
	out, err := CodexHooksStanza("/p", "", ProfileFull)
	require.NoError(t, err)
	for _, want := range []string{hookSessionStartScript, hookHealthScript, hookUserPromptSubmitScript, hookReachScript} {
		assert.Contains(t, out, want)
	}
	for _, never := range []string{hookSessionEndScript, hookPreToolUseScript, hookPreCompactScript} {
		assert.NotContains(t, out, never)
	}
}

// Codex moves context over ~2500 tokens to a file by default. The identity
// load is ~18KB; truncating the self is not an acceptable default.
func TestCodexHooks_ContextIsNeverSpilled(t *testing.T) {
	f := renderCodex(t, "/p")
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse"} {
		require.NotEmpty(t, f.Hooks[ev], ev)
		for _, g := range f.Hooks[ev] {
			for _, h := range g.Hooks {
				require.NotNil(t, h.AdditionalContextLimit, ev)
				assert.Equal(t, 0, *h.AdditionalContextLimit, ev)
			}
		}
	}
}

// Claude Code's stanza must not change: no export prefix, no Codex-only field.
func TestClaudeStanza_Unchanged(t *testing.T) {
	out, err := SettingsStanza("")
	require.NoError(t, err)
	assert.NotContains(t, out, "export CLAUDE_PROJECT_DIR")
	assert.NotContains(t, out, "additionalContextLimit")
}

func TestCodexHooks_PathWithQuoteIsSafe(t *testing.T) {
	out, err := CodexHooksStanza("/Users/o'brien/p", "", ProfileFull)
	require.NoError(t, err)
	assert.Contains(t, out, `export CLAUDE_PROJECT_DIR='/Users/o'\\''brien/p'; `)
}

func TestMergeIntoCodexHooks_WritesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	res, err := MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	assert.Equal(t, filepath.Join(dir, ".codex", "hooks.json"), res.SettingsPath)
	first, err := os.ReadFile(res.SettingsPath)
	require.NoError(t, err)

	again, err := MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	assert.False(t, again.Changed, "a re-run must not duplicate hooks")
	second, err := os.ReadFile(res.SettingsPath)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

// A project's own Codex hooks survive the merge.
func TestMergeIntoCodexHooks_PreservesExistingHooks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o750))
	mine := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo mine"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".codex", "hooks.json"), []byte(mine), 0o600))

	res, err := MergeIntoCodexHooks(dir, "", ProfileFull, false)
	require.NoError(t, err)
	assert.Contains(t, res.Merged, "echo mine")
	assert.Contains(t, res.Merged, hookSessionStartScript)
}

func TestMergeIntoCodexHooks_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	res, err := MergeIntoCodexHooks(dir, "", ProfileFull, true)
	require.NoError(t, err)
	assert.Contains(t, res.Merged, hookSessionStartScript)
	_, statErr := os.Stat(filepath.Join(dir, ".codex", "hooks.json"))
	assert.True(t, os.IsNotExist(statErr))
}

// A knowledge vault has no persona to load, under Codex as under Claude Code.
func TestCodexHooks_RespectsTheProfile(t *testing.T) {
	out, err := CodexHooksStanza("/p", "", ProfileKnowledge)
	require.NoError(t, err)
	assert.NotContains(t, out, hookSessionStartScript)
}

// An event with no hooks must be absent, not null. Codex drops PreCompact and
// SessionEnd; rendering them as `null` hands Codex a malformed event list.
func TestCodexHooks_NoNullEvents(t *testing.T) {
	out, err := CodexHooksStanza("/p", "", ProfileFull)
	require.NoError(t, err)
	assert.NotContains(t, out, "null")
	assert.NotContains(t, out, `"SessionEnd"`)
}
