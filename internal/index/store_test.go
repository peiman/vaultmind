package index_test

import (
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTestRecord(id, relPath string) index.NoteRecord {
	return index.NoteRecord{
		ID:       id,
		Path:     relPath,
		Title:    "Test Note",
		Type:     "concept",
		Status:   "active",
		Created:  "2026-04-03",
		Updated:  "2026-04-03",
		BodyText: "Body text here.",
		Hash:     "abc123",
		MTime:    1234567890,
		IsDomain: true,
		Aliases:  []string{"Test Alias", "Another Alias"},
		Tags:     []string{"test", "example"},
		ExtraKV:  map[string]interface{}{"owner_id": "person-alice", "score": 42.0},
		Links: []index.LinkRecord{
			{DstRaw: "other-concept", EdgeType: "explicit_link", Confidence: "high"},
		},
		Headings: []index.HeadingRecord{
			{Slug: "overview", Level: 2, Title: "Overview"},
		},
		Blocks: []index.BlockRecord{
			{BlockID: "block-abc", StartLine: 10},
		},
	}
}

func TestStoreNote_InsertsIntoAllTables(t *testing.T) {
	db := openTestDB(t)
	rec := buildTestRecord("concept-test", "concepts/test.md")

	err := index.StoreNote(db, rec)
	require.NoError(t, err)

	// notes
	var id, path, title string
	require.NoError(t, db.QueryRow("SELECT id, path, title FROM notes WHERE id = ?", "concept-test").Scan(&id, &path, &title))
	assert.Equal(t, "concept-test", id)
	assert.Equal(t, "concepts/test.md", path)
	assert.Equal(t, "Test Note", title)

	// aliases
	var aliasCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM aliases WHERE note_id = ?", "concept-test").Scan(&aliasCount))
	assert.Equal(t, 2, aliasCount)

	// tags
	var tagCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM tags WHERE note_id = ?", "concept-test").Scan(&tagCount))
	assert.Equal(t, 2, tagCount)

	// frontmatter_kv
	var kvCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM frontmatter_kv WHERE note_id = ?", "concept-test").Scan(&kvCount))
	assert.Equal(t, 2, kvCount)

	// links
	var linkCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM links WHERE src_note_id = ?", "concept-test").Scan(&linkCount))
	assert.Equal(t, 1, linkCount)

	// headings
	var headingCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM headings WHERE note_id = ?", "concept-test").Scan(&headingCount))
	assert.Equal(t, 1, headingCount)

	// blocks
	var blockCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM blocks WHERE note_id = ?", "concept-test").Scan(&blockCount))
	assert.Equal(t, 1, blockCount)
}

func TestStoreNote_FTSRowInserted(t *testing.T) {
	db := openTestDB(t)
	rec := buildTestRecord("concept-fts", "concepts/fts.md")

	require.NoError(t, index.StoreNote(db, rec))

	var ftsCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM fts_notes WHERE note_id = ?", "concept-fts").Scan(&ftsCount))
	assert.Equal(t, 1, ftsCount)
}

func TestStoreNote_DeleteBeforeReinsert(t *testing.T) {
	db := openTestDB(t)
	rec := buildTestRecord("concept-reindex", "concepts/reindex.md")

	require.NoError(t, index.StoreNote(db, rec))

	rec.Title = "Updated Title"
	rec.Tags = []string{"updated"}
	require.NoError(t, index.StoreNote(db, rec))

	var title string
	require.NoError(t, db.QueryRow("SELECT title FROM notes WHERE id = ?", "concept-reindex").Scan(&title))
	assert.Equal(t, "Updated Title", title)

	var tagCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM tags WHERE note_id = ?", "concept-reindex").Scan(&tagCount))
	assert.Equal(t, 1, tagCount)
}

func TestStoreNote_AliasNormalization(t *testing.T) {
	db := openTestDB(t)
	rec := buildTestRecord("concept-alias", "concepts/alias.md")
	rec.Aliases = []string{"  ACT-R  ", "Adaptive Control of Thought"}

	require.NoError(t, index.StoreNote(db, rec))

	rows, err := db.Query("SELECT alias_normalized FROM aliases WHERE note_id = ? ORDER BY alias_normalized", "concept-alias")
	require.NoError(t, err)
	defer rows.Close()

	var normalized []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		normalized = append(normalized, n)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []string{"act-r", "adaptive control of thought"}, normalized)
}

