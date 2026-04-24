package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openFreshDB opens an empty index DB with all migrations applied. The
// trigger-behavior tests below need SQL-level write access without going
// through the indexer (which we want to freely modify), so they bypass
// NewIndexer and talk straight to the store via INSERT/UPDATE.
func openFreshDB(t *testing.T) *index.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "idx.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// insertBareNote seeds a note row with no embeddings so subsequent UPDATEs
// exercise the parity trigger from the "upgrade to embedded" direction the
// indexer actually walks.
func insertBareNote(t *testing.T, db *index.DB, id string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain) VALUES (?, ?, ?, ?, ?, ?)`,
		id, id+".md", "h", 0, "T", true,
	)
	require.NoError(t, err)
}

// The parity trigger must reject any write that leaves a note with a BGE-M3
// dense embedding (1024 float32 = 4096 bytes) and a NULL sparse_embedding.
// This is the exact state the 2026-04-24 ranking bug's 8 notes were in, and
// the state the mean-of-present retriever fix only half-cures. Enforcing at
// the write boundary closes the class.
func TestModalityParityTrigger_RejectsBGEM3DenseWithoutSparse(t *testing.T) {
	db := openFreshDB(t)
	insertBareNote(t, db, "n1")

	bgem3Dense := make([]byte, 4096) // 1024 float32 = BGE-M3 dims
	_, err := db.Exec(`UPDATE notes SET embedding = ? WHERE id = ?`, bgem3Dense, "n1")
	require.Error(t, err, "BGE-M3 dense without sparse must be refused at the schema level")
	assert.Contains(t, err.Error(), "modality", "error must name the failure mode")
}

// Same invariant via the colbert lane: BGE-M3 dense without colbert is
// equally invalid. Both lanes matter; one missing is enough to compress
// hybrid RRF ranking.
func TestModalityParityTrigger_RejectsBGEM3DenseWithoutColbert(t *testing.T) {
	db := openFreshDB(t)
	insertBareNote(t, db, "n1")

	bgem3Dense := make([]byte, 4096)
	sparse := []byte{0x01, 0x02, 0x03}
	_, err := db.Exec(`UPDATE notes SET embedding = ?, sparse_embedding = ? WHERE id = ?`,
		bgem3Dense, sparse, "n1")
	require.Error(t, err, "BGE-M3 dense + sparse but no colbert must be refused")
}

// The trigger must let a fully-populated BGE-M3 row through. Otherwise it
// blocks the happy path and the whole embed workflow breaks.
func TestModalityParityTrigger_AcceptsFullBGEM3(t *testing.T) {
	db := openFreshDB(t)
	insertBareNote(t, db, "n1")

	bgem3Dense := make([]byte, 4096)
	sparse := []byte{0x01}
	colbert := make([]byte, 4096)

	_, err := db.Exec(
		`UPDATE notes SET embedding = ?, sparse_embedding = ?, colbert_embedding = ? WHERE id = ?`,
		bgem3Dense, sparse, colbert, "n1",
	)
	require.NoError(t, err, "atomic all-three write must succeed")
}

// MiniLM dense is 384 float32 = 1536 bytes. The sparse/colbert columns
// don't apply to MiniLM, so those vaults must continue to store dense-only
// without the trigger firing. This is the backward-compatibility contract.
func TestModalityParityTrigger_AcceptsMiniLMDenseAlone(t *testing.T) {
	db := openFreshDB(t)
	insertBareNote(t, db, "n1")

	minilmDense := make([]byte, 1536) // 384 float32
	_, err := db.Exec(`UPDATE notes SET embedding = ? WHERE id = ?`, minilmDense, "n1")
	require.NoError(t, err, "MiniLM dense without sparse/colbert must remain valid")
}

// NULL embedding (unembedded note) is always valid. The trigger must not
// block the upsert path that clears stale embeddings on content change.
func TestModalityParityTrigger_AcceptsAllNull(t *testing.T) {
	db := openFreshDB(t)
	insertBareNote(t, db, "n1")

	// Redundant but explicit: re-NULL everything.
	_, err := db.Exec(
		`UPDATE notes SET embedding = NULL, sparse_embedding = NULL, colbert_embedding = NULL WHERE id = ?`,
		"n1",
	)
	require.NoError(t, err, "all-null remains valid (unembedded note)")
}

// The upsert path in store.go clears all three embeddings on content change.
// After the trigger lands, that behavior must still work — otherwise every
// edit to any note would fail the parity check. Guards the regression.
func TestModalityParityTrigger_UpsertClearOnContentChangeStillWorks(t *testing.T) {
	db := openFreshDB(t)
	insertBareNote(t, db, "n1")

	// Fill in a valid full-BGE-M3 row.
	bgem3Dense := make([]byte, 4096)
	sparse := []byte{0x01}
	colbert := make([]byte, 4096)
	_, err := db.Exec(
		`UPDATE notes SET embedding = ?, sparse_embedding = ?, colbert_embedding = ? WHERE id = ?`,
		bgem3Dense, sparse, colbert, "n1",
	)
	require.NoError(t, err)

	// Now simulate the upsert-on-content-change: clear all three atomically.
	_, err = db.Exec(
		`UPDATE notes SET embedding = NULL, sparse_embedding = NULL, colbert_embedding = NULL WHERE id = ?`,
		"n1",
	)
	require.NoError(t, err, "content-change clearout path must remain valid")
}

// Direct INSERT of a violating row (BGE-M3 dense + NULL sparse) must also be
// rejected — the trigger must cover BEFORE INSERT as well as BEFORE UPDATE.
// A bug here would let seed scripts or tests sneak invalid state in.
func TestModalityParityTrigger_RejectsInsertViolation(t *testing.T) {
	db := openFreshDB(t)

	bgem3Dense := make([]byte, 4096)
	_, err := db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"bad", "bad.md", "h", 0, "T", true, bgem3Dense,
	)
	require.Error(t, err, "INSERT of BGE-M3 dense without sparse/colbert must be rejected")
}
