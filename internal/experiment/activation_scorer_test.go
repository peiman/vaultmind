package experiment_test

import (
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariantGamma(t *testing.T) {
	tests := []struct {
		variant string
		gamma   float64
	}{
		{"compressed-0.2", 0.2},
		{"compressed-0.5", 0.5},
		{"wall-clock", 1.0},
		{"none", 0.0},
	}
	for _, tt := range tests {
		g, ok := experiment.VariantGamma(tt.variant)
		assert.True(t, ok, "variant %q", tt.variant)
		assert.InDelta(t, tt.gamma, g, 0.001)
	}
}

func TestVariantGamma_Unknown(t *testing.T) {
	_, ok := experiment.VariantGamma("unknown")
	assert.False(t, ok)
}

func TestDefaultActivationParams(t *testing.T) {
	p := experiment.DefaultActivationParams(0.2)
	assert.InDelta(t, 0.2, p.Gamma, 0.001)
	assert.InDelta(t, 0.5, p.D, 0.001)
	assert.InDelta(t, 0.6, p.Alpha, 0.001)
	assert.InDelta(t, 0.4, p.Beta, 0.001)
}

func TestComputeBatchScores_Empty(t *testing.T) {
	db := openTestExpDB(t)
	params := experiment.DefaultActivationParams(0.2)
	scores, features, err := experiment.ComputeBatchScores(db, nil, params, nil)
	require.NoError(t, err)
	assert.Empty(t, scores)
	assert.Empty(t, features)
}

func TestComputeBatchScores_WithAccesses(t *testing.T) {
	db := openTestExpDB(t)
	sid, _ := db.StartSession("/vault")
	_ = db.EndSession(sid)

	sid2, _ := db.StartSession("/vault")
	for i := 0; i < 3; i++ {
		_, _ = db.LogEvent(experiment.Event{
			SessionID: sid2, Type: experiment.EventNoteAccess, VaultPath: "/vault",
			Data: map[string]any{"note_id": "note-a", "source": "note_get"},
		})
	}
	_, _ = db.LogEvent(experiment.Event{
		SessionID: sid2, Type: experiment.EventNoteAccess, VaultPath: "/vault",
		Data: map[string]any{"note_id": "note-b", "source": "note_get"},
	})

	params := experiment.DefaultActivationParams(0.2)
	scores, features, err := experiment.ComputeBatchScores(db, []string{"note-a", "note-b", "note-c"}, params, nil)
	require.NoError(t, err)

	assert.Greater(t, scores["note-a"], scores["note-b"])
	assert.Equal(t, 0.0, scores["note-c"])
	assert.Contains(t, features["note-a"], "retrieval_strength")
	assert.Contains(t, features["note-a"], "storage_strength")
}

func TestComputeBatchScores_WithSimilarities(t *testing.T) {
	db := openTestExpDB(t)
	sid, _ := db.StartSession("/vault")

	// Both notes accessed once
	_, _ = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventNoteAccess, VaultPath: "/vault",
		Data: map[string]any{"note_id": "note-a", "source": "search"},
	})
	_, _ = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventNoteAccess, VaultPath: "/vault",
		Data: map[string]any{"note_id": "note-b", "source": "search"},
	})

	params := experiment.DefaultActivationParams(0.2)
	params.Delta = 0.3

	similarities := map[string]float64{
		"note-a": 0.9,
		"note-b": 0.1,
	}

	scores, feats, err := experiment.ComputeBatchScores(db, []string{"note-a", "note-b"}, params, similarities)
	require.NoError(t, err)

	// note-a should score higher due to similarity boost
	assert.Greater(t, scores["note-a"], scores["note-b"])
	assert.InDelta(t, 0.9, feats["note-a"]["similarity"], 0.001)
	assert.InDelta(t, 0.1, feats["note-b"]["similarity"], 0.001)
}

func TestScoreFromData_WithSimilarities(t *testing.T) {
	now := time.Now().UTC()
	accessMap := map[string][]time.Time{
		"note-a": {now.Add(-1 * time.Hour)},
		"note-b": {now.Add(-1 * time.Hour)},
	}
	similarities := map[string]float64{
		"note-a": 0.9, // highly similar to query
		"note-b": 0.1, // not similar
	}
	params := experiment.DefaultActivationParams(1.0)
	params.Delta = 0.3
	scores, features := experiment.ScoreFromData(
		[]string{"note-a", "note-b"}, accessMap, nil, now, params, similarities,
	)

	// note-a should score higher due to similarity boost
	assert.Greater(t, scores["note-a"], scores["note-b"])
	assert.Contains(t, features["note-a"], "similarity")
	assert.InDelta(t, 0.9, features["note-a"]["similarity"], 0.001)
}

func TestScoreFromData_NilSimilarities(t *testing.T) {
	now := time.Now().UTC()
	accessMap := map[string][]time.Time{
		"note-a": {now.Add(-1 * time.Hour)},
	}
	params := experiment.DefaultActivationParams(1.0)
	// nil similarities should work (backward compatible)
	scores, features := experiment.ScoreFromData(
		[]string{"note-a"}, accessMap, nil, now, params, nil,
	)
	assert.Greater(t, scores["note-a"], 0.0)
	assert.Equal(t, 0.0, features["note-a"]["similarity"])
}
