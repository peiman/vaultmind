package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `hooks install --agent cursor` wires Cursor: the scripts and the adapter
// under .vaultmind/scripts, the hooks in .cursor/hooks.json.

func TestHooksInstallCursor_MergeWiresCursor(t *testing.T) {
	dir := t.TempDir()
	vault := filepath.Join(dir, "kb")
	require.NoError(t, os.MkdirAll(vault, 0o750))

	out, _, err := runRootCmd(t, "hooks", "install", dir, "--agent", "cursor", "--merge", "--vault", vault)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dir, ".cursor", "hooks.json")) //nolint:gosec // test path
	require.NoError(t, err)
	var f struct {
		Version int                                   `json:"version"`
		Hooks   map[string][]struct{ Command string } `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(raw, &f))
	assert.Equal(t, 1, f.Version)
	assert.NotEmpty(t, f.Hooks["sessionStart"])
	assert.NotEmpty(t, f.Hooks["postToolUse"])
	assert.FileExists(t, filepath.Join(dir, ".vaultmind", "scripts", "cursor-adapter.sh"))
	assert.NoDirExists(t, filepath.Join(dir, ".claude"), "a Cursor project grows no .claude/ folder")

	text := out.String()
	assert.Contains(t, text, ".cursor/hooks.json")
	assert.Contains(t, text, "per-prompt", "the install says what Cursor cannot do")
}

func TestHooksInstallCursor_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--agent", "cursor", "--merge", "--dry-run")
	require.NoError(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a Cursor dry run must not write scripts, a profile, or hooks.json")
}

func TestHooksInstallCursor_WithoutMergePrintsTheStanza(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runRootCmd(t, "hooks", "install", dir, "--agent", "cursor")
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, ".cursor", "hooks.json"))
	assert.Contains(t, out.String(), "--agent cursor --merge", "the rerun names the agent")
	assert.Contains(t, out.String(), `"version": 1`)
}

func TestHooksInstall_RejectsAnUnknownAgentNamingCursor(t *testing.T) {
	_, _, err := runRootCmd(t, "hooks", "install", t.TempDir(), "--agent", "vim")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cursor")
}

func TestHooksStatusCursor_ACleanInstallPasses(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--agent", "cursor", "--merge")
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.NoError(t, err, "a fresh Cursor install is a clean status: %s", out.String())
	assert.Contains(t, out.String(), "Cursor:")
	assert.NotContains(t, out.String(), "No hook scripts installed")
}

func TestHooksStatusCursor_ADriftedScriptFailsWithTheCursorFix(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--agent", "cursor", "--merge")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "scripts", "vault-code-map.sh"),
		[]byte("#!/bin/bash\necho changed\n"), 0o600))

	out, _, err := runRootCmd(t, "hooks", "status", dir)
	require.Error(t, err)
	assert.Contains(t, out.String(), "drifted")
	assert.Contains(t, out.String(), "--agent cursor --merge --force")
}

func TestHooksUninstallCursor_RemovesTheWiring(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--agent", "cursor", "--merge")
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "hooks", "uninstall", dir, "--agent", "cursor", "--remove-scripts")
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, ".cursor", "hooks.json")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "cursor-adapter.sh")
	assert.NoFileExists(t, filepath.Join(dir, ".vaultmind", "scripts", "cursor-adapter.sh"))
	assert.True(t, strings.Contains(out.String(), "Removed"), out.String())
}
