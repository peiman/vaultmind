package cmdutil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenVaultDB_ValidVault(t *testing.T) {
	vdb, err := cmdutil.OpenVaultDB(testvault.IndexedFixtureVault(t))
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
	vdb, err := cmdutil.OpenVaultDB(testvault.IndexedFixtureVault(t))
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

// --- Guessed vault vs. named vault ------------------------------------------
//
// vault.LoadConfig deliberately returns defaults for a directory with no
// .vaultmind/config.yaml, so ANY directory can serve as a vault when the user
// names one. That is right for a named path and wrong for a guessed one: with
// no --vault, discovery falls back to "." and the command would query whatever
// directory the user happened to be standing in, then report success. Observed
// live 2026-08-12: `vaultmind ask` in a non-vault directory returned
// status "ok", warnings [], zero hits, exit 0 — indistinguishable, to a caller,
// from "your vault genuinely has nothing on this topic". Those two answers
// warrant opposite next actions, so they must not share a representation.
//
// The rule: a NAMED vault stays permissive, a GUESSED one must be real.

func newVaultCmd(t *testing.T, jsonOut bool) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", jsonOut, "")
	cmd.Flags().String("vault", ".", "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	return cmd, &buf
}

// The guessed-vault rule has to be callable by commands that never open a
// VaultDB. `index` is the one that matters: it is the command that CREATES
// .vaultmind/index.db, so it is the command that mints the marker every later
// walk-up finds. Guarding only the readers left the writer open — the same
// mistake fixed for `ask` in v0.3.0 and for `arc candidates` in v0.4.0, each
// time without touching the source.
func TestRequireRealVaultIfGuessed(t *testing.T) {
	realVault := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(realVault, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(realVault, ".vaultmind", "config.yaml"), []byte("types: {}"), 0o600))

	// A directory holding only the model cache is NOT a vault, however much it
	// looks like one — this is the $HOME case the whole change exists for.
	cacheOnly := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(cacheOnly, ".vaultmind", "models"), 0o750))
	t.Run("model cache is refused, and says why", func(t *testing.T) {
		t.Setenv("VAULTMIND_VAULT", "")
		cmd, _ := newVaultCmd(t, false)
		err := cmdutil.RequireRealVaultIfGuessed(cmd, cacheOnly, "index")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cache, not a vault")
	})

	t.Run("guessed non-vault is refused", func(t *testing.T) {
		t.Setenv("VAULTMIND_VAULT", "")
		cmd, _ := newVaultCmd(t, false)
		err := cmdutil.RequireRealVaultIfGuessed(cmd, t.TempDir(), "index")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--vault", "the error names the way out")
	})

	t.Run("guessed real vault is allowed", func(t *testing.T) {
		t.Setenv("VAULTMIND_VAULT", "")
		cmd, _ := newVaultCmd(t, false)
		require.NoError(t, cmdutil.RequireRealVaultIfGuessed(cmd, realVault, "index"))
	})

	t.Run("named non-vault stays permissive", func(t *testing.T) {
		t.Setenv("VAULTMIND_VAULT", "")
		cmd, _ := newVaultCmd(t, false)
		require.NoError(t, cmd.Flags().Set("vault", "somewhere"))
		require.NoError(t, cmdutil.RequireRealVaultIfGuessed(cmd, t.TempDir(), "index"),
			"naming a directory is how you deliberately make one a vault")
	})

	t.Run("json mode writes an envelope and reports already-written", func(t *testing.T) {
		t.Setenv("VAULTMIND_VAULT", "")
		cmd, buf := newVaultCmd(t, true)
		err := cmdutil.RequireRealVaultIfGuessed(cmd, t.TempDir(), "index")
		require.True(t, errors.Is(err, cmdutil.ErrAlreadyWritten))
		var env envelope.Envelope
		require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
		assert.Equal(t, "error", env.Status)
		require.Len(t, env.Errors, 1)
		assert.Equal(t, "vault_not_found", env.Errors[0].Code)
	})
}

func TestOpenVaultDBOrWriteErr_GuessedNonVaultFailsClosed(t *testing.T) {
	t.Setenv("VAULTMIND_VAULT", "") // no env override in play
	dir := t.TempDir()              // a real directory that is NOT a vault
	cmd, buf := newVaultCmd(t, true)

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, dir, "ask")
	assert.Nil(t, vdb)
	require.Error(t, err)
	require.True(t, errors.Is(err, cmdutil.ErrAlreadyWritten))

	var env envelope.Envelope
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "error", env.Status, "a guessed non-vault must not report success")
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "vault_not_found", env.Errors[0].Code)
	assert.Contains(t, env.Errors[0].Message, "--vault",
		"the error must name the way out, not just the problem")
}

func TestOpenVaultDBOrWriteErr_GuessedNonVaultTextIsActionable(t *testing.T) {
	t.Setenv("VAULTMIND_VAULT", "")
	dir := t.TempDir()
	cmd, _ := newVaultCmd(t, false)

	_, err := cmdutil.OpenVaultDBOrWriteErr(cmd, dir, "ask")
	require.Error(t, err)
	assert.False(t, errors.Is(err, cmdutil.ErrAlreadyWritten))
	assert.Contains(t, err.Error(), "vaultmind init",
		"a user with no vault needs to be told how to make one")
}

func TestOpenVaultDBOrWriteErr_NamedVaultFlagStaysPermissive(t *testing.T) {
	t.Setenv("VAULTMIND_VAULT", "")
	dir := t.TempDir()
	cmd, _ := newVaultCmd(t, true)
	require.NoError(t, cmd.Flags().Set("vault", dir)) // marks the flag Changed

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, dir, "ask")
	require.NoError(t, err, "an explicitly named directory stays usable as a vault")
	require.NotNil(t, vdb)
	vdb.Close()
}

func TestOpenVaultDBOrWriteErr_NamedViaEnvStaysPermissive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VAULTMIND_VAULT", dir) // how the installed hook scripts point at a vault
	cmd, _ := newVaultCmd(t, true)

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, dir, "ask")
	require.NoError(t, err, "VAULTMIND_VAULT is a deliberate choice, same as --vault")
	require.NotNil(t, vdb)
	vdb.Close()
}

func TestOpenVaultDBOrWriteErr_GuessedRealVaultStillOpens(t *testing.T) {
	t.Setenv("VAULTMIND_VAULT", "")
	cmd, _ := newVaultCmd(t, true)

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, testvault.IndexedFixtureVault(t), "ask")
	require.NoError(t, err, "discovery landing on a REAL vault must keep working")
	require.NotNil(t, vdb)
	vdb.Close()
}

func TestOpenVaultDBOrWriteErr_CommandWithoutVaultFlagUnaffected(t *testing.T) {
	t.Setenv("VAULTMIND_VAULT", "")
	// Commands that take no --vault flag never went through discovery, so the
	// guard must not retroactively tighten them.
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", true, "")

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, testvault.IndexedFixtureVault(t), "doctor")
	require.NoError(t, err)
	require.NotNil(t, vdb)
	vdb.Close()
}
