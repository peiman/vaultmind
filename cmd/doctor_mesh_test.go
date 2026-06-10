package cmd

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/identity/anchor"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/require"
)

// --- slug resolution from agents.yaml ---------------------------------------

func TestSlugFromAgentsYAML_Matches(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "agents.yaml")
	require.NoError(t, os.WriteFile(reg, []byte(`
agents:
  - slug: "mira"
    project_path: "`+dir+`"
  - slug: "other"
    project_path: "/somewhere/else"
`), 0o644))

	got := slugFromAgentsYAML(reg, dir)
	require.Equal(t, "agent:mira", got)
}

func TestSlugFromAgentsYAML_AlreadyPrefixed(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "agents.yaml")
	require.NoError(t, os.WriteFile(reg, []byte(`
agents:
  - slug: "agent:mira"
    project_path: "`+dir+`"
`), 0o644))

	require.Equal(t, "agent:mira", slugFromAgentsYAML(reg, dir))
}

func TestSlugFromAgentsYAML_NoMatch(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "agents.yaml")
	require.NoError(t, os.WriteFile(reg, []byte(`
agents:
  - slug: "other"
    project_path: "/not/this"
`), 0o644))

	require.Equal(t, "", slugFromAgentsYAML(reg, dir))
}

func TestSlugFromAgentsYAML_EmptyInputs(t *testing.T) {
	require.Equal(t, "", slugFromAgentsYAML("", "/x"))
	require.Equal(t, "", slugFromAgentsYAML("/nope.yaml", ""))
	require.Equal(t, "", slugFromAgentsYAML("/does/not/exist.yaml", "/x"))
}

// --- M4: --json carries authenticated:false + warning status, exit stays 0 ---

func TestAddMeshWarningsToEnvelope_FlipsStatusToWarning(t *testing.T) {
	env := envelope.OK("doctor", nil)
	mi := &query.DoctorMeshIdentity{
		Authenticated: false,
		Status:        query.StatusMeshSelfConsistentUnpinned,
		Warnings:      []string{query.WarnMeshUnpinned},
	}
	addMeshWarningsToEnvelope(env, mi)
	require.Equal(t, "warning", env.Status)
	require.Len(t, env.Warnings, 1)
	require.Equal(t, meshWarningCode, env.Warnings[0].Code)
	require.Equal(t, query.WarnMeshUnpinned, env.Warnings[0].Message)
}

func TestAddMeshWarningsToEnvelope_NilSectionNoChange(t *testing.T) {
	env := envelope.OK("doctor", nil)
	addMeshWarningsToEnvelope(env, nil)
	require.Equal(t, "ok", env.Status)
	require.Empty(t, env.Warnings)
}

func TestAddMeshWarningsToEnvelope_AuthenticatedNoWarnings(t *testing.T) {
	env := envelope.OK("doctor", nil)
	mi := &query.DoctorMeshIdentity{
		Authenticated: true,
		Status:        query.StatusMeshAuthenticated,
	}
	addMeshWarningsToEnvelope(env, mi)
	require.Equal(t, "ok", env.Status)
	require.Empty(t, env.Warnings)
}

