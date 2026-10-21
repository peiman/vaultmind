package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureGitignore_ReportsDirectoryError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind"), []byte("blocked"), 0o600))
	err := vault.EnsureGitignore(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create vault state directory")
}

func TestEnsureGitignore_ReportsCreateError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	state := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.Mkdir(state, 0o500))
	t.Cleanup(func() { _ = os.Chmod(state, 0o750) })
	err := vault.EnsureGitignore(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create vault .gitignore")
}

func TestEnsureGitignore_PreservesExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".vaultmind"), 0o750))
	path := filepath.Join(dir, ".vaultmind", ".gitignore")
	target := filepath.Join(dir, "missing-user-policy")
	require.NoError(t, os.Symlink(target, path))
	require.NoError(t, vault.EnsureGitignore(dir))
	got, err := os.Readlink(path)
	require.NoError(t, err)
	assert.Equal(t, target, got)
	assert.NoFileExists(t, target, "do not follow a user's symlink to create or overwrite its target")
}
