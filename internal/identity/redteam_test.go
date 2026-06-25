package identity_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"
)

// --- Fix 1: low-order / small-subgroup pubkey forgery (cofactorless strict verify) ---

// smallOrderPubkeys are the canonical encodings of the edwards25519 8-torsion
// points (the complete small-order subgroup). Paired with an all-zero
// signature, ANY of them forges acceptance for every message under both stdlib
// crypto/ed25519 and bare ZIP-215, because [S]B = R + [k]A holds for all k when
// A, R, and S are all the identity/zero. VerifyCanonical must reject all of them.
var smallOrderPubkeys = []string{
	"0000000000000000000000000000000000000000000000000000000000000000", // all-zero (identity-ish, the realistic attack)
	"0100000000000000000000000000000000000000000000000000000000000000", // identity point
	"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
	"0000000000000000000000000000000000000000000000000000000000000080",
	"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a",
	"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac03fa",
	"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05",
	"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc85",
}

// TestVerify_RejectsLowOrderForgery is the slice-1 red-team CRITICAL: a
// small-order public key paired with an all-zero signature forges acceptance
// for every message under stdlib crypto/ed25519 (and even under bare ZIP-215,
// which standardizes small-order acceptance). Cofactorless strict
// VerifyCanonical must reject every small-order pubkey — including for the
// realistic registry entry.
func TestVerify_RejectsLowOrderForgery(t *testing.T) {
	zeroSig := make([]byte, ed25519.SignatureSize) // 64 zero bytes

	// The realistic entry a forger would target.
	realistic, err := identity.Canonicalize([]byte(`{"key_epoch":1,"slug":"mira"}`))
	require.NoError(t, err)

	zeroPub := make([]byte, ed25519.PublicKeySize)
	ok, err := identity.VerifyCanonical(zeroPub, realistic, zeroSig)
	assert.False(t, ok,
		"all-zero (small-order) pubkey+sig must NOT verify the realistic entry {key_epoch:1,slug:mira}")
	// A small-order key is a STRUCTURAL reject, not an honest non-match: the
	// caller must be able to tell a forgery-key rejection from a mismatch.
	require.Error(t, err,
		"all-zero small-order pubkey must return a non-nil error (structural reject), not (false, nil)")

	// Every small-order point, over several canonical messages, must be rejected
	// with a non-nil structural error (forgery-key reject, never an honest match).
	msgs := []identity.CanonicalBytes{
		realistic,
		identity.CanonicalBytesFromTrusted([]byte(`{"a":1}`)),
		identity.CanonicalBytesFromTrusted([]byte(`{"slug":"attacker"}`)),
		identity.CanonicalBytesFromTrusted([]byte(`{"key_epoch":2,"slug":"victim"}`)),
	}
	for _, pubHex := range smallOrderPubkeys {
		pub, derr := hex.DecodeString(pubHex)
		require.NoError(t, derr)
		for _, msg := range msgs {
			got, verr := identity.VerifyCanonical(pub, msg, zeroSig)
			assert.False(t, got,
				"small-order pubkey %s must never verify any message: %s", pubHex, msg.Bytes())
			require.Error(t, verr,
				"small-order pubkey %s must be a structural reject (non-nil error): %s", pubHex, msg.Bytes())
		}
	}
}

// TestVerify_RejectsUndecodablePubkey: a 32-byte value that is NOT a valid
// point encoding must be rejected as a structural reject (non-nil error), since
// VerifyCanonical fails closed when the pubkey A does not decode. The encoding y=2
// (0x02 followed by zeros) is undecodable: its y-coordinate yields a non-square,
// so no valid x exists on the curve.
func TestVerify_RejectsUndecodablePubkey(t *testing.T) {
	undecodable := mustDecodeHex(t,
		"0200000000000000000000000000000000000000000000000000000000000000")
	zeroSig := make([]byte, ed25519.SignatureSize)
	ok, err := identity.VerifyCanonical(undecodable, identity.CanonicalBytesFromTrusted([]byte(`{"a":1}`)), zeroSig)
	assert.False(t, ok, "an undecodable 32-byte pubkey must not verify")
	require.ErrorContains(t, err, identity.ErrVerifySmallOrderPubKey,
		"an undecodable pubkey is a structural reject (non-nil error), not an honest non-match")
}

// TestVerify_RejectsWrongLengthPubkey: a public key that is not
// ed25519.PublicKeySize bytes is a structural reject.
func TestVerify_RejectsWrongLengthPubkey(t *testing.T) {
	zeroSig := make([]byte, ed25519.SignatureSize)
	ok, err := identity.VerifyCanonical([]byte{0x01, 0x02}, identity.CanonicalBytesFromTrusted([]byte(`{"a":1}`)), zeroSig)
	assert.False(t, ok, "a wrong-length pubkey must not verify")
	require.ErrorContains(t, err, identity.ErrVerifyBadPubKeyLen,
		"a wrong-length pubkey is a structural reject (non-nil error)")
}

// TestVerifyCanonical_FrozenVectorStillPasses guards the STOP condition: the
// honest RFC-8032 frozen signature MUST still verify under cofactorless strict
// verification (which accepts all honest signatures). If this fails, the check
// was weakened wrongly.
func TestVerifyCanonical_FrozenVectorStillPasses(t *testing.T) {
	pub := ed25519.PublicKey(mustDecodeHex(t, frozenPubkeyHex))
	canonical := identity.CanonicalBytesFromTrusted(mustDecodeHex(t, frozenCanonicalBytesHex))
	sig := mustDecodeHex(t, frozenSignatureHex)
	ok, err := identity.VerifyCanonical(pub, canonical, sig)
	require.NoError(t, err)
	assert.True(t, ok,
		"honest frozen RFC-8032 signature must still verify under cofactorless strict verification")
}

