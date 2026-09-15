package memory_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelated_Mixed(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "mixed"})
	require.NoError(t, err)
	assert.Equal(t, "proj-vaultmind", result.TargetID)
	assert.Equal(t, "mixed", result.Mode)
	assert.Greater(t, len(result.Related), 0)
}

// Explicit mode returns ONLY high-confidence relations — and must be seen to
// EXCLUDE something, or the assertion is decoration (issue #134).
//
// The old version used input "proj-vaultmind", for which explicit, inferred and
// mixed all collapse to the same single high-confidence note. Deleting the
// filter outright (`keep = e.confidence == "high"` -> `keep = true`) left the
// result identical and the test green: it asserted a property the fixture
// guaranteed regardless of the code.
//
// "concept-spreading-activation" is the input where the filter is observable —
// mixed returns 4 relations, two of them medium, so an unfiltered explicit mode
// yields medium items and the Equal assertion fails. Measured, not assumed.
func TestRelated_ExplicitOnly(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	explicit, err := memory.Related(resolver, db,
		memory.RelatedConfig{Input: "concept-spreading-activation", Mode: "explicit"})
	require.NoError(t, err)
	mixed, err := memory.Related(resolver, db,
		memory.RelatedConfig{Input: "concept-spreading-activation", Mode: "mixed"})
	require.NoError(t, err)

	require.NotEmpty(t, explicit.Related,
		"an empty result would make every assertion below vacuously true")
	for _, r := range explicit.Related {
		assert.Equal(t, "high", r.Confidence, "explicit mode must return only high-confidence relations")
	}
	assert.Less(t, len(explicit.Related), len(mixed.Related),
		"explicit must EXCLUDE relations that mixed keeps — equal counts mean the filter did nothing")
}

func TestRelated_InferredOnly(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "inferred"})
	require.NoError(t, err)
	for _, r := range result.Related {
		assert.NotEqual(t, "high", r.Confidence)
	}
}

func TestRelated_HasMetadata(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "mixed"})
	require.NoError(t, err)
	if len(result.Related) > 0 {
		assert.NotEmpty(t, result.Related[0].ID)
		assert.NotEmpty(t, result.Related[0].EdgeType)
	}
}

// TestRelated_InvalidMode verifies that an unrecognised mode falls through to
// "mixed" (default) behaviour — i.e. returns all edges without error.
func TestRelated_InvalidMode(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "nonexistent-mode"})
	require.NoError(t, err)
	// "nonexistent-mode" hits the default case (keep = true), so it behaves like mixed
	assert.NotNil(t, result)
	assert.Greater(t, len(result.Related), 0)
}

// TestRelated_TargetID_IsResolvedCanonical verifies the TargetID in the result
// is the resolved canonical ID rather than raw cfg.Input (I1 fix).
func TestRelated_TargetID_IsResolvedCanonical(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "mixed"})
	require.NoError(t, err)
	// The canonical ID for "proj-vaultmind" is "proj-vaultmind" — they match here,
	// but TargetID must come from the resolver, not raw input.
	assert.Equal(t, "proj-vaultmind", result.TargetID)
}

// TestRelated_UnresolvableInput verifies an error is returned when the input
// cannot be resolved to a known note.
func TestRelated_UnresolvableInput(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "does-not-exist-xyz", Mode: "mixed"})
	assert.Error(t, err)
	assert.Nil(t, result)
}
