package baseline_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/baseline"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/require"
)

// -baseline-update regenerates the committed snapshot when the retrieval
// behavior has legitimately changed (e.g. an intended ranking improvement).
// WITHOUT the flag the test is a regression gate: any aggregate or
// per-query metric drop beyond DefaultTolerance fails.
//
// Regeneration discipline: only run `-baseline-update` when the new
// numbers are *intended*. The diff on baseline.json must be reviewed in
// the commit — a silent snapshot refresh would defeat the gate.
var updateSnapshot = flag.Bool("baseline-update", false,
	"regenerate test/fixtures/baseline/baseline.json from current retrieval")

const (
	fixtureRoot   = "../../test/fixtures/baseline"
	snapshotK     = 5
	snapshotLimit = 10
)

// TestBaseline_GoldenFixture is the full regression gate:
// committed vault + committed queries → committed snapshot. Runs in CI.
func TestBaseline_GoldenFixture(t *testing.T) {
	queriesPath := filepath.Join(fixtureRoot, "queries.yaml")
	snapshotPath := filepath.Join(fixtureRoot, "baseline.json")
	vaultSrc := filepath.Join(fixtureRoot, "vault")

	queries, err := baseline.LoadQueries(queriesPath)
	require.NoError(t, err)

	// Index the committed fixture vault into a tempdir. SQLite DB files
	// aren't committed; the vault+queries are the contract.
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "index.db")
	cfg, err := vault.LoadConfig(vaultSrc)
	require.NoError(t, err)
	_, err = index.NewIndexer(vaultSrc, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	retriever := &query.FTSRetriever{DB: db}
	current, err := baseline.Run(retriever, queries, baseline.RunConfig{K: snapshotK, Limit: snapshotLimit})
	require.NoError(t, err)

	if *updateSnapshot {
		data, err := json.MarshalIndent(current, "", "  ")
		require.NoError(t, err)
		data = append(data, '\n')
		require.NoError(t, os.WriteFile(snapshotPath, data, 0o644))
		t.Logf("snapshot regenerated at %s", snapshotPath)
		return
	}

	snap, err := baseline.LoadSnapshot(snapshotPath)
	require.NoError(t, err, "snapshot missing — run with -baseline-update to generate")

	diff, err := baseline.CompareToSnapshot(current, snap, baseline.DefaultTolerance)
	require.NoError(t, err)
	if !diff.OK {
		for _, r := range diff.Regressions {
			t.Errorf("BASELINE REGRESSION: %s", r)
		}
		t.Errorf("\nIf the retrieval change is intended, regenerate with:\n  go test ./internal/baseline -run TestBaseline_GoldenFixture -baseline-update\nThen review the diff on %s before committing.", snapshotPath)
	}
}
