package query_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildActivationTestStack scaffolds the minimum infra for testing
// the activation lane: a fixture-vault index DB and an empty
// experiment DB. Tests then populate access events / scalar columns
// to exercise specific behaviors.
func buildActivationTestStack(t *testing.T) (*index.DB, *experiment.DB, []string) {
	t.Helper()
	const fixtureVaultPath = "../../test/fixtures/testvault"

	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(fixtureVaultPath)
	require.NoError(t, err)
	idxr := index.NewIndexer(fixtureVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	idxDB, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = idxDB.Close() })

	expPath := filepath.Join(t.TempDir(), "exp.db")
	expDB, err := experiment.Open(expPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = expDB.Close() })

	// Collect known fixture note IDs for tests to access.
	rows, err := idxDB.Query(`SELECT id FROM notes ORDER BY id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NotEmpty(t, ids, "fixture vault must have notes")

	return idxDB, expDB, ids
}

// recordAccessForActivationTest writes a note_access event to the
// experiment DB AND increments the index's scalar access_count, since
// that's the contract index.RecordNoteAccess satisfies in production.
func recordAccessForActivationTest(t *testing.T, idxDB *index.DB, expDB *experiment.DB, sessionID, noteID string) {
	t.Helper()
	require.NoError(t, index.RecordNoteAccess(idxDB, noteID))
	sess := &experiment.Session{DB: expDB, ID: sessionID, VaultPath: "/test"}
	_, err := sess.LogNoteAccessEvent(noteID, "test")
	require.NoError(t, err)
}

// With zero accessed notes, ActivationRetriever returns nothing —
// not an error, just empty. Activation is opt-in: a fresh vault
// before any access logging shouldn't crash the hybrid retriever.
func TestActivationRetriever_EmptyAccessHistory(t *testing.T) {
	idxDB, expDB, _ := buildActivationTestStack(t)
	r := &query.ActivationRetriever{
		DB:     idxDB,
		ExpDB:  expDB,
		Params: experiment.DefaultActivationParams(0.5),
	}

	results, total, err := r.Search(context.Background(), "", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, results)
}

// After accessing a note, the activation retriever returns it ranked
// by activation. The "query" string is ignored — same ranking
// regardless of what the caller asked for.
func TestActivationRetriever_RanksByAccessHistory(t *testing.T) {
	idxDB, expDB, ids := buildActivationTestStack(t)
	require.GreaterOrEqual(t, len(ids), 2)

	sess, err := expDB.StartSession("/test")
	require.NoError(t, err)

	// Access ids[0] three times, ids[1] once. The 3-access note must
	// outrank the 1-access note.
	for i := 0; i < 3; i++ {
		recordAccessForActivationTest(t, idxDB, expDB, sess, ids[0])
	}
	recordAccessForActivationTest(t, idxDB, expDB, sess, ids[1])

	// Fixed scoring clock an hour after the accesses: ages become large and
	// uniform, so frequency (3 vs 1) dominates and the ranking is
	// deterministic — not a coin flip on sub-millisecond access timing.
	clk := time.Now().UTC().Add(time.Hour)
	r := &query.ActivationRetriever{
		DB:     idxDB,
		ExpDB:  expDB,
		Params: experiment.DefaultActivationParams(0.5),
		Now:    func() time.Time { return clk },
	}
	results, _, err := r.Search(context.Background(), "irrelevant query text", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 2, "both accessed notes must appear")

	// First result is the 3-access note.
	assert.Equal(t, ids[0], results[0].ID, "3-access note must outrank 1-access note")
}

// Type filter restricts the activation lane to a specific note type.
// Critical for the use case where the agent asks about concepts and
// shouldn't get recently-touched sources surfacing.
func TestActivationRetriever_TypeFilter(t *testing.T) {
	idxDB, expDB, ids := buildActivationTestStack(t)
	sess, err := expDB.StartSession("/test")
	require.NoError(t, err)

	// Access a concept and a source.
	var conceptID, sourceID string
	for _, id := range ids {
		row, _ := idxDB.QueryNoteByID(id)
		if row == nil {
			continue
		}
		switch row.Type {
		case "concept":
			if conceptID == "" {
				conceptID = id
			}
		case "source":
			if sourceID == "" {
				sourceID = id
			}
		}
	}
	require.NotEmpty(t, conceptID)
	require.NotEmpty(t, sourceID)
	recordAccessForActivationTest(t, idxDB, expDB, sess, conceptID)
	recordAccessForActivationTest(t, idxDB, expDB, sess, sourceID)

	r := &query.ActivationRetriever{DB: idxDB, ExpDB: expDB, Params: experiment.DefaultActivationParams(0.5)}
	results, _, err := r.Search(context.Background(), "", 10, 0, index.SearchFilters{Type: "concept"})
	require.NoError(t, err)
	for _, res := range results {
		assert.Equal(t, "concept", res.Type, "type filter must exclude non-concepts")
	}
}

// Scores must be normalized to [0, 1]. Components-rendering and
// --explain consumers read this field, so a sane scale matters even
// though RRF only uses ranks.
func TestActivationRetriever_ScoresNormalized(t *testing.T) {
	idxDB, expDB, ids := buildActivationTestStack(t)
	require.GreaterOrEqual(t, len(ids), 2)
	sess, err := expDB.StartSession("/test")
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		recordAccessForActivationTest(t, idxDB, expDB, sess, ids[0])
	}
	recordAccessForActivationTest(t, idxDB, expDB, sess, ids[1])

	r := &query.ActivationRetriever{DB: idxDB, ExpDB: expDB, Params: experiment.DefaultActivationParams(0.5)}
	results, _, err := r.Search(context.Background(), "", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for i, res := range results {
		assert.GreaterOrEqual(t, res.Score, 0.0, "result[%d] score must be ≥ 0", i)
		assert.LessOrEqual(t, res.Score, 1.0, "result[%d] score must be ≤ 1", i)
	}
	assert.InDelta(t, 1.0, results[0].Score, 1e-9, "top result must be 1.0")
}

// BuildAutoRetrieverWithActivation appends an "activation" named lane
// to the hybrid retriever when an experiment DB is provided. The lane
// participates in the standard RRF combine alongside fts/dense/sparse/
// colbert. With expDB == nil, the function returns the 4-way retriever
// unchanged — opt-in shape per the calibration-obligation contract.
func TestBuildAutoRetrieverWithActivation_AppendsLaneWhenExpDBProvided(t *testing.T) {
	idxDB, expDB, _ := buildActivationTestStack(t)

	// nil expDB: behaves identically to BuildAutoRetrieverFull.
	resWithoutExp := query.BuildAutoRetrieverWithActivation(idxDB, nil)
	require.NotNil(t, resWithoutExp.Retriever)
	defer resWithoutExp.Cleanup()

	// non-nil expDB: activation lane appended (when retriever is hybrid).
	resWithExp := query.BuildAutoRetrieverWithActivation(idxDB, expDB)
	require.NotNil(t, resWithExp.Retriever)
	defer resWithExp.Cleanup()

	hr, ok := resWithExp.Retriever.(*query.HybridRetriever)
	if !ok {
		t.Skip("retriever is not hybrid (no embeddings on fixture vault); activation-lane append is conditional on hybrid path")
	}

	var names []string
	for _, nr := range hr.Retrievers {
		names = append(names, nr.Name)
	}
	assert.Contains(t, names, "activation", "activation lane must be appended when expDB is provided")
}

// Limit + offset behave like other retrievers — pagination over the
// score-ordered list.
func TestActivationRetriever_LimitAndOffset(t *testing.T) {
	idxDB, expDB, ids := buildActivationTestStack(t)
	require.GreaterOrEqual(t, len(ids), 3)
	sess, err := expDB.StartSession("/test")
	require.NoError(t, err)

	for i, id := range ids[:3] {
		for j := 0; j < (3 - i); j++ {
			recordAccessForActivationTest(t, idxDB, expDB, sess, id)
		}
	}

	// Fixed scoring clock so the three Search calls below agree on ordering
	// (a wall-clock 'now' per call flips near-equal scores between calls).
	clk := time.Now().UTC().Add(time.Hour)
	r := &query.ActivationRetriever{DB: idxDB, ExpDB: expDB, Params: experiment.DefaultActivationParams(0.5), Now: func() time.Time { return clk }}
	all, total, err := r.Search(context.Background(), "", 100, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 3)

	first2, _, err := r.Search(context.Background(), "", 2, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Len(t, first2, 2)
	assert.Equal(t, all[0].ID, first2[0].ID)
	assert.Equal(t, all[1].ID, first2[1].ID)

	skipped, _, err := r.Search(context.Background(), "", 2, 1, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, all[1].ID, skipped[0].ID, "offset=1 must skip the top result")
}
