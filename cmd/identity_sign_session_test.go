package cmd

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/signer"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startCmdSignerWithKey(t *testing.T, priv ed25519.PrivateKey) string {
	t.Helper()
	keyDir, err := os.MkdirTemp("", "vmck")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(keyDir) })
	keyPath := filepath.Join(keyDir, "k.key")
	require.NoError(t, signer.SealPrivateKey(keyPath, priv))

	sockDir, err := os.MkdirTemp("", "vmcs")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "s.sock")
	s, err := signer.New(signer.Config{
		KeyPath:     keyPath,
		SocketPath:  sockPath,
		AllowedUIDs: []uint32{uint32(os.Getuid())},
	})
	require.NoError(t, err)
	require.NoError(t, s.Listen())
	go func() { _ = s.Serve() }()
	t.Cleanup(func() { _ = s.Close() })
	return sockPath
}

func sessionBinding(t *testing.T, slug string, pub ed25519.PublicKey, epoch int) registry.AgentBinding {
	t.Helper()
	pk, err := registry.NewPublicKey(pub)
	require.NoError(t, err)
	return registry.AgentBinding{
		Slug:                    slug,
		DisplayName:             slug,
		PubKey:                  pk,
		KeyEpoch:                epoch,
		ValidFrom:               1,
		ValidUntil:              1 << 40,
		AuthorizedOriginDaemons: []string{"daemon-1"},
	}
}

// sessionMesh pins a two-agent registry and points this project at mira.
// other is enrolled at key_epoch 1 so a wrong slug is visible in the output.
func sessionMesh(t *testing.T, miraPub ed25519.PublicKey) {
	t.Helper()
	rootPub, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	reg := registry.Registry{
		Epoch: 1, ValidFrom: 1, ValidUntil: 1 << 40,
		Agents: []registry.AgentBinding{
			sessionBinding(t, "mira", miraPub, 4),
			sessionBinding(t, "other", otherPub, 1),
		},
	}
	signed, err := registry.SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	raw, err := registry.MarshalDistribution(signed)
	require.NoError(t, err)

	dir := t.TempDir()
	regPath := filepath.Join(dir, "registry.json")
	require.NoError(t, os.WriteFile(regPath, raw, 0o600))
	project := filepath.Join(dir, "project")
	require.NoError(t, os.MkdirAll(project, 0o755))
	agents := filepath.Join(dir, "agents.yaml")
	body := "agents:\n" +
		"  - slug: mira\n" +
		"    project_path: " + project + "\n" +
		"    registry_path: " + regPath + "\n" +
		"  - slug: other\n" +
		"    project_path: /not/this/project\n" +
		"    registry_path: " + regPath + "\n"
	require.NoError(t, os.WriteFile(agents, []byte(body), 0o600))

	t.Setenv(envAgentRegistry, agents)
	t.Setenv(envProjectPath, project)
	pinEnv(t)
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, base64.StdEncoding.EncodeToString(rootPub))
	prevSlug := viper.Get(config.KeyAppDoctorMeshSlug)
	viper.Set(config.KeyAppDoctorMeshSlug, "other")
	t.Cleanup(func() { viper.Set(config.KeyAppDoctorMeshSlug, prevSlug) })
}

func TestIdentitySignSession_NoCallerSlug(t *testing.T) {
	cmd, _, err := RootCmd.Find([]string{"identity", "sign-session"})
	require.NoError(t, err)
	require.NotNil(t, cmd)
	assert.Nil(t, cmd.Flags().Lookup("slug"))
	assert.Nil(t, cmd.Flags().Lookup("key-epoch"))
	assert.Nil(t, cmd.Flags().Lookup("mesh-slug"))
	assert.NotNil(t, cmd.Flags().Lookup("challenge"))
	assert.NotNil(t, cmd.Flags().Lookup("network-id"))
}

