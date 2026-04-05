package query_test

import (
	"bytes"
	"testing"

	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDetector implements git.RepoStateDetector for testing.
type fakeDetector struct {
	state git.RepoState
	err   error
}

func (f *fakeDetector) Detect(_ string) (git.RepoState, error) {
	return f.state, f.err
}

func TestGitStatus_CleanRepo(t *testing.T) {
	detector := &fakeDetector{
		state: git.RepoState{
			RepoDetected:     true,
			Branch:           "main",
			WorkingTreeClean: true,
		},
	}

	result, err := query.GitStatus(detector, "/vault")
	require.NoError(t, err)

	assert.True(t, result.RepoDetected)
	assert.Equal(t, "main", result.Branch)
	assert.False(t, result.Detached)
	assert.False(t, result.MergeInProgress)
	assert.False(t, result.RebaseInProgress)
	assert.True(t, result.WorkingTreeClean)
	assert.Empty(t, result.StagedFiles)
	assert.Empty(t, result.UnstagedFiles)
	assert.Empty(t, result.UntrackedFiles)
}

func TestGitStatus_DirtyRepo(t *testing.T) {
	detector := &fakeDetector{
		state: git.RepoState{
			RepoDetected:     true,
			Branch:           "feature",
			WorkingTreeClean: false,
			UnstagedFiles:    []string{"notes/a.md"},
			UntrackedFiles:   []string{"scratch/temp.md"},
		},
	}

	result, err := query.GitStatus(detector, "/vault")
	require.NoError(t, err)

	assert.False(t, result.WorkingTreeClean)
	assert.Equal(t, []string{"notes/a.md"}, result.UnstagedFiles)
	assert.Equal(t, []string{"scratch/temp.md"}, result.UntrackedFiles)
}

func TestGitStatus_NoRepo(t *testing.T) {
	detector := &fakeDetector{
		state: git.RepoState{RepoDetected: false},
	}

	result, err := query.GitStatus(detector, "/vault")
	require.NoError(t, err)

	assert.False(t, result.RepoDetected)
}

func TestGitStatus_DetectorError(t *testing.T) {
	detector := &fakeDetector{
		err: assert.AnError,
	}

	_, err := query.GitStatus(detector, "/vault")
	assert.Error(t, err)
}

func TestGitStatus_NilSlicesBecomEmptyArrays(t *testing.T) {
	detector := &fakeDetector{
		state: git.RepoState{
			RepoDetected:     true,
			Branch:           "main",
			WorkingTreeClean: true,
		},
	}

	result, err := query.GitStatus(detector, "/vault")
	require.NoError(t, err)

	// JSON serialization needs empty arrays, not null
	assert.NotNil(t, result.StagedFiles)
	assert.NotNil(t, result.UnstagedFiles)
	assert.NotNil(t, result.UntrackedFiles)
}

func TestFormatGitStatus_CleanRepo(t *testing.T) {
	result := &query.GitStatusResult{
		Branch:           "main",
		WorkingTreeClean: true,
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatGitStatus(result, &buf))
	out := buf.String()
	assert.Contains(t, out, "Branch:  main")
	assert.Contains(t, out, "Status:  clean")
	assert.Contains(t, out, "Merge:   none")
}

func TestFormatGitStatus_DirtyRepo(t *testing.T) {
	result := &query.GitStatusResult{
		Branch:           "feature",
		WorkingTreeClean: false,
		UnstagedFiles:    []string{"a.md", "b.md"},
		StagedFiles:      []string{"c.md"},
		UntrackedFiles:   []string{"d.md"},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatGitStatus(result, &buf))
	out := buf.String()
	assert.Contains(t, out, "dirty (2 unstaged, 1 staged, 1 untracked)")
}

func TestFormatGitStatus_MergeInProgress(t *testing.T) {
	result := &query.GitStatusResult{
		Branch:          "main",
		MergeInProgress: true,
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatGitStatus(result, &buf))
	assert.Contains(t, buf.String(), "Merge:   merge in progress")
}

func TestFormatGitStatus_RebaseInProgress(t *testing.T) {
	result := &query.GitStatusResult{
		Branch:           "main",
		RebaseInProgress: true,
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatGitStatus(result, &buf))
	assert.Contains(t, buf.String(), "Merge:   rebase in progress")
}
