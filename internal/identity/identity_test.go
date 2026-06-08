package identity_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Frozen cross-language acceptance vector (RFC 8785 JCS + ed25519).
// These bytes MUST NOT change without a coordinated cross-language bump:
// any drift breaks interop with non-Go verifiers of Contract-B entries.
const (
	frozenInputJSON = `{"valid_until":1780000000,"slug":"mira","authorized_origin_daemons":["daemon-eu-1","daemon-us-2"],"display_name":"Mira ⭐","key_epoch":1,"pubkey":"qZ3Fq2eJ0w8vK1u8m4r6t8y0a2c4e6g8i0k2m4o6q8s=","valid_from":1770000000}`

	frozenCanonicalUTF8 = `{"authorized_origin_daemons":["daemon-eu-1","daemon-us-2"],"display_name":"Mira ⭐","key_epoch":1,"pubkey":"qZ3Fq2eJ0w8vK1u8m4r6t8y0a2c4e6g8i0k2m4o6q8s=","slug":"mira","valid_from":1770000000,"valid_until":1780000000}`

	frozenCanonicalBytesHex = "7b22617574686f72697a65645f6f726967696e5f6461656d6f6e73223a5b226461656d6f6e2d65752d31222c226461656d6f6e2d75732d32225d2c22646973706c61795f6e616d65223a224d69726120e2ad90222c226b65795f65706f6368223a312c227075626b6579223a22715a33467132654a307738764b3175386d34723674387930613263346536673869306b326d346f367138733d222c22736c7567223a226d697261222c2276616c69645f66726f6d223a313737303030303030302c2276616c69645f756e74696c223a313738303030303030307d"

	frozenSeedHex      = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	frozenPubkeyHex    = "79b5562e8fe654f94078b112e8a98ba7901f853ae695bed7e0e3910bad049664"
	frozenSignatureHex = "a753626692139283d3b6b884907eed8d79ce82716a5e34e856ffcdb51ec79609aea8c5843aa8154a664b9c6b191cd28ea23d76ac3790f2f505afd0302f46c80c"

	// The display_name star (⭐, U+2B50) MUST survive canonicalization as
	// raw UTF-8 (0xE2 0xAD 0x90), never \u-escaped.
	frozenStarUTF8Hex = "e2ad90"
)

// mustDecodeHex fails the test on a malformed fixture rather than panicking.
func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err, "fixture hex must decode")
	return b
}

func TestCanonicalize_FrozenVector(t *testing.T) {
	got, err := identity.Canonicalize([]byte(frozenInputJSON))
	require.NoError(t, err)

	wantBytes := mustDecodeHex(t, frozenCanonicalBytesHex)
	assert.True(t, bytes.Equal(wantBytes, got),
		"canonical bytes mismatch:\n got hex=%s\nwant hex=%s", hex.EncodeToString(got), frozenCanonicalBytesHex)

	// The canonical UTF-8 string form must match exactly.
	assert.Equal(t, frozenCanonicalUTF8, string(got))
}

func TestCanonicalize_StarSurvivesAsRawUTF8(t *testing.T) {
	got, err := identity.Canonicalize([]byte(frozenInputJSON))
	require.NoError(t, err)

	starBytes := mustDecodeHex(t, frozenStarUTF8Hex)
	assert.True(t, bytes.Contains(got, starBytes),
		"raw UTF-8 star (e2ad90) must appear in canonical output")
	// The star must survive as raw UTF-8: the literal rune is present and the
	// output contains no \u-escape sequence for it. We build the escape
	// strings from runes to avoid editor/source rendering the escape as a
	// literal star.
	assert.Contains(t, string(got), "⭐",
		"star must remain raw UTF-8")
	lowerEscape := string([]rune{'\\', 'u', '2', 'b', '5', '0'})
	upperEscape := string([]rune{'\\', 'u', '2', 'B', '5', '0'})
	assert.NotContains(t, string(got), lowerEscape,
		"star must NOT be \\u-escaped (lowercase)")
	assert.NotContains(t, string(got), upperEscape,
		"star must NOT be \\u-escaped (uppercase)")
}

