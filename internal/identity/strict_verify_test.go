package identity_test

import (
	"crypto/ed25519"
	"crypto/sha512"
	"testing"

	"filippo.io/edwards25519"
	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Cofactorless strict verification: match ed25519-dalek verify_strict ---
//
// The Go trust-root verifier must reach the SAME verdict as the Rust verifier
// (ed25519-dalek verify_strict, which is cofactorless) on identical bytes.
// ZIP-215 (cofactored) deliberately ACCEPTS small-order / mixed-order R
// components for consensus determinism; verify_strict REJECTS them. Any verdict
// divergence on the trust root is a forgeable seam, so VerifyCanonical must use
// cofactorless strict verification.

// clampScalarBytes applies the standard ed25519 clamping to the lower 32 bytes
// of SHA512(seed), yielding the secret scalar a such that A = [a]B.
func clampScalarBytes(h []byte) []byte {
	a := make([]byte, 32)
	copy(a, h[:32])
	a[0] &= 248
	a[31] &= 127
	a[31] |= 64
	return a
}

// makeSmallOrderRForgery constructs the regression-defining vector: a signature
// whose R component is the identity point (small-order) yet which satisfies the
// COFACTORED verify equation [S]B = R + [k]A, so ZIP-215 wrongly accepts it.
//
// With R = identity, the equation reduces to [S]B = [k]A. Choosing S = a*k mod L
// gives [S]B = [k]([a]B) = [k]A, so it holds exactly. verify_strict (and our
// strict VerifyCanonical) must REJECT it because R is small-order.
func makeSmallOrderRForgery(t *testing.T, seed, msg []byte) (pub ed25519.PublicKey, sig []byte) {
	t.Helper()

	priv := ed25519.NewKeyFromSeed(seed)
	pub = priv.Public().(ed25519.PublicKey)

	// R = identity point (small-order), canonical encoding.
	rPoint := edwards25519.NewIdentityPoint()
	rBytes := rPoint.Bytes()

	// k = SHA512(R || A || msg) reduced mod L.
	hramInput := make([]byte, 0, 32+ed25519.PublicKeySize+len(msg))
	hramInput = append(hramInput, rBytes...)
	hramInput = append(hramInput, pub...)
	hramInput = append(hramInput, msg...)
	hram := sha512.Sum512(hramInput)
	k, err := edwards25519.NewScalar().SetUniformBytes(hram[:])
	require.NoError(t, err, "k must reduce")

	// a = clamped lower half of SHA512(seed); the secret scalar with A = [a]B.
	hSeed := sha512.Sum512(seed)
	aScalar, err := edwards25519.NewScalar().SetBytesWithClamping(clampScalarBytes(hSeed[:]))
	require.NoError(t, err, "a must reduce")

	// S = a*k mod L.
	sScalar := edwards25519.NewScalar().Multiply(aScalar, k)

	sig = make([]byte, ed25519.SignatureSize)
	copy(sig[:32], rBytes)
	copy(sig[32:], sScalar.Bytes())
	return pub, sig
}

// TestVerify_RejectsSmallOrderR is the regression-defining test. The constructed
// vector has a small-order (identity) R and satisfies the COFACTORED equation,
// so the previous ZIP-215 finalize WRONGLY ACCEPTED it. Strict cofactorless
// verification must reject it as a STRUCTURAL forgery (false, non-nil error).
func TestVerify_RejectsSmallOrderR(t *testing.T) {
	seed := mustDecodeHex(t, frozenSeedHex)
	msg := mustDecodeHex(t, frozenCanonicalBytesHex)
	canonical := identity.CanonicalBytesFromTrusted(msg)

	pub, sig := makeSmallOrderRForgery(t, seed, msg)

	// Sanity: the forgery is well-formed (right lengths, decodable points/scalar).
	require.Len(t, sig, ed25519.SignatureSize)
	require.Len(t, pub, ed25519.PublicKeySize)

	ok, err := identity.VerifyCanonical(pub, canonical, sig)
	assert.False(t, ok,
		"a small-order-R forgery satisfying the cofactored equation must NOT verify under strict (cofactorless) verification")
	require.ErrorContains(t, err, identity.ErrVerifySmallOrderR,
		"a small-order R is a STRUCTURAL reject (non-nil error), matching dalek verify_strict; ZIP-215 would have wrongly accepted it")
}

// nonCanonicalSScalarBytes returns a 32-byte S encoding with S >= L (the group
// order), which RFC 8032 / verify_strict reject as non-canonical. It is built as
// L + 1 in little-endian.
func nonCanonicalSScalarBytes() []byte {
	// L = 2^252 + 27742317777372353535851937790883648493 (little-endian, 32 bytes).
	l := []byte{
		0xed, 0xd3, 0xf5, 0x5c, 0x1a, 0x63, 0x12, 0x58,
		0xd6, 0x9c, 0xf7, 0xa2, 0xde, 0xf9, 0xde, 0x14,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10,
	}
	// L + 1 is the smallest non-canonical S >= L.
	carry := uint16(1)
	for i := 0; i < 32 && carry != 0; i++ {
		v := uint16(l[i]) + carry
		l[i] = byte(v & 0xff)
		carry = v >> 8
	}
	return l
}

// TestVerify_RejectsNonCanonicalS: a signature whose S scalar is >= L is
// non-canonical. verify_strict rejects it (SetCanonicalBytes errors), so
// VerifyCanonical must reject it as a structural reject, never an honest match.
func TestVerify_RejectsNonCanonicalS(t *testing.T) {
	pub := ed25519.PublicKey(mustDecodeHex(t, frozenPubkeyHex))
	canonical := identity.CanonicalBytesFromTrusted(mustDecodeHex(t, frozenCanonicalBytesHex))

	// Keep the frozen R; replace S with a non-canonical (S >= L) encoding.
	frozenSig := mustDecodeHex(t, frozenSignatureHex)
	sig := make([]byte, ed25519.SignatureSize)
	copy(sig[:32], frozenSig[:32])
	copy(sig[32:], nonCanonicalSScalarBytes())

	ok, err := identity.VerifyCanonical(pub, canonical, sig)
	assert.False(t, ok, "a non-canonical S (S >= L) must not verify")
	require.ErrorContains(t, err, identity.ErrVerifyNonCanonicalS,
		"a non-canonical S is a STRUCTURAL reject (non-nil error), matching dalek verify_strict")
}

// TestVerify_RejectsNonCanonicalR: an R component encoded non-canonically (y >= p)
// is silently reduced mod p by filippo SetBytes, but dalek verify_strict
// byte-compares the recomputed R against sig[:32] and rejects it. VerifyCanonical
// must therefore reject it as a STRUCTURAL error (not an honest non-match), or Go
// would accept an R encoding Rust rejects — a trust-root verdict divergence.
func TestVerify_RejectsNonCanonicalR(t *testing.T) {
	pub := ed25519.PublicKey(mustDecodeHex(t, frozenPubkeyHex))
	canonical := identity.CanonicalBytesFromTrusted(mustDecodeHex(t, frozenCanonicalBytesHex))

	// y = p + 3 (little-endian), a non-canonical encoding that SetBytes reduces to
	// the canonical y = 3 point — which is on-curve and NOT small-order, so it
	// passes the decode + small-order gates and reaches the non-canonical check.
	nonCanonicalR := mustDecodeHex(t, "f0ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f")
	sig := make([]byte, ed25519.SignatureSize)
	copy(sig[:32], nonCanonicalR)
	copy(sig[32:], mustDecodeHex(t, frozenSignatureHex)[32:]) // any canonical S

	ok, err := identity.VerifyCanonical(pub, canonical, sig)
	assert.False(t, ok, "a non-canonical R encoding (y >= p) must not verify")
	require.ErrorContains(t, err, identity.ErrVerifyNonCanonicalR,
		"a non-canonical R is a STRUCTURAL reject, matching dalek's byte-compare; it must not fall through to an honest non-match (false, nil) or a small-order reject")
}

// TestVerify_StrictStillAcceptsFrozenVector guards the STOP condition: the
// honest RFC-8032 frozen signature must still verify under cofactorless strict
// verification (verify_strict accepts all honest signatures).
func TestVerify_StrictStillAcceptsFrozenVector(t *testing.T) {
	pub := ed25519.PublicKey(mustDecodeHex(t, frozenPubkeyHex))
	canonical := identity.CanonicalBytesFromTrusted(mustDecodeHex(t, frozenCanonicalBytesHex))
	sig := mustDecodeHex(t, frozenSignatureHex)

	ok, err := identity.VerifyCanonical(pub, canonical, sig)
	require.NoError(t, err)
	assert.True(t, ok,
		"the honest frozen RFC-8032 signature must still verify under strict cofactorless verification")
}
