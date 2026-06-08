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

// --- Fix 1: low-order / small-subgroup pubkey forgery (ZIP-215 strict verify) ---

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
// which standardizes small-order acceptance). VerifyCanonical must reject every
// small-order pubkey — including for the realistic registry entry.
func TestVerify_RejectsLowOrderForgery(t *testing.T) {
	zeroSig := make([]byte, ed25519.SignatureSize) // 64 zero bytes

	// The realistic entry a forger would target.
	realistic, err := identity.Canonicalize([]byte(`{"key_epoch":1,"slug":"mira"}`))
	require.NoError(t, err)

	zeroPub := make([]byte, ed25519.PublicKeySize)
	assert.False(t, identity.VerifyCanonical(zeroPub, realistic, zeroSig),
		"all-zero (small-order) pubkey+sig must NOT verify the realistic entry {key_epoch:1,slug:mira}")

	// Every small-order point, over several canonical messages, must be rejected.
	msgs := [][]byte{
		realistic,
		[]byte(`{"a":1}`),
		[]byte(`{"slug":"attacker"}`),
		[]byte(`{"key_epoch":2,"slug":"victim"}`),
	}
	for _, pubHex := range smallOrderPubkeys {
		pub, derr := hex.DecodeString(pubHex)
		require.NoError(t, derr)
		for _, msg := range msgs {
			assert.False(t, identity.VerifyCanonical(pub, msg, zeroSig),
				"small-order pubkey %s must never verify any message: %s", pubHex, msg)
		}
	}
}

// TestVerifyCanonical_FrozenVectorStillPasses guards the STOP condition: the
// honest RFC-8032 frozen signature MUST still verify under ZIP-215 (which
// accepts all honest signatures). If this fails, the check was weakened wrongly.
func TestVerifyCanonical_FrozenVectorStillPasses(t *testing.T) {
	pub := ed25519.PublicKey(mustDecodeHex(t, frozenPubkeyHex))
	canonical := mustDecodeHex(t, frozenCanonicalBytesHex)
	sig := mustDecodeHex(t, frozenSignatureHex)
	assert.True(t, identity.VerifyCanonical(pub, canonical, sig),
		"honest frozen RFC-8032 signature must still verify under ZIP-215")
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
	assert.Error(t, err, "an NFD-normalized string value must be rejected")
}

func TestValidateSchema_AcceptsNFCStringValue(t *testing.T) {
	require.Equal(t, nfcE, norm.NFC.String(nfcE),
		"fixture must already be NFC")

	doc := `{"display_name":"caf` + nfcE + `"}` // "cafe" with precomposed acute
	require.NoError(t, identity.ValidateSchema([]byte(doc)),
		"the NFC form of the same string must be accepted")
}

func TestValidateSchema_RejectsNFDObjectKey(t *testing.T) {
	// A non-NFC object key must be rejected. (It is also non-ASCII; either gate
	// may fire — the point is rejection, not which rule trips.)
	doc := `{"caf` + nfdE + `":1}`
	err := identity.ValidateSchema([]byte(doc))
	assert.Error(t, err, "a non-NFC object key must be rejected")
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

func TestSignCanonical_NilPrivKeyReturnsNil(t *testing.T) {
	assert.Nil(t, identity.SignCanonical(nil, []byte("canonical")),
		"SignCanonical(nil, ...) must fail closed (return nil), not panic")
}

func TestSignCanonical_WrongLengthPrivKeyReturnsNil(t *testing.T) {
	assert.Nil(t, identity.SignCanonical(ed25519.PrivateKey{0x01, 0x02}, []byte("canonical")),
		"SignCanonical with a wrong-length key must fail closed (return nil), not panic")
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
