package initvault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/initvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteConfigOnly_WritesRegistryWithoutScaffold(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.md"), []byte("# mine"), 0o600))

	written, err := initvault.WriteConfigOnly(dir)
	require.NoError(t, err)
	assert.True(t, written)
	assert.FileExists(t, filepath.Join(dir, ".vaultmind", "config.yaml"))

	// The point of this path: someone's existing notes folder must not gain a
	// README and starter notes, which would index as real notes.
	assert.NoFileExists(t, filepath.Join(dir, "README.md"))
	assert.NoDirExists(t, filepath.Join(dir, "arcs"))
	assert.FileExists(t, filepath.Join(dir, "existing.md"), "their files are untouched")
}

// Re-running must not overwrite a registry the user has since edited.
func TestWriteConfigOnly_IsANoOpWhenConfigExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	cfg := filepath.Join(dir, ".vaultmind", "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("types: {mine: {}}\n"), 0o600))

	written, err := initvault.WriteConfigOnly(dir)
	require.NoError(t, err)
	assert.False(t, written, "nothing written")

	body, err := os.ReadFile(cfg) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(body), "mine", "the user's registry survives")
}

// A write failure must surface. Silently proceeding would leave a directory
// that the guard then refuses on the next run, with no explanation of why.
func TestWriteConfigOnly_ReportsAnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	_, err := initvault.WriteConfigOnly(dir)
	require.Error(t, err)
}
