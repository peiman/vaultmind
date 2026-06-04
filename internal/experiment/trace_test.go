package experiment_test

import (
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionRetrievals_ReturnsEventsInChronologicalOrder(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	// Three events at +0, +5, +10 seconds. Inserting in reverse so the return
	// order proves we're ordering by timestamp, not insert order.
	seedRetrievalEvent(t, db, sid, "ask", base.Add(10*time.Second).Format(time.RFC3339),
		map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "c", Rank: 1}}})
	seedRetrievalEvent(t, db, sid, "search", base.Add(5*time.Second).Format(time.RFC3339),
		map[string][]experiment.RetrievalHit{"keyword": {{NoteID: "b", Rank: 1}, {NoteID: "b2", Rank: 2}}})
	seedRetrievalEvent(t, db, sid, "ask", base.Format(time.RFC3339),
		map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "a", Rank: 1}}})

	got, err := db.SessionRetrievals(sid)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "ask", got[0].EventType)
	assert.Equal(t, "a", got[0].Hits[0].NoteID)
	assert.Equal(t, "search", got[1].EventType)
	require.Len(t, got[1].Hits, 2)
	assert.Equal(t, "b", got[1].Hits[0].NoteID)
	assert.Equal(t, 1, got[1].Hits[0].Rank)
	assert.Equal(t, "b2", got[1].Hits[1].NoteID)
	assert.Equal(t, "c", got[2].Hits[0].NoteID)
}

func TestSessionRetrievals_DeduplicatesNotesAcrossVariants(t *testing.T) {
	// An ask with shadow variants emits the same note under several variant
	// keys. SessionRetrievals collapses them so one note appears once per
	// event (with its best rank).
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	ts := time.Now().UTC().Format(time.RFC3339)
	hits := []experiment.RetrievalHit{{NoteID: "shared", Rank: 3}}
	seedRetrievalEvent(t, db, sid, "ask", ts, map[string][]experiment.RetrievalHit{
		"hybrid":         hits,
		"compressed-0.2": hits,
		"wall-clock":     hits,
	})

	got, err := db.SessionRetrievals(sid)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Len(t, got[0].Hits, 1, "shadow variants should not inflate the hit list per event")
	assert.Equal(t, "shared", got[0].Hits[0].NoteID)
}

func TestSessionRetrievals_UnknownSessionReturnsEmpty(t *testing.T) {
	db := openTestDB(t)

	got, err := db.SessionRetrievals("does-not-exist")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestNoteRetrievals_ReturnsEachOccurrenceWithSessionAndTimestamp(t *testing.T) {
	db := openTestDB(t)
	s1, err := db.StartSession("/vault")
	require.NoError(t, err)
	s2, err := db.StartSession("/vault")
	require.NoError(t, err)

	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	// Target note retrieved from two different sessions, different ranks.
	seedRetrievalEvent(t, db, s1, "ask", base.Format(time.RFC3339),
		map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "target", Rank: 2}}})
	seedRetrievalEvent(t, db, s2, "search", base.Add(time.Minute).Format(time.RFC3339),
		map[string][]experiment.RetrievalHit{"keyword": {{NoteID: "target", Rank: 1}}})
	// A decoy event that doesn't mention target.
	seedRetrievalEvent(t, db, s1, "ask", base.Add(30*time.Second).Format(time.RFC3339),
		map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "other", Rank: 1}}})

	got, err := db.NoteRetrievals("target")
	require.NoError(t, err)
	require.Len(t, got, 2, "target appears in two sessions; decoy should not show up")
	// Chronological order.
	assert.Equal(t, s1, got[0].SessionID)
	assert.Equal(t, 2, got[0].Rank)
	assert.Equal(t, s2, got[1].SessionID)
	assert.Equal(t, 1, got[1].Rank)
}

func TestNoteRetrievals_UnknownNoteReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	seedRetrievalEvent(t, db, sid, "ask", time.Now().UTC().Format(time.RFC3339),
		map[string][]experiment.RetrievalHit{"hybrid": {{NoteID: "a", Rank: 1}}})

	got, err := db.NoteRetrievals("nonexistent")
	require.NoError(t, err)
	assert.Empty(t, got)
}
