package vault

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Excluded answers "would Scan read this note?" with Scan's own rules, so a
// writer (import) can refuse to write a note the index would never see.
func TestExcluded_MatchesScansRules(t *testing.T) {
	ex := []string{"README.md", "episodes", "archive/old", "drafts/skip.md"}
	for rel, want := range map[string]bool{
		"README.md":                   true,  // a file by name, at the root
		"docs/README.md":              true,  // and at any depth
		"docs/readme.md":              false, // the name is exact
		"episodes/e.md":               true,  // a folder by name
		"a/episodes/b/e.md":           true,  // at any depth
		"archive/old/n.md":            true,  // a folder by path prefix
		"archive/older/n.md":          false, // a prefix is a whole segment
		"drafts/skip.md":              true,  // a file by exact path
		"other/drafts/skip.md":        false, // exact means exact
		"imported/repo/docs/guide.md": false,
	} {
		assert.Equal(t, want, Excluded(rel, ex), rel)
	}
}
