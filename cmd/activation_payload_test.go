package cmd

import (
	"errors"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAskEventData_IncludesRetrievalVariantInFallback(t *testing.T) {
	result := &query.AskResult{
		Query: "memory",
		TopHits: []query.ScoredResult{
			{ID: "arc-1", Type: "arc", Path: "arcs/arc-1.md", Score: 0.91},
			{ID: "fact-2", Type: "fact", Path: "facts/fact-2.md", Score: 0.73},
		},
	}

	got := buildAskEventData(result, "hybrid", nil, "", false, nil)

	assert.Equal(t, 2, got["top_hits"])
	_, hasPrimary := got["primary_variant"]
	assert.False(t, hasPrimary, "no activation variant when experiment disabled")

	variants, ok := got["variants"].(map[string]any)
	require.True(t, ok)

	hybrid, ok := variants["hybrid"].(map[string]any)
	require.True(t, ok, "retrieval variant should be present under its mode name")

	results, ok := hybrid["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 2)
	first := results[0].(map[string]any)
	assert.Equal(t, "arc-1", first["note_id"])
	assert.Equal(t, 1, first["rank"])
	assert.InDelta(t, 0.91, first["score_final"], 1e-9)
	assert.Equal(t, "arc", first["note_type"])
	assert.Equal(t, "arcs/arc-1.md", first["path"])
}

func TestBuildAskEventData_MergesRetrievalAndShadowVariants(t *testing.T) {
	result := &query.AskResult{
		TopHits: []query.ScoredResult{
			{ID: "n1", Type: "arc", Path: "n1.md", Score: 0.8},
		},
	}
	shadow := map[string]any{
		"compressed-0.2": map[string]any{"results": []any{}},
		"wall-clock":     map[string]any{"results": []any{}},
	}

	got := buildAskEventData(result, "keyword", shadow, "compressed-0.2", true, nil)

	assert.Equal(t, "compressed-0.2", got["primary_variant"])
	assert.Equal(t, 1, got["top_hits"])

	variants, ok := got["variants"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, variants, "keyword", "retrieval variant present")
	assert.Contains(t, variants, "compressed-0.2", "shadow variant preserved")
	assert.Contains(t, variants, "wall-clock", "shadow variant preserved")
}

func TestBuildAskEventData_EmptyTopHitsStillEmitsRetrievalVariant(t *testing.T) {
	result := &query.AskResult{Query: "nothing found"}

	got := buildAskEventData(result, "hybrid", nil, "", false, nil)

	assert.Equal(t, 0, got["top_hits"])
	variants := got["variants"].(map[string]any)
	hybrid := variants["hybrid"].(map[string]any)
	results := hybrid["results"].([]any)
	assert.Empty(t, results)
}

func TestBuildAskEventData_NilResultIsSafe(t *testing.T) {
	got := buildAskEventData(nil, "hybrid", nil, "", false, errors.New("boom"))

	assert.Equal(t, 0, got["top_hits"])
	assert.Equal(t, "boom", got["error"])
	variants := got["variants"].(map[string]any)
	require.Contains(t, variants, "hybrid")
}

func TestBuildAskEventData_ErrorPopulatesErrorField(t *testing.T) {
	result := &query.AskResult{TopHits: []query.ScoredResult{{ID: "n1", Score: 0.5}}}

	got := buildAskEventData(result, "hybrid", nil, "", false, errors.New("retriever failed"))

	assert.Equal(t, "retriever failed", got["error"])
	assert.Equal(t, 1, got["top_hits"])
}

func TestAskRetrievalHits_MapsTopHitsToExperimentShape(t *testing.T) {
	hits := []query.ScoredResult{
		{ID: "n1", Type: "arc", Path: "n1.md", Score: 0.9},
		{ID: "n2", Type: "fact", Path: "n2.md", Score: 0.4},
	}

	got := askRetrievalHits(hits)

	require.Len(t, got, 2)
	assert.Equal(t, "n1", got[0].NoteID)
	assert.Equal(t, 1, got[0].Rank)
	assert.InDelta(t, 0.9, got[0].Score, 1e-9)
	assert.Equal(t, "arc", got[0].NoteType)
	assert.Equal(t, "n1.md", got[0].Path)
	assert.Equal(t, 2, got[1].Rank)
}
