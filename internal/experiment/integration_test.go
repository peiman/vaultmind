package experiment_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_FullFlow(t *testing.T) {
	db := openTestExpDB(t)

	// 1. Start session
	sessionID, err := db.StartSession("/vault")
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sessionID, VaultPath: "/vault"}
	ctx := experiment.WithSession(context.Background(), session)

	// 2. Verify context round-trip
	extracted := experiment.FromContext(ctx)
	require.NotNil(t, extracted)
	assert.Equal(t, sessionID, extracted.ID)

	// 3. Log a search event with variant data
	searchData := map[string]any{
		"variants": map[string]any{
			"none": map[string]any{
				"results": []any{
					map[string]any{"note_id": "concept-spreading-activation", "rank": float64(1)},
					map[string]any{"note_id": "concept-memory-consolidation", "rank": float64(2)},
				},
			},
		},
	}
	searchEventID, err := session.LogSearchEvent("spreading activation", "hybrid", searchData)
	require.NoError(t, err)
	assert.NotEmpty(t, searchEventID)

	// 4. Log note access (triggers outcome linkage)
	accessEventID, err := session.LogNoteAccessEvent("concept-spreading-activation", "note_get")
	require.NoError(t, err)
	assert.NotEmpty(t, accessEventID)

	// 5. Verify outcome was created
	var outcomeCount int
	err = db.QueryRow("SELECT COUNT(*) FROM outcomes").Scan(&outcomeCount)
	require.NoError(t, err)
	assert.Equal(t, 1, outcomeCount)

	var noteID, variant string
	var rank int
	err = db.QueryRow("SELECT note_id, variant, rank FROM outcomes").Scan(&noteID, &variant, &rank)
	require.NoError(t, err)
	assert.Equal(t, "concept-spreading-activation", noteID)
	assert.Equal(t, "none", variant)
	assert.Equal(t, 1, rank)

	// 6. End session
	err = db.EndSession(sessionID)
	require.NoError(t, err)

	// 7. Generate report
	report, err := db.Report([]string{"none"}, 5)
	require.NoError(t, err)
	assert.Equal(t, 1, report.SessionCount)
	assert.Equal(t, 1, report.EventCount) // only search, not note_access
	assert.Equal(t, 1, report.OutcomeCount)
	assert.InDelta(t, 1.0, report.Variants["none"].HitAtK, 0.01)
	assert.InDelta(t, 1.0, report.Variants["none"].MRR, 0.01)
}

func TestIntegration_OrphanRecovery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "experiments.db")

	db, err := experiment.Open(dbPath)
	require.NoError(t, err)

	sessionID, err := db.StartSession("/vault")
	require.NoError(t, err)

	_, err = db.LogEvent(experiment.Event{
		SessionID: sessionID,
		Type:      experiment.EventSearch,
		VaultPath: "/vault",
		Data:      map[string]any{},
	})
	require.NoError(t, err)

	// Close without ending session (simulating crash)
	db.Close()

	// Re-open same DB
	db2, err := experiment.Open(dbPath)
	require.NoError(t, err)
	defer db2.Close()

	count, err := db2.RecoverOrphans()
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	var endedAt string
	err = db2.QueryRow("SELECT ended_at FROM sessions WHERE session_id = ?", sessionID).Scan(&endedAt)
	require.NoError(t, err)
	assert.NotEmpty(t, endedAt)
}
