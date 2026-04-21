package baseline_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/baseline"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildBaselineVault constructs a small curated vault with predictable
// content so query→expected-ID assertions are stable across runs. Topics
// are chosen so each query has a distinct "obvious" top match.
func buildBaselineVault(t *testing.T) *index.DB {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))

	notes := map[string]string{
		"act-r.md": `---
id: c-actr
type: concept
title: ACT-R Cognitive Architecture
---
ACT-R models declarative memory with activation decay.
`,
		"spreading.md": `---
id: c-spreading
type: concept
title: Spreading Activation
---
Spreading activation propagates through associative networks.
`,
		"rrf.md": `---
id: c-rrf
type: concept
title: Reciprocal Rank Fusion
---
Reciprocal rank fusion combines rankings across retrievers.
`,
		"hybrid.md": `---
id: c-hybrid
type: concept
title: Hybrid Retrieval
---
Hybrid retrieval blends dense and sparse signals.
`,
		"graph.md": `---
id: c-graph
type: concept
title: Knowledge Graph Embeddings
---
Knowledge graph embeddings map nodes into vector spaces.
`,
	}
	for name, body := range notes {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Runner contract: given a DB + queries, execute each query through the
// keyword retriever, collect top-K result IDs, compute Hit@K and MRR
// against expected IDs, return a Report with per-query rows + aggregates.
//
// Keyword is intentional: CI can't run model-based retrieval. A baseline
// that requires embeddings couldn't run deterministically as a gate.

func TestRun_ReportsPerQueryAndAggregateMetrics(t *testing.T) {
	db := buildBaselineVault(t)
	retriever := &query.FTSRetriever{DB: db}

	queries := []baseline.Query{
		{Name: "spreading", Text: "spreading activation", Expected: []string{"c-spreading"}},
		{Name: "rrf", Text: "reciprocal rank", Expected: []string{"c-rrf"}},
	}

	report, err := baseline.Run(retriever, queries, baseline.RunConfig{K: 5, Limit: 5})
	require.NoError(t, err)

	require.Len(t, report.Queries, 2, "one row per query")
	for _, q := range report.Queries {
		// With obvious query→note mapping, both should hit at K=5.
		assert.Equal(t, 1.0, q.HitAtK, "query %q must hit on this fixture", q.Name)
		assert.Greater(t, q.ReciprocalRank, 0.0, "query %q must have a non-zero reciprocal rank", q.Name)
	}

	// Aggregates are the mean across queries.
	assert.Equal(t, 1.0, report.HitAtK, "perfect hit rate on this fixture")
	assert.Greater(t, report.MRR, 0.0)
}

// A query that the retriever can't satisfy (no matching body/title tokens)
// must produce a 0/0 row — but not an error. Baselines see partial failures
// all the time and the aggregate should reflect them numerically, not
// crash the runner.
func TestRun_HandlesMissQueriesGracefully(t *testing.T) {
	db := buildBaselineVault(t)
	retriever := &query.FTSRetriever{DB: db}

	queries := []baseline.Query{
		{Name: "nothing", Text: "zzzyyyxxx-nonexistent-term", Expected: []string{"c-actr"}},
	}

	report, err := baseline.Run(retriever, queries, baseline.RunConfig{K: 5, Limit: 5})
	require.NoError(t, err)
	require.Len(t, report.Queries, 1)
	assert.Equal(t, 0.0, report.Queries[0].HitAtK, "miss query registers as 0, not error")
	assert.Equal(t, 0.0, report.Queries[0].ReciprocalRank)
	assert.Equal(t, 0.0, report.HitAtK)
	assert.Equal(t, 0.0, report.MRR)
}

// Empty query slice is a legit edge case (e.g. fixture file with 0 entries).
// Return an empty report with NaN-free zeroed aggregates rather than
// dividing by zero.
func TestRun_EmptyQueriesReturnsZeroedReport(t *testing.T) {
	db := buildBaselineVault(t)
	retriever := &query.FTSRetriever{DB: db}

	report, err := baseline.Run(retriever, nil, baseline.RunConfig{K: 5, Limit: 5})
	require.NoError(t, err)
	assert.Empty(t, report.Queries)
	assert.Equal(t, 0.0, report.HitAtK)
	assert.Equal(t, 0.0, report.MRR)
}

// The Report carries the resolved top-K IDs per query — not just the
// metric. Seeing *which* notes came back is what makes a baseline
// regression actually diagnosable; a number without provenance is a
// "numbers were wrong" trap waiting to happen.
func TestRun_ReportIncludesReturnedIDs(t *testing.T) {
	db := buildBaselineVault(t)
	retriever := &query.FTSRetriever{DB: db}

	queries := []baseline.Query{
		{Name: "hybrid", Text: "hybrid retrieval", Expected: []string{"c-hybrid"}},
	}

	report, err := baseline.Run(retriever, queries, baseline.RunConfig{K: 3, Limit: 3})
	require.NoError(t, err)
	require.Len(t, report.Queries, 1)
	assert.NotEmpty(t, report.Queries[0].ResultIDs,
		"report must carry the returned IDs so a regression is diagnosable, not just a number")
	assert.Contains(t, report.Queries[0].ResultIDs, "c-hybrid",
		"expected ID must appear in ResultIDs (exercises retrieval + projection together)")
}