func TestStoreNote_FrontmatterKVJSONEncoded(t *testing.T) {
	db := openTestDB(t)
	rec := buildTestRecord("concept-kv", "concepts/kv.md")
	rec.ExtraKV = map[string]interface{}{"owner_id": "person-alice"}

	require.NoError(t, index.StoreNote(db, rec))

	var valueJSON string
	require.NoError(t, db.QueryRow("SELECT value_json FROM frontmatter_kv WHERE note_id = ? AND key = ?", "concept-kv", "owner_id").Scan(&valueJSON))

	var v interface{}
	require.NoError(t, json.Unmarshal([]byte(valueJSON), &v))
	assert.Equal(t, "person-alice", v)
}

func TestStoreNote_UnstructuredNote(t *testing.T) {
	db := openTestDB(t)
	rec := index.NoteRecord{
		ID:       "_path:Welcome.md",
		Path:     "Welcome.md",
		Title:    "Welcome",
		BodyText: "Welcome to the vault.",
		Hash:     "deadbeef",
		MTime:    1234567890,
		IsDomain: false,
	}

	require.NoError(t, index.StoreNote(db, rec))

	var isDomain bool
	require.NoError(t, db.QueryRow("SELECT is_domain FROM notes WHERE id = ?", "_path:Welcome.md").Scan(&isDomain))
	assert.False(t, isDomain)
}

func TestStoreNote_DuplicateLinksDeduped(t *testing.T) {
	db := openTestDB(t)

	// Create target note for FK
	target := buildTestRecord("concept-target", "concepts/target.md")
	target.Links = nil
	require.NoError(t, index.StoreNote(db, target))

	rec := buildTestRecord("concept-dup", "concepts/dup.md")
	rec.Links = []index.LinkRecord{
		{DstNoteID: "concept-target", DstRaw: "target", EdgeType: "explicit_link", Confidence: "high", Resolved: true},
		{DstNoteID: "concept-target", DstRaw: "target", EdgeType: "explicit_link", Confidence: "high", Resolved: true},
	}

	// Duplicates are silently ignored (INSERT OR IGNORE), not errors
	require.NoError(t, index.StoreNote(db, rec))

	var linkCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM links WHERE src_note_id = ?", "concept-dup").Scan(&linkCount))
	assert.Equal(t, 1, linkCount, "duplicate links should be deduped to one")
}

func TestStoreNote_DuplicateUnresolvedLinksDeduped(t *testing.T) {
	db := openTestDB(t)

	// Unresolved duplicate wikilinks: same dst_raw, both dst_note_id = NULL.
	// SQLite treats NULL as distinct in unique indexes, so INSERT OR IGNORE
	// stores BOTH. During ResolveLinks, only the first gets resolved (the second
	// hits the unique constraint on UPDATE). This leaves a permanently unresolved
	// phantom link.
	rec := buildTestRecord("concept-src", "concepts/src.md")
	rec.Links = []index.LinkRecord{
		{DstRaw: "SomeTarget", EdgeType: "explicit_link", Confidence: "high"},
		{DstRaw: "SomeTarget", EdgeType: "explicit_link", Confidence: "high"},
	}

	require.NoError(t, index.StoreNote(db, rec))

	var linkCount int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM links WHERE src_note_id = ? AND dst_raw = ?",
		"concept-src", "SomeTarget",
	).Scan(&linkCount))
	assert.Equal(t, 1, linkCount,
		"duplicate unresolved links (NULL dst_note_id) must be deduped to one")
}
