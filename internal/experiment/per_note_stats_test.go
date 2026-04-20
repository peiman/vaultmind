package experiment_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedRetrievalEvent inserts a retrieval event whose event_data carries the
// given variant hits. Returns the event ID.
func seedRetrievalEvent(t *testing.T, db *experiment.DB, sid, eventType, ts string, variants map[string][]experiment.RetrievalHit) {
	t.Helper()
	variantMap := map[string]any{}
	for name, hits := range variants {
		variantMap[name] = experiment.BuildVariantPayload(name, hits)[name]
	}
	payload, err := json.Marshal(map[string]any{"variants": variantMap})
	require.NoError(t, err)

	_, err = db.Exec(
		`INSERT INTO events (event_id, session_id, event_type, timestamp, vault_path, event_data)
		 VALUES (hex(randomblob(8)), ?, ?, ?, ?, ?)`,
		sid, eventType, ts, "/vault", string(payload),
	)
	require.NoError(t, err)
}

func TestPerNoteStats_CountsDistinctEventsPerNote(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	// n1 retrieved in 3 events, n2 in 1 event, n3 in 2 events.
	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	for i, hits := range [][]experiment.RetrievalHit{
		{{NoteID: "n1", Rank: 1}, {NoteID: "n2", Rank: 2}},
		{{NoteID: "n1", Rank: 1}, {NoteID: "n3", Rank: 2}},
		{{NoteID: "n1", Rank: 1}, {NoteID: "n3", Rank: 2}},
	} {
		ts := base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
		seedRetrievalEvent(t, db, sid, "search", ts, map[string][]experiment.RetrievalHit{"hybrid": hits})
	}

	stats, err := db.PerNoteStats()
	require.NoError(t, err)

	byID := indexByNoteID(stats)
	require.Contains(t, byID, "n1")
	require.Contains(t, byID, "n2")
	require.Contains(t, byID, "n3")
	assert.Equal(t, 3, byID["n1"].RetrievalCountTotal)
	assert.Equal(t, 1, byID["n2"].RetrievalCountTotal)
	assert.Equal(t, 2, byID["n3"].RetrievalCountTotal)
}

func TestPerNoteStats_DoesNotInflateCountAcrossVariants(t *testing.T) {
	// Ask events emit the same note under multiple variants (retrieval mode +
	// shadow variants). The count must be per-event, not per-variant-occurrence.
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	hits := []experiment.RetrievalHit{{NoteID: "shared-note", Rank: 1}}
	ts := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	seedRetrievalEvent(t, db, sid, "ask", ts, map[string][]experiment.RetrievalHit{
		"hybrid":         hits,
		"compressed-0.2": hits,
		"wall-clock":     hits,
	})

	stats, err := db.PerNoteStats()
	require.NoError(t, err)

	byID := indexByNoteID(stats)
	require.Contains(t, byID, "shared-note")
	assert.Equal(t, 1, byID["shared-note"].RetrievalCountTotal,
		"single event should count once even when the note appears in multiple variants")
}

func TestPerNoteStats_RecordsFirstAndLastTimestamp(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	first := base.Format(time.RFC3339)
	last := base.Add(2 * time.Hour).Format(time.RFC3339)
	middle := base.Add(1 * time.Hour).Format(time.RFC3339)

	for _, ts := range []string{first, middle, last} {
		seedRetrievalEvent(t, db, sid, "search", ts,
			map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "n1", Rank: 1}}})
	}

	stats, err := db.PerNoteStats()
	require.NoError(t, err)
	byID := indexByNoteID(stats)
	assert.Equal(t, first, byID["n1"].FirstRetrievedTs)
	assert.Equal(t, last, byID["n1"].LastRetrievedTs)
}

func TestPerNoteStats_IgnoresNonRetrievalEvents(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	// A retrieval event and a note_access event — only the first should count.
	ts := time.Now().UTC().Format(time.RFC3339)
	seedRetrievalEvent(t, db, sid, "search", ts,
		map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "n1", Rank: 1}}})
	_, err = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventNoteAccess, VaultPath: "/vault",
		Data: map[string]any{"note_id": "n99", "source": "recall"},
	})
	require.NoError(t, err)

	stats, err := db.PerNoteStats()
	require.NoError(t, err)
	byID := indexByNoteID(stats)
	assert.Contains(t, byID, "n1")
	assert.NotContains(t, byID, "n99", "note_access events are not retrievals")
}

func TestPerNoteStats_OrdersByCountDescThenByLastRetrievedDesc(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	// a: 3 retrievals
	for i := 0; i < 3; i++ {
		ts := base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
		seedRetrievalEvent(t, db, sid, "search", ts, map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "a", Rank: 1}}})
	}
	// b: 1 retrieval, but most recent
	seedRetrievalEvent(t, db, sid, "search", base.Add(time.Hour).Format(time.RFC3339),
		map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "b", Rank: 1}}})
	// c: 1 retrieval, oldest
	seedRetrievalEvent(t, db, sid, "search", base.Add(-time.Hour).Format(time.RFC3339),
		map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "c", Rank: 1}}})

	stats, err := db.PerNoteStats()
	require.NoError(t, err)
	require.Len(t, stats, 3)
	// Most-retrieved first
	assert.Equal(t, "a", stats[0].NoteID)
	// Ties broken by last-retrieved desc: b (newer) before c (older)
	assert.Equal(t, "b", stats[1].NoteID)
	assert.Equal(t, "c", stats[2].NoteID)
}

func indexByNoteID(stats []experiment.NoteStat) map[string]experiment.NoteStat {
	m := make(map[string]experiment.NoteStat, len(stats))
	for _, s := range stats {
		m[s.NoteID] = s
	}
	return m
}
