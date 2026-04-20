package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/spf13/viper"
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
	scores := computeActivationScores(context.Background(), nil, 0)
	assert.Nil(t, scores)
}

func TestComputeActivationScores_NoActivationExperiment(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sid}
	ctx := experiment.WithSession(context.Background(), session)

	// No experiment config in viper → no activation scores
	scores := computeActivationScores(ctx, nil, 0)
	assert.Nil(t, scores)
}

func TestComputeActivationScores_WithSimilarities(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	// Both notes accessed once
	for _, noteID := range []string{"note-a", "note-b"} {
		_, err = db.LogEvent(experiment.Event{
			SessionID: sid, Type: experiment.EventNoteAccess, VaultPath: "/vault",
			Data: map[string]any{"note_id": noteID, "source": "search"},
		})
		require.NoError(t, err)
	}

	session := &experiment.Session{DB: db, ID: sid}
	ctx := experiment.WithSession(context.Background(), session)

	// Configure activation experiment in viper
	viper.Set("experiments", map[string]any{
		"activation": map[string]any{
			"enabled": true,
			"primary": "compressed-0.2",
		},
	})
	t.Cleanup(func() { viper.Set("experiments", map[string]any{}) })

	// Without similarities — Delta stays 0.0
	scoresNoSim := computeActivationScores(ctx, nil, 0)
	require.NotNil(t, scoresNoSim)

	// With similarities — Delta > 0, note-a gets a boost
	similarities := map[string]float64{
		"note-a": 0.95,
		"note-b": 0.1,
	}
	scoresWithSim := computeActivationScores(ctx, similarities, 0.2)
	require.NotNil(t, scoresWithSim)

	// note-a should score higher than note-b due to similarity boost
	assert.Greater(t, scoresWithSim["note-a"], scoresWithSim["note-b"],
		"note-a (sim=0.95) should outscore note-b (sim=0.1) with spreading activation")
}

// Tests for BuildShadowVariantResults and rankedItem moved to
// internal/experiment/shadow_variants_test.go alongside the implementation.