func TestCanonicalize_RejectsInvalidJSON(t *testing.T) {
	_, err := identity.Canonicalize([]byte(`{not json`))
	assert.Error(t, err)
}

func TestSign_ReproducesFrozenSignature(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)

	canonical := mustDecodeHex(t, frozenCanonicalBytesHex)
	sig := identity.Sign(priv, canonical)

	assert.Equal(t, frozenSignatureHex, hex.EncodeToString(sig),
		"signature over canonical bytes must match frozen vector")
}

func TestVerify_FrozenVector(t *testing.T) {
	pub := ed25519.PublicKey(mustDecodeHex(t, frozenPubkeyHex))
	canonical := mustDecodeHex(t, frozenCanonicalBytesHex)
	sig := mustDecodeHex(t, frozenSignatureHex)

	assert.True(t, identity.Verify(pub, canonical, sig))
}

func TestVerify_DerivedPubkeyMatchesFrozen(t *testing.T) {
	// The pubkey in the frozen vector must be the one derived from the seed.
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	assert.Equal(t, frozenPubkeyHex, hex.EncodeToString(pub))
}

func TestSignEntry_VerifyEntry_RoundTrip(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	sig, err := identity.SignEntry(priv, []byte(frozenInputJSON))
	require.NoError(t, err)

	// SignEntry must canonicalize first, so it reproduces the frozen signature
	// even though the input is in non-canonical key order.
	assert.Equal(t, frozenSignatureHex, hex.EncodeToString(sig))

	ok, err := identity.VerifyEntry(pub, []byte(frozenInputJSON), sig)
	require.NoError(t, err)
	assert.True(t, ok, "VerifyEntry must accept a signature produced by SignEntry")
}

func TestSignEntry_InvalidJSONErrors(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)
	_, err := identity.SignEntry(priv, []byte(`{not valid`))
	assert.Error(t, err, "SignEntry must refuse to sign uncanonicalizable JSON")
}

func TestVerifyEntry_TamperedBytesFail(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	sig, err := identity.SignEntry(priv, []byte(frozenInputJSON))
	require.NoError(t, err)

	tampered := `{"valid_until":1780000000,"slug":"mira","authorized_origin_daemons":["daemon-eu-1","daemon-us-2"],"display_name":"Mira ⭐","key_epoch":2,"pubkey":"qZ3Fq2eJ0w8vK1u8m4r6t8y0a2c4e6g8i0k2m4o6q8s=","valid_from":1770000000}`
	ok, err := identity.VerifyEntry(pub, []byte(tampered), sig)
	require.NoError(t, err)
	assert.False(t, ok, "tampered entry must not verify")
}

func TestVerifyEntry_WrongPubkeyFails(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)

	sig, err := identity.SignEntry(priv, []byte(frozenInputJSON))
	require.NoError(t, err)

	// A different key derived from a different seed.
	otherSeed := make([]byte, ed25519.SeedSize)
	otherSeed[0] = 0xFF
	otherPub := ed25519.NewKeyFromSeed(otherSeed).Public().(ed25519.PublicKey)

	ok, err := identity.VerifyEntry(otherPub, []byte(frozenInputJSON), sig)
	require.NoError(t, err)
	assert.False(t, ok, "verification with the wrong pubkey must fail")
}

func TestVerifyEntry_InvalidJSONErrors(t *testing.T) {
	pub := ed25519.PublicKey(mustDecodeHex(t, frozenPubkeyHex))
	_, err := identity.VerifyEntry(pub, []byte(`{broken`), mustDecodeHex(t, frozenSignatureHex))
	assert.Error(t, err, "VerifyEntry must surface a canonicalization error, not silently fail")
}

func TestVerify_WrongLengthSignature(t *testing.T) {
	pub := ed25519.PublicKey(mustDecodeHex(t, frozenPubkeyHex))
	canonical := mustDecodeHex(t, frozenCanonicalBytesHex)
	assert.False(t, identity.Verify(pub, canonical, []byte{0x00}),
		"a malformed (wrong-length) signature must return false, not panic")
}
