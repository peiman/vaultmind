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

// TestLiveBaseline_Identity runs the curated identity-queries.yaml set
// against the LIVE vaultmind-identity vault using whatever retriever the
// auto-builder selects (BGE-M3 hybrid when embeddings are present, FTS
// fallback otherwise). Set VAULTMIND_LIVE_BASELINE=1 to enable; the test
// is dev-only so it never runs in CI.
//
// Purpose: produce one Hit@K / MRR data point for the identity vault so
// claims like "retrieval verified working" are grounded in measurement,
// not vibes. Not committed as a regression gate — the live vault changes
// often, so this is for ad-hoc measurement, not for CI to enforce.
func TestLiveBaseline_Identity(t *testing.T) {
	if os.Getenv("VAULTMIND_LIVE_BASELINE") == "" {
		t.Skip("set VAULTMIND_LIVE_BASELINE=1 to run live-vault baseline measurement")
	}

	const vaultDir = "../../vaultmind-identity"
	const queriesPath = "../../test/fixtures/baseline/identity-queries.yaml"
	const dbRel = ".vaultmind/index.db"

	queries, err := baseline.LoadQueries(queriesPath)
	require.NoError(t, err)
	t.Logf("loaded %d golden queries", len(queries))

	db, err := index.Open(filepath.Join(vaultDir, dbRel))
	require.NoError(t, err)
	defer db.Close()

	// Use the auto-retriever — picks up BGE-M3 hybrid when available.
	r, cleanup, err := query.BuildAutoRetriever(db)
	require.NoError(t, err)
	defer cleanup()

	report, err := baseline.Run(r, queries, baseline.RunConfig{K: 5, Limit: 10})
	require.NoError(t, err)

	t.Logf("aggregate: Hit@5 = %.3f  MRR = %.3f  (n=%d)", report.HitAtK, report.MRR, len(report.Queries))
	for _, q := range report.Queries {
		marker := "✓"
		if q.HitAtK == 0 {
			marker = "✗"
		}
		t.Logf("  %s %-32s rr=%.3f  hit@5=%.0f  expected=%v  got=%v",
			marker, q.Name, q.ReciprocalRank, q.HitAtK, q.Expected, q.ResultIDs)
	}
}
