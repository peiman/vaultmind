package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitWireHooks_ScaffoldsAndWires proves the one-command greenfield setup:
// init creates the vault AND wires the hooks into a separate project dir,
// baking the new vault's absolute path into the merged settings.
func TestInitWireHooks_ScaffoldsAndWires(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(project, 0o750))
	vault := filepath.Join(root, "proj", "vaultmind-identity")

	_, _, err := runRootCmd(t, "init", vault, "--wire-hooks", "--project-dir", project)
	require.NoError(t, err)

	// Vault scaffolded.
	_, err = os.Stat(filepath.Join(vault, ".vaultmind", "config.yaml"))
	require.NoError(t, err, "vault must be scaffolded")

	// Scripts installed into the project's .claude/scripts/.
	_, err = os.Stat(filepath.Join(project, ".claude", "scripts", "vault-recall.sh"))
	require.NoError(t, err, "hook scripts must be installed into the project")

	// Settings merged with the vault path baked in.
	settings, err := os.ReadFile(filepath.Join(project, ".claude", "settings.json"))
	require.NoError(t, err)
	text := string(settings)
	assert.Contains(t, text, "vault-recall.sh", "hooks wired")
	absVault, _ := filepath.Abs(vault)
	assert.Contains(t, text, "VAULTMIND_VAULT='"+absVault+"'", "absolute vault path baked into wiring")
}

// TestInitWireHooks_PreservesExistingHooks proves the merge is additive: a
// project that already has hooks keeps them.
func TestInitWireHooks_PreservesExistingHooks(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "proj")
	claudeDir := filepath.Join(project, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{
  "hooks": { "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "bash /x/theirs.sh" } ] } ] }
}`), 0o600))

	vault := filepath.Join(root, "v")
	_, _, err := runRootCmd(t, "init", vault, "--wire-hooks", "--project-dir", project)
	require.NoError(t, err)

	settings, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(settings), "theirs.sh", "existing hook preserved")
	assert.Contains(t, string(settings), "vault-recall.sh", "our hooks added")
}

// TestInitWireHooks_DryRunWritesNoSettings proves --dry-run previews the merge
// but does not write the project's settings file (the vault is still created).
func TestInitWireHooks_DryRunWritesNoSettings(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(project, 0o750))
	vault := filepath.Join(root, "v")

	out, _, err := runRootCmd(t, "init", vault, "--wire-hooks", "--dry-run", "--project-dir", project)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Dry run", "dry-run announced")

	_, statErr := os.Stat(filepath.Join(project, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not write settings.json")
	// Vault is still scaffolded.
	_, err = os.Stat(filepath.Join(vault, ".vaultmind", "config.yaml"))
	require.NoError(t, err)
}

// TestInit_NoWireHooks_LeavesProjectUntouched confirms the default init does
// not touch .claude/ (backward compatible).
func TestInit_NoWireHooks_LeavesProjectUntouched(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(project, 0o750))
	vault := filepath.Join(root, "v")

	_, _, err := runRootCmd(t, "init", vault, "--project-dir", project)
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(project, ".claude"))
	assert.True(t, os.IsNotExist(statErr), "init without --wire-hooks must not create .claude/")
}
