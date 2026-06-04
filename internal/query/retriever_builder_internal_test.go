package query

import (
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/require"
)

// detectEmbedderForDB on a mixed-state vault (both 384-dim MiniLM and
// 1024-dim BGE-M3 dense rows present) MUST pick BGE-M3. Loading MiniLM on
// a vault with any BGE-M3 rows loses sparse + colbert lanes entirely AND
// produces a dense-dim mismatch against the BGE-M3 majority. Picking
// BGE-M3 keeps full hybrid retrieval over the BGE-M3 fraction; the MiniLM
// minority is temporarily un-matchable on dense (until re-embed). Strictly
// safer than the inverse. See vaultmind#32.
//
// White-box: this test calls the unexported detectEmbedderForDB directly to
// observe which embedder type is constructed without going through the
// retriever-builder's full integration path.
func TestDetectEmbedderForDB_MixedStatePicksBGEM3(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Seed two MiniLM rows (384 floats × 4 bytes = 1536 bytes each).
	miniVec := make([]float32, 384)
	for i := 0; i < 2; i++ {
		_, err = db.Exec(
			`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"m"+string(rune('a'+i)), "m"+string(rune('a'+i))+".md", "h", 0, "T", true,
			index.EncodeEmbedding(miniVec),
		)
		require.NoError(t, err)
	}
	// Seed one BGE-M3 row (with sparse + colbert to satisfy migration 006).
	bgeVec := make([]float32, 1024)
	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding, sparse_embedding, colbert_embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"b0", "b0.md", "h", 0, "T", true,
		index.EncodeEmbedding(bgeVec),
		index.EncodeSparseEmbedding(map[int32]float32{0: 1.0}),
		index.EncodeColBERTEmbedding([][]float32{bgeVec}),
	)
	require.NoError(t, err)

	emb, cleanup, err := detectEmbedderForDB(db)
	if err != nil {
		// Embedder init can fail in environments without the model files
		// (no ORT runtime, no model on disk). The branch under test happens
		// BEFORE the constructor is called: dims detection chose BGE-M3
		// based on the row counts. The error message proves that branch
		// was taken — it'll mention bge-m3, not minilm or hugot defaults.
		require.Contains(t, err.Error(), "BGE-M3",
			"mixed-state vault must take the BGE-M3 branch — error must come from BGE-M3 constructor, not MiniLM fallback")
		return
	}
	defer cleanup()
	_, isBGEM3 := emb.(*embedding.BGEM3Embedder)
	require.True(t, isBGEM3,
		"mixed-state vault must construct a BGE-M3 embedder; got %T", emb)
}
