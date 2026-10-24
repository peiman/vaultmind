package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// otherWorktreeNeedsExclude checks the same project-relative location in every
// other checkout: info/exclude belongs to the common git dir, not one worktree.
// Failure to inspect another checkout conservatively retains the shared block.
func otherWorktreeNeedsExclude(projectDir string) string {
	project, err := filepath.Abs(projectDir)
	if err != nil {
		return fmt.Sprintf("checking project path: %v", err)
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return fmt.Sprintf("checking other worktrees: %v", err)
	}
	root, err := localGitRevParse(project, "--show-toplevel")
	if err != nil {
		return fmt.Sprintf("checking worktree root: %v", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Sprintf("checking worktree root: %v", err)
	}
	rel, err := filepath.Rel(root, project)
	if err != nil {
		return fmt.Sprintf("checking project location: %v", err)
	}
	out, err := localGitOutput(project, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return fmt.Sprintf("checking other worktrees: %v", err)
	}
	for _, field := range strings.Split(out, "\x00") {
		wt, ok := strings.CutPrefix(field, "worktree ")
		if !ok {
			continue
		}
		if physical, evalErr := filepath.EvalSymlinks(wt); evalErr == nil {
			wt = physical
		}
		if wt == root {
			continue
		}
		for _, artifact := range []string{"scripts", "vaultmind-profile"} {
			path := filepath.Join(wt, rel, ".claude", artifact)
			if _, err := os.Stat(path); err == nil {
				return fmt.Sprintf("kept shared git exclude block: another worktree still has %s", path)
			} else if !os.IsNotExist(err) {
				return fmt.Sprintf("checking another worktree at %s: %v", path, err)
			}
		}
	}
	return ""
}
