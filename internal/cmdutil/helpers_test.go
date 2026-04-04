package cmdutil_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenVaultDB_ValidVault(t *testing.T) {
	vdb, err := cmdutil.OpenVaultDB("../../vaultmind-vault")
	require.NoError(t, err)
	defer vdb.Close()

	assert.NotNil(t, vdb.DB)
	assert.NotNil(t, vdb.Config)
	assert.NotEmpty(t, vdb.Config.Types)
}

func TestOpenVaultDB_InvalidPath(t *testing.T) {
	_, err := cmdutil.OpenVaultDB("/nonexistent/path")
	assert.Error(t, err)
}
