package cmd

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/identity/anchor"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/registryfetch"
	"github.com/peiman/vaultmind/internal/identity/relayclient"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Member machines had no local copy of the signed registry (dabir, 2026-09-22):
// doctor could not check it and the watcher could not count down to its lapse.
// `identity fetch-registry` pulls it from the hub. These tests drive the real
// command against a real HTTP server serving a real root-signed registry.

type fetchEnv struct {
	agentsYAML string
	xdgData    string
	rootPub    ed25519.PublicKey
	rootPriv   ed25519.PrivateKey
}

func newFetchEnv(t *testing.T) fetchEnv {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	e := fetchEnv{agentsYAML: filepath.Join(dir, "agents.yaml"), xdgData: t.TempDir()}
	t.Setenv("XDG_DATA_HOME", e.xdgData)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(envAgentRegistry, e.agentsYAML)
	t.Setenv(envDaemonURL, "")
	// The developer's own config may pin a real root (mine does); an earlier
	// test in the suite can have loaded it into viper. Pin the key empty here so
	// every case sees only what it sets up.
	prev := viper.Get(config.KeyAppIdentityfetchregistryRootPubkey)
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, "")
	t.Cleanup(func() { viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, prev) })
	var err error
	e.rootPub, e.rootPriv, err = ed25519.GenerateKey(seedReader("fetch-registry-root-seed-padding!!"))
	require.NoError(t, err)
	return e
}

// serve starts a hub serving body at the directory path; it counts requests.
func serveDirectory(t *testing.T, body []byte) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != relayclient.WellKnownDirectoryPath {
			http.NotFound(w, r)
			return
		}
		hits++
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func (e fetchEnv) signedRegistry(t *testing.T) []byte {
	t.Helper()
	member, _, err := ed25519.GenerateKey(seedReader("fetch-registry-member-seed-padding"))
	require.NoError(t, err)
	raw, _ := buildCmdSignedRegistry(t, e.rootPub, e.rootPriv, "mira", member, time.Now())
	return raw
}

func (e fetchEnv) rootB64() string { return base64.StdEncoding.EncodeToString(e.rootPub) }

// No pinned root, no fetch: without it the command would store whatever the
// connection handed it, which is the exact trust mistake this exists to avoid.
func TestIdentityFetchRegistry_RefusesWithoutAPinnedRoot(t *testing.T) {
	e := newFetchEnv(t)
	srv, hits := serveDirectory(t, e.signedRegistry(t))
	path := filepath.Join(t.TempDir(), "registry.json")

	_, _, err := runRootCmd(t, "identity", "fetch-registry", "--hub", srv.URL, "--registry-file", path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fetchRegistryErrNoRoot)
	assert.Equal(t, 0, *hits, "refused before any network call")
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestIdentityFetchRegistry_RefusesWithoutAHubAddress(t *testing.T) {
	e := newFetchEnv(t)
	_, _, err := runRootCmd(t, "identity", "fetch-registry", "--root-pubkey", e.rootB64(),
		"--registry-file", filepath.Join(t.TempDir(), "registry.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), fetchRegistryErrNoHub)
}

func TestIdentityFetchRegistry_RefusesWithoutARegistryPath(t *testing.T) {
	e := newFetchEnv(t)
	srv, hits := serveDirectory(t, e.signedRegistry(t))
	_, _, err := runRootCmd(t, "identity", "fetch-registry", "--root-pubkey", e.rootB64(), "--hub", srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fetchRegistryErrNoPath)
	assert.Equal(t, 0, *hits)
}

func TestIdentityFetchRegistry_RejectsAMalformedRootFlag(t *testing.T) {
	newFetchEnv(t)
	_, _, err := runRootCmd(t, "identity", "fetch-registry", "--root-pubkey", "not base64!!",
		"--hub", "http://127.0.0.1:1", "--registry-file", filepath.Join(t.TempDir(), "r.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), fetchRegistryErrBadRoot)
}

// The whole path from agents.yaml: hub address, registry path, and the
// declared hub bound for the countdown — plus the enroll anchor as the pin.
func TestIdentityFetchRegistry_ResolvesEverythingFromAgentsYAMLAndTheAnchor(t *testing.T) {
	e := newFetchEnv(t)
	body := e.signedRegistry(t)
	srv, _ := serveDirectory(t, body)
	path := filepath.Join(t.TempDir(), "mesh", "registry.json")
	require.NoError(t, os.WriteFile(e.agentsYAML, []byte("daemon_url: "+srv.URL+"\nregistry_path: "+path+
		"\nregistry_max_staleness_secs: 2592000\n"), 0o600))
	anchorPath := filepath.Join(e.xdgData, "vaultmind", "network-roots.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(anchorPath), 0o700))
	require.NoError(t, anchor.Upsert(anchorPath, anchor.NetworkAnchor{
		NetworkID: registry.NetworkID(e.rootPub), RootPubKey: e.rootB64(), ConfirmedAt: time.Now().Unix(),
	}))

	out, _, err := runRootCmd(t, "identity", "fetch-registry")
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, got, "stored byte-exact")
	assert.Contains(t, out.String(), "installed")
	assert.Contains(t, out.String(), "epoch 1")
	assert.Contains(t, out.String(), "days left", "a declared hub bound gives a countdown")

	out, _, err = runRootCmd(t, "identity", "fetch-registry")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "already current", "a second run is a no-op and says so")
}

// A registry the pinned root did not sign never lands on disk.
func TestIdentityFetchRegistry_RefusesAForeignRegistry(t *testing.T) {
	e := newFetchEnv(t)
	srv, _ := serveDirectory(t, e.signedRegistry(t))
	other, _, err := ed25519.GenerateKey(seedReader("some-other-root-entirely-padding!!"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "registry.json")

	_, _, err = runRootCmd(t, "identity", "fetch-registry", "--hub", srv.URL, "--registry-file", path,
		"--root-pubkey", base64.StdEncoding.EncodeToString(other))
	require.Error(t, err)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestWriteFetchRegistryResult_SaysWhatHappened(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	from, until := now.Add(-24*time.Hour).Unix(), now.Add(365*24*time.Hour).Unix()
	for _, tc := range []struct {
		res  registryfetch.Result
		want string
	}{
		{registryfetch.Result{Epoch: 5, PreviousEpoch: 5}, "registry already current: epoch 5 (/r)"},
		{registryfetch.Result{Epoch: 5, Changed: true}, "registry installed: epoch 5 (/r)"},
		{registryfetch.Result{Epoch: 5, PreviousEpoch: 4, Changed: true}, "registry updated: epoch 4 -> epoch 5 (/r)"},
	} {
		tc.res.ValidFrom, tc.res.ValidUntil = from, until
		var b bytes.Buffer
		require.NoError(t, writeFetchRegistryResult(&b, tc.res, "/r", 30*86400, now))
		assert.Equal(t, tc.want+"; 29 days left before the hub refuses it\n", b.String())

		b.Reset()
		require.NoError(t, writeFetchRegistryResult(&b, tc.res, "/r", 0, now))
		assert.Equal(t, tc.want+"\n", b.String(), "no declared hub bound, no invented countdown")
	}
}
