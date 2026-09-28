package importdocs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The case-matching rules of #189, tested on the syncer's own state, so they
// run on every filesystem. The end-to-end tests in importdocs_test.go need a
// case-sensitive one to create two notes differing only in case, and skip
// elsewhere.
func syncerWith(paths ...string) *syncer {
	all := map[string]note{}
	for _, p := range paths {
		all[p] = note{exists: true, managed: true}
	}
	return newSyncer(Source{Repo: "r"}, "", "imported/r/docs", all, Options{}, writer{})
}

func TestCaseFold_AnExactMatchAlwaysWins(t *testing.T) {
	s := syncerWith("imported/r/docs/Gamma.md", "imported/r/docs/GAMMA.md")
	assert.Equal(t, "imported/r/docs/GAMMA.md", s.existing("imported/r/docs/GAMMA.md"))
	assert.False(t, s.ambiguous("imported/r/docs/GAMMA.md"))
}

func TestCaseFold_ASingleVariantIsTheSameNote(t *testing.T) {
	s := syncerWith("imported/r/docs/Gamma.md")
	assert.Equal(t, "imported/r/docs/Gamma.md", s.existing("imported/r/docs/gamma.md"), "a doc renamed only in case keeps its note")
	assert.False(t, s.ambiguous("imported/r/docs/gamma.md"))
}

func TestCaseFold_SeveralVariantsAreAmbiguousAndNoneIsUsed(t *testing.T) {
	s := syncerWith("imported/r/docs/Gamma.md", "imported/r/docs/GAMMA.md")
	assert.Empty(t, s.existing("imported/r/docs/gamma.md"), "which one the doc meant cannot be told")
	assert.True(t, s.ambiguous("imported/r/docs/gamma.md"))
	assert.Equal(t, []string{"imported/r/docs/GAMMA.md", "imported/r/docs/Gamma.md"},
		s.folded["imported/r/docs/gamma.md"], "sorted, so the report reads the same every run")

	s.claimAll("imported/r/docs/gamma.md")
	assert.True(t, s.claimed["imported/r/docs/Gamma.md"])
	assert.True(t, s.claimed["imported/r/docs/GAMMA.md"], "an ambiguous set is reported once, and none of it pruned")
}

// syncDocs reports the ambiguous doc with every variant named, and claims
// them all, without touching the filesystem (nothing is written or read).
func TestCaseFold_SyncDocsReportsTheAmbiguousDoc(t *testing.T) {
	s := syncerWith("imported/r/docs/Gamma.md", "imported/r/docs/GAMMA.md")
	entries := s.syncDocs([]doc{{Rel: "gamma.md", Source: "r:docs/gamma.md"}})
	assert.Len(t, entries, 1)
	assert.Equal(t, Skipped, entries[0].Action)
	assert.Contains(t, entries[0].Reason, "imported/r/docs/Gamma.md")
	assert.Contains(t, entries[0].Reason, "imported/r/docs/GAMMA.md")
	assert.True(t, s.claimed["imported/r/docs/Gamma.md"])
}
