package cmd

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
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

	out, errOut, err := runRootCmd(t, "identity", "fetch-registry")
	require.NoError(t, err)
	assert.Contains(t, errOut.String(), "pinned root: "+shortRoot(e.rootB64())+" (from enrollment anchor "+anchorPath+")",
		"every run says which root it trusted and where that came from")
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

// --- the 2026-11-08 changeover trap -----------------------------------------
//
// After the root changed, two agents failed with only "root signature does not
// verify": one pinned by a stale ./config.yaml, one (native config mode, no
// config file) by a stale enroll anchor. The refusal now names the pinned root,
// where it came from, and the root the hub advertises — diagnosis only: the
// hub's root is never trusted.

// serveHub serves body at the directory path and, when advertised is non-nil,
// that root at /.well-known/vaultmind-root.
func serveHub(t *testing.T, body []byte, advertised ed25519.PublicKey) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == relayclient.WellKnownDirectoryPath:
			_, _ = w.Write(body)
		case r.URL.Path == relayclient.WellKnownRootPath && advertised != nil:
			_ = json.NewEncoder(w).Encode(relayclient.WellKnownRoot{
				RootPubKey: base64.StdEncoding.EncodeToString(advertised), NetworkID: registry.NetworkID(advertised),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func oldRoot(t *testing.T) ed25519.PublicKey {
	t.Helper()
	return pinKey(t, "the-old-root-before-changeover-pad")
}

func TestIdentityFetchRegistry_StaleConfigPinIsNamedWithTheHubRoot(t *testing.T) {
	e := newFetchEnv(t)
	viper.Set(config.KeyAppIdentityfetchregistryRootPubkey, nil) // let ./config.yaml speak
	t.Setenv(rootPinEnvVar(config.KeyAppIdentityfetchregistryRootPubkey), "")
	old := oldRoot(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	cfg := filepath.Join(cwd, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("app:\n  identityfetchregistry:\n    root_pubkey: "+b64(old)+"\n"), 0o600))
	srv := serveHub(t, e.signedRegistry(t), e.rootPub)
	path := filepath.Join(t.TempDir(), "registry.json")

	_, errOut, err := runRootCmd(t, "identity", "fetch-registry", "--hub", srv.URL, "--registry-file", path)
	require.ErrorIs(t, err, registryfetch.ErrNotVerified)
	pinned := "pinned root: " + shortRoot(b64(old)) + " (from config " + cfg + ")"
	assert.Contains(t, errOut.String(), pinned)
	msg := err.Error()
	assert.Contains(t, msg, pinned)
	assert.Contains(t, msg, "hub root: "+shortRoot(e.rootB64())+" (advertised at "+srv.URL+relayclient.WellKnownRootPath+")")
	assert.Contains(t, msg, "the hub serves a different root; if the network's root changed, update config "+cfg+
		" or pass --root-pubkey")
	assert.NotContains(t, msg, "enrollment anchor")
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "diagnosis never installs the hub's registry")
}

func TestIdentityFetchRegistry_StaleAnchorIsCalledStale(t *testing.T) {
	e := newFetchEnv(t)
	old := oldRoot(t)
	writeAnchor(t, e.xdgData, old)
	anchorPath := filepath.Join(e.xdgData, "vaultmind", networkAnchorFilename)
	srv := serveHub(t, e.signedRegistry(t), e.rootPub)

	_, _, err := runRootCmd(t, "identity", "fetch-registry", "--hub", srv.URL,
		"--registry-file", filepath.Join(t.TempDir(), "registry.json"))
	require.ErrorIs(t, err, registryfetch.ErrNotVerified)
	msg := err.Error()
	assert.Contains(t, msg, "pinned root: "+shortRoot(b64(old))+" (from enrollment anchor "+anchorPath+")")
	assert.Contains(t, msg, "hub root: "+shortRoot(e.rootB64()))
	assert.Contains(t, msg, "update enrollment anchor "+anchorPath+" or pass --root-pubkey")
	// Conditional: on the hub's word alone we may not assert that a correct
	// anchor is wrong — a hostile hub could advertise any root.
	assert.Contains(t, msg, "if the network's root changed, the enrollment anchor is stale: run `identity enroll` for the new root, which makes the new anchor the pinned one")
	assert.NotContains(t, msg, "  the enrollment anchor is stale")
}

func TestIdentityFetchRegistry_FlagPinHintSaysPassTheNewRoot(t *testing.T) {
	e := newFetchEnv(t)
	srv := serveHub(t, e.signedRegistry(t), e.rootPub)
	_, _, err := runRootCmd(t, "identity", "fetch-registry", "--hub", srv.URL, "--root-pubkey", b64(oldRoot(t)),
		"--registry-file", filepath.Join(t.TempDir(), "registry.json"))
	require.ErrorIs(t, err, registryfetch.ErrNotVerified)
	assert.Contains(t, err.Error(), "(from flag --root-pubkey)")
	assert.Contains(t, err.Error(), "if the network's root changed, pass the new root to --root-pubkey")
}

// The hub advertising the pinned root means the root did NOT change: no hint to
// re-pin, which would be wrong advice.
func TestIdentityFetchRegistry_SameRootOnHubGivesNoRepinHint(t *testing.T) {
	e := newFetchEnv(t)
	old := oldRoot(t)
	srv := serveHub(t, e.signedRegistry(t), old)
	_, _, err := runRootCmd(t, "identity", "fetch-registry", "--hub", srv.URL, "--root-pubkey", b64(old),
		"--registry-file", filepath.Join(t.TempDir(), "registry.json"))
	require.ErrorIs(t, err, registryfetch.ErrNotVerified)
	assert.Contains(t, err.Error(), "the hub advertises the pinned root")
	assert.NotContains(t, err.Error(), "different root")
}

func TestIdentityFetchRegistry_HubRootUnavailableStillNamesThePin(t *testing.T) {
	e := newFetchEnv(t)
	srv := serveHub(t, e.signedRegistry(t), nil)
	_, _, err := runRootCmd(t, "identity", "fetch-registry", "--hub", srv.URL, "--root-pubkey", b64(oldRoot(t)),
		"--registry-file", filepath.Join(t.TempDir(), "registry.json"))
	require.ErrorIs(t, err, registryfetch.ErrNotVerified)
	assert.Contains(t, err.Error(), "pinned root: "+shortRoot(b64(oldRoot(t)))+" (from flag --root-pubkey)")
	assert.Contains(t, err.Error(), "hub root: unavailable")
}

func TestIdentityFetchRegistry_HubRootThatIsNoKeyIsUnavailable(t *testing.T) {
	e := newFetchEnv(t)
	body := e.signedRegistry(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == relayclient.WellKnownDirectoryPath {
			_, _ = w.Write(body)
			return
		}
		_, _ = w.Write([]byte(`{"root_pubkey":"not a key"}`))
	}))
	t.Cleanup(srv.Close)
	_, _, err := runRootCmd(t, "identity", "fetch-registry", "--hub", srv.URL, "--root-pubkey", b64(oldRoot(t)),
		"--registry-file", filepath.Join(t.TempDir(), "registry.json"))
	require.ErrorIs(t, err, registryfetch.ErrNotVerified)
	assert.Contains(t, err.Error(), "hub root: unavailable ("+srv.URL+relayclient.WellKnownRootPath+" holds no valid root key)")
	assert.NotContains(t, err.Error(), "different root")
}

