package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkVault creates a directory at rel under root and marks it as a vault by
// adding a .vaultmind/ subdir. Returns the absolute vault path.
func mkVault(t *testing.T, root, rel string) string {
	t.Helper()
	dir := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	return dir
}

func TestDiscoverVaults_FindsDirectChildren(t *testing.T) {
	root := t.TempDir()
	a := mkVault(t, root, "alpha")
	b := mkVault(t, root, "beta")
	// A non-vault directory must be ignored.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notavault"), 0o755))

	got, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	require.NoError(t, err)
	assert.Equal(t, []string{a, b}, got, "direct-child vaults, sorted")
}

func TestDiscoverVaults_RootItselfIsAVault(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o755))

	got, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	require.NoError(t, err)
	assert.Equal(t, []string{root}, got, "the root directory itself counts when it is a vault")
}

func TestDiscoverVaults_SortedDeterministicOrder(t *testing.T) {
	root := t.TempDir()
	// Create out of lexical order; discovery must still return sorted.
	z := mkVault(t, root, "zeta")
	a := mkVault(t, root, "alpha")
	m := mkVault(t, root, "mu")

	got, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	require.NoError(t, err)
	assert.Equal(t, []string{a, m, z}, got, "output is lexically sorted regardless of creation order")
}

func TestDiscoverVaults_NestedWithinDepth(t *testing.T) {
	root := t.TempDir()
	nested := mkVault(t, root, filepath.Join("workspace", "projects", "deep"))

	got, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	require.NoError(t, err)
	assert.Equal(t, []string{nested}, got, "a vault nested within the depth bound is discovered")
}

func TestDiscoverVaults_DoesNotDescendIntoDiscoveredVaults(t *testing.T) {
	root := t.TempDir()
	outer := mkVault(t, root, "outer")
	// A nested vault INSIDE a discovered vault must NOT be reported separately —
	// discovery stops descending once it finds a vault root.
	mkVault(t, outer, "inner")

	got, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	require.NoError(t, err)
	assert.Equal(t, []string{outer}, got,
		"discovery must not descend into a discovered vault's own subtree")
}

func TestDiscoverVaults_SkipsVaultInternals(t *testing.T) {
	root := t.TempDir()
	v := mkVault(t, root, "vault")
	// Put a stray .vaultmind nested under the vault's own .vaultmind — must be
	// ignored, never reported as a second vault.
	require.NoError(t, os.MkdirAll(filepath.Join(v, ".vaultmind", "cache", ".vaultmind"), 0o755))

	got, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	require.NoError(t, err)
	assert.Equal(t, []string{v}, got, "the vault's own .vaultmind internals are never walked")
}

func TestDiscoverVaults_RespectsDepthBound(t *testing.T) {
	root := t.TempDir()
	// Vault buried one level deeper than the bound allows.
	mkVault(t, root, filepath.Join("a", "b", "c", "d", "e", "buried"))

	got, err := discovery.DiscoverVaults(root, 2)
	require.NoError(t, err)
	assert.Empty(t, got, "a vault beyond maxDepth is not discovered")
}

func TestDiscoverVaults_EmptyRootReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	got, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	require.NoError(t, err)
	assert.Empty(t, got, "a root with no vaults yields an empty, non-error result")
}

func TestDiscoverVaults_MissingRootIsError(t *testing.T) {
	_, err := discovery.DiscoverVaults(filepath.Join(t.TempDir(), "does-not-exist"), discovery.DefaultMaxDepth)
	require.Error(t, err, "a non-existent root is a hard error, not a silent empty result")
}

func TestDiscoverVaults_RootIsFileIsError(t *testing.T) {
	f := filepath.Join(t.TempDir(), "afile")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	_, err := discovery.DiscoverVaults(f, discovery.DefaultMaxDepth)
	require.Error(t, err, "a regular file as root is a hard error")
	assert.Contains(t, err.Error(), "not a directory")
}

func TestDiscoverVaults_UnreadableSubdirIsError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions; cannot exercise the read-error path")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) }) // restore so TempDir cleanup works

	_, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	require.Error(t, err, "an unreadable directory below root surfaces as an error, not silent skip")
	assert.Contains(t, err.Error(), "reading directory")
}

func TestDiscoverVaults_IgnoresFilesNamedVaultmind(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tricky")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	// A FILE named .vaultmind must not qualify the directory as a vault.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind"), []byte("x"), 0o644))

	got, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	require.NoError(t, err)
	assert.Empty(t, got, "a .vaultmind FILE (not a directory) does not make a vault")
}
