package query_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVaultPath = "../../vaultmind-vault"

func buildIndexedDB(t *testing.T) *index.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db := testvault.OpenSharedDB(t, testVaultPath, dbPath)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDoctor_ReturnsVaultSummary(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Doctor(db, testVaultPath)
	require.NoError(t, err)

	assert.Equal(t, testVaultPath, result.VaultPath)
	assert.Greater(t, result.TotalFiles, 0)
	assert.Greater(t, result.DomainNotes, 0)
	assert.GreaterOrEqual(t, result.UnstructuredNotes, 0)
	assert.Equal(t, result.TotalFiles, result.DomainNotes+result.UnstructuredNotes)
}

func TestDoctor_ReportsUnresolvedLinks(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Doctor(db, testVaultPath)
	require.NoError(t, err)

	// Body wikilinks are unresolved (dst_note_id is NULL)
	assert.GreaterOrEqual(t, result.Issues.UnresolvedLinks, 0)
}

func TestDoctor_DetectsPathPseudoIDLinks(t *testing.T) {
	db := buildIndexedDB(t)

	_, err := db.Exec("INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"test-src-pseudo", "concepts/test-src-pseudo.md", "Source", "abc", 0, true)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO links (src_note_id, dst_note_id, dst_raw, edge_type, resolved, confidence)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"test-src-pseudo", "_path:NonExistent.md", "NonExistent", "explicit_link", true, "high")
	require.NoError(t, err)

	result, docErr := query.Doctor(db, testVaultPath)
	require.NoError(t, docErr)

	assert.Greater(t, result.Issues.PathPseudoIDLinks, 0,
		"should detect links resolved to _path: pseudo-IDs")

	found := false
	for _, pl := range result.Issues.PathPseudoIDDetails {
		if pl.TargetRaw == "NonExistent" {
			found = true
		}
	}
	assert.True(t, found, "should include the pseudo-ID link in details")
}

func TestDoctor_DetectsObsidianIncompatibleLinks(t *testing.T) {
	db := buildIndexedDB(t)

	// Insert a resolved link that uses title format instead of filename format.
	// This simulates [[Context Pack]] resolving to concept-context-pack via title,
	// but Obsidian won't find it because the file is context-pack.md.
	_, err := db.Exec("INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"test-target", "concepts/test-target.md", "Test Target", "abc", 0, true)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"test-source", "concepts/test-source.md", "Test Source", "def", 0, true)
	require.NoError(t, err)

	// Link that uses title "Test Target" instead of filename "test-target"
	_, err = db.Exec(`INSERT INTO links (src_note_id, dst_note_id, dst_raw, edge_type, resolved, confidence)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"test-source", "test-target", "Test Target", "explicit_link", true, "high")
	require.NoError(t, err)

	result, docErr := query.Doctor(db, testVaultPath)
	require.NoError(t, docErr)

	assert.Greater(t, result.Issues.ObsidianIncompatibleLinks, 0,
		"should detect links using title format instead of filename format")

	found := false
	for _, il := range result.Issues.IncompatibleLinkDetails {
		if il.TargetRaw == "Test Target" && il.SuggestedFix == "test-target" {
			found = true
		}
	}
	assert.True(t, found, "should suggest filename fix for title-format link")
}
