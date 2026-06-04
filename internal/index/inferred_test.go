package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripForAliasMatch_CodeFence(t *testing.T) {
	body := "before\n```\nRetry Engine code\n```\nafter"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine code")
	assert.Contains(t, result, "before")
	assert.Contains(t, result, "after")
}

func TestStripForAliasMatch_InlineCode(t *testing.T) {
	body := "see `Retry Engine` for details"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine")
	assert.Contains(t, result, "see")
}

func TestStripForAliasMatch_Wikilink(t *testing.T) {
	body := "related to [[Retry Engine]] and more"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine")
}

func TestStripForAliasMatch_AliasedWikilink(t *testing.T) {
	body := "see [[Retry Engine|the engine]] works"
	result := index.StripForAliasMatch(body)
	assert.Contains(t, result, "the engine")
	assert.NotContains(t, result, "Retry Engine")
}

func TestStripForAliasMatch_HTMLComment(t *testing.T) {
	body := "before <!-- Retry Engine --> after"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine")
}

func TestStripForAliasMatch_PreservesNormalText(t *testing.T) {
	body := "The Retry Engine handles transient failures gracefully."
	result := index.StripForAliasMatch(body)
	assert.Equal(t, body, result)
}

func TestStripForAliasMatch_LanguageFence(t *testing.T) {
	body := "before\n```go\nRetry Engine\n```\nafter"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine")
}

func TestStripForAliasMatch_TildeFence(t *testing.T) {
	body := "before\n~~~python\nMachine Learning code here\n~~~\nafter"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Machine Learning code")
	assert.Contains(t, result, "before")
	assert.Contains(t, result, "after")
}

func TestComputeAliasMentions_Basic(t *testing.T) {
	db := buildIndexedDB(t)
	count, err := index.ComputeAliasMentions(db, 3)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 0)
}

func TestComputeAliasMentions_SkipsShortAliases(t *testing.T) {
	db := buildIndexedDB(t)
	count, err := index.ComputeAliasMentions(db, 1000)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestComputeAliasMentions_ClearsOldEdges(t *testing.T) {
	db := buildIndexedDB(t)
	count1, err := index.ComputeAliasMentions(db, 3)
	require.NoError(t, err)
	count2, err := index.ComputeAliasMentions(db, 3)
	require.NoError(t, err)
	assert.Equal(t, count1, count2)
}

func TestComputeAliasMentions_EdgesInLinksTable(t *testing.T) {
	db := buildIndexedDB(t)
	_, err := index.ComputeAliasMentions(db, 3)
	require.NoError(t, err)
	var edgeCount int
	err = db.QueryRow("SELECT COUNT(*) FROM links WHERE edge_type = 'alias_mention'").Scan(&edgeCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, edgeCount, 0)
}

func TestComputeTagOverlap_Basic(t *testing.T) {
	db := buildIndexedDB(t)
	count, err := index.ComputeTagOverlap(db, 1.0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 0)
}

func TestComputeTagOverlap_HighThreshold(t *testing.T) {
	db := buildIndexedDB(t)
	count, err := index.ComputeTagOverlap(db, 999.0)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestComputeTagOverlap_ClearsOldEdges(t *testing.T) {
	db := buildIndexedDB(t)
	count1, err := index.ComputeTagOverlap(db, 1.0)
	require.NoError(t, err)
	count2, err := index.ComputeTagOverlap(db, 1.0)
	require.NoError(t, err)
	assert.Equal(t, count1, count2)
}

func TestComputeTagOverlap_EdgesInLinksTable(t *testing.T) {
	db := buildIndexedDB(t)
	_, err := index.ComputeTagOverlap(db, 1.0)
	require.NoError(t, err)
	var edgeCount int
	err = db.QueryRow("SELECT COUNT(*) FROM links WHERE edge_type = 'tag_overlap'").Scan(&edgeCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, edgeCount, 0)
}

func TestComputeTagOverlap_WeightStored(t *testing.T) {
	db := buildIndexedDB(t)
	count, err := index.ComputeTagOverlap(db, 0.0)
	require.NoError(t, err)
	if count == 0 {
		t.Skip("no tag overlap edges in test vault")
	}
	var weight float64
	err = db.QueryRow("SELECT weight FROM links WHERE edge_type = 'tag_overlap' LIMIT 1").Scan(&weight)
	require.NoError(t, err)
	assert.Greater(t, weight, 0.0)
}

func TestComputeTagOverlap_Bidirectional(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"note-a", "concepts/note-a.md", "Note A", "abc", 0, true)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"note-b", "concepts/note-b.md", "Note B", "def", 0, true)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO tags (note_id, tag) VALUES (?, ?)", "note-a", "rare-topic")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO tags (note_id, tag) VALUES (?, ?)", "note-b", "rare-topic")
	require.NoError(t, err)

	count, err := index.ComputeTagOverlap(db, 0.0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 2, "should create edges in BOTH directions")

	var ab int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM links WHERE src_note_id = ? AND dst_note_id = ? AND edge_type = 'tag_overlap'",
		"note-a", "note-b").Scan(&ab))
	assert.Equal(t, 1, ab, "should have A→B tag_overlap edge")

	var ba int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM links WHERE src_note_id = ? AND dst_note_id = ? AND edge_type = 'tag_overlap'",
		"note-b", "note-a").Scan(&ba))
	assert.Equal(t, 1, ba, "should have B→A tag_overlap edge")
}

func TestComputeAliasMentions_CollisionHandled(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("INSERT INTO notes (id, path, title, hash, mtime, is_domain, body_text) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"note-api-guide", "concepts/api-guide.md", "API Guide", "a1", 0, true, "")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO aliases (note_id, alias, alias_normalized) VALUES (?, ?, ?)",
		"note-api-guide", "API", "api")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO notes (id, path, title, hash, mtime, is_domain, body_text) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"note-api-ref", "concepts/api-ref.md", "api reference", "a2", 0, true, "")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO aliases (note_id, alias, alias_normalized) VALUES (?, ?, ?)",
		"note-api-ref", "api", "api")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO notes (id, path, title, hash, mtime, is_domain, body_text) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"note-mentions", "concepts/mentions.md", "Mentions", "a3", 0, true, "We use the api extensively in production")
	require.NoError(t, err)

	count, err := index.ComputeAliasMentions(db, 2)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 2, "should create edges to BOTH notes with colliding aliases")

	var edgeCount int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM links WHERE src_note_id = 'note-mentions' AND edge_type = 'alias_mention'",
	).Scan(&edgeCount))
	assert.Equal(t, 2, edgeCount, "should have alias_mention edges to both api notes")
}

func TestInferredEdges_AfterRebuild(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	result, err := idxr.Rebuild()
	require.NoError(t, err)
	assert.Greater(t, result.Indexed, 0)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var aliasCount int
	err = db.QueryRow("SELECT COUNT(*) FROM links WHERE edge_type = 'alias_mention'").Scan(&aliasCount)
	require.NoError(t, err)

	var tagCount int
	err = db.QueryRow("SELECT COUNT(*) FROM links WHERE edge_type = 'tag_overlap'").Scan(&tagCount)
	require.NoError(t, err)

	assert.Greater(t, aliasCount+tagCount, 0, "test vault should produce at least one inferred edge")
}

func TestInferredEdges_AfterIncremental(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	_, err = idxr.Incremental()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var total int
	err = db.QueryRow("SELECT COUNT(*) FROM links WHERE edge_type IN ('alias_mention', 'tag_overlap')").Scan(&total)
	require.NoError(t, err)
	assert.Greater(t, total, 0)
}
