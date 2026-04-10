package cmd

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestExpDB(t *testing.T) *experiment.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "experiment.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestComputeActivationScores_NilSession(t *testing.T) {
	scores := computeActivationScores(context.Background())
	assert.Nil(t, scores)
}

func TestComputeActivationScores_NoActivationExperiment(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sid}
	ctx := experiment.WithSession(context.Background(), session)

	// No experiment config in viper → no activation scores
	scores := computeActivationScores(ctx)
	assert.Nil(t, scores)
}

func TestBuildVariantResults_EmptyItems(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sid}
	actDef := experiment.ExperimentDef{
		Enabled: true,
		Primary: "compressed-0.2",
	}

	result := buildVariantResults(session, actDef, nil)
	assert.Contains(t, result, "compressed-0.2")

	variant := result["compressed-0.2"].(map[string]any)
	results := variant["results"].([]any)
	assert.Empty(t, results)
}

func TestBuildVariantResults_WithItems(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	// Record some access history so ScoreFromData has data to work with
	_, err = db.Exec(
		`INSERT INTO events (event_id, session_id, event_type, timestamp, vault_path, event_data)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"evt-1", sid, "note_access", time.Now().UTC().Format(time.RFC3339), "/vault",
		`{"note_id":"note-a","source":"search"}`,
	)
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sid}
	actDef := experiment.ExperimentDef{
		Enabled: true,
		Primary: "compressed-0.2",
		Shadows: []string{"wall-clock"},
	}

	items := []rankedItem{
		{ID: "note-a", Rank: 1},
		{ID: "note-b", Rank: 2},
	}

	result := buildVariantResults(session, actDef, items)

	// Both variants should be present
	assert.Contains(t, result, "compressed-0.2")
	assert.Contains(t, result, "wall-clock")

	// Each variant should have results for both items
	for _, variant := range []string{"compressed-0.2", "wall-clock"} {
		v := result[variant].(map[string]any)
		results := v["results"].([]any)
		assert.Len(t, results, 2, "variant %s should have 2 results", variant)

		first := results[0].(map[string]any)
		assert.Equal(t, "note-a", first["note_id"])
		assert.Equal(t, 1, first["rank"])

		second := results[1].(map[string]any)
		assert.Equal(t, "note-b", second["note_id"])
		assert.Equal(t, 2, second["rank"])
	}
}

// rankedItemsFromIDs converts a slice of note IDs (in rank order) to rankedItems.
// Test-only helper for building ranked items from plain ID lists.
func rankedItemsFromIDs(ids []string) []rankedItem {
	items := make([]rankedItem, len(ids))
	for i, id := range ids {
		items[i] = rankedItem{ID: id, Rank: i + 1}
	}
	return items
}

func TestRankedItemsFromIDs(t *testing.T) {
	ids := []string{"note-a", "note-b", "note-c"}
	items := rankedItemsFromIDs(ids)

	require.Len(t, items, 3)
	assert.Equal(t, rankedItem{ID: "note-a", Rank: 1}, items[0])
	assert.Equal(t, rankedItem{ID: "note-b", Rank: 2}, items[1])
	assert.Equal(t, rankedItem{ID: "note-c", Rank: 3}, items[2])
}

func TestRankedItemsFromIDs_Empty(t *testing.T) {
	items := rankedItemsFromIDs(nil)
	assert.Empty(t, items)
}
