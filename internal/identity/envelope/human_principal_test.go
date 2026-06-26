package envelope

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// frozenCanonical is the EXACT JCS-canonical form of the S3 human-principal
// sample (a bridge-attested chat message), byte-locked against workhorse's Rust
// canonicalize_envelope. CanonicalizeEnvelope MUST reproduce these bytes.
const frozenCanonical = `{"alg_version":1,"body":"hello from a human","from_agent":"agent:bridge","from_class":"bridge","key_epoch":1,"kind":"chat","nonce":"n1","room":"mesh","seq":5,"ts":1782360000,"vouches_for":"human:siavoush"}`

// frozenSHA256 is the dual hash-pin over frozenCanonical; workhorse pins the
// same value on the Rust side.
const frozenSHA256 = "730268fa5a353e2461cd55bade24409ff5a183f189ad9e613a76860d35ad5bfd"

// nfdName is a deliberately non-NFC string: 'e' + U+0301 COMBINING ACUTE ACCENT.
// Written as ASCII escapes so the source bytes are unambiguous (an editor cannot
// silently re-normalize it to the NFC single code point U+00E9).
const nfdName = "é"

// frozenFields builds the Fields for the frozen bridge sample.
func frozenFields() Fields {
	return Fields{
		AlgVersion: AlgVersion,
		Body:       "hello from a human",
		FromAgent:  "agent:bridge",
		FromClass:  strptr(FromClassBridge),
		KeyEpoch:   1,
		Kind:       strptr(KindChat),
		Nonce:      "n1",
		Room:       strptr("mesh"),
		Seq:        5,
		TS:         1782360000,
		VouchesFor: strptr("human:siavoush"),
	}
}

// TestCanonicalize_FrozenHumanPrincipalSample is the byte-parity acceptance test:
// the frozen sample canonicalizes to the EXACT frozen bytes AND its sha256 equals
// the dual hash-pin shared with workhorse's Rust verifier.
func TestCanonicalize_FrozenHumanPrincipalSample(t *testing.T) {
	canonical, err := CanonicalizeEnvelope(frozenFields())
	require.NoError(t, err)

	assert.Equal(t, frozenCanonical, string(canonical.Bytes()),
		"canonical bytes must match the frozen cross-language sample")

	sum := sha256.Sum256(canonical.Bytes())
	assert.Equal(t, frozenSHA256, hex.EncodeToString(sum[:]),
		"sha256 of the canonical bytes must match the dual hash-pin")
}

// TestCanonicalize_PlainAgentBackwardCompat proves a plain agent message (all 4
// new fields omitted) canonicalizes to the SAME bytes as before this change — no
// field is added when absent (absent != null).
func TestCanonicalize_PlainAgentBackwardCompat(t *testing.T) {
	f := validFields() // none of the 4 new fields set
	canonical, err := CanonicalizeEnvelope(f)
	require.NoError(t, err)

	s := string(canonical.Bytes())
	assert.NotContains(t, s, "from_class")
	assert.NotContains(t, s, "kind")
	assert.NotContains(t, s, "vouches_for")
	assert.NotContains(t, s, "gate_ref")
	// The exact pre-change canonical form for validFields().
	want := `{"alg_version":1,"body":"hello ⭐","from_agent":"mira","key_epoch":1,"nonce":"YWJjZGVmZ2hpamtsbW5vcA==","room":"dev","seq":7,"ts":2000000}`
	assert.Equal(t, want, s)
}

// TestHumanPrincipalGates_Reject covers every new structural wrap-side gate.
func TestHumanPrincipalGates_Reject(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Fields)
		errMsg string
	}{
		{"invalid_from_class", func(f *Fields) { f.FromClass = strptr("robot") }, ErrFromClassInvalid},
		{"invalid_kind", func(f *Fields) { f.Kind = strptr("ping") }, ErrKindInvalid},
		{"non_nfc_vouches_for", func(f *Fields) { f.VouchesFor = strptr(nfdName) }, ErrFieldNotNFC},
		{"bridge_without_vouches_for", func(f *Fields) { f.VouchesFor = nil }, ErrBridgeNeedsVouch},
		{"vouches_for_without_bridge", func(f *Fields) {
			f.FromClass = strptr(FromClassAgent)
			f.VouchesFor = strptr("human:siavoush")
		}, ErrVouchNeedsBridge},
		{"approval_without_gate_ref", func(f *Fields) { f.Kind = strptr(KindApproval) }, ErrApprovalNeedsGateRef},
		{"gate_ref_without_approval", func(f *Fields) { f.GateRef = strptr("gate-1") }, ErrGateRefNeedsApproval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := frozenFields()
			tc.mutate(&f)
			_, err := CanonicalizeEnvelope(f)
			require.Error(t, err)
			assert.EqualError(t, err, tc.errMsg)
		})
	}
}

