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
// baking a portable vault path into the merged settings.
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
	assert.Contains(t, text, `VAULTMIND_VAULT=\"$CLAUDE_PROJECT_DIR/vaultmind-identity\"`)
	assert.NotContains(t, text, vault)
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
	_, statErr = os.Stat(filepath.Join(project, ".claude", "scripts"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not write the hook scripts")
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

func TestInitWireHooks_RelativeKnowledgeVaultIsPortable(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	_, _, err := runRootCmd(t, "init", "./knowledge", "--wire-hooks", "--project-dir", project)
	require.NoError(t, err)
	commands := settingsCommands(t, filepath.Join(project, ".claude", "settings.json"))
	health := commandWith(t, commands, "vaultmind-health.sh")
	assert.Contains(t, health, `VAULTMIND_VAULT="$CLAUDE_PROJECT_DIR/knowledge"`)
	assert.NotContains(t, health, filepath.Join(project, "knowledge"))
}

func TestInitWireHooks_LocalReportsGitExclude(t *testing.T) {
	for _, mode := range []string{"install", "dry run", "non-git"} {
		t.Run(mode, func(t *testing.T) {
			project, path := localExcludeRepo(t)
			if mode == "non-git" {
				project = t.TempDir()
			}
			args := []string{"init", filepath.Join(t.TempDir(), "knowledge"), "--wire-hooks", "--local", "--project-dir", project}
			if mode == "dry run" {
				args = append(args, "--dry-run")
			}
			out, _, err := runRootCmd(t, args...)
			require.NoError(t, err)
			switch mode {
			case "install":
				assert.Contains(t, out.String(), "kept out of git:")
				assert.Contains(t, out.String(), path)
				assert.Contains(t, readLocalExclude(t, path), localExcludeStart)
			case "dry run":
				assert.Contains(t, out.String(), "Would add to "+path)
				assert.NotContains(t, readLocalExclude(t, path), localExcludeStart)
			case "non-git":
				assert.Contains(t, out.String(), "git exclusion skipped:")
			}
		})
	}
	out, _, err := runRootCmd(t, "init", "--help")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "git info/exclude")
}
