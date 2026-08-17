package mutation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The write side of the symlink class, and the sharper half: the scanner leaked
// a file OUT of the operator's filesystem, this one writes INTO it.
//
// FixWikilinks walks the vault and calls os.WriteFile on every path it finds.
// os.WriteFile follows a symlink, so a note `notes.md -> ~/.zshrc` in a cloned
// vault turns `doctor --heal` into a write primitive: the target is replaced
// with rewritten note content.
//
// Confinement does not catch this. The mutator's own path check would pass —
// notes.md IS inside the vault. "Stays under the root" and "is not a link" are
// different predicates, and only the second one stops this.
func TestFixWikilinks_DoesNotWriteThroughASymlink(t *testing.T) {
	outside := t.TempDir()
	victim := filepath.Join(outside, "zshrc")
	original := "export PATH=/usr/local/bin:$PATH\n"
	require.NoError(t, os.WriteFile(victim, []byte(original), 0o644))

	vaultDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  concept:\n    required: [title]\n"), 0o644))

	// The link target the rewrite resolves against: a note whose filename stem
	// (alpha) differs from its title (Alpha Concept), so a bare [[Alpha Concept]]
	// is rewritten to [[alpha|Alpha Concept]].
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "alpha.md"),
		[]byte("---\nid: concept-alpha\ntype: concept\ntitle: Alpha Concept\n---\nBody.\n"), 0o644))

	// A real note holding the fixable link, so the run has legitimate work.
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "gamma.md"),
		[]byte("---\nid: concept-gamma\ntype: concept\ntitle: Gamma\n---\nSee [[Alpha Concept]].\n"), 0o644))

	// The attack: a *.md symlink pointing at a file outside the vault, whose
	// content contains a fixable link so the rewriter would have something to
	// write back.
	require.NoError(t, os.WriteFile(victim, []byte(original+"# see [[Alpha Concept]]\n"), 0o644))
	require.NoError(t, os.Symlink(victim, filepath.Join(vaultDir, "notes.md")))

	before, err := os.ReadFile(victim)
	require.NoError(t, err)

	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	dbPath := filepath.Join(t.TempDir(), "index.db")
	_, err = index.NewIndexer(vaultDir, dbPath, cfg).Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	result, err := mutation.FixWikilinks(db, vaultDir, true)
	require.NoError(t, err)

	after, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"a file outside the vault was rewritten through a symlink — doctor --heal is a write primitive")

	assert.Equal(t, []string{"notes.md"}, result.SkippedSymlinks,
		"the skipped path belongs on the result, not in a log line the caller never reads")
}
