package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/git"
)

// GitStatusResult is the JSON response for the git status command.
type GitStatusResult struct {
	RepoDetected     bool     `json:"repo_detected"`
	Branch           string   `json:"branch"`
	Detached         bool     `json:"detached"`
	MergeInProgress  bool     `json:"merge_in_progress"`
	RebaseInProgress bool     `json:"rebase_in_progress"`
	WorkingTreeClean bool     `json:"working_tree_clean"`
	StagedFiles      []string `json:"staged_files"`
	UnstagedFiles    []string `json:"unstaged_files"`
	UntrackedFiles   []string `json:"untracked_files"`
}

// GitStatus detects git repository state and returns the result.
func GitStatus(detector git.RepoStateDetector, vaultPath string) (*GitStatusResult, error) {
	state, err := detector.Detect(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("detecting git state: %w", err)
	}

	result := &GitStatusResult{
		RepoDetected:     state.RepoDetected,
		Branch:           state.Branch,
		Detached:         state.Detached,
		MergeInProgress:  state.MergeInProgress,
		RebaseInProgress: state.RebaseInProgress,
		WorkingTreeClean: state.WorkingTreeClean,
		StagedFiles:      ensureSlice(state.StagedFiles),
		UnstagedFiles:    ensureSlice(state.UnstagedFiles),
		UntrackedFiles:   ensureSlice(state.UntrackedFiles),
	}

	return result, nil
}

// ensureSlice returns an empty slice if s is nil (for JSON [] instead of null).
func ensureSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
