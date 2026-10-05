package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hooks install's --vault names the path to bake into the hooks; left out,
// the hooks look for <project>/vaultmind-identity when they run. It must not
// pick up whatever vault the shell happens to be standing in: from inside one,
// the vault discovery other commands use baked it into every hook.
func TestHooksInstall_DoesNotBakeTheVaultItIsRunFrom(t *testing.T) {
	here := t.TempDir()
	makeVault(t, here)
	t.Chdir(here)
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".codex"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".codex", "hooks.json"), []byte(`{"hooks":{}}`), 0o600))

	for _, agent := range []string{"claude", "codex"} {
		out, _, err := runRootCmd(t, "hooks", "install", project, "--agent", agent, "--merge", "--dry-run", "--force")
		require.NoError(t, err, agent)
		assert.NotContains(t, out.String(), "VAULTMIND_VAULT", agent)
	}
}

// An explicit --vault is still what the hooks get.
func TestHooksInstall_BakesAnExplicitVault(t *testing.T) {
	t.Chdir(t.TempDir())
	project, vault := t.TempDir(), t.TempDir()
	makeVault(t, vault)
	out, _, err := runRootCmd(t, "hooks", "install", project, "--vault", vault, "--merge", "--dry-run", "--force")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "VAULTMIND_VAULT='"+vault+"'")
}
