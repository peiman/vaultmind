package index_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rebuildBudgetPerNote is the wall-clock allowance per note for a rebuild.
const rebuildBudgetPerNote = time.Second

func TestRebuild_CompletesInReasonableTime(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)

	start := time.Now()
	result, err := idxr.Rebuild()
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Greater(t, result.Indexed, 0)

	// A guard against an order-of-magnitude regression, not a benchmark. Idle
	// runs are ~8ms/note; the budget is 1s/note because every main run is
	// instrumented for coverage on shared runners, and a stalled runner once
	// took 250ms/note (an 8-note rebuild at 2.00s against the old 200ms/note
	// budget, failing a release pipeline: #172). Skipped under -race: the
	// detector's instrumentation adds heavy, variable overhead that makes any
	// wall-clock budget meaningless. The correctness assertions above still run.
	if !raceEnabled {
		perNote := time.Duration(result.Indexed) * rebuildBudgetPerNote
		assert.Less(t, duration, perNote, "rebuild should stay within %v per note", rebuildBudgetPerNote)
	}

	t.Logf("Indexed %d notes in %v (DurationMs=%d)", result.Indexed, duration, result.DurationMs)
}
