package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAccessTestDB opens an empty index and seeds the note rows the access
// recorder requires — RecordNoteAccessAs inserts via SELECT FROM notes, so an
// unseeded id records nothing and a test would pass by writing no rows at all.
func newAccessTestDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "access.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for _, id := range []string{"note-a", "note-b", "note-c", "legacy-agent", "legacy-hook"} {
		_, err := db.Exec(
			`INSERT INTO notes (id, path, title, type, hash, mtime) VALUES (?, ?, ?, 'reference', 'h', 0)`,
			id, id+".md", id)
		require.NoError(t, err)
	}
	return db
}

// `vaultmind self` answers "what have I actually engaged with". It answered it
// by EXCLUDING the hook and neighbour callers, which worked only because those
// callers never delivered any note text — caller was a proxy for delivery.
//
// The delivery work made both of them deliver, and the proxy inverted: on a live
// 4,014-row ledger `self` reported 47 accessed notes, excluding 87% of rows, a
// growing share of them genuine reads. It got worse the better delivery worked.
//
// Caller cannot be repaired into a delivery signal — resolveCaller collapses
// anything containing "hook" to CallerHook, overriding the explicit
// target/neighbour labels the ask path passes. So delivery is recorded directly
// and asked directly.
func TestListDeliveredNotes_UsesDeliveryNotCaller(t *testing.T) {
	db := newAccessTestDB(t)

	// A hook access that DID deliver — the case the caller filter now gets wrong.
	require.NoError(t, index.RecordNoteAccessDelivered(db, "note-a", index.CallerHook, true))
	// A hook access that did not.
	require.NoError(t, index.RecordNoteAccessDelivered(db, "note-b", index.CallerHook, false))
	// A neighbour fan-out that delivered an excerpt.
	require.NoError(t, index.RecordNoteAccessDelivered(db, "note-c", index.CallerAgentNeighbor, true))

	stats, err := index.ListDeliveredNotes(db)
	require.NoError(t, err)

	got := map[string]bool{}
	for _, s := range stats {
		got[s.NoteID] = true
	}

	assert.True(t, got["note-a"], "a hook access that delivered content IS a read, whoever fired it")
	assert.True(t, got["note-c"], "so is a neighbour whose excerpt was handed over")
	assert.False(t, got["note-b"], "a hook access that delivered nothing is not a read")
}

// Rows written before delivery tracking existed carry NULL, and NULL is not 0.
// For those rows the caller heuristic WAS correct — a hook row really did
// deliver nothing — so history keeps its original meaning rather than being
// retroactively asserted either way.
func TestListDeliveredNotes_LegacyRowsFallBackToCallerMeaning(t *testing.T) {
	db := newAccessTestDB(t)

	// Written the pre-migration way: no delivery recorded at all.
	require.NoError(t, index.RecordNoteAccessAs(db, "legacy-agent", index.CallerAgent))
	require.NoError(t, index.RecordNoteAccessAs(db, "legacy-hook", index.CallerHook))

	stats, err := index.ListDeliveredNotes(db)
	require.NoError(t, err)

	got := map[string]bool{}
	for _, s := range stats {
		got[s.NoteID] = true
	}

	assert.True(t, got["legacy-agent"],
		"a pre-migration agent access was a deliberate read; NULL must not erase it")
	assert.False(t, got["legacy-hook"],
		"a pre-migration hook access delivered nothing — that is what caller meant at the time")
}

// The delivery bit must survive the round-trip, or the ledger is recording a
// value nothing can read — the defect this whole line of work exists to remove.
func TestRecordNoteAccessDelivered_PersistsTheBit(t *testing.T) {
	db := newAccessTestDB(t)
	require.NoError(t, index.RecordNoteAccessDelivered(db, "note-a", index.CallerAgent, true))
	require.NoError(t, index.RecordNoteAccessDelivered(db, "note-b", index.CallerAgent, false))

	var delivered, withheld int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM note_accesses WHERE note_id = 'note-a' AND body_delivered = 1`).Scan(&delivered))
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM note_accesses WHERE note_id = 'note-b' AND body_delivered = 0`).Scan(&withheld))

	assert.Equal(t, 1, delivered)
	assert.Equal(t, 1, withheld)
}
