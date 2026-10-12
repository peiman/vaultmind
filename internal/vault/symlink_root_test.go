package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// filepath.WalkDir lstats its root. A root that is a symlink to the vault is
// not a directory, so the walk returns that one entry and stops. Scan then
// reports no notes.
func TestScan_FollowsASymlinkedVaultRoot(t *testing.T) {
	real := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(real, "note.md"), []byte("# Note"), 0o644))
	link := filepath.Join(t.TempDir(), "current")
	require.NoError(t, os.Symlink(real, link))

	scan, err := vault.Scan(link, nil)
	require.NoError(t, err)
	require.Len(t, scan.Files, 1, "a symlinked vault root must be the directory it names")
	assert.Equal(t, "note.md", scan.Files[0].RelPath)
}

// The reported pointer is a relative link (vaultmind-identity -> workhorse-vault),
// not an absolute one. EvalSymlinks has to resolve it from the link's directory.
func TestScan_FollowsARelativeSymlinkRoot(t *testing.T) {
	parent := t.TempDir()
	real := filepath.Join(parent, "vault")
	require.NoError(t, os.Mkdir(real, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(real, "note.md"), []byte("# Note"), 0o644))
	link := filepath.Join(parent, "vaultmind-identity")
	require.NoError(t, os.Symlink("vault", link))

	scan, err := vault.Scan(link, nil)
	require.NoError(t, err)
	require.Len(t, scan.Files, 1)
	assert.Equal(t, "note.md", scan.Files[0].RelPath)
}

// Resolving the root is the operator's path, not permission to follow links
// the vault contains. secrets.md -> outside must still be skipped.
func TestScan_SymlinkedRootDoesNotFollowNotesInside(t *testing.T) {
	real := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(real, "real.md"), []byte("# Real"), 0o644))
	outside := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("SECRET"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(real, "secrets.md")))
	link := filepath.Join(t.TempDir(), "current")
	require.NoError(t, os.Symlink(real, link))

	scan, err := vault.Scan(link, nil)
	require.NoError(t, err)
	require.Len(t, scan.Files, 1)
	assert.Equal(t, "real.md", scan.Files[0].RelPath)
	assert.Equal(t, []string{"secrets.md"}, scan.SkippedSymlinks)
}

// Confinement follows the same root the walk uses. A path inside the vault
// the link names is inside; a path that leaves that directory is not.
func TestResolveInside_SymlinkedRootIsTheRealDirectory(t *testing.T) {
	realParent := t.TempDir()
	real := filepath.Join(realParent, "vault")
	require.NoError(t, os.Mkdir(real, 0o750))
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))

	got, err := vault.ResolveInside(link, filepath.Join("notes", "a.md"))
	require.NoError(t, err)
	resolved, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(resolved, "notes", "a.md"), got)

	_, err = vault.ResolveInside(link, filepath.Join("..", "outside.md"))
	require.ErrorIs(t, err, vault.ErrEscapesVault)
}

// A real directory stays the path Abs produced. Rewriting every vault through
// EvalSymlinks would turn /var into /private/var and move every confined path.
//
// The ancestor symlink is built here. A Linux CI temp dir has no /var ->
// /private/var above it, so comparing Abs with ResolveRoot on t.TempDir()
// would pass an implementation that always calls EvalSymlinks.
func TestResolveRoot_RealDirectoryIsNotRewritten(t *testing.T) {
	real := t.TempDir()
	ancestor := filepath.Join(t.TempDir(), "ancestor")
	require.NoError(t, os.Symlink(real, ancestor))
	dir := filepath.Join(ancestor, "vault")
	require.NoError(t, os.Mkdir(dir, 0o750))

	got, err := vault.ResolveRoot(dir)
	require.NoError(t, err)
	abs, err := filepath.Abs(dir)
	require.NoError(t, err)
	assert.Equal(t, abs, got)

	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.NotEqual(t, resolved, got, "an ancestor symlink must stay in the path Abs produced")
}

// /link/vault, where /link is the symlink and vault is a real directory.
// The final element is not a link, so the path is not rewritten, and the
// walk still finds the note.
func TestScan_SymlinkedParentIsWalked(t *testing.T) {
	realParent := t.TempDir()
	real := filepath.Join(realParent, "vault")
	require.NoError(t, os.Mkdir(real, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(real, "note.md"), []byte("# Note"), 0o644))
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(realParent, link))
	via := filepath.Join(link, "vault")

	scan, err := vault.Scan(via, nil)
	require.NoError(t, err)
	require.Len(t, scan.Files, 1)
	assert.Equal(t, "note.md", scan.Files[0].RelPath)

	got, err := vault.ResolveRoot(via)
	require.NoError(t, err)
	abs, err := filepath.Abs(via)
	require.NoError(t, err)
	assert.Equal(t, abs, got)
	resolved, err := filepath.EvalSymlinks(via)
	require.NoError(t, err)
	assert.NotEqual(t, resolved, got)
}

// A directory symlink inside the vault is not a root. WalkDir does not
// follow it, so a folder of secrets on the other side is not indexed, and
// a symlink loop finishes.
func TestScan_DirectorySymlinkIsNotFollowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"), []byte("# Real"), 0o644))
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.md"), []byte("SECRET"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "linked")))
	require.NoError(t, os.Symlink(".", filepath.Join(dir, "loop")))

	scan, err := vault.Scan(dir, nil)
	require.NoError(t, err)
	require.Len(t, scan.Files, 1)
	assert.Equal(t, "real.md", scan.Files[0].RelPath)
	assert.GreaterOrEqual(t, scan.Entries, 3, "the note and both directory symlinks are visits; the walk does not enter them")
	assert.False(t, scan.SawNothing())
	for _, f := range scan.Files {
		assert.NotContains(t, f.RelPath, "secret")
	}
}

// Entries counts visits, not indexable notes. README and .trash are default
// excludes. The note inside .trash is not visited. An empty directory visits
// nothing under the root.
func TestScan_EntriesCountVisitsNotIndexableFiles(t *testing.T) {
	dir := t.TempDir()
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)

	scan, err := vault.Scan(dir, cfg.Vault.Exclude)
	require.NoError(t, err)
	assert.Equal(t, 0, scan.Entries)
	assert.True(t, scan.Walkable)
	assert.True(t, scan.SawNothing())
	root, err := filepath.Abs(dir)
	require.NoError(t, err)
	assert.Equal(t, root, scan.Root)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Vault\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".trash"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".trash", "gone.md"), []byte("# gone\n"), 0o644))

	scan, err = vault.Scan(dir, cfg.Vault.Exclude)
	require.NoError(t, err)
	assert.Empty(t, scan.Files)
	assert.Equal(t, 2, scan.Entries, "README and .trash count; the note inside .trash is not visited")
	assert.False(t, scan.SawNothing())
}

func TestResolveRoot_SymlinkResolvesToTheTarget(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "current")
	require.NoError(t, os.Symlink(real, link))

	got, err := vault.ResolveRoot(link)
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
