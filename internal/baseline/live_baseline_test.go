//go:build dev

package baseline_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/baseline"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLiveBaseline runs a curated golden-query set against a LIVE vault
// using whatever retriever the auto-builder selects (BGE-M3 hybrid when
// embeddings are present, FTS fallback otherwise). Set
// VAULTMIND_LIVE_BASELINE=<vault-name> to enable; the test is dev-only so
// it never runs in CI.
//
// Vault names map to (vaultDir, queriesPath) pairs below. To add a vault:
//
//  1. Curate a queries.yaml file (see test/fixtures/baseline/identity-queries.yaml
//     for the format and curation conventions — small expected sets,
//     high-confidence ground truth, cover varied retrieval-lane exercises).
//  2. Add an entry to liveBaselineVaults below.
//  3. Run: VAULTMIND_LIVE_BASELINE=<name> go test -tags "dev ORT" -v -run TestLiveBaseline ./internal/baseline/...
//
// Purpose: produce Hit@K / MRR data points for live vaults so claims like
// "retrieval verified working" are grounded in measurement, not vibes.
//
// Floor gating (added 2026-04-29): each vault carries minHitAtK and minMRR
// thresholds; the test fails when either drops below its floor. Floors are
// set conservatively (~5pp below the post-sleep-wave measurement) so the
// gate catches catastrophic regressions without flapping on rank shuffles.
// Live vault snapshots aren't committed (vaults change too often), but the
// queries.yaml + the floors are the contract.
func TestLiveBaseline(t *testing.T) {
	name := os.Getenv("VAULTMIND_LIVE_BASELINE")
	if name == "" {
		t.Skip("set VAULTMIND_LIVE_BASELINE=identity|research to run")
	}

	type vaultSpec struct {
		dir       string
		queries   string
		minHitAtK float64
		minMRR    float64
	}
	// Floors set 2026-04-29 after sleep+brain content wave brought research
	// vault to 230+ sources / 184 concepts. Measured numbers at gate
	// installation: identity Hit@5 = 1.000, MRR = 0.921 (n=19); research
	// Hit@5 = 0.975, MRR = 0.822 (n=40, one pre-existing miss on
	// decision-structured-vs-embeddings). Floors leave ~5pp headroom for
	// rank shuffle on benign content additions; a drop past them is a real
	// regression, not noise.
	liveBaselineVaults := map[string]vaultSpec{
		"identity": {
			dir:       "../../vaultmind-identity",
			queries:   "../../test/fixtures/baseline/identity-queries.yaml",
			minHitAtK: 0.95,
			minMRR:    0.85,
		},
		"research": {
			dir:       "../../vaultmind-vault",
			queries:   "../../test/fixtures/baseline/research-queries.yaml",
			minHitAtK: 0.95,
			minMRR:    0.75,
		},
	}
	spec, ok := liveBaselineVaults[name]
	if !ok {
		t.Fatalf("unknown vault %q — known: identity, research", name)
	}

	queries, err := baseline.LoadQueries(spec.queries)
	require.NoError(t, err)
	t.Logf("loaded %d golden queries for vault %q", len(queries), name)

	db, err := index.Open(filepath.Join(spec.dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer db.Close()

	r, cleanup, err := query.BuildAutoRetriever(db)
	require.NoError(t, err)
	defer cleanup()

	report, err := baseline.Run(r, queries, baseline.RunConfig{K: 5, Limit: 10})
	require.NoError(t, err)

	t.Logf("aggregate (%s): Hit@5 = %.3f  MRR = %.3f  (n=%d)", name, report.HitAtK, report.MRR, len(report.Queries))
	for _, q := range report.Queries {
		marker := "✓"
		if q.HitAtK == 0 {
			marker = "✗"
		}
		t.Logf("  %s %-40s rr=%.3f  hit@5=%.0f  expected=%v",
			marker, q.Name, q.ReciprocalRank, q.HitAtK, q.Expected)
	}

	assert.GreaterOrEqual(t, report.HitAtK, spec.minHitAtK,
		"Hit@5 floor breach for %q: got %.3f, floor %.3f. Investigate before regenerating floors.",
		name, report.HitAtK, spec.minHitAtK)
	assert.GreaterOrEqual(t, report.MRR, spec.minMRR,
		"MRR floor breach for %q: got %.3f, floor %.3f. Top-1 quality regressed; check ranking changes.",
		name, report.MRR, spec.minMRR)
}
