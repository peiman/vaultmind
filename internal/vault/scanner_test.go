package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScan_FindsMarkdownFiles(t *testing.T) {
	files, err := vault.Scan("../../vaultmind-vault", testExcludes())
	require.NoError(t, err)

	assert.Greater(t, len(files), 30, "should find 30+ notes in test vault")

	for _, f := range files {
		assert.False(t, filepath.IsAbs(f.RelPath), "paths should be vault-relative")
		assert.Equal(t, ".md", filepath.Ext(f.RelPath))
	}
}

func TestScan_ExcludesPatterns(t *testing.T) {
	files, err := vault.Scan("../../vaultmind-vault", testExcludes())
	require.NoError(t, err)

	for _, f := range files {
		assert.NotContains(t, f.RelPath, ".obsidian")
		assert.NotContains(t, f.RelPath, ".vaultmind")
		assert.NotContains(t, f.RelPath, "templates")
	}
}

func TestScan_ReturnsFileInfo(t *testing.T) {
	files, err := vault.Scan("../../vaultmind-vault", testExcludes())
	require.NoError(t, err)
	require.NotEmpty(t, files)

	f := files[0]
	assert.NotEmpty(t, f.RelPath)
	assert.NotEmpty(t, f.AbsPath)
	assert.NotZero(t, f.ModTime)
}

func TestScan_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	files, err := vault.Scan(dir, testExcludes())
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestScan_NonExistentDirectory(t *testing.T) {
	_, err := vault.Scan("/nonexistent/path", testExcludes())
	assert.Error(t, err)
}

func TestScan_NestedDirectories(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "deep.md"), []byte("# Deep"), 0o644))

	files, err := vault.Scan(dir, nil)
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, filepath.Join("a", "b", "c", "deep.md"), files[0].RelPath)
}

func testExcludes() []string {
	return []string{".git", ".obsidian", ".trash", ".vaultmind", "templates"}
}
