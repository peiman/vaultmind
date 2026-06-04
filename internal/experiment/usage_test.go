package experiment_test

import (
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageSummary_EmptyDBReturnsZeros(t *testing.T) {
	db := openTestDB(t)

	got, err := db.UsageSummary(10)
	require.NoError(t, err)
	assert.Equal(t, 0, got.TotalSessions)
	assert.Equal(t, 0, got.RetrievalEventCount)
	assert.Equal(t, 0, got.UniqueNotesRecalled)
	assert.Empty(t, got.TopNotes)
	assert.Equal(t, 0, got.GapStats.Count)
}

func TestUsageSummary_AggregatesAcrossSessionsAndEvents(t *testing.T) {
	db := openTestDB(t)

	// Two sessions 30 minutes apart.
	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	_, err := db.Exec(`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
		"s1", "/vault", base.Format(time.RFC3339))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
		"s2", "/vault", base.Add(30*time.Minute).Format(time.RFC3339))
	require.NoError(t, err)

	// 3 retrieval events: s1 surfaces n1+n2, s1 surfaces n1, s2 surfaces n3.
	for i, hits := range [][]experiment.RetrievalHit{
		{{NoteID: "n1", Rank: 1}, {NoteID: "n2", Rank: 2}},
		{{NoteID: "n1", Rank: 1}},
		{{NoteID: "n3", Rank: 1}},
	} {
		ts := base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
		sid := "s1"
		if i == 2 {
			sid = "s2"
		}
		seedRetrievalEvent(t, db, sid, "search", ts, map[string][]experiment.RetrievalHit{"hybrid": hits})
	}

	got, err := db.UsageSummary(10)
	require.NoError(t, err)
	assert.Equal(t, 2, got.TotalSessions)
	assert.Equal(t, 3, got.RetrievalEventCount)
	assert.Equal(t, 3, got.UniqueNotesRecalled, "n1, n2, n3 each appeared at least once")
	require.Len(t, got.TopNotes, 3)
	assert.Equal(t, "n1", got.TopNotes[0].NoteID, "n1 retrieved twice ranks first")
	assert.Equal(t, 2, got.TopNotes[0].RetrievalCountTotal)
}

func TestUsageSummary_TopNLimits(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	// 5 unique notes, each retrieved once
	for i, id := range []string{"a", "b", "c", "d", "e"} {
		ts := base.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		seedRetrievalEvent(t, db, sid, "search", ts, map[string][]experiment.RetrievalHit{
			"hybrid": {{NoteID: id, Rank: 1}},
		})
	}

	got, err := db.UsageSummary(3)
	require.NoError(t, err)
	assert.Equal(t, 5, got.UniqueNotesRecalled)
	assert.Len(t, got.TopNotes, 3, "TopNotes respects the requested limit")
}

func TestUsageSummary_ComputesSessionGapStats(t *testing.T) {
	db := openTestDB(t)
	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	// 4 sessions at +0, +1, +4, +9 minutes → gaps of 60, 180, 300 seconds.
	for i, s := range []string{"s1", "s2", "s3", "s4"} {
		_, err := db.Exec(`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
			s, "/vault", base.Add(time.Duration(i*i)*time.Minute).Format(time.RFC3339))
		require.NoError(t, err)
	}

	got, err := db.UsageSummary(0)
	require.NoError(t, err)
	assert.Equal(t, 3, got.GapStats.Count)
	assert.Equal(t, int64(180), got.GapStats.MedianSeconds, "median of {60,180,300}")
	assert.Equal(t, int64(300), got.GapStats.MaxSeconds)
	// P90 of 3 values: ceil(0.9*3) = 3 → sorted[2] = 300
	assert.Equal(t, int64(300), got.GapStats.P90Seconds)
}

func TestUsageSummary_GapStatsEmptyWhenSingleSession(t *testing.T) {
	db := openTestDB(t)
	_, err := db.StartSession("/vault")
	require.NoError(t, err)

	got, err := db.UsageSummary(10)
	require.NoError(t, err)
	assert.Equal(t, 1, got.TotalSessions)
	assert.Equal(t, 0, got.GapStats.Count, "one session → no gaps")
	assert.Equal(t, int64(0), got.GapStats.MedianSeconds)
}
