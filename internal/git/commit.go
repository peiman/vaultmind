package git

import (
	"fmt"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Committer handles git staging and committing via go-git.
type Committer struct{}

// CommitFiles stages the given paths and creates a commit.
// Returns the full hex-encoded commit SHA on success.
// Paths are relative to the repository root.
func (c *Committer) CommitFiles(repoPath string, paths []string, message string) (string, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("opening repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("getting worktree: %w", err)
	}

	for _, p := range paths {
		if _, err := wt.Add(p); err != nil {
			return "", fmt.Errorf("staging %q: %w", p, err)
		}
	}

	sig, err := authorSignature(repo)
	if err != nil {
		return "", err
	}

	hash, err := wt.Commit(message, &gogit.CommitOptions{
		Author: sig,
	})
	if err != nil {
		return "", fmt.Errorf("committing: %w", err)
	}

	return hash.String(), nil
}

// authorSignature reads the author from git config, falling back to defaults.
// repo.Config() returns the merged config with correct precedence (local > global > system).
func authorSignature(repo *gogit.Repository) (*object.Signature, error) {
	name, email := "VaultMind", "vaultmind@local"

	if cfg, err := repo.Config(); err == nil {
		if cfg.User.Name != "" {
			name = cfg.User.Name
		}
		if cfg.User.Email != "" {
			email = cfg.User.Email
		}
	}

	return &object.Signature{
		Name:  name,
		Email: email,
		When:  time.Now(),
	}, nil
}
