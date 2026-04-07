package cmdutil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/spf13/cobra"
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

func TestVaultDB_GetIndexHash(t *testing.T) {
	vdb, err := cmdutil.OpenVaultDB("../../vaultmind-vault")
	require.NoError(t, err)
	defer vdb.Close()

	hash := vdb.GetIndexHash()
	assert.NotEmpty(t, hash, "index hash should not be empty")
	assert.Len(t, hash, 64, "SHA-256 hex should be 64 chars")

	hash2 := vdb.GetIndexHash()
	assert.Equal(t, hash, hash2, "hash should be cached")
}

func TestOpenVaultDBOrWriteErr_JSONOutput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", true, "")

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, "/nonexistent/path", "test-command")
	assert.Nil(t, vdb)
	require.Error(t, err)
	assert.True(t, errors.Is(err, cmdutil.ErrAlreadyWritten))

	var env envelope.Envelope
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	assert.Equal(t, "test-command", env.Command)
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "vault_not_found", env.Errors[0].Code)
}

func TestOpenVaultDBOrWriteErr_VaultNotFoundCode(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", true, "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	_, err := cmdutil.OpenVaultDBOrWriteErr(cmd, "/nonexistent/path", "test")
	require.True(t, errors.Is(err, cmdutil.ErrAlreadyWritten))

	var env envelope.Envelope
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "vault_not_found", env.Errors[0].Code,
		"non-existent path should produce vault_not_found code")
}

func TestOpenVaultDBOrWriteErr_TextOutput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, "/nonexistent/path", "test-command")
	assert.Nil(t, vdb)
	require.Error(t, err)
	assert.False(t, errors.Is(err, cmdutil.ErrAlreadyWritten))
	assert.Contains(t, err.Error(), "does not exist")
}
