package experiment_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivation_EndToEnd(t *testing.T) {
	db := openTestExpDB(t)

	s1, _ := db.StartSession("/vault")
	for i := 0; i < 5; i++ {
		_, _ = db.LogEvent(experiment.Event{
			SessionID: s1, Type: experiment.EventNoteAccess, VaultPath: "/vault",
			Data: map[string]any{"note_id": "frequently-accessed", "source": "note_get"},
		})
	}
	_, _ = db.LogEvent(experiment.Event{
		SessionID: s1, Type: experiment.EventNoteAccess, VaultPath: "/vault",
		Data: map[string]any{"note_id": "rarely-accessed", "source": "note_get"},
	})
	_ = db.EndSession(s1)

	noteIDs := []string{"frequently-accessed", "rarely-accessed", "never-accessed"}

	// compressed-0.2
	params02 := experiment.DefaultActivationParams(0.2)
	scores02, feats02, err := experiment.ComputeBatchScores(db, noteIDs, params02, nil)
	require.NoError(t, err)
	assert.Greater(t, scores02["frequently-accessed"], scores02["rarely-accessed"])
	assert.Equal(t, 0.0, scores02["never-accessed"])
	assert.InDelta(t, 5.0, feats02["frequently-accessed"]["access_count"], 0.01)

	// wall-clock
	paramsWC := experiment.DefaultActivationParams(1.0)
	scoresWC, _, err := experiment.ComputeBatchScores(db, noteIDs, paramsWC, nil)
	require.NoError(t, err)
	assert.Greater(t, scoresWC["frequently-accessed"], scoresWC["rarely-accessed"])

	// none (gamma=0)
	paramsNone := experiment.DefaultActivationParams(0.0)
	scoresNone, _, err := experiment.ComputeBatchScores(db, noteIDs, paramsNone, nil)
	require.NoError(t, err)
	// With gamma=0, only active session time counts. Storage still works.
	assert.Greater(t, scoresNone["frequently-accessed"], 0.0)

	// Variant lookup
	for _, v := range []string{"compressed-0.2", "compressed-0.5", "wall-clock", "none"} {
		_, ok := experiment.VariantGamma(v)
		assert.True(t, ok, "variant %q should be recognized", v)
	}
}
