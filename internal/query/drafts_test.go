package query_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListDraftNotes_ReportsIDPathAndAge(t *testing.T) {
	db := buildRetrieverTestDB(t)
	row, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row)

	vault := t.TempDir()
	rel := "concepts/quartz-draft.md"
	require.NoError(t, os.MkdirAll(filepath.Join(vault, "concepts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vault, rel), []byte("draft"), 0o644))
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(vault, rel), now.Add(-74*time.Hour), now.Add(-74*time.Hour)))
	_, err = db.Exec(`UPDATE notes SET status = ?, path = ? WHERE id = ?`, index.StatusDraft, rel, row.ID)
	require.NoError(t, err)

	got, err := query.ListDraftNotes(db, vault, now)
	require.NoError(t, err)
	require.Equal(t, 1, got.Count)
	require.Len(t, got.Notes, 1)
	assert.Equal(t, row.ID, got.Notes[0].ID)
	assert.Equal(t, rel, got.Notes[0].Path)
	assert.Equal(t, 3, got.Notes[0].AgeDays)
}

func TestCheckUncommittedNotes_SkipsAVaultThatIsNotAGitRepo(t *testing.T) {
	got, err := query.CheckUncommittedNotes(t.TempDir(), 7, time.Now())
	require.NoError(t, err)
	assert.True(t, got.Skipped)
	assert.Equal(t, 7, got.ThresholdDays)
	assert.Contains(t, got.Reason, "not a git repository")
	assert.Empty(t, got.Notes)
}

func TestCheckUncommittedNotes_FlagsOnlyOldUntrackedMarkdown(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	repo := initUncommittedRepo(t, dir, now)

	writeAged(t, filepath.Join(dir, "old.md"), now.Add(-8*24*time.Hour))
	writeAged(t, filepath.Join(dir, "fresh.md"), now.Add(-time.Hour))
	writeAged(t, filepath.Join(dir, "boundary.md"), now.Add(-7*24*time.Hour))
	writeAged(t, filepath.Join(dir, "old.txt"), now.Add(-30*24*time.Hour))
	writeAged(t, filepath.Join(dir, "committed.md"), now.Add(-30*24*time.Hour))

	edited := filepath.Join(dir, "edited.md")
	require.NoError(t, os.WriteFile(edited, []byte("changed body\n"), 0o644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("edited.md")
	require.NoError(t, err)
	_, err = wt.Commit("edit", &gogit.CommitOptions{Author: gitAuthor(now)})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(edited, []byte("changed body again\n"), 0o644))
	writeAged(t, edited, now.Add(-9*24*time.Hour))

	got, err := query.CheckUncommittedNotes(dir, 7, now)
	require.NoError(t, err)
	assert.False(t, got.Skipped)
	assert.Empty(t, got.Reason)
	assert.Equal(t, 7, got.ThresholdDays)

	byPath := map[string]query.DoctorUncommittedNote{}
	for _, n := range got.Notes {
		byPath[n.Path] = n
	}
	require.Contains(t, byPath, "old.md")
	assert.Equal(t, "untracked", byPath["old.md"].State)
	assert.Equal(t, 8, byPath["old.md"].AgeDays)
	require.Contains(t, byPath, "edited.md")
	assert.Equal(t, "modified", byPath["edited.md"].State)
	assert.Equal(t, 9, byPath["edited.md"].AgeDays)
	assert.NotContains(t, byPath, "fresh.md", "a file newer than the threshold stays quiet")
	assert.NotContains(t, byPath, "boundary.md", "exactly N days is still inside the window")
	assert.NotContains(t, byPath, "committed.md", "a committed file is not uncommitted, however old its mtime")
	assert.NotContains(t, byPath, "old.txt", "only markdown notes are checked")
	assert.Equal(t, len(byPath), got.Count)
}

func TestCheckUncommittedNotes_LimitsToAVaultInsideARepo(t *testing.T) {
	repo := t.TempDir()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	initUncommittedRepo(t, repo, now)
	vault := filepath.Join(repo, "vault")
	require.NoError(t, os.MkdirAll(vault, 0o755))
	writeAged(t, filepath.Join(vault, "old.md"), now.Add(-8*24*time.Hour))
	writeAged(t, filepath.Join(vault, "fresh.md"), now.Add(-time.Hour))
	writeAged(t, filepath.Join(repo, "outside.md"), now.Add(-30*24*time.Hour))

	got, err := query.CheckUncommittedNotes(vault, 7, now)
	require.NoError(t, err)
	assert.False(t, got.Skipped)
	byPath := map[string]string{}
	for _, n := range got.Notes {
		byPath[n.Path] = n.State
	}
	assert.Equal(t, "untracked", byPath["old.md"])
	assert.NotContains(t, byPath, "fresh.md")
	assert.NotContains(t, byPath, "outside.md")
	assert.NotContains(t, byPath, "vault/old.md", "the path is relative to the vault, not the repo root")
	assert.Equal(t, len(byPath), got.Count)
}

func TestCheckUncommittedNotes_FollowsAGitfile(t *testing.T) {
	parent := t.TempDir()
	vault := filepath.Join(parent, "vault")
	gitDir := filepath.Join(parent, "gitdir")
	require.NoError(t, os.MkdirAll(vault, 0o755))
	cmd := exec.Command("git", "init", "--separate-git-dir", gitDir, vault)
	// A commit hook exports GIT_DIR for this repository. Left set, git init
	// treats that directory as the target and refuses --separate-git-dir.
	cmd.Env = withoutGitLocation(os.Environ())
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	info, err := os.Lstat(filepath.Join(vault, ".git"))
	require.NoError(t, err)
	assert.False(t, info.IsDir(), ".git is a gitlink file, not a directory")

	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	writeAged(t, filepath.Join(vault, "old.md"), now.Add(-8*24*time.Hour))

	got, err := query.CheckUncommittedNotes(vault, 7, now)
	require.NoError(t, err)
	assert.False(t, got.Skipped, got.Reason)
	require.Len(t, got.Notes, 1)
	assert.Equal(t, "old.md", got.Notes[0].Path)
	assert.Equal(t, "untracked", got.Notes[0].State)
}

func initUncommittedRepo(t *testing.T, dir string, now time.Time) *gogit.Repository {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "committed.md"), []byte("# committed\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "edited.md"), []byte("# edited\n"), 0o644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("committed.md")
	require.NoError(t, err)
	_, err = wt.Add("edited.md")
	require.NoError(t, err)
	_, err = wt.Commit("init", &gogit.CommitOptions{Author: gitAuthor(now)})
	require.NoError(t, err)
	return repo
}

// withoutGitLocation drops the variables a surrounding git process exports.
// The check under test uses go-git. This only keeps the fixture's own git init
// from binding to the repository that launched the test.
func withoutGitLocation(env []string) []string {
	drop := map[string]bool{
		"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true,
		"GIT_COMMON_DIR": true, "GIT_PREFIX": true, "GIT_OBJECT_DIRECTORY": true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	}
	kept := make([]string, 0, len(env))
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if !drop[key] {
			kept = append(kept, e)
		}
	}
	return kept
}

func gitAuthor(now time.Time) *object.Signature {
	return &object.Signature{Name: "Test", Email: "test@example.com", When: now}
}

func writeAged(t *testing.T, path string, when time.Time) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		require.NoError(t, os.WriteFile(path, []byte("note\n"), 0o644))
	}
	require.NoError(t, os.Chtimes(path, when, when))
}
