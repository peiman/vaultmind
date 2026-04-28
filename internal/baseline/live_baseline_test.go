//go:build dev

package baseline_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/baseline"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
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
// "retrieval verified working" are grounded in measurement, not vibes. Not
// committed as a regression gate — live vaults change often, so this is
// for ad-hoc measurement only.
func TestLiveBaseline(t *testing.T) {
	name := os.Getenv("VAULTMIND_LIVE_BASELINE")
	if name == "" {
		t.Skip("set VAULTMIND_LIVE_BASELINE=identity|research to run")
	}

	type vaultSpec struct {
		dir     string
		queries string
	}
	liveBaselineVaults := map[string]vaultSpec{
		"identity": {
			dir:     "../../vaultmind-identity",
			queries: "../../test/fixtures/baseline/identity-queries.yaml",
		},
		"research": {
			dir:     "../../vaultmind-vault",
			queries: "../../test/fixtures/baseline/research-queries.yaml",
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
}
