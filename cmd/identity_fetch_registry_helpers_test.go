package cmd

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/identity/anchor"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two resolvers gave two answers on one machine (#151): fetch-registry
// verified against the config pin while doctor, reading only the enroll
// anchor, called the same registry NOT authenticated. One resolver now.

// pinEnv isolates the pin sources: no config pin, no anchor, unless a test
// sets one. The developer's own config may pin a real root.
func pinEnv(t *testing.T) (xdgData string) {
	t.Helper()
	xdgData = t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgData)
	prev := viper.Get(config.KeyAppIdentityfetchregistryRootPubkey)
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, "")
	t.Cleanup(func() { viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, prev) })
	return xdgData
}

func pinKey(t *testing.T, seed string) ed25519.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(seedReader(seed))
	require.NoError(t, err)
	return pub
}

func b64(k ed25519.PublicKey) string { return base64.StdEncoding.EncodeToString(k) }

func writeAnchor(t *testing.T, xdgData string, pub ed25519.PublicKey) {
	t.Helper()
	p := filepath.Join(xdgData, "vaultmind", networkAnchorFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
	require.NoError(t, anchor.Upsert(p, anchor.NetworkAnchor{
		NetworkID: registry.NetworkID(pub), RootPubKey: b64(pub), ConfirmedAt: time.Now().Unix(),
	}))
}

func TestResolveRootPin_NothingPinnedIsNoPin(t *testing.T) {
	pinEnv(t)
	pin, err := resolveRootPin("", "--flag")
	require.NoError(t, err)
	assert.Nil(t, pin.pub)
	assert.False(t, pin.declared)
}

func TestResolveRootPin_ConfigPinAlone(t *testing.T) {
	pinEnv(t)
	k := pinKey(t, "root-pin-config-seed-padding-xxxx")
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, b64(k))

	pin, err := resolveRootPin("", "--flag")
	require.NoError(t, err)
	assert.Equal(t, k, pin.pub)
	assert.Equal(t, registry.NetworkID(k), pin.networkID)
	assert.True(t, pin.declared)
}

func TestResolveRootPin_AnchorAlone(t *testing.T) {
	xdg := pinEnv(t)
	k := pinKey(t, "root-pin-anchor-seed-padding-xxxx")
	writeAnchor(t, xdg, k)

	pin, err := resolveRootPin("", "--flag")
	require.NoError(t, err)
	assert.Equal(t, k, pin.pub)
}

func TestResolveRootPin_FlagBeatsConfigBeatsAnchor(t *testing.T) {
	xdg := pinEnv(t)
	flagK := pinKey(t, "root-pin-flag-seed-padding-xxxxxx")
	cfgK := pinKey(t, "root-pin-config-seed-padding-xxxx")
	writeAnchor(t, xdg, pinKey(t, "root-pin-anchor-seed-padding-xxxx"))
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, b64(cfgK))

	pin, err := resolveRootPin(b64(flagK), "--flag")
	require.NoError(t, err)
	assert.Equal(t, flagK, pin.pub, "an explicit flag wins")

	pin, err = resolveRootPin("", "--flag")
	require.NoError(t, err)
	assert.Equal(t, cfgK, pin.pub, "the config pin beats the anchor")
}

// A value an operator typed that does not decode is an error naming where it
// came from — never a silent fall-through to a weaker source.
func TestResolveRootPin_MalformedValuesNameTheirSource(t *testing.T) {
	pinEnv(t)
	_, err := resolveRootPin("not base64!!", "--mesh-root-pubkey")
	require.ErrorContains(t, err, "--mesh-root-pubkey")

	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, "not base64!!")
	_, err = resolveRootPin("", "--mesh-root-pubkey")
	require.ErrorContains(t, err, config.KeyAppIdentityfetchregistryRootPubkey)
}

// The #151 case end to end: a machine pinned ONLY through the config key.
// With no running signer the pinned path's honest verdict is key-mismatch —
// the same verdict the anchor-pinned test gets — and never "unpinned".
func TestDoctor_MeshConfigPinIsAPin(t *testing.T) {
	chdirToTemp(t)
	pinEnv(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("AGENT_CHAT_DAEMON_URL", "http://127.0.0.1:1")

	rootPub, rootPriv, err := ed25519.GenerateKey(seedReader("doctor-cmd-offline-root-seed-pad!!"))
	require.NoError(t, err)
	memberPub := pinKey(t, "doctor-cmd-offline-member-seed-pad")
	regBytes, _ := buildCmdSignedRegistry(t, rootPub, rootPriv, "mira", memberPub, time.Now())
	regFile := filepath.Join(t.TempDir(), "registry.json")
	require.NoError(t, os.WriteFile(regFile, regBytes, 0o644))
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, b64(rootPub))

	vault := buildCleanIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json",
		"--mesh-registry", regFile, "--mesh-slug", "agent:mira")
	require.NoError(t, err)

	var je jsonMeshEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &je))
	require.NotNil(t, je.Result.MeshIdentity)
	assert.Equal(t, query.StatusMeshKeyMismatch, je.Result.MeshIdentity.Status,
		"a config-pinned machine is on the PINNED path, not self-consistent-unpinned")
}
