package experiment_test

import (
	"testing"
	"time"

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

	// Score an hour after the accesses, not at time.Now(). "More accesses ranks
	// higher" is a long-run property: retrieval strength is a power law in
	// elapsed time, so in the first moments a single newer access outranks five
	// older ones. Event timestamps are stored at second resolution while now
	// carries nanoseconds, so scoring at now made this test's verdict depend on
	// whether two LogEvent calls straddled a second boundary — which on a loaded
	// CI runner they eventually did. See
	// TestScoreFromData_CountWinsOnlyOnceRecencyFades for the two regimes.
	scoreAt := time.Now().UTC().Add(time.Hour)

	// compressed-0.2
	params02 := experiment.DefaultActivationParams(0.2)
	scores02, feats02, err := experiment.ComputeBatchScoresAt(db, noteIDs, params02, nil, scoreAt)
	require.NoError(t, err)
	assert.Greater(t, scores02["frequently-accessed"], scores02["rarely-accessed"])
	assert.Equal(t, 0.0, scores02["never-accessed"])
	assert.InDelta(t, 5.0, feats02["frequently-accessed"]["access_count"], 0.01)

	// wall-clock
	paramsWC := experiment.DefaultActivationParams(1.0)
	scoresWC, _, err := experiment.ComputeBatchScoresAt(db, noteIDs, paramsWC, nil, scoreAt)
	require.NoError(t, err)
	assert.Greater(t, scoresWC["frequently-accessed"], scoresWC["rarely-accessed"])

	// none (gamma=0)
	paramsNone := experiment.DefaultActivationParams(0.0)
	scoresNone, _, err := experiment.ComputeBatchScoresAt(db, noteIDs, paramsNone, nil, scoreAt)
	require.NoError(t, err)
	// With gamma=0, only active session time counts. Storage still works.
	assert.Greater(t, scoresNone["frequently-accessed"], 0.0)

	// Variant lookup
	for _, v := range []string{"compressed-0.2", "compressed-0.5", "wall-clock", "none"} {
		_, ok := experiment.VariantGamma(v)
		assert.True(t, ok, "variant %q should be recognized", v)
	}
}
