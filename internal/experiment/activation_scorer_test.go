package experiment_test

import (
	"testing"

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
	scores, features, err := experiment.ComputeBatchScores(db, nil, params)
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
	scores, features, err := experiment.ComputeBatchScores(db, []string{"note-a", "note-b", "note-c"}, params)
	require.NoError(t, err)

	assert.Greater(t, scores["note-a"], scores["note-b"])
	assert.Equal(t, 0.0, scores["note-c"])
	assert.Contains(t, features["note-a"], "retrieval_strength")
	assert.Contains(t, features["note-a"], "storage_strength")
}