// The hub's root is shown re-encoded from the decoded key, never as the raw
// string it sent: the decoder skips embedded \r\n, so the raw text could
// differ from the key it actually names.
func TestIdentityFetchRegistry_HubRootIsShownReEncoded(t *testing.T) {
	e := newFetchEnv(t)
	body := e.signedRegistry(t)
	raw := e.rootB64()
	sneaky := raw[:2] + `\r\n` + raw[2:]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == relayclient.WellKnownDirectoryPath {
			_, _ = w.Write(body)
			return
		}
		_, _ = w.Write([]byte(`{"root_pubkey":"` + sneaky + `"}`))
	}))
	t.Cleanup(srv.Close)

	out, _, err := runRootCmd(t, "identity", "fetch-registry", "--output-format", "json", "--hub", srv.URL,
		"--root-pubkey", b64(oldRoot(t)), "--registry-file", filepath.Join(t.TempDir(), "registry.json"))
	require.Error(t, err)
	var env fetchRegistryJSONEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, raw, env.Result.HubAdvertisedRoot, "canonical encoding of the decoded key")
	assert.Contains(t, env.Errors[0].Message, "hub root: "+shortRoot(raw)+" ")
}

type fetchRegistryJSONEnvelope struct {
	Status string `json:"status"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		PinnedRoot        string `json:"pinned_root"`
		PinnedRootSource  string `json:"pinned_root_source"`
		PinnedRootOrigin  string `json:"pinned_root_origin"`
		HubAdvertisedRoot string `json:"hub_advertised_root"`
		Epoch             int    `json:"epoch"`
		Changed           bool   `json:"changed"`
	} `json:"result"`
}

// JSON mode has ONE envelope shape: a run that fails before or after the pin
// resolves still answers with the fetch-registry envelope and a classifying code.
func TestIdentityFetchRegistry_JSONErrorsShareOneShapeAndClassify(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code string
		pin  bool
	}{
		{"no root", []string{"--hub", "http://127.0.0.1:1"}, "config_missing", false},
		{"bad root", []string{"--root-pubkey", "not base64!!", "--hub", "http://127.0.0.1:1"}, "config_invalid", false},
		{"no hub", []string{"--root-pubkey", "ROOT"}, "config_missing", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newFetchEnv(t)
			args := []string{"identity", "fetch-registry", "--output-format", "json",
				"--registry-file", filepath.Join(t.TempDir(), "registry.json")}
			for _, a := range tc.args {
				if a == "ROOT" {
					a = e.rootB64()
				}
				args = append(args, a)
			}
			out, _, err := runRootCmd(t, args...)
			require.Error(t, err)
			var env fetchRegistryJSONEnvelope
			require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
			assert.Equal(t, "error", env.Status)
			require.Len(t, env.Errors, 1)
			assert.Equal(t, tc.code, env.Errors[0].Code)
			if tc.pin {
				assert.Equal(t, e.rootB64(), env.Result.PinnedRoot)
			} else {
				assert.Empty(t, env.Result.PinnedRoot)
				assert.Empty(t, env.Result.PinnedRootSource)
			}
		})
	}
}

func TestIdentityFetchRegistry_JSONCarriesThePinAndItsSource(t *testing.T) {
	e := newFetchEnv(t)
	writeAnchor(t, e.xdgData, e.rootPub)
	anchorPath := filepath.Join(e.xdgData, "vaultmind", networkAnchorFilename)
	srv := serveHub(t, e.signedRegistry(t), e.rootPub)

	out, errOut, err := runRootCmd(t, "identity", "fetch-registry", "--output-format", "json", "--hub", srv.URL,
		"--registry-file", filepath.Join(t.TempDir(), "registry.json"))
	require.NoError(t, err)
	assert.NotContains(t, errOut.String(), "pinned root:", "JSON mode carries the pin as fields, not prose")
	var env fetchRegistryJSONEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, e.rootB64(), env.Result.PinnedRoot)
	assert.Equal(t, rootPinSourceAnchor, env.Result.PinnedRootSource)
	assert.Equal(t, anchorPath, env.Result.PinnedRootOrigin)
	assert.Equal(t, 1, env.Result.Epoch)
	assert.True(t, env.Result.Changed)
}

func TestIdentityFetchRegistry_JSONRefusalCarriesBothRoots(t *testing.T) {
	e := newFetchEnv(t)
	old := oldRoot(t)
	srv := serveHub(t, e.signedRegistry(t), e.rootPub)

	out, _, err := runRootCmd(t, "identity", "fetch-registry", "--output-format", "json", "--hub", srv.URL,
		"--root-pubkey", b64(old), "--registry-file", filepath.Join(t.TempDir(), "registry.json"))
	require.Error(t, err)
	var env fetchRegistryJSONEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, "error", env.Status)
	assert.Equal(t, b64(old), env.Result.PinnedRoot)
	assert.Equal(t, rootPinSourceFlag, env.Result.PinnedRootSource)
	assert.Equal(t, "--root-pubkey", env.Result.PinnedRootOrigin)
	assert.Equal(t, e.rootB64(), env.Result.HubAdvertisedRoot)
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "not_verified", env.Errors[0].Code)
	assert.Contains(t, out.String(), "the hub serves a different root")
}
