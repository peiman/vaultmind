package cmd

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/peiman/vaultmind/internal/signhealth"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signEnvelopeEpoch1 is from mira at key_epoch 1. sessionMesh binds mira at 4.
const signEnvelopeEpoch1 = `{"alg_version":1,"body":"hello","from_agent":"mira","key_epoch":1,"nonce":"YWJjZGVmZ2hpamtsbW5vcA==","room":"dev","seq":7,"ts":2000000}`

// signEnvelopeEpoch4 matches sessionMesh's binding for mira.
const signEnvelopeEpoch4 = `{"alg_version":1,"body":"hello","from_agent":"mira","key_epoch":4,"nonce":"YWJjZGVmZ2hpamtsbW5vcA==","room":"dev","seq":7,"ts":2000000}`

// signEnvelopeEpoch1Golden is what sign-envelope printed for signEnvelopeEpoch1
// under miraSeed before the binding check existed (captured from main
// at 5bcf7e8). When the check is skipped, the output must not move a byte.
const signEnvelopeEpoch1Golden = `{"from_pubkey":"sNr5Pd9a5LhUtD2IjN0LM7Oq8wgS5beeUyEUIsI3VrQ=","key_epoch":1,"sig":"vdbJQs5+m3rIIBH/f7cVvD7JHft9+VoWS4nO83wiYMNIi6UpYeZGj+HRp0844Cz9VGL+ICfGKZPEPNh0l8WcDg=="}` + "\n"

// miraSeed derives the key signEnvelopeEpoch1Golden was signed with.
const miraSeed = "sign-envelope-binding-check"

func signEnvelopeKey(t *testing.T, seed string) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(seedReader(seed))
	require.NoError(t, err)
	return pub, priv
}

// runSignEnvelope writes env to a file and runs sign-envelope against sock,
// passing pub as the from_pubkey hint.
func runSignEnvelope(t *testing.T, sock, env string, pub ed25519.PublicKey) (*bytes.Buffer, error) {
	t.Helper()
	envPath := filepath.Join(t.TempDir(), "env.json")
	require.NoError(t, os.WriteFile(envPath, []byte(env), 0o600))
	out, _, err := runRootCmd(t, "identity", "sign-envelope", "--file", envPath,
		"--signer-socket", sock, "--from-pubkey", base64.StdEncoding.EncodeToString(pub))
	return out, err
}

// unpinMesh leaves no pin and no agents.yaml: the ceremony user's machine.
func unpinMesh(t *testing.T) {
	t.Helper()
	pinEnv(t)
	t.Setenv(envAgentRegistry, filepath.Join(t.TempDir(), "none.yaml"))
}

// sessionMeshRegistryFile is the registry file sessionMesh wrote.
func sessionMeshRegistryFile(t *testing.T) string {
	t.Helper()
	path := registryFileFromAgentsYAML(registryPath(), projectPath())
	require.NotEmpty(t, path)
	return path
}

func isolateSignHealth(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	xdg.SetAppName("vaultmind")
}

func readSignHealth(t *testing.T, agent string) map[string]any {
	t.Helper()
	path, err := xdg.StateFile(filepath.Join("sign-health", signhealth.SanitizeAgent(agent)+".json"))
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var record map[string]any
	require.NoError(t, json.Unmarshal(raw, &record))
	return record
}

// When this machine has no mesh configured, sign-envelope signs exactly as it
// did before the check existed, even for an epoch no registry would accept.
func TestIdentitySignEnvelope_NotConfiguredSignsAsBefore(t *testing.T) {
	pub, priv := signEnvelopeKey(t, miraSeed)

	t.Run("no pin and no agents.yaml", func(t *testing.T) {
		unpinMesh(t)
		out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch1, pub)
		require.NoError(t, err)
		assert.Equal(t, signEnvelopeEpoch1Golden, out.String())
	})

	t.Run("registry file but no pin", func(t *testing.T) {
		sessionMesh(t, pub)
		viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, "")
		out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch1, pub)
		require.NoError(t, err)
		assert.Equal(t, signEnvelopeEpoch1Golden, out.String())
	})

	t.Run("pin but no registry file", func(t *testing.T) {
		sessionMesh(t, pub)
		agents := os.Getenv(envAgentRegistry)
		raw, err := os.ReadFile(agents)
		require.NoError(t, err)
		raw = bytes.ReplaceAll(raw, []byte("registry_path:"), []byte("not_registry_path:"))
		require.NoError(t, os.WriteFile(agents, raw, 0o600))
		out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch1, pub)
		require.NoError(t, err)
		assert.Equal(t, signEnvelopeEpoch1Golden, out.String())
	})
}

func TestIdentitySignEnvelope_EpochMismatchRefused(t *testing.T) {
	pub, priv := signEnvelopeKey(t, miraSeed)
	sessionMesh(t, pub)
	out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch1, pub)
	require.ErrorContains(t, err, identitycli.ErrEnvelopeEpochMismatch)
	assert.Contains(t, err.Error(), "mira")
	assert.Contains(t, err.Error(), "key_epoch 1")
	assert.Contains(t, err.Error(), "key_epoch 4")
	assert.Empty(t, out.String())
}

