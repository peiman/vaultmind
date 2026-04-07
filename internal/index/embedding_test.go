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
