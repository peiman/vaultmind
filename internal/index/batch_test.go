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

	// Per-note threshold so the test remains meaningful as the vault grows and
	// survives parallel-load contention under `task check`. Idle runs are
	// ~8ms/note; 200ms/note leaves ample headroom.
	perNote := time.Duration(result.Indexed) * 200 * time.Millisecond
	assert.Less(t, duration, perNote, "rebuild should stay within %v per note", 200*time.Millisecond)

	t.Logf("Indexed %d notes in %v (DurationMs=%d)", result.Indexed, duration, result.DurationMs)
}
