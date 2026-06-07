// Package index_test — coverage gap tests for internal/index.
//
// Targets genuine uncovered behaviors identified in the 77.3 % baseline:
//
//   - ListAccessedNotesByCaller          (was 0%)
//   - DetectEmbeddingDimsCounts          (was 0%)
//   - StoreSparseEmbedding nonexistent   (was 71.4%)
//   - StoreColBERTEmbedding nonexistent  (was 71.4%)
//   - resolveCaller env-var-only path    (was 87.5%)
//   - containsHookSubstring short-str    (was 71.4%)
//   - ListAccessedNotesExcludingCaller empty→unfiltered delegation (was 66.7%)
//   - DeleteNoteByPath nonexistent path  (was 70%)
//   - AllNoteTitles on empty DB          (was 78.6%)
//
// Each test asserts real observable behavior — return values, error messages,
// database state — not just that lines execute.
package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// tempDB opens a fresh in-memory-equivalent temp-dir SQLite DB for one test.
func tempDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "idx.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedNote inserts a minimal note row into the given DB.
func seedNote(t *testing.T, db *index.DB, id, path string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, type, title, is_domain)
		 VALUES (?, ?, 'h', 0, 'concept', ?, true)`,
		id, path, id,
	)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// ListAccessedNotesByCaller
// ---------------------------------------------------------------------------

// ListAccessedNotesByCaller returns only events whose caller matches the
// given string. A note accessed by a different caller must not appear.
func TestListAccessedNotesByCaller_ReturnsOnlyMatchingCaller(t *testing.T) {
	db := tempDB(t)
	seedNote(t, db, "agent-note", "agent.md")
	seedNote(t, db, "hook-note", "hook.md")

	require.NoError(t, index.RecordNoteAccessAs(db, "agent-note", index.CallerAgent))
	require.NoError(t, index.RecordNoteAccessAs(db, "hook-note", index.CallerHook))

	// Ask for agent-only view — hook-note must be absent.
	results, err := index.ListAccessedNotesByCaller(db, index.CallerAgent)
	require.NoError(t, err)
	require.Len(t, results, 1, "only the agent-accessed note should appear")
	assert.Equal(t, "agent-note", results[0].NoteID)
	assert.Equal(t, 1, results[0].AccessCount)
}

// ListAccessedNotesByCaller with a caller that has no events returns an empty
// slice — callers must handle the zero-result path without error.
func TestListAccessedNotesByCaller_NoneForCallerReturnsEmpty(t *testing.T) {
	db := tempDB(t)
	seedNote(t, db, "n1", "n1.md")
	require.NoError(t, index.RecordNoteAccessAs(db, "n1", index.CallerAgent))

	// Hook has no events.
	results, err := index.ListAccessedNotesByCaller(db, index.CallerHook)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// ListAccessedNotesByCaller with an empty caller string behaves the same as
// ListAccessedNotes — it returns all accessed notes regardless of caller.
func TestListAccessedNotesByCaller_EmptyCallerReturnsAll(t *testing.T) {
	db := tempDB(t)
	seedNote(t, db, "x", "x.md")
	seedNote(t, db, "y", "y.md")
	require.NoError(t, index.RecordNoteAccessAs(db, "x", index.CallerAgent))
	require.NoError(t, index.RecordNoteAccessAs(db, "y", index.CallerHook))

	// Empty string = no caller filter.
	results, err := index.ListAccessedNotesByCaller(db, "")
	require.NoError(t, err)
	assert.Len(t, results, 2, "empty caller must not filter — returns all")
}

// ---------------------------------------------------------------------------
// DetectEmbeddingDimsCounts
// ---------------------------------------------------------------------------

// DetectEmbeddingDimsCounts returns an empty slice when no embeddings are stored.
func TestDetectEmbeddingDimsCounts_NoEmbeddingsReturnsEmpty(t *testing.T) {
	db := tempDB(t)
	counts, err := index.DetectEmbeddingDimsCounts(db)
	require.NoError(t, err)
	assert.Empty(t, counts, "empty vault should return zero EmbeddingDimsCount entries")
}

// DetectEmbeddingDimsCounts returns one entry when all notes use the same
// dimensionality — a consistent BGE-M3 (1024-dim) vault.
func TestDetectEmbeddingDimsCounts_SingleModelReturnsSingleEntry(t *testing.T) {
	db := tempDB(t)
	seedNote(t, db, "n1", "n1.md")
	seedNote(t, db, "n2", "n2.md")

	vec1024 := make([]float32, 1024)
	vec1024[0] = 1.0
	encodedDense := index.EncodeEmbedding(vec1024)
	encodedSparse := index.EncodeSparseEmbedding(map[int32]float32{0: 1.0})
	encodedColbert := index.EncodeColBERTEmbedding([][]float32{vec1024})

	for _, id := range []string{"n1", "n2"} {
		_, err := db.Exec(
			`UPDATE notes SET embedding = ?, sparse_embedding = ?, colbert_embedding = ? WHERE id = ?`,
			encodedDense, encodedSparse, encodedColbert, id,
		)
		require.NoError(t, err)
	}

	counts, err := index.DetectEmbeddingDimsCounts(db)
	require.NoError(t, err)
	require.Len(t, counts, 1, "all notes at same dims → exactly one entry")
	assert.Equal(t, 1024, counts[0].Dims)
	assert.Equal(t, 2, counts[0].Count)
}

// DetectEmbeddingDimsCounts returns two entries for a mixed-state vault
// (MiniLM 384-dim notes coexisting with BGE-M3 1024-dim notes).
func TestDetectEmbeddingDimsCounts_MixedModelReturnsTwoEntries(t *testing.T) {
	db := tempDB(t)
	seedNote(t, db, "old", "old.md")
	seedNote(t, db, "new", "new.md")

	vec384 := make([]float32, 384)
	vec384[0] = 1.0
	_, err := db.Exec(`UPDATE notes SET embedding = ? WHERE id = 'old'`, index.EncodeEmbedding(vec384))
	require.NoError(t, err)

	vec1024 := make([]float32, 1024)
	vec1024[0] = 1.0
	encodedDense := index.EncodeEmbedding(vec1024)
	encodedSparse := index.EncodeSparseEmbedding(map[int32]float32{0: 1.0})
	encodedColbert := index.EncodeColBERTEmbedding([][]float32{vec1024})
	_, err = db.Exec(
		`UPDATE notes SET embedding = ?, sparse_embedding = ?, colbert_embedding = ? WHERE id = 'new'`,
		encodedDense, encodedSparse, encodedColbert,
	)
	require.NoError(t, err)

	counts, err := index.DetectEmbeddingDimsCounts(db)
	require.NoError(t, err)
	assert.Len(t, counts, 2, "two distinct dims should yield two entries")

	dimSet := map[int]int{}
	for _, c := range counts {
		dimSet[c.Dims] = c.Count
	}
	assert.Contains(t, dimSet, 384, "MiniLM entry must be present")
	assert.Contains(t, dimSet, 1024, "BGE-M3 entry must be present")
}

// ---------------------------------------------------------------------------
// StoreSparseEmbedding / StoreColBERTEmbedding — nonexistent note error path
// ---------------------------------------------------------------------------

// StoreSparseEmbedding returns an error and includes a clear message when the
// note ID does not exist in the index.
func TestStoreSparseEmbedding_NonexistentNoteErrors(t *testing.T) {
	db := tempDB(t)
	err := index.StoreSparseEmbedding(db, "does-not-exist", map[int32]float32{1: 0.5})
	require.Error(t, err, "storing sparse embedding for nonexistent note must error")
	assert.Contains(t, err.Error(), "no note found", "error message must identify the cause")
}

// StoreColBERTEmbedding returns an error when the note ID does not exist.
func TestStoreColBERTEmbedding_NonexistentNoteErrors(t *testing.T) {
	db := tempDB(t)
	err := index.StoreColBERTEmbedding(db, "phantom", [][]float32{{0.1, 0.2}})
	require.Error(t, err, "storing ColBERT embedding for nonexistent note must error")
	assert.Contains(t, err.Error(), "no note found", "error message must identify the cause")
}

// ---------------------------------------------------------------------------
// resolveCaller — env-var-only path (non-hook env var, no explicit arg)
// ---------------------------------------------------------------------------

// When VAULTMIND_CALLER is set to a non-hook value and no explicit caller
// is passed (RecordNoteAccess, not RecordNoteAccessAs), the env var value
// is recorded verbatim in the event log.
func TestRecordNoteAccess_NonHookEnvVarUsedAsCaller(t *testing.T) {
	db := tempDB(t)
	seedNote(t, db, "env-note", "env.md")

	// Set a non-hook env var.  resolveCaller path: envCaller="custom-tool",
	// containsHookSubstring=false, explicit="", envCaller != "" → return envCaller.
	t.Setenv("VAULTMIND_CALLER", "custom-tool")
	require.NoError(t, index.RecordNoteAccess(db, "env-note"))

	// The event is in the log; ListAccessedNotesByCaller("custom-tool") finds it.
	results, err := index.ListAccessedNotesByCaller(db, "custom-tool")
	require.NoError(t, err)
	require.Len(t, results, 1, "event with env-var caller must be retrievable by that caller")
	assert.Equal(t, "env-note", results[0].NoteID)
}

// ---------------------------------------------------------------------------
// containsHookSubstring — short-string branch (len(s) < len("hook") = 4)
// ---------------------------------------------------------------------------

// When VAULTMIND_CALLER is set to a string shorter than 4 chars ("hook"
// cannot be a substring), it is used verbatim — not classified as hook.
func TestRecordNoteAccess_ShortEnvVarNotClassifiedAsHook(t *testing.T) {
	db := tempDB(t)
	seedNote(t, db, "short-env", "short.md")

	// "ok" has len 2 < len("hook") = 4 → containsHookSubstring returns false.
	t.Setenv("VAULTMIND_CALLER", "ok")
	require.NoError(t, index.RecordNoteAccess(db, "short-env"))

	// The note must appear in ListAccessedNotesByCaller("ok"), not "hook".
	byShort, err := index.ListAccessedNotesByCaller(db, "ok")
	require.NoError(t, err)
	assert.Len(t, byShort, 1, "short env-var caller must not be reclassified as hook")

	byHook, err := index.ListAccessedNotesByCaller(db, index.CallerHook)
	require.NoError(t, err)
	assert.Empty(t, byHook, "short env-var must not produce a hook event")
}

// ---------------------------------------------------------------------------
// ListAccessedNotesExcludingCaller — empty string delegates to ListAccessedNotes
// ---------------------------------------------------------------------------

// ListAccessedNotesExcludingCaller with an empty excluded caller returns the
// full unfiltered view (same as ListAccessedNotes). This covers the delegation
// branch at the top of the function.
func TestListAccessedNotesExcludingCaller_EmptyExcludedReturnsAll(t *testing.T) {
	db := tempDB(t)
	seedNote(t, db, "a", "a.md")
	seedNote(t, db, "b", "b.md")
	require.NoError(t, index.RecordNoteAccessAs(db, "a", index.CallerAgent))
	require.NoError(t, index.RecordNoteAccessAs(db, "b", index.CallerHook))

	// Empty string → no exclusion → all 2 accessed notes returned.
	results, err := index.ListAccessedNotesExcludingCaller(db, "")
	require.NoError(t, err)
	assert.Len(t, results, 2, "empty excludedCaller must not filter — returns all accessed notes")
}

// ---------------------------------------------------------------------------
// DeleteNoteByPath — nonexistent path error
// ---------------------------------------------------------------------------

// DeleteNoteByPath returns an error when the path does not exist in the index.
// The error must propagate rather than silently succeed.
func TestDeleteNoteByPath_NonexistentPathErrors(t *testing.T) {
	db := tempDB(t)
	// No notes in the DB.
	err := index.DeleteNoteByPath(db, "not-there.md")
	require.Error(t, err, "deleting a nonexistent note path must return an error")
	assert.Contains(t, err.Error(), "not-there.md", "error must reference the path that was not found")
}

// ---------------------------------------------------------------------------
// AllNoteTitles — empty DB returns empty slice (not error)
// ---------------------------------------------------------------------------

// AllNoteTitles on a freshly-opened empty DB returns an empty slice.
// Callers must handle the zero-result path without treating it as an error.
func TestAllNoteTitles_EmptyDBReturnsEmptySlice(t *testing.T) {
	db := tempDB(t)
	titles, err := db.AllNoteTitles()
	require.NoError(t, err)
	assert.Empty(t, titles, "empty index must return empty slice, not error")
}

// AllNoteTitles returns entries for every indexed note. Covers the iteration
// loop path with multiple notes and asserts that titles are returned correctly.
func TestAllNoteTitles_ReturnsAllIndexedNotes(t *testing.T) {
	db := tempDB(t)
	// Two domain notes with distinct titles.
	require.NoError(t, index.StoreNote(db, index.NoteRecord{
		ID: "concept-x", Path: "x.md", Title: "Explicit Title",
		Hash: "h1", MTime: 1, IsDomain: true,
	}))
	require.NoError(t, index.StoreNote(db, index.NoteRecord{
		ID: "concept-y", Path: "y.md", Title: "Another Title",
		Hash: "h2", MTime: 1, IsDomain: true,
	}))

	titles, err := db.AllNoteTitles()
	require.NoError(t, err)
	assert.Len(t, titles, 2, "both notes must appear in the result")

	byID := map[string]string{}
	for _, nt := range titles {
		byID[nt.ID] = nt.Title
	}
	assert.Equal(t, "Explicit Title", byID["concept-x"])
	assert.Equal(t, "Another Title", byID["concept-y"])
}
