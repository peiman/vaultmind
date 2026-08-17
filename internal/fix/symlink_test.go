package fix_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/fix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The frontmatter backfill writes through the mutator, which confines every
// path it touches — and confinement is the wrong predicate for this. A note
// `notes.md -> <outside>/config` IS inside the vault by any prefix check, and
// the write still lands on the target.
//
// The second half matters as much as the first: a backfill that quietly passed
// over a file and then reported a count is telling the operator the vault was
// examined when it was not.
func TestRunBackfill_DoesNotWriteThroughASymlink(t *testing.T) {
	outside := t.TempDir()
	victim := filepath.Join(outside, "config.md")
	original := "---\nid: not-yours\ntype: concept\ntitle: Outside\n---\nUntouched.\n"
	require.NoError(t, os.WriteFile(victim, []byte(original), 0o644))

	vaultDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  concept:\n    required: [title]\n    optional: [created]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "real.md"),
		[]byte("---\nid: concept-real\ntype: concept\ntitle: Real\n---\nBody.\n"), 0o644))
	require.NoError(t, os.Symlink(victim, filepath.Join(vaultDir, "linked.md")))

	result, err := fix.RunBackfill(fix.Config{VaultPath: vaultDir, Apply: true})
	require.NoError(t, err)

	after, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, original, string(after),
		"a file outside the vault was rewritten through a symlink — confinement does not stop this")

	assert.Equal(t, []string{"linked.md"}, result.SkippedSymlinks,
		"a file the backfill never opened must be named, not absorbed into a scanned count")
	assert.NotContains(t, pathsOf(result), "linked.md",
		"the skipped file must not also appear as examined")
}

func pathsOf(r *fix.Result) []string {
	out := make([]string, 0, len(r.Items))
	for _, it := range r.Items {
		out = append(out, it.Path)
	}
	return out
}
