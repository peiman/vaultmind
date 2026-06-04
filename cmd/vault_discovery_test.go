package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noEnv(string) string { return "" }

func TestWalkUpForVault_FindsNearestAncestor(t *testing.T) {
	root := t.TempDir()
	// root/project/.vaultmind, querying from root/project/sub/deep
	project := filepath.Join(root, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".vaultmind"), 0o750))
	deep := filepath.Join(project, "sub", "deep")
	require.NoError(t, os.MkdirAll(deep, 0o750))

	// ceiling=root keeps the walk hermetic (won't escape to /tmp etc.).
	assert.Equal(t, project, walkUpForVault(deep, root), "walks up to the dir containing .vaultmind/")
	assert.Equal(t, project, walkUpForVault(project, root), "matches at the start dir itself")
}

func TestWalkUpForVault_NoneFound(t *testing.T) {
	root := t.TempDir() // no .vaultmind anywhere under a fresh temp dir
	assert.Equal(t, "", walkUpForVault(root, root), "no .vaultmind up to the ceiling → empty")
}

func TestWalkUpForVault_StopsAtCeiling(t *testing.T) {
	root := t.TempDir()
	// .vaultmind exists ABOVE the ceiling — must NOT be matched.
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o750))
	mid := filepath.Join(root, "mid")
	deep := filepath.Join(mid, "deep")
	require.NoError(t, os.MkdirAll(deep, 0o750))
	assert.Equal(t, "", walkUpForVault(deep, mid), "ceiling bounds the walk; ancestor vault above it is ignored")
}

func TestDiscoverVaultPath_EnvWins(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o750))
	getenv := func(k string) string {
		if k == "VAULTMIND_VAULT" {
			return "/explicit/from/env"
		}
		return ""
	}
	// Env beats walk-up even though a .vaultmind exists at root.
	assert.Equal(t, "/explicit/from/env", discoverVaultPath(".", getenv, root, root))
}

func TestDiscoverVaultPath_WalkUpWhenNoEnv(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o750))
	assert.Equal(t, root, discoverVaultPath(".", noEnv, root, root))
}

func TestDiscoverVaultPath_FallbackWhenNothing(t *testing.T) {
	root := t.TempDir() // no .vaultmind, no env; ceiling=root keeps it hermetic
	assert.Equal(t, ".", discoverVaultPath(".", noEnv, root, root))
}

func TestDiscoverVaultPath_EmptyEnvIgnored(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o750))
	whitespaceEnv := func(string) string { return "   " }
	assert.Equal(t, root, discoverVaultPath(".", whitespaceEnv, root, root), "blank env is ignored; walk-up used")
}
