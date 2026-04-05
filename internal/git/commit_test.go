package git

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitter_CommitFiles(t *testing.T) {
	dir, _ := initTestRepo(t)

	// Create a new file to commit
	newFile := filepath.Join(dir, "notes", "test.md")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "notes"), 0o755))
	require.NoError(t, os.WriteFile(newFile, []byte("# Test Note"), 0o644))

	c := &Committer{}
	sha, err := c.CommitFiles(dir, []string{"notes/test.md"}, "vaultmind: test commit")
	require.NoError(t, err)
	assert.NotEmpty(t, sha)
	assert.Len(t, sha, 40) // full hex SHA

	// Verify commit exists in log
	repo, err := gogit.PlainOpen(dir)
	require.NoError(t, err)
	head, err := repo.Head()
	require.NoError(t, err)
	commit, err := repo.CommitObject(head.Hash())
	require.NoError(t, err)
	assert.Equal(t, "vaultmind: test commit", commit.Message)
}

func TestCommitter_CommitFiles_OnlyStagesSpecifiedFiles(t *testing.T) {
	dir, _ := initTestRepo(t)

	// Create two files, only commit one
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("b"), 0o644))

	c := &Committer{}
	_, err := c.CommitFiles(dir, []string{"a.md"}, "commit only a")
	require.NoError(t, err)

	// b.md should still be untracked
	repo, err := gogit.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	status, err := wt.Status()
	require.NoError(t, err)

	assert.Equal(t, gogit.Untracked, status.File("b.md").Worktree)
}

func TestCommitter_CommitFiles_ModifiedFile(t *testing.T) {
	dir, _ := initTestRepo(t)

	// Modify existing file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("updated"), 0o644))

	c := &Committer{}
	sha, err := c.CommitFiles(dir, []string{"README.md"}, "update readme")
	require.NoError(t, err)
	assert.NotEmpty(t, sha)

	// Verify clean after commit
	repo, err := gogit.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	status, err := wt.Status()
	require.NoError(t, err)
	assert.True(t, status.IsClean())
}

func TestCommitter_CommitFiles_NotARepo(t *testing.T) {
	dir := t.TempDir()
	c := &Committer{}
	_, err := c.CommitFiles(dir, []string{"file.md"}, "msg")
	assert.Error(t, err)
}

func TestCommitter_CommitFiles_MultipleFiles(t *testing.T) {
	dir, _ := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("b"), 0o644))

	c := &Committer{}
	sha, err := c.CommitFiles(dir, []string{"a.md", "b.md"}, "commit both")
	require.NoError(t, err)
	assert.NotEmpty(t, sha)

	// Verify both are committed
	repo, err := gogit.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	status, err := wt.Status()
	require.NoError(t, err)
	assert.True(t, status.IsClean())
}
