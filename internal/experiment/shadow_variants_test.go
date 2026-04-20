package experiment_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openTestDB opens a fresh experiment DB in a temp directory.
func openTestDB(t *testing.T) *experiment.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := experiment.Open(filepath.Join(dir, "exp.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestBuildShadowVariantResults_ReturnsPerVariantResults(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	session := &experiment.Session{DB: db, ID: sid}

	actDef := experiment.ExperimentDef{
		Enabled: true,
		Primary: "compressed-0.2",
		Shadows: []string{"wall-clock", "none"},
	}

	got := experiment.BuildShadowVariantResults(session, actDef, []string{"n1", "n2"})

	// One entry per variant (primary + shadows).
	assert.Contains(t, got, "compressed-0.2")
	assert.Contains(t, got, "wall-clock")
	assert.Contains(t, got, "none")

	// Each variant's results are ranked positionally from the input note IDs.
	for _, name := range []string{"compressed-0.2", "wall-clock", "none"} {
		variant := got[name].(map[string]any)
		results := variant["results"].([]any)
		require.Len(t, results, 2, "variant %s should have 2 results", name)
		first := results[0].(map[string]any)
		assert.Equal(t, "n1", first["note_id"])
		assert.Equal(t, 1, first["rank"])
		second := results[1].(map[string]any)
		assert.Equal(t, "n2", second["note_id"])
		assert.Equal(t, 2, second["rank"])
	}
}

func TestBuildShadowVariantResults_SkipsUnknownVariantNames(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	session := &experiment.Session{DB: db, ID: sid}

	actDef := experiment.ExperimentDef{
		Enabled: true,
		Primary: "compressed-0.2",
		Shadows: []string{"bogus-variant"},
	}

	got := experiment.BuildShadowVariantResults(session, actDef, []string{"n1"})

	assert.Contains(t, got, "compressed-0.2")
	assert.NotContains(t, got, "bogus-variant", "unknown variant names are skipped (logged at debug)")
}

func TestBuildShadowVariantResults_EmptyNoteIDsProducesEmptyResults(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	session := &experiment.Session{DB: db, ID: sid}

	actDef := experiment.ExperimentDef{Enabled: true, Primary: "none"}

	got := experiment.BuildShadowVariantResults(session, actDef, nil)

	variant := got["none"].(map[string]any)
	assert.Empty(t, variant["results"].([]any))
}

// Timestamp reference used implicitly by the function's "now" clock; tests
// do not assert activation feature values because those are covered by
// ScoreFromData tests. This test asserts structural contract only.
var _ = time.Time{}
