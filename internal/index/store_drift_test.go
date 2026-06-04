package index_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStoreNote_ClearsEmbeddingOnContentChange — when a note's hash changes,
// its embeddings become stale (they were computed from the old body_text).
// The UPSERT must clear them so the next `index --embed` run picks up the
// drift and re-embeds. Without this, semantic retrieval silently returns
// hits against outdated content.
func TestStoreNote_ClearsEmbeddingOnContentChange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "idx.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Seed: note with an embedding attached.
	vec := make([]float32, 384)
	vec[0] = 0.5
	_, err = db.Exec(
		`INSERT INTO notes (id, path, title, hash, mtime, is_domain, embedding, sparse_embedding, colbert_embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "Old title", "HASH_OLD", 100, true,
		index.EncodeEmbedding(vec), []byte("sparse-blob"), []byte("colbert-blob"),
	)
	require.NoError(t, err)

	// StoreNote with a NEW hash representing content change.
	require.NoError(t, index.StoreNote(db, index.NoteRecord{
		ID:       "n1",
		Path:     "n1.md",
		Title:    "Updated title",
		BodyText: "updated body",
		Hash:     "HASH_NEW",
		MTime:    200,
		IsDomain: true,
	}))

	var emb, sparse, colbert sql.NullString
	err = db.QueryRow(
		`SELECT embedding, sparse_embedding, colbert_embedding FROM notes WHERE id = ?`, "n1",
	).Scan(&emb, &sparse, &colbert)
	require.NoError(t, err)
	assert.False(t, emb.Valid, "embedding should be cleared when hash changed")
	assert.False(t, sparse.Valid, "sparse_embedding should be cleared when hash changed")
	assert.False(t, colbert.Valid, "colbert_embedding should be cleared when hash changed")
}

// TestStoreNote_PreservesEmbeddingOnSameHash — mtime-only updates (file
// touched but content unchanged) should not invalidate embeddings.
func TestStoreNote_PreservesEmbeddingOnSameHash(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "idx.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	vec := make([]float32, 384)
	vec[0] = 0.7
	encoded := index.EncodeEmbedding(vec)
	_, err = db.Exec(
		`INSERT INTO notes (id, path, title, hash, mtime, is_domain, embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "Title", "HASH_SAME", 100, true, encoded,
	)
	require.NoError(t, err)

	require.NoError(t, index.StoreNote(db, index.NoteRecord{
		ID:       "n1",
		Path:     "n1.md",
		Title:    "Title",
		BodyText: "same body",
		Hash:     "HASH_SAME",
		MTime:    300, // mtime changed but hash same
		IsDomain: true,
	}))

	var emb []byte
	err = db.QueryRow(`SELECT embedding FROM notes WHERE id = ?`, "n1").Scan(&emb)
	require.NoError(t, err)
	assert.Equal(t, encoded, emb, "embedding preserved when hash unchanged")
}