// TestCanonicalize_ApprovalWithGateRef proves the approval/gate_ref pair
// canonicalizes (both present is the valid approval shape). vouches_for here is
// required because from_class=bridge.
func TestCanonicalize_ApprovalWithGateRef(t *testing.T) {
	f := frozenFields()
	f.Kind = strptr(KindApproval)
	f.GateRef = strptr("gate:abc")
	canonical, err := CanonicalizeEnvelope(f)
	require.NoError(t, err)
	s := string(canonical.Bytes())
	assert.Contains(t, s, `"kind":"approval"`)
	assert.Contains(t, s, `"gate_ref":"gate:abc"`)
}

// regBridge builds a single-binding registry for slug at key_epoch=1 holding pub
// with the given class + vouch allowlist.
func regBridge(t *testing.T, slug string, pub ed25519.PublicKey, class string, allow []string) registry.Registry {
	t.Helper()
	pk, err := registry.NewPublicKey(pub)
	require.NoError(t, err)
	return registry.Registry{
		Epoch: 1, ValidFrom: 0, ValidUntil: 1 << 40,
		Agents: []registry.AgentBinding{{
			Slug: slug, DisplayName: "Bridge", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 0, ValidUntil: 1 << 40,
			Class: class, VouchAllowlist: allow,
		}},
	}
}

// signFrozen signs the frozen sample directly with priv (deterministic).
func signFrozen(t *testing.T, priv ed25519.PrivateKey, f Fields) []byte {
	t.Helper()
	canonical, err := CanonicalizeEnvelope(f)
	require.NoError(t, err)
	sig, err := identity.SignCanonical(priv, canonical)
	require.NoError(t, err)
	return sig
}

// TestVerify_FromClassBridge_FailClosed_NoClassBinding proves that a from_class=
// bridge claim is REJECTED when the resolved binding has no class set (defaults
// to agent) — authenticated != authorized; the registry must GRANT the class.
func TestVerify_FromClassBridge_FailClosed_NoClassBinding(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	f := frozenFields()
	sig := signFrozen(t, priv, f)

	reg := regBridge(t, f.FromAgent, pub, "", nil) // Class unset => agent
	ok, err := VerifyEnvelope(reg, f, sig, time.Unix(1782360000, 0))
	require.Error(t, err)
	assert.False(t, ok)
	assert.EqualError(t, err, ErrVerifyClassMismatch)
}

// TestVerify_FromClassBridge_GrantedAndAllowlisted proves a binding that GRANTS
// class=bridge with a matching vouch allowlist verifies the bridge envelope.
func TestVerify_FromClassBridge_GrantedAndAllowlisted(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	f := frozenFields()
	sig := signFrozen(t, priv, f)

	reg := regBridge(t, f.FromAgent, pub, FromClassBridge, []string{"human:siavoush"})
	ok, err := VerifyEnvelope(reg, f, sig, time.Unix(1782360000, 0))
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestVerify_FromClassBridge_VouchNotAllowed proves a vouch outside the binding's
// allowlist is a structural reject even with class granted.
func TestVerify_FromClassBridge_VouchNotAllowed(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	f := frozenFields()
	f.VouchesFor = strptr("human:peiman")
	sig := signFrozen(t, priv, f)

	reg := regBridge(t, f.FromAgent, pub, FromClassBridge, []string{"human:siavoush"})
	ok, err := VerifyEnvelope(reg, f, sig, time.Unix(1782360000, 0))
	require.Error(t, err)
	assert.False(t, ok)
	assert.EqualError(t, err, ErrVerifyVouchNotAllowed)
}

// TestVerify_FromClassBridge_EmptyAllowlistRejects proves an empty allowlist
// authorizes NO vouch even when the class is granted (fail closed).
func TestVerify_FromClassBridge_EmptyAllowlistRejects(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	f := frozenFields()
	sig := signFrozen(t, priv, f)

	reg := regBridge(t, f.FromAgent, pub, FromClassBridge, nil)
	ok, err := VerifyEnvelope(reg, f, sig, time.Unix(1782360000, 0))
	require.Error(t, err)
	assert.False(t, ok)
	assert.EqualError(t, err, ErrVerifyVouchNotAllowed)
}

// TestVerify_PlainAgent_BackwardCompat proves a plain agent envelope (no
// from_class) verifies exactly as before — the legacy path is untouched.
func TestVerify_PlainAgent_BackwardCompat(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	f := validFields()
	sig := signFrozen(t, priv, f)

	reg := regWith(t, f.FromAgent, pub)
	ok, err := VerifyEnvelope(reg, f, sig, time.Unix(2_000_000, 0))
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestEffectiveClass_DefaultsToAgent proves the empty-Class default semantics.
func TestEffectiveClass_DefaultsToAgent(t *testing.T) {
	assert.Equal(t, registry.ClassAgent, registry.AgentBinding{}.EffectiveClass())
	assert.Equal(t, FromClassBridge, registry.AgentBinding{Class: FromClassBridge}.EffectiveClass())
}
