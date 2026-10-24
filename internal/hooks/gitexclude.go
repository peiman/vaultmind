package hooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/git"
)

const (
	gitExcludeStart = "# vaultmind hooks install --local (do not edit inside this block)"
	gitExcludeEnd   = "# end vaultmind"
)

// GitExcludeResult reports checkout-local exclusions. Added lists the literal
// patterns written (or proposed under DryRun). Exclusion failures are advisory;
// SkippedReason explains skipped changes or blocks retained for other worktrees.
type GitExcludeResult struct {
	Path          string   `json:"path,omitempty"`
	Added         []string `json:"added"`
	Removed       []string `json:"removed,omitempty"`
	DryRun        bool     `json:"dry_run,omitempty"`
	SkippedReason string   `json:"skipped_reason,omitempty"`
}

// localGitExclude owns only marked blocks for this project's two paths. In a
// shared exclude file, another subproject's block belongs to that subproject.
// Git discovery and filesystem failures must never fail hook provisioning.
func localGitExclude(projectDir string, remove, dryRun bool) *GitExcludeResult {
	res := &GitExcludeResult{Added: []string{}, DryRun: dryRun}
	path, entries, err := localGitExcludePaths(projectDir)
	res.Path = path
	if err != nil {
		res.SkippedReason = err.Error()
		return res
	}
	existing, err := readSettingsFile(path)
	if err != nil {
		res.SkippedReason = err.Error()
		return res
	}
	updated, changed := editGitExclude(string(existing), entries, remove)
	if len(changed) == 0 {
		return res
	}
	if remove {
		res.SkippedReason = otherWorktreeNeedsExclude(projectDir)
		if res.SkippedReason != "" {
			return res
		}
	}
	if !dryRun {
		perm := os.FileMode(0o600)
		if info, statErr := os.Stat(path); statErr == nil {
			perm = info.Mode().Perm()
		}
		if err = os.Mkdir(filepath.Dir(path), 0o750); err == nil || errors.Is(err, os.ErrExist) {
			err = atomicWriteFile(path, []byte(updated), perm)
		}
		if err != nil {
			res.SkippedReason = fmt.Sprintf("updating %s: %v", path, err)
			return res
		}
	}
	if remove {
		res.Removed = changed
	} else {
		res.Added = changed
	}
	return res
}

func localGitExcludePaths(projectDir string) (string, []string, error) {
	project, err := filepath.Abs(projectDir)
	if err != nil {
		return "", nil, err
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return "", nil, err
	}
	root, err := localGitRevParse(project, "--show-toplevel")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", nil, fmt.Errorf("git not found on PATH: %w", err)
		}
		return "", nil, fmt.Errorf("not a git work tree or git discovery failed: %w", err)
	}
	path, err := localGitRevParse(project, "--git-path", "info/exclude")
	if err != nil {
		return "", nil, fmt.Errorf("resolving git info/exclude: %w", err)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(project, path)
	}
	path, err = checkedGitExcludePath(project, path)
	if err != nil {
		return path, nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return path, nil, err
	}
	rel, err := filepath.Rel(root, project)
	if err != nil {
		return path, nil, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.ContainsAny(rel, "\r\n") {
		return path, nil, fmt.Errorf("project path cannot be represented in git exclude: %q", rel)
	}
	prefix := ""
	if rel != "." {
		// Treat project directory names literally, including gitignore globs.
		escape := strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`, "]", `\]`, " ", `\ `)
		prefix = "/" + escape.Replace(filepath.ToSlash(rel))
	}
	return path, []string{prefix + "/.claude/scripts/", prefix + "/.claude/vaultmind-profile"}, nil
}

// checkedGitExcludePath permits only info/exclude inside an existing common
// git directory. Resolve the parent before checking containment so an info
// symlink cannot redirect a write outside the repository. A missing info
// directory is allowed; its existing parent must be the common directory.
func checkedGitExcludePath(project, path string) (string, error) {
	common, err := localGitRevParse(project, "--git-common-dir")
	if err != nil {
		return path, fmt.Errorf("resolving git common directory: %w", err)
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(project, common)
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return path, err
	}
	info, err := os.Stat(common)
	if err != nil || !info.IsDir() {
		return path, fmt.Errorf("git common directory does not exist: %s", common)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if errors.Is(err, os.ErrNotExist) {
		parent = filepath.Join(common, "info")
		if filepath.Clean(filepath.Dir(path)) != parent {
			return path, fmt.Errorf("exclude path outside git common directory: %s", path)
		}
	} else if err != nil {
		return path, err
	}
	path = filepath.Join(parent, filepath.Base(path))
	// Resolve an existing file too: atomic rename must update a symlink's
	// target, never replace the link. A dangling symlink cannot be resolved
	// safely and is skipped; only an absent regular path may be created.
	if _, statErr := os.Lstat(path); statErr == nil {
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return path, err
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return path, statErr
	}
	rel, err := filepath.Rel(common, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path, fmt.Errorf("exclude path outside git common directory: %s", path)
	}
	return path, nil
}

// Discover the requested project without inheriting another repository's
// location. Keep discovery controls such as GIT_CEILING_DIRECTORIES.
func localGitRevParse(project string, args ...string) (string, error) {
	return localGitOutput(project, append([]string{"rev-parse"}, args...)...)
}

func localGitOutput(project string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: private caller supplies only fixed git commands and flags; project sets cwd, never an argument.
	cmd.Dir = project
	cmd.Env = git.WithoutLocation(os.Environ())
	out, err := cmd.Output()
	return strings.TrimSuffix(string(out), "\n"), err
}

type gitExcludeBlock struct {
	start, end, closing int
	entries             []string
}

// Parse offsets, retaining the exact bytes of every line outside our blocks.
func gitExcludeBlocks(text string) []gitExcludeBlock {
	var blocks []gitExcludeBlock
	var current *gitExcludeBlock
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		value := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		switch {
		case value == gitExcludeStart:
			current = &gitExcludeBlock{start: offset}
		case value == gitExcludeEnd && current != nil:
			current.closing, current.end = offset, offset+len(line)
			blocks = append(blocks, *current)
			current = nil
		case current != nil:
			current.entries = append(current.entries, value)
		}
		offset += len(line)
	}
	return blocks
}

func editGitExclude(text string, entries []string, remove bool) (string, []string) {
	blocks := gitExcludeBlocks(text)
	if remove {
		var removed []string
		// Work backwards so offsets remain valid after each removal.
		for i := len(blocks) - 1; i >= 0; i-- {
			block := blocks[i]
			if !ownsGitExcludeBlock(block, entries) {
				continue
			}
			removed = append(removed, block.entries...)
			text = text[:block.start] + text[block.end:]
		}
		return text, removed
	}
	listed := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var added []string
	for _, entry := range entries {
		if !slices.Contains(listed, entry) {
			added = append(added, entry)
		}
	}
	if len(added) == 0 {
		return text, nil
	}
	lines := strings.Join(added, "\n") + "\n"
	for _, block := range blocks {
		if ownsGitExcludeBlock(block, entries) {
			return text[:block.closing] + lines + text[block.closing:], added
		}
	}
	// Prepend: appending to a file without a final newline needs a separator
	// outside the block, which uninstall could not safely identify and remove.
	return gitExcludeStart + "\n" + lines + gitExcludeEnd + "\n" + text, added
}

func ownsGitExcludeBlock(block gitExcludeBlock, entries []string) bool {
	for _, entry := range entries {
		if slices.Contains(block.entries, entry) {
			return true
		}
	}
	return false
}
