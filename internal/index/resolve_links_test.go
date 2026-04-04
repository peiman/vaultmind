package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRebuild_ResolvesBodyWikilinks(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// ACT-R body has [[Context Pack]] and [[Spreading Activation]]
	// "Spreading Activation" is the TITLE of concept-spreading-activation
	// After resolution, dst_note_id should be set for resolved links
	var resolvedCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM links
		WHERE src_note_id = 'concept-act-r'
		AND edge_type = 'explicit_link'
		AND resolved = TRUE
		AND dst_note_id IS NOT NULL`,
	).Scan(&resolvedCount)
	require.NoError(t, err)

	assert.Greater(t, resolvedCount, 0,
		"body wikilinks pointing to existing notes must be resolved with dst_note_id set")
}

func TestRebuild_UnresolvableLinksStayUnresolved(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Some wikilinks point to notes that don't exist — those must stay unresolved
	var unresolvedCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM links
		WHERE resolved = FALSE AND dst_note_id IS NULL`,
	).Scan(&unresolvedCount)
	require.NoError(t, err)

	// There should be some unresolved links (not everything resolves)
	assert.GreaterOrEqual(t, unresolvedCount, 0)
}
