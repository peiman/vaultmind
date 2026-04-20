package experiment_test

import (
	"errors"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAskEventData_IncludesRetrievalVariantInFallback(t *testing.T) {
	got := experiment.BuildAskEventData(experiment.AskEventParams{
		RetrievalMode: "hybrid",
		TopHits: []experiment.RetrievalHit{
			{NoteID: "arc-1", Rank: 1, Score: 0.91, NoteType: "arc", Path: "arcs/arc-1.md"},
			{NoteID: "fact-2", Rank: 2, Score: 0.73, NoteType: "fact", Path: "facts/fact-2.md"},
		},
	})

	assert.Equal(t, 2, got["top_hits"])
	_, hasPrimary := got["primary_variant"]
	assert.False(t, hasPrimary, "no activation variant when experiment disabled")

	variants, ok := got["variants"].(map[string]any)
	require.True(t, ok)
	hybrid, ok := variants["hybrid"].(map[string]any)
	require.True(t, ok)
	results := hybrid["results"].([]any)
	require.Len(t, results, 2)
	assert.Equal(t, "arc-1", results[0].(map[string]any)["note_id"])
}

func TestBuildAskEventData_MergesRetrievalAndShadowVariants(t *testing.T) {
	got := experiment.BuildAskEventData(experiment.AskEventParams{
		RetrievalMode: "keyword",
		TopHits:       []experiment.RetrievalHit{{NoteID: "n1", Rank: 1, Score: 0.8, NoteType: "arc", Path: "n1.md"}},
		ShadowVariants: map[string]any{
			"compressed-0.2": map[string]any{"results": []any{}},
			"wall-clock":     map[string]any{"results": []any{}},
		},
		PrimaryVariant: "compressed-0.2",
		ActivationOn:   true,
	})

	assert.Equal(t, "compressed-0.2", got["primary_variant"])
	variants := got["variants"].(map[string]any)
	assert.Contains(t, variants, "keyword")
	assert.Contains(t, variants, "compressed-0.2")
	assert.Contains(t, variants, "wall-clock")
}

func TestBuildAskEventData_ErrorPopulatesErrorField(t *testing.T) {
	got := experiment.BuildAskEventData(experiment.AskEventParams{
		RetrievalMode: "hybrid",
		RetrievalErr:  errors.New("retriever failed"),
	})

	assert.Equal(t, "retriever failed", got["error"])
	assert.Equal(t, 0, got["top_hits"])
}

func TestBuildAskEventData_EmptyTopHitsStillEmitsRetrievalVariant(t *testing.T) {
	got := experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "hybrid"})

	variants := got["variants"].(map[string]any)
	hybrid := variants["hybrid"].(map[string]any)
	assert.Empty(t, hybrid["results"].([]any))
	_, hasErr := got["error"]
	assert.False(t, hasErr, "zero hits is success, not error")
}

func TestBuildAskEventData_ShadowVariantCollisionIsOverwrittenDeterministically(t *testing.T) {
	// When a shadow variant name collides with the retrieval mode, the shadow
	// payload wins (documented behavior — the collision log warns the caller).
	shadowPayload := map[string]any{"results": []any{"shadow-won"}}
	got := experiment.BuildAskEventData(experiment.AskEventParams{
		RetrievalMode:  "hybrid",
		TopHits:        []experiment.RetrievalHit{{NoteID: "n1", Rank: 1}},
		ShadowVariants: map[string]any{"hybrid": shadowPayload},
	})

	variants := got["variants"].(map[string]any)
	assert.Equal(t, shadowPayload, variants["hybrid"])
}
