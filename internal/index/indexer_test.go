package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVaultPath = "../../vaultmind-vault"

func TestRebuild_IndexesAllNotes(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	result, err := idxr.Rebuild()
	require.NoError(t, err)

	assert.Greater(t, result.Indexed, 0)
	assert.Equal(t, 0, result.Errors)
}

func TestRebuild_PopulatesNotesTable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Check a known domain note
	var id, title string
	var isDomain bool
	err = db.QueryRow("SELECT id, title, is_domain FROM notes WHERE id = ?", "concept-act-r").
		Scan(&id, &title, &isDomain)
	require.NoError(t, err)
	assert.Equal(t, "concept-act-r", id)
	assert.Equal(t, "ACT-R", title)
	assert.True(t, isDomain)
}

func TestRebuild_IndexesDomainAndUnstructured(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	result, err := idxr.Rebuild()
	require.NoError(t, err)

	assert.Greater(t, result.DomainNotes, 0)
	// Welcome.md is unstructured
	assert.Greater(t, result.UnstructuredNotes, 0)
}

func TestRebuild_PopulatesAliases(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// ACT-R has aliases
	var aliasCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM aliases WHERE note_id = ?", "concept-act-r").Scan(&aliasCount))
	assert.Greater(t, aliasCount, 0)
}

func TestRebuild_PopulatesLinks(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// ACT-R body has wikilinks to Context Pack, Spreading Activation, etc.
	var linkCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM links WHERE src_note_id = ?", "concept-act-r").Scan(&linkCount))
	assert.Greater(t, linkCount, 0)
}

func TestRebuild_PopulatesFTS(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Search for "cognitive architecture" should find ACT-R
	rows, err := db.Query("SELECT note_id FROM fts_notes WHERE fts_notes MATCH ?", "cognitive architecture")
	require.NoError(t, err)
	defer rows.Close()

	var noteIDs []string
	for rows.Next() {
		var nid string
		require.NoError(t, rows.Scan(&nid))
		noteIDs = append(noteIDs, nid)
	}
	require.NoError(t, rows.Err())

	assert.Contains(t, noteIDs, "concept-act-r")
}

func TestRebuild_UsesCorrectEdgeTypes(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Body wikilinks should be stored as "explicit_link", not "wikilink"
	var explicitLinkCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM links WHERE edge_type = 'explicit_link'").Scan(&explicitLinkCount))
	assert.Greater(t, explicitLinkCount, 0, "body wikilinks must use edge_type 'explicit_link'")

	// Frontmatter relations should be stored as "explicit_relation"
	var explicitRelationCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM links WHERE edge_type = 'explicit_relation'").Scan(&explicitRelationCount))
	assert.Greater(t, explicitRelationCount, 0, "frontmatter relations must use edge_type 'explicit_relation'")

	// No parser-native edge types should exist
	var wikilinkCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM links WHERE edge_type = 'wikilink'").Scan(&wikilinkCount))
	assert.Equal(t, 0, wikilinkCount, "parser 'wikilink' type must be mapped to 'explicit_link'")
}

func TestRebuild_ResultContainsTimingInfo(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	result, err := idxr.Rebuild()
	require.NoError(t, err)

	assert.NotEmpty(t, result.DBPath)
	assert.Greater(t, result.DurationMs, int64(0))
	assert.NotEmpty(t, result.CompletedAt)
}

func TestRebuild_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	result1, err := idxr.Rebuild()
	require.NoError(t, err)

	result2, err := idxr.Rebuild()
	require.NoError(t, err)

	assert.Equal(t, result1.Indexed, result2.Indexed)
}

func TestIndexFile_UpdatesExistingNote(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)
	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	err = idxr.IndexFile("projects/proj-vaultmind.md")
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	hashes, err := db.NoteHashes()
	require.NoError(t, err)
	_, exists := hashes["projects/proj-vaultmind.md"]
	assert.True(t, exists)
}

func TestIndexFile_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)
	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	err = idxr.IndexFile("nonexistent/file.md")
	assert.Error(t, err)
}
