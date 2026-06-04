package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MeasureNoiseFloor: N is the max cosine any probe reaches against any note.
// mockEmbedder returns {1,0,0} for every probe; note A is {1,0,0} (cosine 1.0)
// and note B is {0,1,0} (cosine 0) → N = 1.0. The single note-to-note pair has
// cosine 0 → mu = sigma = 0, count = 1.
func TestMeasureNoiseFloor_ComputesFloorAndDispersion(t *testing.T) {
	db := buildRetrieverTestDB(t)
	a, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	b, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NoError(t, index.StoreEmbedding(db, a.ID, []float32{1, 0, 0}))
	require.NoError(t, index.StoreEmbedding(db, b.ID, []float32{0, 1, 0}))

	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	cal, err := query.MeasureNoiseFloor(context.Background(), embedder, db)
	require.NoError(t, err)

	assert.InDelta(t, 1.0, cal.NoiseFloor, 1e-6, "a probe parallel to a note → N = 1.0")
	assert.Equal(t, len(noisefloor.DefaultProbes), cal.NoiseFloorProbes)
	assert.Equal(t, 2, cal.NoteCount)
	assert.Equal(t, 3, cal.EmbeddingDims)
	assert.Equal(t, 1, cal.NTNSampleCount, "two notes → one pair")
	assert.InDelta(t, 0.0, cal.NTNCosineMu, 1e-6, "orthogonal pair → mean cosine 0")
	assert.InDelta(t, 0.0, cal.NTNCosineSigma, 1e-6)
}

// A vault mid-migration can hold embeddings of two dimensionalities (e.g. some
// MiniLM-384 notes, some BGE-M3-1024). CosineSimilarity returns 0 on a dim
// mismatch, which would silently drag the measured N and dispersion toward
// garbage and store a corrupt snapshot as federated evidence. MeasureNoiseFloor
// must REFUSE such a vault rather than calibrate it.
func TestMeasureNoiseFloor_MixedDimensionalityErrors(t *testing.T) {
	db := buildRetrieverTestDB(t)
	a, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	b, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NoError(t, index.StoreEmbedding(db, a.ID, []float32{1, 0, 0}))    // 3-dim
	require.NoError(t, index.StoreEmbedding(db, b.ID, []float32{0, 1, 0, 0})) // 4-dim — mismatch

	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	_, err = query.MeasureNoiseFloor(context.Background(), embedder, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mixed-dimensionality")
}

func TestMeasureNoiseFloor_NilEmbedderErrors(t *testing.T) {
	db := buildRetrieverTestDB(t)
	_, err := query.MeasureNoiseFloor(context.Background(), nil, db)
	require.Error(t, err)
}

// No embeddings → there's nothing to calibrate against; surface a clear error
// rather than dividing by zero or returning a meaningless floor.
func TestMeasureNoiseFloor_NoEmbeddingsErrors(t *testing.T) {
	db := buildRetrieverTestDB(t) // notes indexed, but no embeddings stored
	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	_, err := query.MeasureNoiseFloor(context.Background(), embedder, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embeddings")
}
