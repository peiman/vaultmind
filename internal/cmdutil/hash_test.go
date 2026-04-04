package cmdutil_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenVaultDB_HasIndexHash(t *testing.T) {
	vdb, err := cmdutil.OpenVaultDB("../../vaultmind-vault")
	require.NoError(t, err)
	defer vdb.Close()

	hash := vdb.IndexHash()
	assert.NotEmpty(t, hash, "index hash must be computed")
	assert.Len(t, hash, 64, "must be SHA-256 hex (64 chars)")
}
