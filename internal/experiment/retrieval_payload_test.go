package experiment_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildVariantPayload_MapsHitsToResults(t *testing.T) {
	hits := []experiment.RetrievalHit{
		{NoteID: "arc-1", Rank: 1, Score: 0.92, NoteType: "arc", Path: "arcs/arc-1.md"},
		{NoteID: "fact-2", Rank: 2, Score: 0.71, NoteType: "fact", Path: "facts/fact-2.md"},
	}

	got := experiment.BuildVariantPayload("hybrid", hits)

	variant, ok := got["hybrid"].(map[string]any)
	require.True(t, ok, "variant key should be a map")

	results, ok := variant["results"].([]any)
	require.True(t, ok, "results should be a slice")
	require.Len(t, results, 2)

	first, ok := results[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "arc-1", first["note_id"])
	assert.Equal(t, 1, first["rank"])
	assert.InDelta(t, 0.92, first["score_final"], 1e-9)
	assert.Equal(t, "arc", first["note_type"])
	assert.Equal(t, "arcs/arc-1.md", first["path"])
}

func TestBuildVariantPayload_EmptyHitsReturnsEmptyResultsSlice(t *testing.T) {
	got := experiment.BuildVariantPayload("none", nil)

	variant, ok := got["none"].(map[string]any)
	require.True(t, ok)

	results, ok := variant["results"].([]any)
	require.True(t, ok, "results should be a slice, not nil")
	assert.Empty(t, results)
}

func TestBuildVariantPayload_IncludesScoreComponentsWhenPresent(t *testing.T) {
	hits := []experiment.RetrievalHit{
		{
			NoteID: "n1", Rank: 1, Score: 0.8, NoteType: "arc", Path: "n1.md",
			Scores: map[string]float64{"fts": 0.6, "dense": 0.9},
		},
	}

	got := experiment.BuildVariantPayload("hybrid", hits)
	results := got["hybrid"].(map[string]any)["results"].([]any)
	first := results[0].(map[string]any)

	scores, ok := first["scores"].(map[string]float64)
	require.True(t, ok, "scores should be present when hit provides components")
	assert.InDelta(t, 0.6, scores["fts"], 1e-9)
	assert.InDelta(t, 0.9, scores["dense"], 1e-9)
}

func TestBuildVariantPayload_OmitsScoresWhenEmpty(t *testing.T) {
	hits := []experiment.RetrievalHit{
		{NoteID: "n1", Rank: 1, Score: 0.8, NoteType: "arc", Path: "n1.md"},
	}

	got := experiment.BuildVariantPayload("hybrid", hits)
	first := got["hybrid"].(map[string]any)["results"].([]any)[0].(map[string]any)

	_, has := first["scores"]
	assert.False(t, has, "scores key should be absent when no components provided")
}
