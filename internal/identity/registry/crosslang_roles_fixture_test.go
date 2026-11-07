package registry

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rolesFixturePath is the cross-language acceptance fixture for agent roles.
// It is a separate file from cross_language_vectors.json: that file is the
// roles-free contract agent-chat's Rust tests bind to, and its schema may
// reject a roles key. Regenerate with:
//
//	go test ./internal/identity/registry/ -run TestGenerate_CrossLanguageRolesFixture -update
const rolesFixturePath = "testdata/cross_language_roles_vectors.json"

// rolesFixture is one valid root-signed envelope whose bindings carry a bridge
// role, a code_signing role, and no roles, plus named schema rejects. Every
// reject envelope is signed by the same root, so the failure is the role schema.
type rolesFixture struct {
	RootPubKey      string            `json:"root_pubkey"`
	Now             int64             `json:"now"`
	MaxStalenessSec int64             `json:"max_staleness_secs"`
	Valid           rolesValidCase    `json:"valid"`
	Reject          []rolesRejectCase `json:"reject"`
}

type rolesValidCase struct {
	Envelope       json.RawMessage   `json:"envelope"`
	PersistedEpoch int               `json:"persisted_highest_epoch"`
	Bindings       []expectedBinding `json:"bindings"`
}

type expectedBinding struct {
	Slug  string         `json:"slug"`
	Roles []expectedRole `json:"roles"`
}

type expectedRole struct {
	Kind       string   `json:"kind"`
	VouchesFor []string `json:"vouches_for,omitempty"`
	Platforms  []string `json:"platforms,omitempty"`
}

type rolesRejectCase struct {
	Name     string          `json:"name"`
	Reason   string          `json:"reason"`
	Expect   string          `json:"expect"`
	Envelope json.RawMessage `json:"envelope"`
}

func buildRolesFixture(t *testing.T) rolesFixture {
	t.Helper()
	rootPub, rootPriv := fixedEd25519(t, 0xC1)
	const (
		now       int64 = 2_000_000
		maxStale  int64 = 86_400
		regFrom   int64 = 1_990_000
		regUntil  int64 = 3_000_000
		epoch           = 10
		persisted       = 9
	)
	bridge := rolesFixtureBinding(t, 0xC2, "bridge", "Bridge", []Role{
		NewBridgeRole([]string{"human:peiman"}),
	})
	signer := rolesFixtureBinding(t, 0xC3, "signer", "Signer", []Role{
		NewCodeSigningRole([]string{"darwin-arm64"}),
	})
	plain := rolesFixtureBinding(t, 0xC4, "plain", "Plain", nil)
	reg := Registry{
		Epoch: epoch, ValidFrom: regFrom, ValidUntil: regUntil,
		Agents: []AgentBinding{bridge, signer, plain},
	}
	validEnv := genEnvelope(t, rootPriv, reg)

	// Rejects are signed over an otherwise-valid one-binding registry. The role
	// value is patched into the wire JSON before the root signs, so the signature
	// is valid and the schema is not.
	base := Registry{
		Epoch: epoch, ValidFrom: regFrom, ValidUntil: regUntil,
		Agents: []AgentBinding{rolesFixtureBinding(t, 0xC2, "bridge", "Bridge", nil)},
	}
	// non_nfc uses U+0301 as an escape (nfdAcute) so the source cannot precompose
	// "human:peima\u0301n" into a single code point before it is signed.
	nfdVouch := `[{"kind":"bridge","vouches_for":["human:peima` + nfdAcute + `n"]}]`
	rejects := []struct {
		name, reason, roles string
	}{
		{"unknown_kind", "role kind is not bridge or code_signing", `[{"kind":"wizard","vouches_for":["human:peiman"]}]`},
		{"bridge_extra_field", "bridge role carries a field other than kind and vouches_for", `[{"kind":"bridge","vouches_for":["human:peiman"],"extra":true}]`},
		{"bridge_missing_vouches_for", "bridge role has no vouches_for list", `[{"kind":"bridge"}]`},
		{"code_signing_missing_platforms", "code_signing role has no platforms list", `[{"kind":"code_signing"}]`},
		{"roles_not_array", "roles is a JSON object, not an array", `{"kind":"bridge"}`},
		{"roles_null", "roles is JSON null, not an array", `null`},
		{"vouches_for_null", "vouches_for is JSON null, not an array of strings", `[{"kind":"bridge","vouches_for":null}]`},
		{"non_string_in_vouches_for", "vouches_for contains a non-string element", `[{"kind":"bridge","vouches_for":["human:peiman",1]}]`},
		{"non_nfc_vouches_for", "vouches_for is not Unicode NFC", nfdVouch},
	}
	cases := make([]rolesRejectCase, 0, len(rejects))
	for _, rc := range rejects {
		cases = append(cases, rolesRejectCase{
			Name: rc.name, Reason: rc.reason, Expect: "reject",
			Envelope: signPatchedRoles(t, rootPriv, base, json.RawMessage(rc.roles)),
		})
	}
	return rolesFixture{
		RootPubKey:      b64(rootPub),
		Now:             now,
		MaxStalenessSec: maxStale,
		Valid: rolesValidCase{
			Envelope:       validEnv,
			PersistedEpoch: persisted,
			Bindings: []expectedBinding{
				{Slug: "bridge", Roles: []expectedRole{{Kind: "bridge", VouchesFor: []string{"human:peiman"}}}},
				{Slug: "signer", Roles: []expectedRole{{Kind: "code_signing", Platforms: []string{"darwin-arm64"}}}},
				{Slug: "plain", Roles: []expectedRole{}},
			},
		},
		Reject: cases,
	}
}

