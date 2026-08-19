package experiment_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scorer's inputs are the fix's real surface. IsActivationSignal being
// correct is worth nothing if the queries that feed the scorer never call it —
// that is the same "a value computed and never used" shape as the rest of this
// class of bug.
func seedAccessEvents(t *testing.T) *experiment.DB {
	t.Helper()
	db, err := experiment.OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	sessionID, err := db.StartSession("/v")
	require.NoError(t, err)
	session := &experiment.Session{DB: db, ID: sessionID, VaultPath: "/v"}

	// A real read.
	_, err = session.LogNoteAccessEvent("real-note", experiment.AccessSourceRead, true)
	require.NoError(t, err)
	// A phantom: ranked first, body withheld. This is what pointers-only produced
	// on every hook firing.
	_, err = session.LogNoteAccessEvent("phantom-note", experiment.AccessSourceAsk, false)
	require.NoError(t, err)
	// Neighbour expansion — selected, never shown.
	_, err = session.LogNoteAccessEvent("neighbour-note", experiment.AccessSourceNeighbors, false)
	require.NoError(t, err)
	// An ask that DID hand over a body: a genuine retrieval.
	_, err = session.LogNoteAccessEvent("delivered-note", experiment.AccessSourceAsk, true)
	require.NoError(t, err)

	return db
}

func TestAccessedNoteIDs_ExcludesPhantoms(t *testing.T) {
	db := seedAccessEvents(t)

	ids, err := db.AccessedNoteIDs()
	require.NoError(t, err)

	assert.Contains(t, ids, "real-note")
	assert.Contains(t, ids, "delivered-note", "an ask that delivered its body is a genuine retrieval")
	assert.NotContains(t, ids, "phantom-note",
		"ranking first is not being read — counting it is what made ranking self-reinforcing")
	assert.NotContains(t, ids, "neighbour-note",
		"the pack selected it; the agent saw a title at most")
}

func TestNoteAccessTimes_ExcludesPhantoms(t *testing.T) {
	db := seedAccessEvents(t)

	phantom, err := db.NoteAccessTimes("phantom-note")
	require.NoError(t, err)
	assert.Empty(t, phantom, "a withheld body must contribute no recency signal")

	real, err := db.NoteAccessTimes("real-note")
	require.NoError(t, err)
	assert.Len(t, real, 1)

	delivered, err := db.NoteAccessTimes("delivered-note")
	require.NoError(t, err)
	assert.Len(t, delivered, 1)
}

// Events written before body_delivered existed carry no such field. Absence must
// read as "not proven delivered" — the entire reason this went unnoticed is that
// a missing value was treated as a benign one.
func TestNoteAccessTimes_LegacyAskEventsDoNotCount(t *testing.T) {
	db, err := experiment.OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	sessionID, err := db.StartSession("/v")
	require.NoError(t, err)

	// Written the way the pre-fix code wrote it: source only, no delivery field.
	_, err = db.LogEvent(experiment.Event{
		SessionID: sessionID,
		Type:      experiment.EventNoteAccess,
		VaultPath: "/v",
		Data:      map[string]any{"note_id": "legacy-note", "source": experiment.AccessSourceAsk},
	})
	require.NoError(t, err)

	times, err := db.NoteAccessTimes("legacy-note")
	require.NoError(t, err)
	assert.Empty(t, times,
		"every pre-fix ask event looks like this; if absence counted as delivery the "+
			"historical phantoms would stay in the ranking forever")
}
