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

// A note reachable by BOTH a human-authored edge and an inferred one must be
// reported by its STRONGEST edge, not by whichever the database returned first.
//
// `confidence` is provenance, not certainty (decision-confidence-as-provenance):
// high = a human wrote the link, medium = VaultMind inferred it, low = an agent
// wrote it back unreviewed. Dedup marked `seen` on first occurrence, so a
// relationship a person typed into frontmatter could be reported as a machine
// guess purely because SQLite returned the inferred edge first.
//
// Measured on the fixture before the fix: `mixed` returned concept-act-r as
// alias_mention/medium while `explicit` found the same note at
// explicit_link/high — the same relationship, two different provenances,
// decided by row order.
func TestRelated_MixedReportsTheStrongestEdgeNotTheFirst(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	mixed, err := memory.Related(resolver, db,
		memory.RelatedConfig{Input: "concept-spreading-activation", Mode: "mixed"})
	require.NoError(t, err)
	explicit, err := memory.Related(resolver, db,
		memory.RelatedConfig{Input: "concept-spreading-activation", Mode: "explicit"})
	require.NoError(t, err)

	// Every note explicit mode calls high must also be high in mixed mode.
	// Explicit is a strict subset, so mixed cannot know LESS about provenance.
	highInExplicit := map[string]bool{}
	for _, r := range explicit.Related {
		highInExplicit[r.ID] = true
	}
	require.NotEmpty(t, highInExplicit, "precondition: explicit mode must return something")

	for _, r := range mixed.Related {
		if highInExplicit[r.ID] {
			assert.Equal(t, "high", r.Confidence,
				"%s has a human-authored edge that explicit mode finds; mixed mode must not "+
					"report it as machine-inferred just because another edge sorted first", r.ID)
		}
	}
}