func TestIdentitySignSession_SignsResolvedBinding(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sessionMesh(t, pub)
	sock := startCmdSignerWithKey(t, priv)
	challenge := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	const networkID = "vmnet1:00112233445566778899aabbccddeeff"
	before := time.Now().Unix()

	out, _, err := runRootCmd(t, "identity", "sign-session",
		"--challenge", challenge, "--network-id", networkID, "--signer-socket", sock)
	require.NoError(t, err)

	var got struct {
		Statement identity.SessionStatement `json:"statement"`
		Sig       string                    `json:"sig"`
		PubKey    string                    `json:"pubkey"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, "mira", got.Statement.Slug)
	assert.Equal(t, int64(4), got.Statement.KeyEpoch)
	assert.Equal(t, identity.SessionPurpose, got.Statement.Purpose)
	assert.Equal(t, challenge, got.Statement.Challenge)
	assert.Equal(t, networkID, got.Statement.NetworkID)
	assert.GreaterOrEqual(t, got.Statement.TS, before)
	assert.LessOrEqual(t, got.Statement.TS, time.Now().Unix())

	sig, err := base64.StdEncoding.DecodeString(got.Sig)
	require.NoError(t, err)
	gotPub, err := base64.StdEncoding.DecodeString(got.PubKey)
	require.NoError(t, err)
	assert.Equal(t, []byte(pub), gotPub)
	ok, err := identity.VerifySessionStatement(pub, got.Statement, sig)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIdentitySignSession_FailClosed(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, wrongPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	challenge := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	const networkID = "vmnet1:00112233445566778899aabbccddeeff"

	t.Run("wrong signer key", func(t *testing.T) {
		sessionMesh(t, pub)
		sock := startCmdSignerWithKey(t, wrongPriv)
		out, _, err := runRootCmd(t, "identity", "sign-session",
			"--challenge", challenge, "--network-id", networkID, "--signer-socket", sock)
		require.ErrorContains(t, err, identitycli.ErrSessionKeyMismatch)
		assert.Empty(t, out.String())
	})

	t.Run("bad challenge", func(t *testing.T) {
		sessionMesh(t, pub)
		sock := startCmdSignerWithKey(t, priv)
		out, _, err := runRootCmd(t, "identity", "sign-session",
			"--challenge", "not-base64", "--network-id", networkID, "--signer-socket", sock)
		require.ErrorContains(t, err, identity.ErrSessionChallenge)
		assert.Empty(t, out.String())
	})

	t.Run("no pinned root", func(t *testing.T) {
		sessionMesh(t, pub)
		viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, "")
		out, _, err := runRootCmd(t, "identity", "sign-session",
			"--challenge", challenge, "--network-id", networkID, "--signer-socket", filepath.Join(t.TempDir(), "nope.sock"))
		require.ErrorContains(t, err, identitycli.ErrSessionNoPin)
		assert.Empty(t, out.String())
	})

	t.Run("no registry file", func(t *testing.T) {
		sessionMesh(t, pub)
		agents := os.Getenv(envAgentRegistry)
		raw, err := os.ReadFile(agents)
		require.NoError(t, err)
		raw = bytes.ReplaceAll(raw, []byte("registry_path:"), []byte("not_registry_path:"))
		require.NoError(t, os.WriteFile(agents, raw, 0o600))
		out, _, err := runRootCmd(t, "identity", "sign-session",
			"--challenge", challenge, "--network-id", networkID, "--signer-socket", filepath.Join(t.TempDir(), "nope.sock"))
		require.ErrorContains(t, err, identitycli.ErrSessionNoRegistry)
		assert.Empty(t, out.String())
	})

	t.Run("slug is not enrolled", func(t *testing.T) {
		sessionMesh(t, pub)
		agents := os.Getenv(envAgentRegistry)
		raw, err := os.ReadFile(agents)
		require.NoError(t, err)
		raw = bytes.Replace(raw, []byte("slug: mira"), []byte("slug: ghost"), 1)
		require.NoError(t, os.WriteFile(agents, raw, 0o600))
		out, _, err := runRootCmd(t, "identity", "sign-session",
			"--challenge", challenge, "--network-id", networkID, "--signer-socket", filepath.Join(t.TempDir(), "nope.sock"))
		require.ErrorContains(t, err, identitycli.ErrSessionNotEnrolled)
		assert.Empty(t, out.String())
	})

	t.Run("no slug for this project", func(t *testing.T) {
		sessionMesh(t, pub)
		t.Setenv(envProjectPath, t.TempDir())
		out, _, err := runRootCmd(t, "identity", "sign-session",
			"--challenge", challenge, "--network-id", networkID, "--signer-socket", filepath.Join(t.TempDir(), "nope.sock"))
		require.ErrorContains(t, err, identitycli.ErrSessionNoSlug)
		assert.Empty(t, out.String())
	})

	t.Run("default socket when unset", func(t *testing.T) {
		sessionMesh(t, pub)
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		out, _, err := runRootCmd(t, "identity", "sign-session",
			"--challenge", challenge, "--network-id", networkID)
		require.Error(t, err)
		assert.Empty(t, out.String())
	})

	t.Run("unreachable signer", func(t *testing.T) {
		sessionMesh(t, pub)
		out, _, err := runRootCmd(t, "identity", "sign-session",
			"--challenge", challenge, "--network-id", networkID,
			"--signer-socket", filepath.Join(t.TempDir(), "nope.sock"))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), identitycli.ErrSessionNoSlug)
		assert.Empty(t, out.String())
	})

	t.Run("unknown slug flag", func(t *testing.T) {
		out, _, err := runRootCmd(t, "identity", "sign-session", "--slug", "other")
		require.Error(t, err)
		assert.Empty(t, out.String())
	})

	t.Run("declared staleness rejects an old registry", func(t *testing.T) {
		sessionMesh(t, pub)
		agents := os.Getenv(envAgentRegistry)
		raw, err := os.ReadFile(agents)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(agents, append(raw, []byte("registry_max_staleness_secs: 1\n")...), 0o600))
		out, _, err := runRootCmd(t, "identity", "sign-session",
			"--challenge", challenge, "--network-id", networkID,
			"--signer-socket", filepath.Join(t.TempDir(), "nope.sock"))
		require.ErrorContains(t, err, identitycli.ErrSessionRegistry)
		require.ErrorContains(t, err, registry.ErrStale)
		assert.Empty(t, out.String())
	})
}
