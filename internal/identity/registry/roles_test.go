package registry

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rolesFreeCanonicalHex is the JCS canonical encoding of rolesFreeRegistry, captured
// from main before AgentBinding grew Roles. A roles-free registry must stay
// byte-identical to that encoding: omitempty on an empty roles list is what keeps
// already-signed registries and the frozen cross-language fixture valid.
const rolesFreeCanonicalHex = "7b226167656e7473223a5b7b22617574686f72697a65645f6f726967696e5f6461656d6f6e73223a5b226461656d6f6e2d65752d31225d2c22646973706c61795f6e616d65223a224d697261222c226b65795f65706f6368223a312c227075626b6579223a226f4a716c3948706e575941762b56583433433071464b584a6e534f2b6c2f686b456e2f354f4452567050413d222c22736c7567223a226d697261222c2276616c69645f66726f6d223a313737303030303030302c2276616c69645f756e74696c223a313738303030303030307d5d2c2265706f6368223a352c2276616c69645f66726f6d223a313737303030303030302c2276616c69645f756e74696c223a313738303030303030307d"

// rolesFreeRegistry is the registry whose pre-roles canonical bytes are pinned above.
// The agent key is the fixed 0x22 seed, so the pubkey (and therefore the bytes) cannot drift.
func rolesFreeRegistry(t *testing.T, roles []Role) Registry {
	t.Helper()
	pub, _ := fixedEd25519(t, 0x22)
	pk, err := NewPublicKey(pub)
	require.NoError(t, err)
	return Registry{
		Epoch: 5, ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
		Agents: []AgentBinding{{
			Slug: "mira", DisplayName: "Mira", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
			AuthorizedOriginDaemons: []string{"daemon-eu-1"},
			Roles:                   roles,
		}},
	}
}

func TestCanonicalBytes_RolesFreeMatchesPreRolesGolden(t *testing.T) {
	for _, roles := range [][]Role{nil, {}, {}} {
		cb, err := canonicalBytes(rolesFreeRegistry(t, roles))
		require.NoError(t, err)
		assert.Equal(t, rolesFreeCanonicalHex, hex.EncodeToString(cb.Bytes()))
		assert.NotContains(t, string(cb.Bytes()), "roles")
	}
}

func TestCanonicalBytes_EmitsRolesInGivenOrder(t *testing.T) {
	bridge := NewBridgeRole([]string{"human:peiman", "human:ada"})
	signing := NewCodeSigningRole([]string{"darwin-arm64", "linux-amd64"})

	cb, err := canonicalBytes(rolesFreeRegistry(t, []Role{bridge, signing}))
	require.NoError(t, err)
	const want = `"roles":[{"kind":"bridge","vouches_for":["human:peiman","human:ada"]},{"kind":"code_signing","platforms":["darwin-arm64","linux-amd64"]}]`
	assert.Contains(t, string(cb.Bytes()), want)

	reversed, err := canonicalBytes(rolesFreeRegistry(t, []Role{signing, bridge}))
	require.NoError(t, err)
	const wantReversed = `"roles":[{"kind":"code_signing","platforms":["darwin-arm64","linux-amd64"]},{"kind":"bridge","vouches_for":["human:peiman","human:ada"]}]`
	assert.Contains(t, string(reversed.Bytes()), wantReversed)
	assert.NotEqual(t, cb.Bytes(), reversed.Bytes(), "role array order is signed as given")
}

func TestCanonicalBytes_EmptyParameterListIsAnEmptyArray(t *testing.T) {
	cb, err := canonicalBytes(rolesFreeRegistry(t, []Role{NewBridgeRole(nil)}))
	require.NoError(t, err)
	assert.Contains(t, string(cb.Bytes()), `"roles":[{"kind":"bridge","vouches_for":[]}]`)
	assert.NotContains(t, string(cb.Bytes()), "null")

	cb, err = canonicalBytes(rolesFreeRegistry(t, []Role{NewCodeSigningRole([]string{})}))
	require.NoError(t, err)
	assert.Contains(t, string(cb.Bytes()), `"roles":[{"kind":"code_signing","platforms":[]}]`)
}

func TestCanonicalBytes_RejectsZeroRole(t *testing.T) {
	_, err := canonicalBytes(rolesFreeRegistry(t, []Role{{}}))
	require.ErrorContains(t, err, ErrMalformedRole)
}

func TestRoleConstructors_DoNotAliasCallerSlices(t *testing.T) {
	in := []string{"human:peiman"}
	role := NewBridgeRole(in)
	in[0] = "human:mutated"
	assert.Equal(t, []string{"human:peiman"}, role.VouchesFor())

	got := role.VouchesFor()
	got[0] = "human:mutated"
	assert.Equal(t, []string{"human:peiman"}, role.VouchesFor())

	assert.Equal(t, RoleKindBridge, role.Kind())
	assert.Nil(t, role.Platforms())
	assert.False(t, role.IsZero())

	signing := NewCodeSigningRole([]string{"darwin-arm64"})
	assert.Equal(t, RoleKindCodeSigning, signing.Kind())
	assert.Equal(t, []string{"darwin-arm64"}, signing.Platforms())
	assert.Nil(t, signing.VouchesFor())
	assert.True(t, Role{}.IsZero())
}

