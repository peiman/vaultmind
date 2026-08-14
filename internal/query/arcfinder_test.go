package query

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeArcEmbedder returns a fixed vector so ranking can be tested without a
// model. Only Embed is exercised; the rest satisfies embedding.Embedder.
type fakeArcEmbedder struct {
	vec []float32
	err error
}

func (f fakeArcEmbedder) Embed(context.Context, string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.vec, nil
}
func (f fakeArcEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) { return nil, nil }
func (f fakeArcEmbedder) Dims() int                                                 { return len(f.vec) }
func (f fakeArcEmbedder) Close() error                                              { return nil }

// The neighbours must be ordered by actual closeness, best first — the ordering
// IS the aid. An unordered trio tells the reader nothing they couldn't get by
// listing the vault.
//
// The scores must also be cosines, not the hybrid retriever's fused rank. The
// first version of this used the fused score and printed 0.02 for every
// neighbour: three ties, no discrimination, a number that looks like evidence
// and isn't. This repo already had an arc about that exact confusion.
func TestArcFinder_RanksByCosineBestFirst(t *testing.T) {
	f := &ArcFinder{
		embedder: fakeArcEmbedder{vec: []float32{1, 0}},
		arcs: []arcVector{
			{id: "arc-orthogonal", vec: []float32{0, 1}},   // cosine 0
			{id: "arc-identical", vec: []float32{1, 0}},    // cosine 1
			{id: "arc-similar", vec: []float32{0.9, 0.44}}, // between
		},
	}

	got, err := f.NearestArcs(context.Background(), "anything", 3)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "arc-identical", got[0].ID)
	assert.Equal(t, "arc-similar", got[1].ID)
	assert.Equal(t, "arc-orthogonal", got[2].ID)
	assert.InDelta(t, 1.0, got[0].Score, 0.001, "an identical vector scores 1 — a cosine, not a rank score")
}

func TestArcFinder_RespectsLimit(t *testing.T) {
	f := &ArcFinder{
		embedder: fakeArcEmbedder{vec: []float32{1, 0}},
		arcs: []arcVector{
			{id: "a", vec: []float32{1, 0}},
			{id: "b", vec: []float32{0.9, 0.1}},
			{id: "c", vec: []float32{0.5, 0.5}},
		},
	}

	got, err := f.NearestArcs(context.Background(), "q", 2)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

// Degenerate inputs are quiet no-ops, never errors: neighbours are an aid, and
// a vault with no arcs must still produce its candidate list.
func TestArcFinder_DegenerateInputsAreQuietNoOps(t *testing.T) {
	cases := map[string]*ArcFinder{
		"nil finder":  nil,
		"no embedder": {arcs: []arcVector{{id: "a", vec: []float32{1, 0}}}},
		"no arcs":     {embedder: fakeArcEmbedder{vec: []float32{1, 0}}},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := f.NearestArcs(context.Background(), "q", 3)
			require.NoError(t, err)
			assert.Empty(t, got)
		})
	}

	t.Run("blank text", func(t *testing.T) {
		f := &ArcFinder{embedder: fakeArcEmbedder{vec: []float32{1, 0}}, arcs: []arcVector{{id: "a", vec: []float32{1, 0}}}}
		got, err := f.NearestArcs(context.Background(), "   ", 3)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// An embedding failure is reported, never rendered as "no similar arcs" — which
// would read as "this proposal is novel" and invert the meaning.
func TestArcFinder_EmbedFailureIsReported(t *testing.T) {
	f := &ArcFinder{
		embedder: fakeArcEmbedder{err: errors.New("model unavailable")},
		arcs:     []arcVector{{id: "a", vec: []float32{1, 0}}},
	}

	_, err := f.NearestArcs(context.Background(), "q", 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model unavailable")
}
