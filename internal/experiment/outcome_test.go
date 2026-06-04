package experiment_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedSearchEvent inserts an event row directly for test setup.
func seedSearchEvent(t *testing.T, db *experiment.DB, sessionID, eventID string, data map[string]any) {
	t.Helper()
	dataJSON, err := json.Marshal(data)
	require.NoError(t, err)
	ts := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(
		`INSERT INTO events
		   (event_id, session_id, event_type, timestamp, vault_path, event_data)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		eventID, sessionID, experiment.EventSearch, ts, "/tmp/vault", string(dataJSON),
	)
	require.NoError(t, err)
}

// seedSession inserts a session row directly for test setup.
func seedSession(t *testing.T, db *experiment.DB, sessionID, startedAt string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
		sessionID, "/tmp/vault", startedAt,
	)
	require.NoError(t, err)
}

// variantData builds the expected event_data JSON shape with one variant.
func variantData(variantName string, results []map[string]any) map[string]any {
	return map[string]any{
		"variants": map[string]any{
			variantName: map[string]any{
				"results": results,
			},
		},
	}
}

func TestLinkOutcomes_MatchesNoteInResults(t *testing.T) {
	db := openTestExpDB(t)

	sessionID, err := db.StartSession("/tmp/vault")
	require.NoError(t, err)

	data := variantData("dense", []map[string]any{
		{"note_id": "note-abc", "rank": 1},
		{"note_id": "note-xyz", "rank": 2},
	})
	seedSearchEvent(t, db, sessionID, "event-001", data)

	count, err := db.LinkOutcomes(sessionID, "note-abc", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify the outcome row contents.
	var noteID, variant string
	var rank int
	err = db.QueryRow(
		`SELECT note_id, variant, rank FROM outcomes WHERE event_id = ?`, "event-001",
	).Scan(&noteID, &variant, &rank)
	require.NoError(t, err)
	assert.Equal(t, "note-abc", noteID)
	assert.Equal(t, "dense", variant)
	assert.Equal(t, 1, rank)
}

func TestLinkOutcomes_NoMatchReturnsZero(t *testing.T) {
	db := openTestExpDB(t)

	sessionID, err := db.StartSession("/tmp/vault")
	require.NoError(t, err)

	data := variantData("dense", []map[string]any{
		{"note_id": "note-abc", "rank": 1},
	})
	seedSearchEvent(t, db, sessionID, "event-002", data)

	count, err := db.LinkOutcomes(sessionID, "note-not-in-results", 1)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestLinkOutcomes_MultipleVariants(t *testing.T) {
	db := openTestExpDB(t)

	sessionID, err := db.StartSession("/tmp/vault")
	require.NoError(t, err)

	data := map[string]any{
		"variants": map[string]any{
			"dense": map[string]any{
				"results": []map[string]any{
					{"note_id": "note-target", "rank": 1},
					{"note_id": "note-other", "rank": 2},
				},
			},
			"sparse": map[string]any{
				"results": []map[string]any{
					{"note_id": "note-other", "rank": 1},
					{"note_id": "note-target", "rank": 3},
				},
			},
		},
	}
	seedSearchEvent(t, db, sessionID, "event-003", data)

	count, err := db.LinkOutcomes(sessionID, "note-target", 1)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Verify ranks per variant.
	rows, err := db.Query(
		`SELECT variant, rank FROM outcomes WHERE event_id = ? ORDER BY variant`,
		"event-003",
	)
	require.NoError(t, err)
	defer rows.Close()

	type outcome struct {
		variant string
		rank    int
	}
	var got []outcome
	for rows.Next() {
		var o outcome
		require.NoError(t, rows.Scan(&o.variant, &o.rank))
		got = append(got, o)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []outcome{
		{variant: "dense", rank: 1},
		{variant: "sparse", rank: 3},
	}, got)
}

func TestLinkOutcomes_LooksBackPreviousSessions(t *testing.T) {
	db := openTestExpDB(t)

	// Session 1 (older) has a search event containing the note.
	seedSession(t, db, "session-old", "2024-01-01T10:00:00Z")
	data := variantData("hybrid", []map[string]any{
		{"note_id": "note-found", "rank": 2},
	})
	seedSearchEvent(t, db, "session-old", "event-old-001", data)

	// Session 2 (current) has the note_access event.
	seedSession(t, db, "session-current", "2024-01-01T11:00:00Z")

	// With window=2, both sessions are within scope.
	count, err := db.LinkOutcomes("session-current", "note-found", 2)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestLinkOutcomes_IgnoresNoteAccessEvents(t *testing.T) {
	db := openTestExpDB(t)

	sessionID, err := db.StartSession("/tmp/vault")
	require.NoError(t, err)

	// Insert a note_access event (not a search/ask/context_pack event).
	noteAccessData := map[string]any{
		"variants": map[string]any{
			"dense": map[string]any{
				"results": []map[string]any{
					{"note_id": "note-target", "rank": 1},
				},
			},
		},
	}
	dataJSON, err := json.Marshal(noteAccessData)
	require.NoError(t, err)
	ts := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(
		`INSERT INTO events
		   (event_id, session_id, event_type, timestamp, vault_path, event_data)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"event-note-access", sessionID, experiment.EventNoteAccess, ts, "/tmp/vault", string(dataJSON),
	)
	require.NoError(t, err)

	// Also insert an index_embed event with variants shape (should also be ignored).
	_, err = db.Exec(
		`INSERT INTO events
		   (event_id, session_id, event_type, timestamp, vault_path, event_data)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"event-index-embed", sessionID, experiment.EventIndexEmbed, ts, "/tmp/vault", string(dataJSON),
	)
	require.NoError(t, err)

	count, err := db.LinkOutcomes(sessionID, "note-target", 1)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "note_access and index_embed events must not be sources for outcome linkage")
}