func rolesFixtureBinding(t *testing.T, fill byte, slug, name string, roles []Role) AgentBinding {
	t.Helper()
	pub, _ := fixedEd25519(t, fill)
	pk, err := NewPublicKey(pub)
	require.NoError(t, err)
	return AgentBinding{
		Slug: slug, DisplayName: name, PubKey: pk, KeyEpoch: 1,
		ValidFrom: 1_990_000, ValidUntil: 3_000_000,
		AuthorizedOriginDaemons: []string{"daemon-1"},
		Roles:                   roles,
	}
}

// signPatchedRoles root-signs reg after replacing the first binding's roles with
// rolesJSON. Numbers stay in their original JSON text so the canonical bytes do
// not depend on float rounding. The returned value is distribution-envelope JSON.
func signPatchedRoles(t *testing.T, rootPriv ed25519.PrivateKey, reg Registry, rolesJSON json.RawMessage) json.RawMessage {
	t.Helper()
	wire, err := toWire(reg)
	require.NoError(t, err)
	raw, err := json.Marshal(wire)
	require.NoError(t, err)
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))
	var agents []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(obj["agents"], &agents))
	require.NotEmpty(t, agents)
	agents[0]["roles"] = rolesJSON
	agentsRaw, err := json.Marshal(agents)
	require.NoError(t, err)
	obj["agents"] = agentsRaw
	patched, err := json.Marshal(obj)
	require.NoError(t, err)
	canonical, err := identity.Canonicalize(patched)
	require.NoError(t, err)
	sig, err := identity.SignCanonical(rootPriv, canonical)
	require.NoError(t, err)
	out, err := MarshalDistribution(SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0})
	require.NoError(t, err)
	return out
}

func marshalRolesFixture(t *testing.T) []byte {
	t.Helper()
	out, err := json.MarshalIndent(buildRolesFixture(t), "", "  ")
	require.NoError(t, err)
	return append(out, '\n')
}

func TestGenerate_CrossLanguageRolesFixture(t *testing.T) {
	if !*updateFixture {
		t.Skip("run with -update to regenerate the roles cross-language fixture")
	}
	out := marshalRolesFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(rolesFixturePath), 0o755))
	require.NoError(t, os.WriteFile(rolesFixturePath, out, 0o644))
	t.Logf("wrote %s (%d bytes)", rolesFixturePath, len(out))
}

