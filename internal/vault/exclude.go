package vault

import (
	"path/filepath"
	"strings"
)

// excludeSet is a vault's exclude list, matched the way Scan skips: a folder
// by its name or by a vault-relative path prefix ("archive/old"), a file by
// its name or its exact vault-relative path. One matcher, so a writer can ask
// "would the index read this?" and get Scan's answer, not a copy of it.
type excludeSet map[string]bool

func newExcludeSet(excludes []string) excludeSet {
	s := make(excludeSet, len(excludes))
	for _, e := range excludes {
		s[e] = true
	}
	return s
}

// dir reports whether the folder named name at vault-relative relDir is
// skipped with everything under it.
func (s excludeSet) dir(name, relDir string) bool {
	if s[name] {
		return true
	}
	for pattern := range s {
		if strings.Contains(pattern, string(filepath.Separator)) || strings.Contains(pattern, "/") {
			// Path-style pattern: match against relative path
			cleanPattern := filepath.Clean(pattern)
			if relDir == cleanPattern || strings.HasPrefix(relDir, cleanPattern+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

// file reports whether the file at vault-relative relPath is skipped: a
// basename match (e.g. "README.md" — vault meta, not a knowledge note) or an
// exact vault-relative path.
func (s excludeSet) file(relPath string) bool {
	return s[filepath.Base(relPath)] || s[relPath]
}

// Excluded reports whether Scan would pass over the note at the
// vault-relative relPath under excludes: the file itself is excluded, or a
// folder above it is.
func Excluded(relPath string, excludes []string) bool {
	s := newExcludeSet(excludes)
	relPath = filepath.Clean(relPath)
	for dir := filepath.Dir(relPath); dir != "."; dir = filepath.Dir(dir) {
		if s.dir(filepath.Base(dir), dir) {
			return true
		}
	}
	return s.file(relPath)
}