// jsonMeshEnvelope decodes the M4-relevant slice of the doctor --json envelope.
type jsonMeshEnvelope struct {
	Status   string `json:"status"`
	Warnings []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"warnings"`
	Result struct {
		MeshIdentity *struct {
			Authenticated bool   `json:"authenticated"`
			Status        string `json:"status"`
			KeyPresent    bool   `json:"key_present"`
		} `json:"mesh_identity"`
	} `json:"result"`
}

// TestDoctorJSON_MeshUnpinnedCarriesAuthenticatedFalseAndWarning is the M4
// end-to-end assertion: with a key present (mesh signal) but NO pin, the --json
// envelope carries status=warning, a structured warning, and
// result.mesh_identity.authenticated=false — and the command exits 0.
func TestDoctorJSON_MeshUnpinnedCarriesAuthenticatedFalseAndWarning(t *testing.T) {
	chdirToTemp(t)
	xdgData := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgData)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	// Point the daemon URL at a closed loopback port so tier-3 is unreachable and
	// the unpinned path stays deterministic (no live daemon in CI).
	t.Setenv("AGENT_CHAT_DAEMON_URL", "http://127.0.0.1:1")

	// Create a 0600, 64-byte identity key file so a mesh signal exists but there
	// is no anchor → UNPINNED path.
	keyPath := filepath.Join(xdgData, "vaultmind", "identity-signer.key")
	require.NoError(t, os.MkdirAll(filepath.Dir(keyPath), 0o700))
	require.NoError(t, os.WriteFile(keyPath, make([]byte, ed25519.PrivateKeySize), 0o600))

	vault := buildCleanIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err, "doctor exits 0 even with a mesh warning")

	var je jsonMeshEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &je))
	require.NotNil(t, je.Result.MeshIdentity, "mesh section present (key signal)")
	require.True(t, je.Result.MeshIdentity.KeyPresent)
	require.False(t, je.Result.MeshIdentity.Authenticated, "M4: authenticated false")
	require.Equal(t, query.StatusMeshSelfConsistentUnpinned, je.Result.MeshIdentity.Status)
	require.Equal(t, "warning", je.Status, "M4: status flips to warning")
	require.NotEmpty(t, je.Warnings, "M4: structured warning surfaced for jq")
}

// TestDoctorJSON_NoMeshSignalOmitsSection asserts the section is ABSENT from
// --json when no mesh substrate exists (no key, no anchor, no flag, no daemon).
func TestDoctorJSON_NoMeshSignalOmitsSection(t *testing.T) {
	chdirToTemp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("AGENT_CHAT_DAEMON_URL", "http://127.0.0.1:1")

	vault := buildCleanIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)

	var je jsonMeshEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &je))
	require.Nil(t, je.Result.MeshIdentity, "section omitted when no mesh signal")
	require.Equal(t, "ok", je.Status)
}

// TestDoctor_MeshFlagPassedForcesSection asserts that passing --mesh-root-pubkey
// alone is a mesh signal that surfaces the section even with no key file.
func TestDoctor_MeshFlagPassedForcesSection(t *testing.T) {
	chdirToTemp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("AGENT_CHAT_DAEMON_URL", "http://127.0.0.1:1")

	_, rootPriv, err := ed25519.GenerateKey(seedReader("doctor-cmd-flag-root-seed-padding!!"))
	require.NoError(t, err)
	rootPub := rootPriv.Public().(ed25519.PublicKey)

	vault := buildCleanIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json",
		"--mesh-root-pubkey", base64.StdEncoding.EncodeToString(rootPub))
	require.NoError(t, err)

	var je jsonMeshEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &je))
	require.NotNil(t, je.Result.MeshIdentity, "section present when --mesh-root-pubkey passed")
	require.False(t, je.Result.MeshIdentity.Authenticated)
}

// --- writeMeshIdentity human rendering --------------------------------------

func TestWriteMeshIdentity_Nil(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeMeshIdentity(&buf, nil, false))
	require.Empty(t, buf.String())
}

func TestWriteMeshIdentity_AuthenticatedGreen(t *testing.T) {
	var buf bytes.Buffer
	mi := &query.DoctorMeshIdentity{
		KeyPresent: true, KeyModeOK: true, KeySizeOK: true,
		SignerReachable: true,
		Authenticated:   true,
		Status:          query.StatusMeshAuthenticated,
		DaemonReachable: true,
		DaemonMode:      query.DaemonModeAdvisoryConfigured,
	}
	require.NoError(t, writeMeshIdentity(&buf, mi, false))
	s := buf.String()
	require.Contains(t, s, meshSectionHeader)
	require.Contains(t, s, query.StatusMeshAuthenticated)
	require.Contains(t, s, meshKeyPresentYes)
	require.NotContains(t, s, meshKeyModeBad)
}

func TestWriteMeshIdentity_DaemonReachableAndHeartbeatFresh(t *testing.T) {
	var buf bytes.Buffer
	mi := &query.DoctorMeshIdentity{
		KeyPresent: true, KeyModeOK: true, KeySizeOK: true,
		Status:                query.StatusMeshNotEnrolled,
		DaemonReachable:       true,
		DaemonMode:            query.DaemonModePlaintext,
		WatcherHeartbeatFresh: true,
	}
	require.NoError(t, writeMeshIdentity(&buf, mi, false))
	s := buf.String()
	require.Contains(t, s, meshDaemonReachable)
	require.Contains(t, s, query.DaemonModePlaintext)
	require.Contains(t, s, meshHeartbeatFresh)
}

func TestWriteMeshIdentity_DaemonUnreachableNoHeartbeatLine(t *testing.T) {
	var buf bytes.Buffer
	mi := &query.DoctorMeshIdentity{
		Status:                query.StatusMeshSelfConsistentUnpinned,
		DaemonReachable:       false,
		WatcherHeartbeatFresh: false,
	}
	require.NoError(t, writeMeshIdentity(&buf, mi, false))
	s := buf.String()
	require.Contains(t, s, meshDaemonUnreachable)
	require.NotContains(t, s, meshHeartbeatFresh)
}

func TestWriteMeshIdentity_KeyAbsentAndSignerUp(t *testing.T) {
	var buf bytes.Buffer
	mi := &query.DoctorMeshIdentity{
		KeyPresent:      false,
		KeySizeOK:       false,
		SignerReachable: true,
		Status:          query.StatusMeshSelfConsistentUnpinned,
	}
	require.NoError(t, writeMeshIdentity(&buf, mi, false))
	s := buf.String()
	require.Contains(t, s, meshKeyPresentNo)
	require.Contains(t, s, meshSignerUp)
}

func TestWriteMeshIdentity_KeySizeBadShown(t *testing.T) {
	var buf bytes.Buffer
	mi := &query.DoctorMeshIdentity{
		KeyPresent: true, KeyModeOK: true, KeySizeOK: false,
		Status: query.StatusMeshSelfConsistentUnpinned,
	}
	require.NoError(t, writeMeshIdentity(&buf, mi, false))
	require.Contains(t, buf.String(), meshKeySizeBad)
}

func TestWriteMeshIdentity_WarningsRendered(t *testing.T) {
	var buf bytes.Buffer
	mi := &query.DoctorMeshIdentity{
		KeyPresent: true, KeyModeOK: false, KeySizeOK: true,
		Status:   query.StatusMeshSelfConsistentUnpinned,
		Warnings: []string{query.WarnMeshKeyMode, query.WarnMeshUnpinned},
	}
	require.NoError(t, writeMeshIdentity(&buf, mi, false))
	s := buf.String()
	require.Contains(t, s, meshKeyModeBad)
	require.Contains(t, s, query.WarnMeshUnpinned)
	require.Contains(t, s, query.WarnMeshKeyMode)
}

// seedReader returns an io.Reader yielding a deterministic 32-byte ed25519 seed
// from a low-entropy string (gitleaks-friendly: no real key material).
func seedReader(seed string) *bytes.Reader {
	b := make([]byte, ed25519.SeedSize)
	copy(b, seed)
	return bytes.NewReader(b)
}

// --- offline registry + anchor auto-discovery end-to-end --------------------

// TestDoctor_MeshOfflineRegistryWithAnchorPin drives the AUTHENTICATED pinned
// path through the cmd layer: an enroll-persisted anchor supplies the pin, and
// --mesh-registry supplies the signed registry offline. With no running signer
// the keyless selfVerify cannot pass, so the honest verdict is key-mismatch (NOT
// green) — proving the cmd layer wires the pin + registry + slug correctly while
// reserving green for a cryptographically proven binding.
func TestDoctor_MeshOfflineRegistryWithAnchorPin(t *testing.T) {
	chdirToTemp(t)
	xdgData := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgData)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("AGENT_CHAT_DAEMON_URL", "http://127.0.0.1:1")

	now := time.Now()
	rootPub, rootPriv, err := ed25519.GenerateKey(seedReader("doctor-cmd-offline-root-seed-pad!!"))
	require.NoError(t, err)
	memberPub, _, err := ed25519.GenerateKey(seedReader("doctor-cmd-offline-member-seed-pad"))
	require.NoError(t, err)

	regBytes, nid := buildCmdSignedRegistry(t, rootPub, rootPriv, "agent:mira", memberPub, now)

	// Persist the OOB anchor where doctor auto-discovers it.
	anchorPath := filepath.Join(xdgData, "vaultmind", "network-roots.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(anchorPath), 0o700))
	require.NoError(t, anchor.Upsert(anchorPath, anchor.NetworkAnchor{
		NetworkID:   nid,
		RootPubKey:  base64.StdEncoding.EncodeToString(rootPub),
		ConfirmedAt: now.Unix(),
	}))

	regFile := filepath.Join(t.TempDir(), "registry.json")
	require.NoError(t, os.WriteFile(regFile, regBytes, 0o644))

	vault := buildCleanIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json",
		"--mesh-registry", regFile, "--mesh-slug", "agent:mira")
	require.NoError(t, err)

	var je jsonMeshEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &je))
	require.NotNil(t, je.Result.MeshIdentity)
	require.False(t, je.Result.MeshIdentity.Authenticated,
		"no running signer ⇒ keyless selfVerify cannot prove possession ⇒ never green")
	require.Equal(t, query.StatusMeshKeyMismatch, je.Result.MeshIdentity.Status)
}

// TestDoctor_MeshOfflineRegistryUnreadable asserts a named --mesh-registry file
// that cannot be read is a HARD error (the operator named a specific file).
func TestDoctor_MeshOfflineRegistryUnreadable(t *testing.T) {
	chdirToTemp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("AGENT_CHAT_DAEMON_URL", "http://127.0.0.1:1")

	vault := buildCleanIndexedTestVault(t)
	_, _, err := runRootCmd(t, "doctor", "--vault", vault,
		"--mesh-registry", filepath.Join(t.TempDir(), "does-not-exist.json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), meshDoctorErrRegistry)
}

// TestDoctor_MeshBadRootPubkeyFlag asserts a malformed --mesh-root-pubkey is a
// hard error (the operator asked for a specific pin).
func TestDoctor_MeshBadRootPubkeyFlag(t *testing.T) {
	chdirToTemp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("AGENT_CHAT_DAEMON_URL", "http://127.0.0.1:1")

	vault := buildCleanIndexedTestVault(t)
	_, _, err := runRootCmd(t, "doctor", "--vault", vault,
		"--mesh-root-pubkey", "not-valid-base64!!!")
	require.Error(t, err)
	require.Contains(t, err.Error(), meshDoctorErrParse)
}

// buildCmdSignedRegistry roots a one-binding registry for the cmd-layer tests.
func buildCmdSignedRegistry(t *testing.T, rootPub ed25519.PublicKey, rootPriv ed25519.PrivateKey, slug string, memberPub ed25519.PublicKey, now time.Time) ([]byte, string) {
	t.Helper()
	pk, err := registry.NewPublicKey(memberPub)
	require.NoError(t, err)
	reg := registry.Registry{
		Epoch:      1,
		ValidFrom:  now.Add(-time.Hour).Unix(),
		ValidUntil: now.Add(24 * time.Hour).Unix(),
		Agents: []registry.AgentBinding{{
			Slug:                    slug,
			DisplayName:             "Member",
			PubKey:                  pk,
			KeyEpoch:                1,
			ValidFrom:               now.Add(-time.Hour).Unix(),
			ValidUntil:              now.Add(24 * time.Hour).Unix(),
			AuthorizedOriginDaemons: []string{"daemon:local"},
		}},
	}
	env, err := registry.SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	raw, err := registry.MarshalDistribution(env)
	require.NoError(t, err)
	return raw, registry.NetworkID(rootPub)
}