func TestVerifyAndLoad_RolesRoundTrip(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	base := rolesFreeRegistry(t, nil).Agents[0]

	plain := base
	plain.Slug = "plain"

	bridge := base
	bridge.Slug = "bridge"
	bridge.Roles = []Role{
		NewBridgeRole([]string{"human:peiman"}),
		NewCodeSigningRole(nil),
	}

	signer := base
	signer.Slug = "signer"
	signer.Roles = []Role{NewCodeSigningRole([]string{"darwin-arm64", "linux-amd64"})}

	reg := rolesFreeRegistry(t, nil)
	reg.Agents = []AgentBinding{bridge, signer, plain}

	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	loaded, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	require.NoError(t, err)

	assertAgentRoles(t, loaded, "bridge", bridge.Roles)
	assertAgentRoles(t, loaded, "signer", signer.Roles)
	assertAgentRoles(t, loaded, "plain", nil)
}

func TestVerifyAndLoad_AcceptsEmptyRolesArray(t *testing.T) {
	rootPub, env := signedBodyWithRoles(t, `[]`)
	loaded, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	require.NoError(t, err)
	require.Empty(t, loaded.Agents[0].Roles)
}

func TestVerifyAndLoad_RejectsMalformedRoles(t *testing.T) {
	cases := []struct {
		name  string
		roles string
	}{
		{name: "unknown_kind", roles: `[{"kind":"wizard","vouches_for":["human:peiman"]}]`},
		{name: "bridge_extra_field", roles: `[{"kind":"bridge","vouches_for":["human:peiman"],"extra":true}]`},
		{name: "bridge_missing_vouches_for", roles: `[{"kind":"bridge"}]`},
		{name: "code_signing_missing_platforms", roles: `[{"kind":"code_signing"}]`},
		{name: "roles_not_array", roles: `{"kind":"bridge"}`},
		{name: "bridge_with_platforms", roles: `[{"kind":"bridge","vouches_for":[],"platforms":["darwin-arm64"]}]`},
		{name: "code_signing_extra_field", roles: `[{"kind":"code_signing","platforms":[],"extra":1}]`},
		{name: "vouches_for_not_array", roles: `[{"kind":"bridge","vouches_for":"human:peiman"}]`},
		{name: "roles_null", roles: `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rootPub, env := signedBodyWithRoles(t, tc.roles)
			_, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
			require.ErrorContains(t, err, ErrMalformedRole)
		})
	}
}

func TestVerifyAndLoad_IgnoresUnknownBindingFieldButParsesRoles(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	reg := rolesFreeRegistry(t, []Role{NewBridgeRole([]string{"human:peiman"})})
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(env.Registry, &obj))
	agents, ok := obj["agents"].([]any)
	require.True(t, ok)
	binding, ok := agents[0].(map[string]any)
	require.True(t, ok)
	binding["note"] = "not a role field"
	patched, err := json.Marshal(obj)
	require.NoError(t, err)
	canonical, err := identity.Canonicalize(patched)
	require.NoError(t, err)
	sig, err := identity.SignCanonical(rootPriv, canonical)
	require.NoError(t, err)

	loaded, _, err := VerifyAndLoad(rootPub, SignedRegistry{Registry: canonical.Bytes(), RootSig: sig}, 4, fixedNow, time.Hour*24*365)
	require.NoError(t, err)
	assertAgentRoles(t, loaded, "mira", reg.Agents[0].Roles)
}

func assertAgentRoles(t *testing.T, reg Registry, slug string, want []Role) {
	t.Helper()
	var got []Role
	found := false
	for _, b := range reg.Agents {
		if b.Slug == slug {
			got = b.Roles
			found = true
			break
		}
	}
	require.True(t, found, "slug %s", slug)
	require.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i].Kind(), got[i].Kind(), "slug %s role %d kind", slug, i)
		assert.Equal(t, want[i].VouchesFor(), got[i].VouchesFor(), "slug %s role %d vouches_for", slug, i)
		assert.Equal(t, want[i].Platforms(), got[i].Platforms(), "slug %s role %d platforms", slug, i)
	}
}

// signedBodyWithRoles root-signs a registry whose only interesting deviation is
// the roles value, so a VerifyAndLoad failure is the role schema and not the signature.
func signedBodyWithRoles(t *testing.T, rolesJSON string) (ed25519.PublicKey, SignedRegistry) {
	t.Helper()
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	pub, _ := fixedEd25519(t, 0x22)
	body := `{"agents":[{"authorized_origin_daemons":["daemon-eu-1"],"display_name":"Mira",` +
		`"key_epoch":1,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `",` +
		`"roles":` + rolesJSON + `,"slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`
	canonical, err := identity.Canonicalize([]byte(body))
	require.NoError(t, err)
	sig, err := identity.SignCanonical(rootPriv, canonical)
	require.NoError(t, err)
	return rootPub, SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0}
}
