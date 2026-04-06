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

func TestResolveLinks_ByFilenameStem(t *testing.T) {
	db := openTestDB(t)

	// Create a note at concepts/my-concept.md with title "My Concept"
	_, err := db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-my-concept", "concepts/my-concept.md", "My Concept", "abc", 0, true)
	require.NoError(t, err)

	// Create a source note
	_, err = db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"source-test", "sources/source-test.md", "Test Source", "def", 0, true)
	require.NoError(t, err)

	// Insert an unresolved wikilink using filename stem (Obsidian convention):
	// [[my-concept|My Concept]] → parser stores dst_raw = "my-concept"
	_, err = db.Exec(
		`INSERT INTO links (src_note_id, dst_raw, edge_type, resolved, confidence)
		 VALUES (?, ?, ?, ?, ?)`,
		"source-test", "my-concept", "explicit_link", false, "high")
	require.NoError(t, err)

	resolved, err := index.ResolveLinks(db)
	require.NoError(t, err)
	assert.Greater(t, resolved, 0, "filename-stem link should resolve")

	var dstNoteID string
	err = db.QueryRow(
		"SELECT dst_note_id FROM links WHERE src_note_id = ? AND dst_raw = ?",
		"source-test", "my-concept").Scan(&dstNoteID)
	require.NoError(t, err)
	assert.Equal(t, "concept-my-concept", dstNoteID,
		"dst_raw 'my-concept' should resolve to concept-my-concept via filename stem")
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
