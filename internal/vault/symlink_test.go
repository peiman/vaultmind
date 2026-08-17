package vault_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entryFor returns the fs.DirEntry a walk would hand back for name — Lstat
// semantics, so a symlink describes itself rather than its target.
func entryFor(t *testing.T, dir, name string) fs.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.Name() == name {
			return e
		}
	}
	t.Fatalf("no entry %q in %s", name, dir)
	return nil
}

func TestSkipSymlink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(dir, "real.md"), filepath.Join(dir, "link.md")))

	t.Run("a regular file is not skipped", func(t *testing.T) {
		rel, skip := vault.SkipSymlink(dir, filepath.Join(dir, "real.md"), entryFor(t, dir, "real.md"))
		assert.False(t, skip)
		assert.Empty(t, rel, "nothing to report when nothing was skipped")
	})

	t.Run("a symlink is skipped and named relative to the root", func(t *testing.T) {
		rel, skip := vault.SkipSymlink(dir, filepath.Join(dir, "link.md"), entryFor(t, dir, "link.md"))
		assert.True(t, skip)
		assert.Equal(t, "link.md", rel)
	})

	// filepath.Rel fails when the two paths cannot be expressed relative to one
	// another. Reporting the absolute path is uglier and still names the file,
	// which is the entire job — returning "" would put the caller back where it
	// started, reporting that something was skipped without saying what.
	t.Run("an unrelatable root still names the file", func(t *testing.T) {
		abs := filepath.Join(dir, "link.md")
		rel, skip := vault.SkipSymlink("relative/root", abs, entryFor(t, dir, "link.md"))
		assert.True(t, skip)
		assert.Equal(t, abs, rel)
	})
}
