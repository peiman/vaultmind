package index_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/require"
)

func TestRebuild_DuplicateWikilinksDoNotCrashResolution(t *testing.T) {
	// Create a vault with a note that links to the same target twice
	vaultDir := t.TempDir()
	vmDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "config.yaml"), []byte("types:\n  concept:\n    required: [title]"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "target.md"),
		[]byte("---\nid: target-note\ntype: concept\ntitle: Target\ncreated: 2026-04-03\nvm_updated: 2026-04-03\n---\nTarget body."), 0o644))

	// This note links to [[Target]] twice in the body
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "source.md"),
		[]byte("---\nid: source-note\ntype: concept\ntitle: Source\ncreated: 2026-04-03\nvm_updated: 2026-04-03\n---\nFirst ref to [[Target]] and second ref to [[Target]]."), 0o644))

	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)

	idxr := index.NewIndexer(vaultDir, dbPath, cfg)
	result, err := idxr.Rebuild()
	require.NoError(t, err, "rebuild with duplicate wikilinks must not crash")
	require.Greater(t, result.Indexed, 0)

	// The duplicate wikilinks should still resolve (at least one)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var resolvedCount int
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*) FROM links
		WHERE src_note_id = 'source-note'
		AND edge_type = 'explicit_link'
		AND resolved = TRUE`).Scan(&resolvedCount))
	require.Greater(t, resolvedCount, 0, "duplicate wikilinks must still resolve")
}
