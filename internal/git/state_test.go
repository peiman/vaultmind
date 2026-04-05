package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initTestRepo creates a git repo with an initial commit in a temp dir.
func initTestRepo(t *testing.T) (string, *gogit.Repository) {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	// Configure user for commits
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test"
	cfg.User.Email = "test@test.com"
	err = repo.SetConfig(cfg)
	require.NoError(t, err)

	// Create initial commit
	wt, err := repo.Worktree()
	require.NoError(t, err)
	f := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(f, []byte("# Test"), 0o644))
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	_, err = wt.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)
	return dir, repo
}

func TestGoGitDetector_CleanRepo(t *testing.T) {
	dir, _ := initTestRepo(t)
	d := &GoGitDetector{}

	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.True(t, state.RepoDetected)
	assert.False(t, state.Detached)
	assert.False(t, state.MergeInProgress)
	assert.False(t, state.RebaseInProgress)
	assert.True(t, state.WorkingTreeClean)
	assert.Empty(t, state.StagedFiles)
	assert.Empty(t, state.UnstagedFiles)
	assert.Empty(t, state.UntrackedFiles)
}

func TestGoGitDetector_Branch(t *testing.T) {
	dir, _ := initTestRepo(t)
	d := &GoGitDetector{}

	state, err := d.Detect(dir)
	require.NoError(t, err)

	// go-git defaults to "master" for PlainInit
	assert.Contains(t, []string{"main", "master"}, state.Branch)
}

func TestGoGitDetector_UntrackedFile(t *testing.T) {
	dir, _ := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.md"), []byte("new"), 0o644))

	d := &GoGitDetector{}
	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.False(t, state.WorkingTreeClean)
	assert.Contains(t, state.UntrackedFiles, "new.md")
}

func TestGoGitDetector_UnstagedChanges(t *testing.T) {
	dir, _ := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified"), 0o644))

	d := &GoGitDetector{}
	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.False(t, state.WorkingTreeClean)
	assert.Contains(t, state.UnstagedFiles, "README.md")
}

func TestGoGitDetector_StagedChanges(t *testing.T) {
	dir, repo := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("staged change"), 0o644))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("README.md")
	require.NoError(t, err)

	d := &GoGitDetector{}
	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.False(t, state.WorkingTreeClean)
	assert.Contains(t, state.StagedFiles, "README.md")
}

func TestGoGitDetector_DetachedHead(t *testing.T) {
	dir, repo := initTestRepo(t)

	head, err := repo.Head()
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	err = wt.Checkout(&gogit.CheckoutOptions{Hash: head.Hash()})
	require.NoError(t, err)

	d := &GoGitDetector{}
	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.True(t, state.Detached)
}

func TestGoGitDetector_MergeInProgress(t *testing.T) {
	dir, _ := initTestRepo(t)
	// Simulate merge by creating MERGE_HEAD
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"),
		[]byte("0000000000000000000000000000000000000000\n"), 0o644))

	d := &GoGitDetector{}
	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.True(t, state.MergeInProgress)
}

func TestGoGitDetector_RebaseInProgress(t *testing.T) {
	dir, _ := initTestRepo(t)
	// Simulate rebase by creating rebase-merge directory
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "rebase-merge"), 0o755))

	d := &GoGitDetector{}
	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.True(t, state.RebaseInProgress)
}

func TestGoGitDetector_RebaseApply(t *testing.T) {
	dir, _ := initTestRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "rebase-apply"), 0o755))

	d := &GoGitDetector{}
	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.True(t, state.RebaseInProgress)
}

func TestGoGitDetector_NotARepo(t *testing.T) {
	dir := t.TempDir()
	d := &GoGitDetector{}

	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.False(t, state.RepoDetected)
}

func TestGoGitDetector_Subdirectory(t *testing.T) {
	dir, _ := initTestRepo(t)
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	d := &GoGitDetector{}
	state, err := d.Detect(sub)
	require.NoError(t, err)

	assert.True(t, state.RepoDetected)
}

func TestGoGitDetector_NewBranch(t *testing.T) {
	dir, repo := initTestRepo(t)

	head, err := repo.Head()
	require.NoError(t, err)
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), head.Hash())
	require.NoError(t, repo.Storer.SetReference(ref))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	err = wt.Checkout(&gogit.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature")})
	require.NoError(t, err)

	// Also set up tracking so go-git reports the branch correctly
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.Branches["feature"] = &gogitconfig.Branch{
		Name: "feature",
	}
	require.NoError(t, repo.SetConfig(cfg))

	d := &GoGitDetector{}
	state, err := d.Detect(dir)
	require.NoError(t, err)

	assert.Equal(t, "feature", state.Branch)
	assert.False(t, state.Detached)
}
