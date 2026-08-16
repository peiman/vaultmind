package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScan_FindsMarkdownFiles(t *testing.T) {
	scan, err := vault.Scan(testvault.FixtureVault(), testExcludes())
	require.NoError(t, err)
	files := scan.Files

	assert.Greater(t, len(files), 0, "fixture vault should contain notes")

	for _, f := range files {
		assert.False(t, filepath.IsAbs(f.RelPath), "paths should be vault-relative")
		assert.Equal(t, ".md", filepath.Ext(f.RelPath))
	}
}

func TestScan_ExcludesPatterns(t *testing.T) {
	scan, err := vault.Scan(testvault.FixtureVault(), testExcludes())
	require.NoError(t, err)
	files := scan.Files

	for _, f := range files {
		assert.NotContains(t, f.RelPath, ".obsidian")
		assert.NotContains(t, f.RelPath, ".vaultmind")
		assert.NotContains(t, f.RelPath, "templates")
	}
}

func TestScan_ReturnsFileInfo(t *testing.T) {
	scan, err := vault.Scan(testvault.FixtureVault(), testExcludes())
	require.NoError(t, err)
	files := scan.Files
	require.NotEmpty(t, files)

	f := files[0]
	assert.NotEmpty(t, f.RelPath)
	assert.NotEmpty(t, f.AbsPath)
	assert.NotZero(t, f.ModTime)
}

func TestScan_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	scan, err := vault.Scan(dir, testExcludes())
	require.NoError(t, err)
	files := scan.Files
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

	scan, err := vault.Scan(dir, nil)
	require.NoError(t, err)
	files := scan.Files
	assert.Len(t, files, 1)
	assert.Equal(t, filepath.Join("a", "b", "c", "deep.md"), files[0].RelPath)
}

func TestScan_ExcludeByPath(t *testing.T) {
	dir := t.TempDir()

	// Create directory structure
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "concepts"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "archive", "old"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "concepts", "note.md"), []byte("content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "archive", "old", "note.md"), []byte("content"), 0o644))

	// Exclude "archive/old" by path
	scan, err := vault.Scan(dir, []string{"archive/old"})
	require.NoError(t, err)
	files := scan.Files

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.RelPath
	}

	assert.Contains(t, paths, filepath.Join("concepts", "note.md"))
	assert.NotContains(t, paths, filepath.Join("archive", "old", "note.md"),
		"archive/old should be excluded by path pattern")
}

func TestScan_ExcludeByName(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "concepts"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "templates"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "concepts", "note.md"), []byte("c"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "templates", "tmpl.md"), []byte("t"), 0o644))

	scan, err := vault.Scan(dir, []string{"templates"})
	require.NoError(t, err)
	files := scan.Files

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.RelPath
	}
	assert.Contains(t, paths, filepath.Join("concepts", "note.md"))
	assert.NotContains(t, paths, filepath.Join("templates", "tmpl.md"))
}

// Excludes apply to files by basename, not just directories — so "README.md"
// in the exclude list keeps a vault's own meta README out of the index instead
// of letting it pollute every query as a blank-titled hit.
func TestScan_ExcludesFileByName(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "concepts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Vault meta"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "concepts", "note.md"), []byte("c"), 0o644))

	scan, err := vault.Scan(dir, []string{"README.md"})
	require.NoError(t, err)
	files := scan.Files

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.RelPath
	}
	assert.Contains(t, paths, filepath.Join("concepts", "note.md"), "knowledge notes must still index")
	assert.NotContains(t, paths, "README.md", "a file basename in the exclude list must be skipped, not just dirs")
}

func testExcludes() []string {
	return []string{".git", ".obsidian", ".trash", ".vaultmind", "templates"}
}

// WalkDir does not descend into directory symlinks, but it hands back a FILE
// symlink named *.md, and os.ReadFile follows it. Before this guard, a note
// `secrets.md -> ~/.ssh/id_rsa` in a cloned vault was hashed, parsed, stored in
// FTS, embedded, returned by `ask`, and left sitting in index.db.
func TestScan_DoesNotFollowFileSymlinks(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "id_rsa")
	require.NoError(t, os.WriteFile(secret, []byte("PRIVATE KEY MATERIAL"), 0o600))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"), []byte("# Real"), 0o644))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, "secrets.md")))

	scan, err := vault.Scan(dir, nil)
	require.NoError(t, err)

	for _, f := range scan.Files {
		assert.NotEqual(t, "secrets.md", f.RelPath,
			"a *.md symlink was collected, and os.ReadFile follows it — untrusted vault content "+
				"becomes a read primitive, and the target lands in FTS and the embeddings")
	}
	assert.Len(t, scan.Files, 1, "the real note must still index")
	assert.Equal(t, []string{"secrets.md"}, scan.SkippedSymlinks,
		"a skipped note must be nameable; from the outside, skipped and missing look identical")
}

// The rule is the link, not the destination. Resolving first and confining
// after would need EvalSymlinks (a check-then-read window) and would let one
// file enter the index under two paths — the duplicate-id class H1 closed. The
// cost is that a legitimate in-vault symlink stops being indexed, which is
// exactly why it is reported by name rather than dropped.
func TestScan_SkipsSymlinkEvenWhenTargetIsInsideTheVault(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "notes"), 0o750))
	target := filepath.Join(dir, "notes", "real.md")
	require.NoError(t, os.WriteFile(target, []byte("# Real"), 0o644))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "alias.md")))

	scan, err := vault.Scan(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"alias.md"}, scan.SkippedSymlinks)
	require.Len(t, scan.Files, 1)
	assert.Equal(t, filepath.Join("notes", "real.md"), scan.Files[0].RelPath,
		"the real file must index exactly once, under its own path")
}
