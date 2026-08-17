package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dataview lint reports per-file findings, so a file it never opened must not
// be counted as clean. The scanner refuses to follow symlinks (they could point
// anywhere the user can read), which means lint silently covered fewer files
// than the vault holds unless it says so.
func TestExecuteDataviewLint_ReportsSkippedSymlinks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  concept:\n    required: [title]\n"), 0o644))
	writeTestNote(t, dir, "concepts/alpha.md", "---\nid: concept-alpha\ntype: concept\ntitle: Alpha\n---\nBody.\n")

	outside := t.TempDir()
	target := filepath.Join(outside, "elsewhere.md")
	require.NoError(t, os.WriteFile(target, []byte("not mine to read"), 0o644))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "linked.md")))

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o750))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)

	result, _, err := executeDataviewLint(&cobra.Command{}, dir)
	require.NoError(t, err)

	var found bool
	for _, iss := range result.Issues {
		if iss.Rule == "symlink_skipped" {
			found = true
			assert.Equal(t, "linked.md", iss.Path)
		}
	}
	assert.True(t, found,
		"a symlink the linter never opened must appear as an issue, not vanish from a clean report")
}
