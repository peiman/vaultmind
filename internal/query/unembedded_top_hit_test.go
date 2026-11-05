package query_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A note indexed after the last `index --embed` has no stored embedding.
// FTS can still rank it first, and the RRF gap then calls a lone keyword hit
// strong — separation from hit #2, not relevance. With a noise floor and an
// embedder, that label must be capped at weak and the result must say why.
func TestAsk_UnembeddedTopHit_CappedAtWeak(t *testing.T) {
	db := vaultWithUnembeddedTopHit(t)
	retriever := &query.FTSRetriever{DB: db}

	result, err := query.Ask(context.Background(), retriever, graph.NewResolver(db), db, query.AskConfig{
		Query:         "xylophone",
		Budget:        4000,
		MaxItems:      5,
		SearchLimit:   5,
		Embedder:      &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3},
		NoiseFloor:    0.45,
		HasNoiseFloor: true,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(result.TopHits), 2, "the gap needs a second hit")
	require.Equal(t, unembeddedTopHitID, result.TopHits[0].ID, "the unembedded note must be the top hit")
	require.NotNil(t, result.Similarities, "other notes are embedded, so similarities were computed")
	_, hasCosine := result.Similarities[unembeddedTopHitID]
	require.False(t, hasCosine, "the top hit has no stored embedding")

	gap := (result.TopHits[0].Score - result.TopHits[1].Score) / result.TopHits[0].Score
	require.GreaterOrEqual(t, gap, 0.015,
		"fixture must separate enough that the RRF-gap label is strong or moderate; got scores %.4f / %.4f",
		result.TopHits[0].Score, result.TopHits[1].Score)

	assert.Equal(t, noisefloor.ConfidenceWeak, result.TopHitConfidence,
		"an unembedded top hit must not be labelled moderate or strong")
	assert.True(t, result.TopHitUnembedded)
	assert.False(t, result.NoiseFloorApplied, "there is no cosine to judge, so the floor is not applied")

	raw, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"top_hit_unembedded":true`)

	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(result, &buf))
	out := buf.String()
	assert.Contains(t, out, "[top-hit confidence: weak")
	assert.Contains(t, out, "top hit has no embedding — relevance can't be judged; run vaultmind index --embed")
	assert.NotContains(t, out, "A medication decision added after the last embed.",
		"a weak top hit is pointers-only, so the note text is not delivered")
}

// An embedded top hit is judged by its cosine, as before. The cap must not
// touch a hit the floor can actually measure.
func TestAsk_EmbeddedTopHit_KeepsNoiseFloorLabel(t *testing.T) {
	db := vaultWithUnembeddedTopHit(t)
	require.NoError(t, index.StoreEmbedding(db, unembeddedTopHitID, []float32{1, 0, 0}))
	retriever := &query.FTSRetriever{DB: db}
	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}

	result, err := query.Ask(context.Background(), retriever, graph.NewResolver(db), db, query.AskConfig{
		Query:         "xylophone",
		Budget:        4000,
		MaxItems:      5,
		SearchLimit:   5,
		Embedder:      embedder,
		NoiseFloor:    0.45,
		HasNoiseFloor: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.TopHits)
	require.Equal(t, unembeddedTopHitID, result.TopHits[0].ID)
	require.Contains(t, result.Similarities, unembeddedTopHitID)

	_, want := noisefloor.Relevance(result.TopHitCosine, result.NoiseFloor, result.NoiseFloorSigma, noisefloor.DefaultNoiseFloor(embedder.Dims()))
	assert.True(t, result.NoiseFloorApplied)
	assert.Equal(t, want, result.TopHitConfidence)
	assert.Equal(t, noisefloor.ConfidenceStrong, result.TopHitConfidence, "cosine 1.0 clears the floor")
	assert.False(t, result.TopHitUnembedded)

	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(result, &buf))
	assert.NotContains(t, buf.String(), "no embedding")
}

const unembeddedTopHitID = "meds-decision"

// vaultWithUnembeddedTopHit is a two-note vault. The query token is packed
// into the first note, which has no embedding, and mentioned once in the
// second, which is embedded. FTS therefore ranks the unembedded note first,
// and min-max of two different BM25 scores leaves a wide RRF gap.
func vaultWithUnembeddedTopHit(t *testing.T) *index.DB {
	t.Helper()
	db := openTestDB(t)
	require.NoError(t, index.StoreNote(db, index.NoteRecord{
		ID:       unembeddedTopHitID,
		Path:     "meds.md",
		Title:    "Medication decision xylophone",
		Type:     "note",
		Hash:     "h1",
		MTime:    1,
		BodyText: strings.Repeat("xylophone medication decision ", 40),
		BodyRaw:  "A medication decision added after the last embed.",
	}))
	require.NoError(t, index.StoreNote(db, index.NoteRecord{
		ID:       "garden-log",
		Path:     "garden.md",
		Title:    "Garden",
		Type:     "note",
		Hash:     "h2",
		MTime:    1,
		BodyText: "tomatoes soil compost and one xylophone mention",
		BodyRaw:  "Garden notes, embedded.",
	}))
	require.NoError(t, index.StoreEmbedding(db, "garden-log", []float32{0, 1, 0}))
	return db
}
