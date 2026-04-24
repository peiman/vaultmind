package testvault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const researchVaultPath = "../../vaultmind-vault"

func TestSharedIndexDBPath_ReturnsSamePathAcrossCalls(t *testing.T) {
	first := testvault.SharedIndexDBPath(t, researchVaultPath)
	second := testvault.SharedIndexDBPath(t, researchVaultPath)

	assert.Equal(t, first, second, "shared DB path must be stable across calls within a test process")

	info, err := os.Stat(first)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0), "shared DB file must be non-empty")
}

func TestOpenSharedDB_ReturnsUsableIndependentCopy(t *testing.T) {
	dstA := filepath.Join(t.TempDir(), "a.db")
	dstB := filepath.Join(t.TempDir(), "b.db")

	dbA := testvault.OpenSharedDB(t, researchVaultPath, dstA)
	t.Cleanup(func() { _ = dbA.Close() })
	dbB := testvault.OpenSharedDB(t, researchVaultPath, dstB)
	t.Cleanup(func() { _ = dbB.Close() })

	// Both copies must be usable concurrently and query the same vault content.
	rowA, err := dbA.QueryNoteByID("concept-act-r")
	require.NoError(t, err)
	require.NotNil(t, rowA)
	rowB, err := dbB.QueryNoteByID("concept-act-r")
	require.NoError(t, err)
	require.NotNil(t, rowB)
	assert.Equal(t, rowA.ID, rowB.ID)

	// The copies must live at distinct filesystem paths so mutations in one
	// don't leak into the other.
	assert.NotEqual(t, dstA, dstB)
	_, err = os.Stat(dstA)
	assert.NoError(t, err)
	_, err = os.Stat(dstB)
	assert.NoError(t, err)
}
