package git

import (
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
)

// RepoStateDetector abstracts git state detection for dependency injection.
type RepoStateDetector interface {
	Detect(vaultPath string) (RepoState, error)
}

// GoGitDetector implements RepoStateDetector using go-git.
type GoGitDetector struct{}

// Detect reads the git repository state at vaultPath.
// Returns RepoState{RepoDetected: false} if no repo is found (not an error).
func (d *GoGitDetector) Detect(vaultPath string) (RepoState, error) {
	repo, openErr := gogit.PlainOpenWithOptions(vaultPath, &gogit.PlainOpenOptions{
		DetectDotGit: true,
	})
	if openErr != nil {
		// Not a git repo — callers treat absence as a valid state, not an error.
		return RepoState{RepoDetected: false}, nil //nolint:nilerr
	}

	state := RepoState{RepoDetected: true}

	// Branch and detached state
	head, headErr := repo.Head()
	if headErr != nil {
		// Empty repo or other issue — still detected, branch stays empty.
		state.Branch = ""
	} else if head.Name().IsBranch() {
		state.Branch = head.Name().Short()
	} else {
		state.Detached = true
		state.Branch = head.Hash().String()[:7]
	}

	// Merge/rebase detection via .git sentinel files
	dotGit, dotGitErr := dotGitPath(repo)
	if dotGitErr == nil {
		state.MergeInProgress = fileExists(filepath.Join(dotGit, "MERGE_HEAD"))
		state.RebaseInProgress = dirExists(filepath.Join(dotGit, "rebase-merge")) ||
			dirExists(filepath.Join(dotGit, "rebase-apply"))
	}

	// Working tree status
	wt, wtErr := repo.Worktree()
	if wtErr != nil {
		// Cannot read worktree — return partial state; worktree errors are non-fatal.
		return state, nil //nolint:nilerr
	}
	status, statusErr := wt.Status()
	if statusErr != nil {
		// Cannot read status — return partial state; status errors are non-fatal.
		return state, nil //nolint:nilerr
	}

	state.WorkingTreeClean = status.IsClean()
	for filePath, fileStatus := range status {
		if fileStatus.Staging == gogit.Untracked && fileStatus.Worktree == gogit.Untracked {
			state.UntrackedFiles = append(state.UntrackedFiles, filePath)
			continue
		}
		if fileStatus.Staging != gogit.Unmodified && fileStatus.Staging != gogit.Untracked {
			state.StagedFiles = append(state.StagedFiles, filePath)
		}
		if fileStatus.Worktree != gogit.Unmodified && fileStatus.Worktree != gogit.Untracked {
			state.UnstagedFiles = append(state.UnstagedFiles, filePath)
		}
	}

	return state, nil
}

// dotGitPath resolves the .git directory for a repository.
func dotGitPath(repo *gogit.Repository) (string, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	return filepath.Join(wt.Filesystem.Root(), ".git"), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
