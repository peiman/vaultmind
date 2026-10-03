package importdocs

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// gitFilter is what git leaves out of the work tree an import reads. A
// repository's ignored files (generated output, scratch, deliberately kept
// out) were outnumbering its tracked docs up to four to one in a repo-root
// import (measured 2026-10-03).
type gitFilter struct {
	matcher gitignore.Matcher
	prefix  string          // the import root, relative to the work tree
	tracked map[string]bool // files git tracks, and every folder holding one
}

// newGitFilter returns the filter for root, or nil when root is not in a git
// work tree, the repository cannot be read, or root is itself ignored —
// naming an ignored folder is asking for it.
func newGitFilter(root string) *gitFilter {
	repo, err := gogit.PlainOpenWithOptions(root, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil
	}
	top, err := filepath.EvalSymlinks(wt.Filesystem.Root())
	if err != nil {
		return nil
	}
	prefix, err := filepath.Rel(top, root)
	if err != nil || strings.HasPrefix(prefix, "..") {
		return nil
	}
	prefix = filepath.ToSlash(prefix)
	if prefix == "." {
		prefix = ""
	}
	// Global excludes first, so the repository's own rules (negations
	// included) have the last word, as git applies them.
	patterns, _ := gitignore.LoadGlobalPatterns(osfs.New("/"))
	if len(patterns) == 0 {
		patterns = defaultGlobalPatterns()
	}
	repoPatterns, _ := gitignore.ReadPatterns(wt.Filesystem, nil)
	f := &gitFilter{matcher: gitignore.NewMatcher(append(patterns, repoPatterns...)), prefix: prefix, tracked: map[string]bool{}}
	if prefix != "" && f.matcher.Match(strings.Split(prefix, "/"), true) {
		return nil
	}
	if idx, err := repo.Storer.Index(); err == nil {
		for _, e := range idx.Entries {
			for p := e.Name; p != "." && p != ""; p = path.Dir(p) {
				f.tracked[p] = true
			}
		}
	}
	return f
}

// defaultGlobalPatterns reads the global ignore file where git looks when
// core.excludesfile is unset: $XDG_CONFIG_HOME/git/ignore, else
// ~/.config/git/ignore. go-git reads only core.excludesfile.
func defaultGlobalPatterns() []gitignore.Pattern {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(home, ".config")
	}
	// nosemgrep: go-path-traversal -- git's own config location for this user, not input
	raw, err := os.ReadFile(filepath.Join(dir, "git", "ignore")) //nolint:gosec // same
	if err != nil {
		return nil
	}
	var ps []gitignore.Pattern
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ps = append(ps, gitignore.ParsePattern(line, nil))
	}
	return ps
}

// ignored reports whether git leaves rel (relative to the import root) out.
// A tracked file is kept although a pattern matches it (force-added), and so
// is a folder that holds one.
func (f *gitFilter) ignored(rel string, isDir bool) bool {
	if f == nil {
		return false
	}
	full := path.Join(f.prefix, rel)
	if f.tracked[full] {
		return false
	}
	return f.matcher.Match(strings.Split(full, "/"), isDir)
}