func TestCrossLanguageRolesFixture_MatchesGenerator(t *testing.T) {
	if *updateFixture {
		t.Skip("skipping match check during -update regeneration")
	}
	committed, err := os.ReadFile(rolesFixturePath)
	require.NoError(t, err, "committed roles fixture must exist; run with -update to generate it")
	assert.Equal(t, string(committed), string(marshalRolesFixture(t)),
		"committed roles fixture diverges from the generator — regenerate with: "+
			"go test ./internal/identity/registry/ -run TestGenerate_CrossLanguageRolesFixture -update")
}

func TestCrossLanguageRolesFixture_ValidRoundTripsRoles(t *testing.T) {
	fx := loadRolesFixture(t)
	rootPub := mustB64Pub(t, fx.RootPubKey)
	parsed, err := ParseDistribution(fx.Valid.Envelope)
	require.NoError(t, err)
	reg, _, err := VerifyAndLoad(rootPub, parsed, fx.Valid.PersistedEpoch,
		time.Unix(fx.Now, 0), time.Duration(fx.MaxStalenessSec)*time.Second)
	require.NoError(t, err)
	require.Len(t, reg.Agents, len(fx.Valid.Bindings))
	for i, want := range fx.Valid.Bindings {
		got := reg.Agents[i]
		assert.Equal(t, want.Slug, got.Slug)
		require.Len(t, got.Roles, len(want.Roles), "slug %s", want.Slug)
		for j, role := range want.Roles {
			assert.Equal(t, RoleKind(role.Kind), got.Roles[j].Kind())
			assert.Equal(t, role.VouchesFor, got.Roles[j].VouchesFor())
			assert.Equal(t, role.Platforms, got.Roles[j].Platforms())
		}
	}

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(parsed.Registry, &body))
	var agents []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body["agents"], &agents))
	for _, agent := range agents {
		var slug string
		require.NoError(t, json.Unmarshal(agent["slug"], &slug))
		if slug == "plain" {
			_, present := agent["roles"]
			assert.False(t, present, "a binding with no roles must omit the roles key")
		}
	}
}

func TestCrossLanguageRolesFixture_RejectsAreSchemaNotSignature(t *testing.T) {
	fx := loadRolesFixture(t)
	rootPub := mustB64Pub(t, fx.RootPubKey)
	want := []struct {
		name string
		err  string
	}{
		{"unknown_kind", ErrMalformedRole},
		{"bridge_extra_field", ErrMalformedRole},
		{"bridge_missing_vouches_for", ErrMalformedRole},
		{"code_signing_missing_platforms", ErrMalformedRole},
		{"roles_not_array", ErrMalformedRole},
		{"roles_null", ErrMalformedRole},
		{"vouches_for_null", ErrMalformedRole},
		{"non_string_in_vouches_for", ErrMalformedRole},
		{"non_nfc_vouches_for", identity.ErrSchemaNotNFC},
	}
	require.Len(t, fx.Reject, len(want))
	for i, rc := range fx.Reject {
		t.Run(rc.Name, func(t *testing.T) {
			assert.Equal(t, want[i].name, rc.Name)
			assert.Equal(t, "reject", rc.Expect)
			parsed, err := ParseDistribution(rc.Envelope)
			require.NoError(t, err, "the envelope itself is well formed")
			ok, err := identity.VerifyCanonical(rootPub, identity.CanonicalBytesFromTrusted(parsed.Registry), parsed.RootSig)
			require.NoError(t, err)
			assert.True(t, ok, "root signature must hold; the reject is the schema")
			_, _, err = VerifyAndLoad(rootPub, parsed, fx.Valid.PersistedEpoch,
				time.Unix(fx.Now, 0), time.Duration(fx.MaxStalenessSec)*time.Second)
			require.ErrorContains(t, err, want[i].err)
		})
	}
}

func loadRolesFixture(t *testing.T) rolesFixture {
	t.Helper()
	data, err := os.ReadFile(rolesFixturePath)
	require.NoError(t, err, "committed roles fixture must exist; run with -update to generate it")
	var fx rolesFixture
	require.NoError(t, json.Unmarshal(data, &fx))
	return fx
}
