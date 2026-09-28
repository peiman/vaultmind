package cmdutil

import (
	"testing"

	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Opening a vault does not hash its index. The hash reads the whole file — a
// 1 GB index on a 418-note BGE-M3 vault — and only JSON envelopes report it,
// yet every open paid for it: 18% of the CPU of a three-vault recall query
// (profiled 2026-09-28). It is computed on the first GetIndexHash, then kept.
func TestOpenVaultDB_DoesNotHashUntilAsked(t *testing.T) {
	before := indexHashes.Load()
	vdb, err := OpenVaultDB(testvault.IndexedFixtureVault(t))
	require.NoError(t, err)
	defer vdb.Close()
	assert.Equal(t, before, indexHashes.Load(), "opening a vault must not read the whole index")

	first := vdb.GetIndexHash()
	second := vdb.GetIndexHash()
	assert.Len(t, first, 64)
	assert.Equal(t, first, second)
	assert.Equal(t, before+1, indexHashes.Load(), "hashed once, on first request")
}
