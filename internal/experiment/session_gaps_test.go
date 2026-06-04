package experiment_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionGaps_FirstSessionHasNoPredecessor(t *testing.T) {
	db := openTestDB(t)
	start := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
		"s1", "/vault", start)
	require.NoError(t, err)

	gaps, err := db.SessionGaps()
	require.NoError(t, err)
	require.Len(t, gaps, 1)
	assert.Equal(t, "s1", gaps[0].SessionID)
	assert.False(t, gaps[0].PrevSessionID.Valid, "first session has no predecessor")
	assert.False(t, gaps[0].GapSeconds.Valid, "first session has no gap")
}

func TestSessionGaps_ComputesIntervalBetweenSessions(t *testing.T) {
	db := openTestDB(t)
	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	starts := []string{
		base.Format(time.RFC3339),                                   // s1
		base.Add(2 * time.Hour).Format(time.RFC3339),                // s2: +7200s
		base.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339), // s3: +1800s
	}
	for i, ts := range starts {
		_, err := db.Exec(`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
			[]string{"s1", "s2", "s3"}[i], "/vault", ts)
		require.NoError(t, err)
	}

	gaps, err := db.SessionGaps()
	require.NoError(t, err)
	require.Len(t, gaps, 3)

	// Chronological ascending.
	assert.Equal(t, "s1", gaps[0].SessionID)
	assert.Equal(t, "s2", gaps[1].SessionID)
	assert.Equal(t, "s3", gaps[2].SessionID)

	// Gaps.
	assert.False(t, gaps[0].GapSeconds.Valid)
	assert.True(t, gaps[1].GapSeconds.Valid)
	assert.Equal(t, int64(7200), gaps[1].GapSeconds.Int64)
	assert.True(t, gaps[2].GapSeconds.Valid)
	assert.Equal(t, int64(1800), gaps[2].GapSeconds.Int64)

	// Previous session ID threaded through.
	assert.Equal(t, "s1", gaps[1].PrevSessionID.String)
	assert.Equal(t, "s2", gaps[2].PrevSessionID.String)
}

func TestSessionGaps_IgnoresSessionStartOrderInsert(t *testing.T) {
	// Insert out-of-order; ordering must be by started_at, not insert order.
	db := openTestDB(t)
	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	// Insert s2 first, then s1
	_, err := db.Exec(`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
		"s2", "/vault", base.Add(time.Hour).Format(time.RFC3339))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
		"s1", "/vault", base.Format(time.RFC3339))
	require.NoError(t, err)

	gaps, err := db.SessionGaps()
	require.NoError(t, err)
	require.Len(t, gaps, 2)
	assert.Equal(t, "s1", gaps[0].SessionID, "ordering is by started_at ascending")
	assert.Equal(t, "s2", gaps[1].SessionID)
	assert.Equal(t, int64(3600), gaps[1].GapSeconds.Int64)
}
