package registry

import (
	"encoding/hex"
	"testing"

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
