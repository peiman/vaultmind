package embedding_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The weights are ~2.2 GB. Relocating the cache must move them, never orphan
// them behind an empty new directory — a re-download is the one outcome worse
// than the ambiguity being fixed.
func TestDefaultCacheDir_MigratesLegacyWeightsInsteadOfRedownloading(t *testing.T) {
	// cmd/root.go sets this on every CLI run; a package test must mirror it.
	// With it unset DefaultCacheDir falls back to the legacy directory — the
	// safe direction, since that never orphans 2.2 GB of weights.
	xdg.SetAppName("vaultmind")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	legacy := filepath.Join(home, ".vaultmind", "models")
	require.NoError(t, os.MkdirAll(legacy, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "weights.onnx"), []byte("pretend 2.2GB"), 0o600))

	dir := embedding.DefaultCacheDir()

	assert.NotContains(t, dir, filepath.Join(".vaultmind", "models"),
		"the cache no longer lives behind the vault marker")
	assert.FileExists(t, filepath.Join(dir, "weights.onnx"), "existing weights came along")
	assert.NoDirExists(t, legacy, "and the legacy directory is gone, so $HOME stops looking like a vault")
}

// With no legacy cache, the XDG location is used directly — the ordinary case
// for a fresh install.
func TestDefaultCacheDir_UsesXDGWhenThereIsNothingToMigrate(t *testing.T) {
	xdg.SetAppName("vaultmind")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	dir := embedding.DefaultCacheDir()
	assert.NotContains(t, dir, filepath.Join(".vaultmind", "models"))
	assert.Contains(t, dir, "models")
}

// An already-migrated cache must not be touched again, and a legacy directory
// left behind (by an older binary re-downloading, which does happen) must not
// win over it.
func TestDefaultCacheDir_PrefersTheMigratedCacheOverALingeringLegacyOne(t *testing.T) {
	xdg.SetAppName("vaultmind")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	legacy := filepath.Join(home, ".vaultmind", "models")
	require.NoError(t, os.MkdirAll(legacy, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "stale.onnx"), []byte("older binary"), 0o600))

	first := embedding.DefaultCacheDir() // migrates
	require.NoError(t, os.MkdirAll(legacy, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "stale.onnx"), []byte("older binary again"), 0o600))

	second := embedding.DefaultCacheDir()
	assert.Equal(t, first, second, "the migrated cache stays the answer")
	assert.FileExists(t, filepath.Join(second, "stale.onnx"), "and still holds the migrated weights")
}

// An unisolated DefaultCacheDir renames ~/.vaultmind/models into the XDG
// cache. The guard has to panic before that rename, including on a machine
// where the XDG directory does not exist yet. A unique app name is the case
// that would move the real directory: the usual name is a no-op once its
// target already exists.
func TestDefaultCacheDir_PanicsInsteadOfMovingAnUnisolatedHome(t *testing.T) {
	const probe = "vm-cache-guard-probe"
	prev := xdg.GetAppName()
	xdg.SetAppName(probe)
	t.Cleanup(func() { xdg.SetAppName(prev) })

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	root, err := xdg.CacheDirPath()
	require.NoError(t, err)
	require.Contains(t, root, probe)
	_, statErr := os.Stat(root)
	if !os.IsNotExist(statErr) {
		t.Fatalf("probe cache %s already exists; refusing to run", root)
	}
	legacy := filepath.Join(home, ".vaultmind", "models")
	before, beforeErr := os.Stat(legacy)

	defer func() {
		r := recover()
		require.NotNil(t, r, "DefaultCacheDir returned instead of refusing to touch the process home")
		msg := fmt.Sprint(r)
		assert.Contains(t, msg, "HOME")
		assert.Contains(t, msg, "XDG_CACHE_HOME")
		_, afterRoot := os.Stat(root)
		assert.True(t, os.IsNotExist(afterRoot), "created %s before refusing", root)
		after, afterErr := os.Stat(legacy)
		if beforeErr != nil {
			assert.True(t, os.IsNotExist(afterErr), "legacy model cache appeared: %v", afterErr)
			return
		}
		require.NoError(t, afterErr, "legacy model cache was moved")
		assert.True(t, before.ModTime().Equal(after.ModTime()), "legacy model cache was touched")
	}()

	_ = embedding.DefaultCacheDir()
	t.Fatal("expected a panic")
}
