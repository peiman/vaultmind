package query_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sectionedDB holds a short note and a long note cut into sections. Section i
// (0-based) gets the unit vector on axis i+1, the long note's own (truncated)
// vector sits on axis 0, and the short note between them, so a query on one
// axis picks out exactly one unit. Returns the section ids in order.
func sectionedDB(t *testing.T) (*index.DB, []string) {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	short := index.NoteRecord{ID: "concept-short", Path: "concepts/short.md", Title: "Short", Type: "concept", BodyText: "A short note.", Hash: "h-short", IsDomain: true}
	require.NoError(t, index.StoreNote(db, short))
	var md strings.Builder
	for _, h := range []string{"Install", "Configure", "Troubleshoot"} {
		md.WriteString("## " + h + "\n\n" + h + " " + strings.Repeat("word ", 1200) + "\n\n")
	}
	for md.Len() < 9000*4 {
		md.WriteString(strings.Repeat("word ", 1200) + "\n\n")
	}
	long := index.NoteRecord{ID: "ref-long", Path: "refs/long.md", Title: "Long Guide", Type: "reference", BodyText: md.String(), Hash: "h-long", IsDomain: true}
	long.Sections = index.SectionsFor(long.BodyText, long.BodyText)
	require.NoError(t, index.StoreNote(db, long))

	const dims = 6
	axis := func(i int) []float32 { v := make([]float32, dims); v[i] = 1; return v }
	require.NoError(t, index.StoreEmbedding(db, "ref-long", axis(0)))
	require.NoError(t, index.StoreEmbedding(db, "concept-short", []float32{0.5, 0.5, 0, 0, 0, 0}))
	require.NoError(t, index.StoreSparseEmbedding(db, "ref-long", map[int32]float32{100: 1}))
	require.NoError(t, index.StoreSparseEmbedding(db, "concept-short", map[int32]float32{200: 1}))

	rows, err := db.Query(`SELECT id FROM sections WHERE note_id = 'ref-long' ORDER BY ordinal`)
	require.NoError(t, err)
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Close())
	require.GreaterOrEqual(t, len(ids), 3)
	for i, id := range ids {
		_, err := db.Exec(`UPDATE sections SET embedding = ?, sparse_embedding = ? WHERE id = ?`,
			index.EncodeEmbedding(axis(min(i+1, dims-1))), index.EncodeSparseEmbedding(map[int32]float32{int32(i + 1): 1}), id)
		require.NoError(t, err)
	}
	return db, ids
}

func axisEmbedder(i int) *mockEmbedder {
	v := make([]float32, 6)
	v[i] = 1
	return &mockEmbedder{vec: v, dims: 6}
}

func countID(rs []retrieval.ScoredResult, id string) int {
	n := 0
	for _, r := range rs {
		if r.ID == id {
			n++
		}
	}
	return n
}

// The dense lane scores a long note by its sections: the hit is the note, it
// names the section that matched, appears once, and its snippet is that
// section's text — not the note's opening.
func TestEmbeddingRetriever_ALongNoteMatchesByItsSection(t *testing.T) {
	db, ids := sectionedDB(t)
	r := &query.EmbeddingRetriever{DB: db, Embedder: axisEmbedder(2)} // section 1: "Configure"
	got, total, err := r.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.Equal(t, "ref-long", got[0].ID)
	assert.Equal(t, ids[1], got[0].Section)
	assert.True(t, strings.HasPrefix(got[0].Snippet, "Configure"), got[0].Snippet)
	assert.Equal(t, 1, countID(got, "ref-long"), "one hit per note however many sections score")
	assert.Equal(t, 2, total, "the total counts notes, not sections")
	assert.Empty(t, got[1].Section, "a short note names no section")
}

func TestSparseRetriever_ALongNoteMatchesByItsSection(t *testing.T) {
	db, ids := sectionedDB(t)
	r := &query.SparseRetriever{DB: db, EmbedSparse: func(context.Context, string) (map[int32]float32, error) {
		return map[int32]float32{3: 1}, nil // section 2: "Troubleshoot"
	}}
	got, _, err := r.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.Equal(t, "ref-long", got[0].ID)
	assert.Equal(t, ids[2], got[0].Section)
	assert.Equal(t, 1, countID(got, "ref-long"))
}

// A long note's similarity is its best section's, not its truncated vector's:
// relevance and spreading activation see what the lanes saw.
func TestNoteSimilarities_ALongNoteIsItsBestSection(t *testing.T) {
	db, _ := sectionedDB(t)
	sims, err := query.NoteSimilarities(context.Background(), "q", axisEmbedder(2), db)
	require.NoError(t, err)
	assert.InDelta(t, 1.0, sims["ref-long"], 1e-6)
	assert.Contains(t, sims, "concept-short")
	assert.Len(t, sims, 2, "keyed by note")
}

// Fusion: the keyword lane matches the whole note, the dense lane a section;
// the fused hit is the note and carries the section from the lane that ranked
// it best.
func TestHybridRetriever_CarriesTheMatchedSection(t *testing.T) {
	db, ids := sectionedDB(t)
	h := &query.HybridRetriever{Retrievers: []retrieval.NamedRetriever{
		{Name: "fts", Retriever: &staticRetriever{results: []retrieval.ScoredResult{{ID: "concept-short"}, {ID: "ref-long"}}}},
		{Name: "dense", Retriever: &query.EmbeddingRetriever{DB: db, Embedder: axisEmbedder(2)}},
	}}
	got, _, err := h.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	var long *retrieval.ScoredResult
	for i := range got {
		if got[i].ID == "ref-long" {
			long = &got[i]
		}
	}
	require.NotNil(t, long)
	assert.Equal(t, ids[1], long.Section)
	assert.Equal(t, 1, countID(got, "ref-long"))
}

// Calibration measures the noise floor over the units the lanes search.
func TestMeasureNoiseFloor_RunsOverUnits(t *testing.T) {
	db, _ := sectionedDB(t)
	_, err := query.MeasureNoiseFloor(context.Background(), axisEmbedder(1), db)
	require.NoError(t, err)
}

var _ embedding.Embedder = &mockEmbedder{}
