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

	// 42 notes should index in under 5 seconds even without batching
	// With batching it should be well under 1 second
	assert.Less(t, duration, 5*time.Second, "rebuild should complete quickly")

	t.Logf("Indexed %d notes in %v (DurationMs=%d)", result.Indexed, duration, result.DurationMs)
}
