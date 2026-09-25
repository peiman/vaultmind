package navigate

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Naming a repository by its origin remote. The folder a repository is cloned
// into differs between machines ("vaultmind-oss" here, "vaultmind" there), so
// a `repo:` prefix in `paths:` names the repository the way its remote does —
// the last path segment of the origin URL, without ".git". Without an origin,
// the folder name is all there is.

const (
	gitdirPrefix   = "gitdir:"
	worktreesDir   = "worktrees"
	gitConfigFile  = "config"
	originSection  = `[remote "origin"]`
	urlKey         = "url"
	gitURLSuffix   = ".git"
	sectionOpening = "["
)

// repoName names the repository rooted at root.
func repoName(root string) string {
	if cfg := gitConfigPath(root); cfg != "" {
		if name := repoFromURL(originURL(cfg)); name != "" {
			return name
		}
	}
	return filepath.Base(root)
}

// gitConfigPath finds the repository's config: .git/config in a clone; for a
// worktree (.git is a file pointing into <main>/.git/worktrees/<name>), the
// main repository's config, which holds the remotes.
func gitConfigPath(root string) string {
	marker := filepath.Join(root, gitMarker)
	info, err := os.Stat(marker)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return filepath.Join(marker, gitConfigFile)
	}
	// nosemgrep: go-path-traversal -- the .git file of the repository enclosing the file being read
	raw, err := os.ReadFile(marker) //nolint:gosec // same
	if err != nil {
		return ""
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), gitdirPrefix))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(root, gitdir)
	}
	if filepath.Base(filepath.Dir(gitdir)) == worktreesDir {
		gitdir = filepath.Dir(filepath.Dir(gitdir))
	}
	return filepath.Join(gitdir, gitConfigFile)
}

// originURL reads the url of [remote "origin"] from a git config file.
func originURL(cfg string) string {
	// nosemgrep: go-path-traversal -- that repository's own git config
	f, err := os.Open(cfg) //nolint:gosec // same
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	inOrigin := false
	for sc := bufio.NewScanner(f); sc.Scan(); {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, sectionOpening) {
			inOrigin = line == originSection
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok && inOrigin && strings.TrimSpace(key) == urlKey {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// repoFromURL is the last path segment of a remote URL, without ".git".
func repoFromURL(url string) string {
	url = strings.TrimSuffix(strings.TrimRight(url, "/"), gitURLSuffix)
	if i := strings.LastIndexAny(url, "/:"); i >= 0 {
		url = url[i+1:]
	}
	return url
}
