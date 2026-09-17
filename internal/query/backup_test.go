package query_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commitAt writes a file and commits it with a fixed timestamp, so "days since
// the oldest unpushed commit" is measured rather than approximated.
func commitAt(t *testing.T, repo *gogit.Repository, dir, name string, when time.Time) plumbing.Hash {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(name)
	require.NoError(t, err)
	h, err := wt.Commit("add "+name, &gogit.CommitOptions{
		Author:    &object.Signature{Name: "t", Email: "t@example.com", When: when},
		Committer: &object.Signature{Name: "t", Email: "t@example.com", When: when},
	})
	require.NoError(t, err)
	return h
}

func initRepo(t *testing.T) (string, *gogit.Repository) {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	return dir, repo
}

// A vault outside git is REPORTED, not skipped. "Could not tell" and "fine"
// must not look the same — that equivalence is the whole reason this exists.
func TestCheckBackup_NotAGitRepoIsReported(t *testing.T) {
	got, err := query.CheckBackup(t.TempDir(), time.Now())
	require.NoError(t, err)

	assert.Equal(t, query.BackupStateNoRepo, got.State)
	assert.True(t, got.AtRisk())
	assert.Contains(t, got.Warnings, query.WarnBackupNoRepo)
}

// THE ONE THAT ACTUALLY HAPPENED.
//
// The vault had commits and a configured remote — just not one the branch
// tracked. Every "is there a remote?" check would have answered yes while 42
// commits sat on a single disk for 28 days. The question has to be about the
// UPSTREAM.
func TestCheckBackup_RemoteExistsButBranchTracksNothing(t *testing.T) {
	dir, repo := initRepo(t)
	head := commitAt(t, repo, dir, "a.md", time.Now().Add(-40*24*time.Hour))

	// A remote exists — an unrelated one, exactly as the real failure had.
	_, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "scaffold-upstream",
		URLs: []string{"https://example.com/someone-elses-repo.git"},
	})
	require.NoError(t, err)

	// And it has a RESOLVABLE tracking ref, so an implementation that falls
	// back to "any remote will do" would report this vault as backed up. The
	// first version of this test omitted that ref: the fallback then failed to
	// resolve and returned no_upstream for the wrong reason, so the mutation
	// survived. The fixture has to make the two implementations disagree.
	require.NoError(t, repo.Storer.SetReference(
		plumbing.NewHashReference("refs/remotes/scaffold-upstream/master", head)))

	got, err := query.CheckBackup(dir, time.Now())
	require.NoError(t, err)

	assert.Equal(t, query.BackupStateNoUpstream, got.State,
		"a remote that the branch does not track backs up nothing")
	assert.True(t, got.AtRisk())
	assert.Contains(t, got.Warnings, query.WarnBackupNoUpstream)
}

// Behind by long enough to matter: the count AND the age both surface.
func TestCheckBackup_BehindReportsCountAndAge(t *testing.T) {
	dir, repo := initRepo(t)
	now := time.Now()
	base := commitAt(t, repo, dir, "a.md", now.Add(-60*24*time.Hour))

	// Pretend the upstream stopped here, then keep working locally.
	require.NoError(t, repo.Storer.SetReference(
		plumbing.NewHashReference("refs/remotes/origin/master", base)))
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.Branches["master"] = &config.Branch{Name: "master", Remote: "origin", Merge: "refs/heads/master"}
	require.NoError(t, repo.SetConfig(cfg))

	commitAt(t, repo, dir, "b.md", now.Add(-28*24*time.Hour))
	commitAt(t, repo, dir, "c.md", now.Add(-1*24*time.Hour))

	got, err := query.CheckBackup(dir, now)
	require.NoError(t, err)

	assert.Equal(t, query.BackupStateBehind, got.State)
	assert.Equal(t, 2, got.Unpushed, "both local-only commits must be counted")
	assert.Equal(t, 28, got.OldestUnpushedDays,
		"age comes from the OLDEST unpushed commit — a count alone hides how much is at risk")
	assert.NotEmpty(t, got.Warnings)
	assert.True(t, got.AtRisk())
}

// Working state is quiet. A warning that fires every day is one nobody reads.
func TestCheckBackup_RecentUnpushedWorkDoesNotWarn(t *testing.T) {
	dir, repo := initRepo(t)
	now := time.Now()
	base := commitAt(t, repo, dir, "a.md", now.Add(-10*24*time.Hour))
	require.NoError(t, repo.Storer.SetReference(
		plumbing.NewHashReference("refs/remotes/origin/master", base)))
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.Branches["master"] = &config.Branch{Name: "master", Remote: "origin", Merge: "refs/heads/master"}
	require.NoError(t, repo.SetConfig(cfg))

	commitAt(t, repo, dir, "b.md", now.Add(-1*time.Hour))

	got, err := query.CheckBackup(dir, now)
	require.NoError(t, err)

	assert.Equal(t, query.BackupStateBehind, got.State)
	assert.Equal(t, 1, got.Unpushed)
	assert.Empty(t, got.Warnings,
		"an hour-old unpushed commit is normal working state, not a finding")
}

// Fully pushed is the only state that is NOT at risk.
func TestCheckBackup_CurrentIsTheOnlySafeState(t *testing.T) {
	dir, repo := initRepo(t)
	now := time.Now()
	h := commitAt(t, repo, dir, "a.md", now.Add(-5*24*time.Hour))
	require.NoError(t, repo.Storer.SetReference(
		plumbing.NewHashReference("refs/remotes/origin/master", h)))
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.Branches["master"] = &config.Branch{Name: "master", Remote: "origin", Merge: "refs/heads/master"}
	require.NoError(t, repo.SetConfig(cfg))

	got, err := query.CheckBackup(dir, now)
	require.NoError(t, err)

	assert.Equal(t, query.BackupStateCurrent, got.State)
	assert.Zero(t, got.Unpushed)
	assert.Empty(t, got.Warnings)
	assert.False(t, got.AtRisk(), "only a pushed vault is safe")
}

// A branch configured to track something never fetched reports as unbacked —
// the config says yes and the evidence says nothing has gone anywhere.
func TestCheckBackup_ConfiguredButNeverFetchedIsNotBackedUp(t *testing.T) {
	dir, repo := initRepo(t)
	commitAt(t, repo, dir, "a.md", time.Now().Add(-30*24*time.Hour))
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.Branches["master"] = &config.Branch{Name: "master", Remote: "origin", Merge: "refs/heads/master"}
	require.NoError(t, repo.SetConfig(cfg))

	got, err := query.CheckBackup(dir, time.Now())
	require.NoError(t, err)

	assert.Equal(t, query.BackupStateNoUpstream, got.State)
	assert.True(t, got.AtRisk())
}
