package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHooksInstallMerge_WiresSettingsWithoutClobbering is the end-to-end proof
// of the onboarding fix: `hooks install --merge` writes the scripts AND merges
// the wiring into a project that already has its own hooks, leaving the
// foreign hook intact.
func TestHooksInstallMerge_WiresSettingsWithoutClobbering(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o750))
	settings := filepath.Join(claudeDir, "settings.json")
	require.NoError(t, os.WriteFile(settings, []byte(`{
  "hooks": { "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "bash /x/their-existing-hook.sh" } ] } ] },
  "permissions": { "allow": ["Bash(ls:*)"] }
}`), 0o600))

	_, _, err := runRootCmd(t, "hooks", "install", dir, "--vault", filepath.Join(dir, "their-vault"), "--merge")
	require.NoError(t, err)

	written, err := os.ReadFile(settings)
	require.NoError(t, err)
	text := string(written)
	assert.Contains(t, text, "their-existing-hook.sh", "foreign hook must survive the merge")
	assert.Contains(t, text, "vault-recall.sh", "our recall hook must be wired")
	assert.Contains(t, text, "load-persona.sh", "our persona hook must be wired")
	assert.Contains(t, text, "VAULTMIND_VAULT='"+filepath.Join(dir, "their-vault")+"'", "vault path baked in")
	assert.Contains(t, text, "permissions", "unrelated settings preserved")
}

func TestHooksInstallMerge_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runRootCmd(t, "hooks", "install", dir, "--merge", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Dry run", "dry-run must announce itself")

	_, statErr := os.Stat(filepath.Join(dir, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not create settings.json")
}

func TestHooksInstallMerge_Idempotent(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--merge")
	require.NoError(t, err)
	settings := filepath.Join(dir, ".claude", "settings.json")
	first, err := os.ReadFile(settings)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "hooks", "install", dir, "--merge")
	require.NoError(t, err)
	second, err := os.ReadFile(settings)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second), "second merge must not change the file")
	assert.Contains(t, out.String(), "already wired", "second run reports no change")
}

func TestHooksInstallMerge_LocalTargetsLocalFile(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--merge", "--local")
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, ".claude", "settings.local.json"))
	require.NoError(t, err, "settings.local.json must be created with --local")
	_, statErr := os.Stat(filepath.Join(dir, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(statErr), "settings.json must NOT be touched with --local")
}

// TestHooksUninstall_RemovesOursKeepsTheirs proves the round-trip:
// install --merge then uninstall leaves the project's own hooks intact.
func TestHooksUninstall_RemovesOursKeepsTheirs(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o750))
	settings := filepath.Join(claudeDir, "settings.json")
	require.NoError(t, os.WriteFile(settings, []byte(`{
  "hooks": { "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "bash /x/their-existing-hook.sh" } ] } ] }
}`), 0o600))

	_, _, err := runRootCmd(t, "hooks", "install", dir, "--merge")
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "hooks", "uninstall", dir, "--remove-scripts")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Removed")

	written, err := os.ReadFile(settings)
	require.NoError(t, err)
	text := string(written)
	assert.Contains(t, text, "their-existing-hook.sh", "foreign hook must survive uninstall")
	assert.NotContains(t, text, "vault-recall.sh", "our hooks must be removed")

	// --remove-scripts deletes the installed scripts.
	_, statErr := os.Stat(filepath.Join(claudeDir, "scripts", "vault-recall.sh"))
	assert.True(t, os.IsNotExist(statErr), "scripts must be deleted with --remove-scripts")
}

// TestHooksInstallMerge_SkippedWhenScriptsConflict proves the core safety
// invariant: a script conflict (existing copy with different content, no
// --force) aborts before the merge, so settings is never wired to point at an
// unresolved script.
func TestHooksInstallMerge_SkippedWhenScriptsConflict(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scriptsDir, 0o750))
	// Pre-place a conflicting copy of one canonical script.
	require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, "vault-recall.sh"),
		[]byte("#!/bin/bash\n# user-edited, different content\n"), 0o700))

	_, _, err := runRootCmd(t, "hooks", "install", dir, "--merge")
	require.Error(t, err, "a script conflict must surface as an error")

	_, statErr := os.Stat(filepath.Join(dir, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(statErr),
		"settings.json must NOT be written when scripts conflict — never wire unresolved scripts")
}

func TestHooksInstallMerge_DryRunRequiresMerge(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--dry-run")
	require.Error(t, err, "--dry-run without --merge must be rejected")

	// And nothing was written (the guard runs before any install work).
	_, statErr := os.Stat(filepath.Join(dir, ".claude", "scripts"))
	assert.True(t, os.IsNotExist(statErr), "rejected dry-run must not write scripts")
}

func TestHooksInstallMerge_JSONPayloadShape(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runRootCmd(t, "hooks", "install", dir, "--merge", "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			ProjectDir string `json:"project_dir"` // from embedded InstallResult
			Merge      struct {
				Changed bool   `json:"changed"`
				Path    string `json:"settings_path"`
			} `json:"merge"` // added only under --merge
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.NotEmpty(t, env.Result.ProjectDir, "embedded InstallResult fields stay at top level")
	assert.True(t, env.Result.Merge.Changed, "merge sub-object present and populated")
	assert.Contains(t, env.Result.Merge.Path, "settings.json")
}

func TestHooksUninstall_JSONReportsRemoved(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", dir, "--merge")
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "hooks", "uninstall", dir, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			Removed []string `json:"removed"`
			Changed bool     `json:"changed"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.True(t, env.Result.Changed)
	assert.Contains(t, env.Result.Removed, "vault-recall.sh")
}