// --- Fix 2: schema gate wired into the signing path ---

func TestSignEntry_RejectsFloatDecimal(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)
	_, err := identity.SignEntry(priv, []byte(`{"val":1.0}`))
	assert.Error(t, err, "SignEntry must run the schema gate and reject a float (1.0)")
}

func TestSignEntry_RejectsFloatExponent(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)
	_, err := identity.SignEntry(priv, []byte(`{"val":1e2}`))
	assert.Error(t, err, "SignEntry must reject an exponent float (1e2)")
}

func TestSignEntry_RejectsIntAbove2Pow53(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)
	_, err := identity.SignEntry(priv, []byte(`{"val":9007199254740993}`))
	assert.Error(t, err, "SignEntry must reject an integer above 2^53")
}

func TestSignEntry_RejectsNonASCIIKey(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)
	// Key "na" + U+00EF (ï) — a non-ASCII object key.
	_, err := identity.SignEntry(priv, []byte("{\"naïve\":1}"))
	assert.Error(t, err, "SignEntry must reject a non-ASCII object key")
}

func TestVerifyEntry_RejectsSchemaViolation(t *testing.T) {
	pub := ed25519.PublicKey(mustDecodeHex(t, frozenPubkeyHex))
	sig := mustDecodeHex(t, frozenSignatureHex)
	_, err := identity.VerifyEntry(pub, []byte(`{"val":1.0}`), sig)
	assert.Error(t, err, "VerifyEntry must run the schema gate and reject a float (1.0)")
}

// --- Fix 3: strings (and keys) must be NFC ---

// NFD/NFC fixtures are built from explicit code points (\u escapes) so the
// bytes are unambiguous regardless of how this source file is saved:
//   - nfdE: 'e' (U+0065) + COMBINING ACUTE ACCENT (U+0301) — decomposed.
//   - nfcE: PRECOMPOSED LATIN SMALL LETTER E WITH ACUTE (U+00E9) — composed.
const (
	nfdE = "é"
	nfcE = "é"
)

func TestValidateSchema_RejectsNFDStringValue(t *testing.T) {
	// Sanity-check the fixture is genuinely NFD (its NFC form must differ).
	require.NotEqual(t, nfdE, norm.NFC.String(nfdE),
		"fixture must be NFD: its NFC form must differ")

	doc := `{"display_name":"caf` + nfdE + `"}` // "cafe" + combining acute
	err := identity.ValidateSchema([]byte(doc))
	require.ErrorContains(t, err, identity.ErrSchemaNotNFC,
		"an NFD-normalized string value must be rejected by the NFC rule")
}

func TestValidateSchema_AcceptsNFCStringValue(t *testing.T) {
	require.Equal(t, nfcE, norm.NFC.String(nfcE),
		"fixture must already be NFC")

	doc := `{"display_name":"caf` + nfcE + `"}` // "cafe" with precomposed acute
	require.NoError(t, identity.ValidateSchema([]byte(doc)),
		"the NFC form of the same string must be accepted")
}

func TestValidateSchema_RejectsNonASCIIObjectKeyNFD(t *testing.T) {
	// Object keys are ASCII-only by the schema's own rule, so a key containing
	// any non-ASCII codepoint (here an NFD-decomposed "café") is rejected by the
	// ASCII-key gate — which always runs first, since every non-NFC codepoint is
	// necessarily non-ASCII. (This is why the schema's separate isNFC(key) check
	// was dead code: it could never be reached for a non-ASCII key.)
	doc := `{"caf` + nfdE + `":1}`
	err := identity.ValidateSchema([]byte(doc))
	require.ErrorContains(t, err, identity.ErrSchemaNonASCIIKey,
		"a non-ASCII object key must be rejected by the ASCII-key rule")
}

func TestValidateSchema_FrozenStarStillNFC(t *testing.T) {
	// The frozen vector's "Mira ⭐" is already NFC and must keep passing.
	require.NoError(t, identity.ValidateSchema([]byte(frozenInputJSON)),
		"the frozen entry (already NFC) must still pass after the NFC gate")
}

// --- Fix 4: nil / wrong-length private key must not panic ---

func TestSignEntry_NilPrivKeyReturnsError(t *testing.T) {
	_, err := identity.SignEntry(nil, []byte(`{"slug":"mira"}`))
	assert.Error(t, err, "SignEntry(nil, ...) must return an error, not panic")
}

func TestSignCanonical_NilPrivKeyReturnsError(t *testing.T) {
	sig, err := identity.SignCanonical(nil, identity.CanonicalBytesFromTrusted([]byte("canonical")))
	assert.Nil(t, sig, "SignCanonical(nil, ...) must not return a signature")
	require.Error(t, err,
		"SignCanonical(nil, ...) must return an error (fail closed), not panic")
}

func TestSignCanonical_WrongLengthPrivKeyReturnsError(t *testing.T) {
	sig, err := identity.SignCanonical(ed25519.PrivateKey{0x01, 0x02}, identity.CanonicalBytesFromTrusted([]byte("canonical")))
	assert.Nil(t, sig, "SignCanonical with a wrong-length key must not return a signature")
	require.Error(t, err,
		"SignCanonical with a wrong-length key must return an error (fail closed), not panic")
}

// --- Fix 5: unbounded recursion in ValidateSchema ---

func TestValidateSchema_RejectsDeeplyNested(t *testing.T) {
	// 10,000 levels of nested objects must be rejected by the depth limit
	// rather than overflowing the stack.
	depth := 10000
	doc := strings.Repeat(`{"a":`, depth) + `1` + strings.Repeat(`}`, depth)
	err := identity.ValidateSchema([]byte(doc))
	assert.Error(t, err, "a 10,000-level nested document must be rejected (no panic)")
}
