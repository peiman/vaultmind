package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeEmbedding_RoundTrip(t *testing.T) {
	original := []float32{0.1, -0.5, 3.14, 0, -1e-6}
	encoded := index.EncodeEmbedding(original)
	assert.Len(t, encoded, len(original)*4, "each float32 is 4 bytes")

	decoded, err := index.DecodeEmbedding(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeEmbedding_InvalidLength(t *testing.T) {
	_, err := index.DecodeEmbedding([]byte{0x01, 0x02, 0x03}) // 3 bytes, not divisible by 4
	assert.Error(t, err)
}

func TestEncodeEmbedding_Empty(t *testing.T) {
	encoded := index.EncodeEmbedding(nil)
	assert.Nil(t, encoded)
}

func TestDecodeEmbedding_Empty(t *testing.T) {
	decoded, err := index.DecodeEmbedding(nil)
	require.NoError(t, err)
	assert.Nil(t, decoded)
}

func buildEmbeddingTestDB(t *testing.T) *index.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	vaultPath := "../../vaultmind-vault"
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStoreAndLoadEmbedding(t *testing.T) {
	db := buildEmbeddingTestDB(t)
	row, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row, "test vault must contain spreading-activation.md")

	vec := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	err = index.StoreEmbedding(db, row.ID, vec)
	require.NoError(t, err)

	loaded, err := index.LoadEmbedding(db, row.ID)
	require.NoError(t, err)
	assert.Equal(t, vec, loaded)
}

func TestLoadEmbedding_NoEmbedding(t *testing.T) {
	db := buildEmbeddingTestDB(t)
	row, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row)
	loaded, err := index.LoadEmbedding(db, row.ID)
	require.NoError(t, err)
	assert.Nil(t, loaded, "note without embedding should return nil")
}

func TestLoadAllEmbeddings(t *testing.T) {
	db := buildEmbeddingTestDB(t)

	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)
	row2, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NotNil(t, row2)

	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{0.1, 0.2}))
	require.NoError(t, index.StoreEmbedding(db, row2.ID, []float32{0.3, 0.4}))

	all, err := index.LoadAllEmbeddings(db)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	ids := map[string]bool{all[0].NoteID: true, all[1].NoteID: true}
	assert.True(t, ids[row1.ID])
	assert.True(t, ids[row2.ID])
}

func TestStoreEmbedding_NonexistentNote(t *testing.T) {
	db := buildEmbeddingTestDB(t)
	err := index.StoreEmbedding(db, "nonexistent-note-id", []float32{0.1, 0.2})
	assert.Error(t, err, "should error when note ID does not exist")
	assert.Contains(t, err.Error(), "no note found")
}

func TestEncodeSparseEmbedding_RoundTrip(t *testing.T) {
	original := map[int32]float32{100: 1.0, 500: 0.5, 9999: 0.001}
	encoded := index.EncodeSparseEmbedding(original)
	assert.Equal(t, len(original)*8, len(encoded), "each entry is 8 bytes")

	decoded, err := index.DecodeSparseEmbedding(encoded)
	require.NoError(t, err)
	assert.Len(t, decoded, 3)
	assert.InDelta(t, 1.0, decoded[100], 1e-6)
	assert.InDelta(t, 0.5, decoded[500], 1e-6)
	assert.InDelta(t, 0.001, decoded[9999], 1e-6)
}

func TestEncodeSparseEmbedding_Empty(t *testing.T) {
	assert.Nil(t, index.EncodeSparseEmbedding(nil))
}

func TestDecodeSparseEmbedding_Empty(t *testing.T) {
	decoded, err := index.DecodeSparseEmbedding(nil)
	require.NoError(t, err)
	assert.Empty(t, decoded, "empty input returns empty map")
}

func TestDecodeSparseEmbedding_InvalidLength(t *testing.T) {
	_, err := index.DecodeSparseEmbedding([]byte{1, 2, 3})
	assert.Error(t, err)
}

func TestEncodeColBERTEmbedding_RoundTrip(t *testing.T) {
	original := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}
	encoded := index.EncodeColBERTEmbedding(original)
	assert.Equal(t, 24, len(encoded))

	decoded, err := index.DecodeColBERTEmbedding(encoded, 3)
	require.NoError(t, err)
	require.Len(t, decoded, 2)
	assert.InDelta(t, 0.1, decoded[0][0], 1e-6)
	assert.InDelta(t, 0.6, decoded[1][2], 1e-6)
}

func TestEncodeColBERTEmbedding_Empty(t *testing.T) {
	assert.Nil(t, index.EncodeColBERTEmbedding(nil))
}

func TestDecodeColBERTEmbedding_InvalidLength(t *testing.T) {
	_, err := index.DecodeColBERTEmbedding([]byte{1, 2, 3, 4, 5}, 3)
	assert.Error(t, err)
}

func TestHasEmbeddings(t *testing.T) {
	db := buildEmbeddingTestDB(t)

	has, err := index.HasEmbeddings(db)
	require.NoError(t, err)
	assert.False(t, has, "fresh index has no embeddings")

	row, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row)
	require.NoError(t, index.StoreEmbedding(db, row.ID, []float32{0.1}))

	has, err = index.HasEmbeddings(db)
	require.NoError(t, err)
	assert.True(t, has)
}