func TestIdentitySignEnvelope_ConfiguredButBrokenRefused(t *testing.T) {
	pub, priv := signEnvelopeKey(t, miraSeed)

	t.Run("registry file missing", func(t *testing.T) {
		sessionMesh(t, pub)
		path := sessionMeshRegistryFile(t)
		require.NoError(t, os.Remove(path))
		out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
		require.ErrorContains(t, err, identitycli.ErrEnvelopeRegistry)
		assert.Contains(t, err.Error(), path)
		assert.Empty(t, out.String())
	})

	t.Run("registry fails verification under the pin", func(t *testing.T) {
		sessionMesh(t, pub)
		path := sessionMeshRegistryFile(t)
		viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, b64(pinKey(t, "some-other-root")))
		out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
		require.ErrorContains(t, err, identitycli.ErrEnvelopeRegistry)
		assert.Contains(t, err.Error(), path)
		assert.Empty(t, out.String())
	})

	t.Run("registry file is not a registry", func(t *testing.T) {
		sessionMesh(t, pub)
		path := sessionMeshRegistryFile(t)
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))
		out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
		require.ErrorContains(t, err, identitycli.ErrEnvelopeRegistry)
		assert.Contains(t, err.Error(), path)
		assert.Empty(t, out.String())
	})
}

func TestIdentitySignEnvelope_SignerKeyDiffersPrintsNothing(t *testing.T) {
	pub, _ := signEnvelopeKey(t, miraSeed)
	_, otherPriv := signEnvelopeKey(t, "not-mira")
	sessionMesh(t, pub)
	out, err := runSignEnvelope(t, startCmdSignerWithKey(t, otherPriv), signEnvelopeEpoch4, pub)
	require.ErrorContains(t, err, identitycli.ErrEnvelopeKeyMismatch)
	assert.Empty(t, out.String())
}

func TestIdentitySignEnvelope_MatchingBindingSigns(t *testing.T) {
	pub, priv := signEnvelopeKey(t, miraSeed)

	unpinMesh(t)
	before, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
	require.NoError(t, err)

	sessionMesh(t, pub)
	got, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
	require.NoError(t, err)
	assert.Equal(t, before.String(), got.String())
}

func TestIdentitySignEnvelope_RecordsFailedAttempt(t *testing.T) {
	isolateSignHealth(t)
	pub, priv := signEnvelopeKey(t, miraSeed)
	sessionMesh(t, pub)
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, b64(pinKey(t, "some-other-root")))

	out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
	require.ErrorContains(t, err, identitycli.ErrEnvelopeRegistry)
	assert.Empty(t, out.String())
	record := readSignHealth(t, "mira")
	assert.Equal(t, "mira", record["agent"])
	assert.Equal(t, false, record["ok"])
	assert.Contains(t, record["error"], identitycli.ErrEnvelopeRegistry)
	assert.Equal(t, Version, record["version"])
}

func TestIdentitySignEnvelope_LaterSuccessOverwritesFailure(t *testing.T) {
	isolateSignHealth(t)
	pub, priv := signEnvelopeKey(t, miraSeed)
	sessionMesh(t, pub)
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, b64(pinKey(t, "some-other-root")))
	_, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
	require.Error(t, err)

	unpinMesh(t)
	out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
	require.NoError(t, err)
	assert.NotEmpty(t, out.String())
	record := readSignHealth(t, "mira")
	assert.Equal(t, true, record["ok"])
	assert.NotContains(t, record, "error")
}

func TestIdentitySignEnvelope_UnparsableEnvelopeRecordsNothing(t *testing.T) {
	isolateSignHealth(t)
	pub, priv := signEnvelopeKey(t, miraSeed)
	unpinMesh(t)
	_, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), "not json", pub)
	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(os.Getenv("XDG_STATE_HOME"), "vaultmind", "sign-health"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestIdentitySignEnvelope_StateWriteFailureLeavesResultUnchanged(t *testing.T) {
	isolateSignHealth(t)
	stateBlocker := filepath.Join(t.TempDir(), "state-blocker")
	require.NoError(t, os.WriteFile(stateBlocker, []byte("not a directory"), 0o600))
	t.Setenv("XDG_STATE_HOME", stateBlocker)
	pub, priv := signEnvelopeKey(t, miraSeed)
	sessionMesh(t, pub)
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, b64(pinKey(t, "some-other-root")))

	out, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
	require.ErrorContains(t, err, identitycli.ErrEnvelopeRegistry)
	assert.Empty(t, out.String())
}

func TestIdentitySignEnvelope_StateWriteFailureLeavesSuccessUnchanged(t *testing.T) {
	isolateSignHealth(t)
	pub, priv := signEnvelopeKey(t, miraSeed)
	unpinMesh(t)
	want, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
	require.NoError(t, err)

	stateBlocker := filepath.Join(t.TempDir(), "state-blocker")
	require.NoError(t, os.WriteFile(stateBlocker, []byte("not a directory"), 0o600))
	t.Setenv("XDG_STATE_HOME", stateBlocker)
	got, err := runSignEnvelope(t, startCmdSignerWithKey(t, priv), signEnvelopeEpoch4, pub)
	require.NoError(t, err)
	assert.Equal(t, want.String(), got.String())
}
