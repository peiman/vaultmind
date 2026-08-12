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
	assert.Equal(t, project, walkUpForVault(deep, root, ""), "walks up to the dir containing .vaultmind/")
	assert.Equal(t, project, walkUpForVault(project, root, ""), "matches at the start dir itself")
}

func TestWalkUpForVault_NoneFound(t *testing.T) {
	root := t.TempDir() // no .vaultmind anywhere under a fresh temp dir
	assert.Equal(t, "", walkUpForVault(root, root, ""), "no .vaultmind up to the ceiling → empty")
}

func TestWalkUpForVault_StopsAtCeiling(t *testing.T) {
	root := t.TempDir()
	// .vaultmind exists ABOVE the ceiling — must NOT be matched.
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o750))
	mid := filepath.Join(root, "mid")
	deep := filepath.Join(mid, "deep")
	require.NoError(t, os.MkdirAll(deep, 0o750))
	assert.Equal(t, "", walkUpForVault(deep, mid, ""), "ceiling bounds the walk; ancestor vault above it is ignored")
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
	assert.Equal(t, "/explicit/from/env", discoverVaultPath(".", getenv, root, root, ""))
}

func TestDiscoverVaultPath_WalkUpWhenNoEnv(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o750))
	assert.Equal(t, root, discoverVaultPath(".", noEnv, root, root, ""))
}

func TestDiscoverVaultPath_FallbackWhenNothing(t *testing.T) {
	root := t.TempDir() // no .vaultmind, no env; ceiling=root keeps it hermetic
	assert.Equal(t, ".", discoverVaultPath(".", noEnv, root, root, ""))
}

func TestDiscoverVaultPath_EmptyEnvIgnored(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o750))
	whitespaceEnv := func(string) string { return "   " }
	assert.Equal(t, root, discoverVaultPath(".", whitespaceEnv, root, root, ""), "blank env is ignored; walk-up used")
}

// --- The home-directory guard -----------------------------------------------
//
// A .vaultmind/ at $HOME is nearly always an accident — one errant `vaultmind
// index` run from the home directory leaves the marker behind. Because the
// walk-up ran all the way to the filesystem root, that stray marker silently
// captured EVERY invocation made anywhere under $HOME that wasn't already
// inside another vault: `ask` answered from the wrong vault, and the remedy it
// printed ("run vaultmind index --embed") aimed the indexer at the entire home
// directory. Observed live 2026-08-12 — vault_path resolved to /Users/peiman
// while the envelope reported status "ok".
//
// So $HOME is never auto-selected. It stays reachable, but only when named
// deliberately (--vault or VAULTMIND_VAULT): a vault that large should be a
// choice, never a default.

func TestWalkUpForVault_SkipsHomeDirectory(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".vaultmind"), 0o750))
	work := filepath.Join(home, "dev", "someproject")
	require.NoError(t, os.MkdirAll(work, 0o750))

	assert.Equal(t, "", walkUpForVault(work, home, home),
		"a stray .vaultmind at $HOME must not be auto-selected")
}

func TestWalkUpForVault_SkipsHomeEvenWhenStartDirIsHome(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".vaultmind"), 0o750))

	assert.Equal(t, "", walkUpForVault(home, home, home),
		"standing in $HOME does not make $HOME the discovered vault")
}

func TestWalkUpForVault_HomeGuardDoesNotBlockVaultsBelowHome(t *testing.T) {
	home := t.TempDir()
	// The realistic layout: a stray vault at $HOME AND a genuine one below it.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".vaultmind"), 0o750))
	project := filepath.Join(home, "dev", "myproject")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".vaultmind"), 0o750))
	deep := filepath.Join(project, "sub", "deep")
	require.NoError(t, os.MkdirAll(deep, 0o750))

	assert.Equal(t, project, walkUpForVault(deep, home, home),
		"the nearest real vault below $HOME still wins; the guard skips only $HOME itself")
}

func TestWalkUpForVault_EmptyHomeDisablesTheGuard(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o750))

	assert.Equal(t, root, walkUpForVault(root, root, ""),
		`home="" means no home is known; the guard must not then fire on every dir`)
}

func TestWalkUpForVault_HomeGuardIgnoresTrailingSeparator(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".vaultmind"), 0o750))

	// os.UserHomeDir() is normally clean, but HOME=/Users/peiman/ is legal and
	// a raw string compare would miss it, silently restoring the old behaviour.
	assert.Equal(t, "", walkUpForVault(home, home, home+string(filepath.Separator)),
		"a trailing separator on $HOME must not defeat the guard")
}

func TestDiscoverVaultPath_HomeVaultStillReachableViaEnv(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".vaultmind"), 0o750))
	getenv := func(k string) string {
		if k == "VAULTMIND_VAULT" {
			return home
		}
		return ""
	}

	assert.Equal(t, home, discoverVaultPath(".", getenv, home, home, home),
		"the guard blocks only AUTO-selection; an explicit env override still reaches $HOME")
}

func TestDiscoverVaultPath_FallsBackWhenOnlyHomeHasAVault(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".vaultmind"), 0o750))
	work := filepath.Join(home, "dev", "someproject")
	require.NoError(t, os.MkdirAll(work, 0o750))

	assert.Equal(t, ".", discoverVaultPath(".", noEnv, work, home, home),
		"with $HOME skipped and nothing else found, the caller's fallback stands")
}
