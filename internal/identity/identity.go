// Package identity implements the Contract-B trust-root signing core:
// RFC 8785 (JCS) canonicalization, ed25519 signing/verification over the
// canonical bytes, and the Contract-B schema-validation gate.
//
// The cardinal rule of this package is that signatures are ALWAYS computed
// over canonical bytes. To make that rule type-enforced, canonical bytes carry
// their own type (CanonicalBytes): SignCanonical/VerifyCanonical accept ONLY
// CanonicalBytes, so a caller cannot pass raw JSON into the low-level signing
// path — that is a COMPILE error, not a runtime footgun.
//
// Callers should prefer SignEntry/VerifyEntry: these are the VALIDATED entry
// points — they run the Contract-B schema gate (ValidateSchema) AND canonicalize
// before signing/verifying, so a caller cannot accidentally sign a
// non-conformant or non-canonical entry.
//
// SignCanonical/VerifyCanonical are the low-level primitives. They BYPASS the
// schema gate and assume the caller already holds JCS-canonical bytes (e.g. a
// registry/replay path verifying a frozen vector). Do not use them on untrusted
// raw JSON — prefer SignEntry/VerifyEntry there.
//
// Verification is COFACTORLESS strict verification, matching the Rust verifier
// on the other side of this trust root (ed25519-dalek verify_strict). The two
// verifiers MUST reach the same verdict on identical trust-root bytes; any
// divergence is a forgeable seam. ZIP-215 (cofactored) deliberately ACCEPTS
// small-order / mixed-order R components for blockchain-consensus determinism,
// whereas verify_strict REJECTS them — so verify_strict is the ruleset that
// keeps Go and Rust in lockstep. The strict check rejects, as STRUCTURAL
// failures:
//
//  1. A small-order (or undecodable) public key A. The forgery {A=0, R=0, S=0}
//     satisfies [S]B = R + [k]A for EVERY message, so any small-order pubkey is
//     a universal-forgery vector. A point is small-order iff [8]A is the
//     identity (8 is the edwards25519 cofactor).
//  2. A small-order (or undecodable) signature R component. ZIP-215 would
//     accept these; verify_strict (and so this code) rejects them.
//  3. A non-canonical S scalar (S >= L, the group order).
//
// The signature is then confirmed cofactorlessly via [S]B - [k]A == R.
//
// Signing stays on stdlib crypto/ed25519 (it emits canonical RFC-8032
// signatures, so the frozen vector still verifies under strict verification).
//
// TODO(contract-b registry slice): introduce a SignedEntry domain type bundling
// {entry, pubkey, sig} and a validating PublicKey constructor; both are deferred
// to the registry slice so this slice stays the signing/validation core only.
package identity

import (
	"crypto/ed25519"
	"crypto/sha512"
	"crypto/subtle"
	"fmt"

	"filippo.io/edwards25519"
	"github.com/gowebpki/jcs"
)

// CanonicalBytes is the RFC 8785 (JCS) canonical encoding of a JSON document —
// the exact byte string that is signed and verified.
//
// It is a STRUCT wrapping an unexported []byte rather than a `type
// CanonicalBytes []byte` alias on purpose: a named slice type still accepts a
// raw []byte by implicit assignment (Go assignability allows it because []byte
// is an unnamed type), which would leave the "signed the wrong bytes" bypass
// open. Wrapping the bytes in a struct closes that hole — SignCanonical/
// VerifyCanonical REQUIRE a CanonicalBytes, and a raw []byte does NOT compile in
// their place.
//
// The blessed constructor is Canonicalize. The only other way to mint one is
// CanonicalBytesFromTrusted, a deliberately-named (greppable) escape hatch for
// the registry/replay path that already holds known-canonical bytes (e.g. a
// frozen vector). Both make construction explicit, never implicit.
type CanonicalBytes struct {
	b []byte
}

// Bytes returns the underlying canonical byte string. The returned slice aliases
// the internal storage; callers must not mutate it.
func (c CanonicalBytes) Bytes() []byte { return c.b }

// CanonicalBytesFromTrusted wraps bytes the caller asserts are ALREADY
// JCS-canonical, WITHOUT re-canonicalizing or validating them. It is the
// registry/replay escape hatch (e.g. verifying a frozen vector from known
// canonical bytes). The name is intentionally explicit so misuse is greppable —
// prefer Canonicalize for any path that holds raw JSON.
func CanonicalBytesFromTrusted(b []byte) CanonicalBytes { return CanonicalBytes{b: b} }

// errCanonicalize wraps a JCS transform failure with package context.
const errCanonicalize = "identity: canonicalize"

// errBadPrivKeyLen is returned by SignCanonical (and therefore by SignEntry,
// which delegates to it) when the private key is nil or the wrong length,
// instead of letting crypto/ed25519.Sign panic.
const errBadPrivKeyLen = "identity: sign: private key must be ed25519.PrivateKeySize bytes"

// errVerifyHRAM wraps the (provably unreachable) failure to reduce the 64-byte
// SHA-512 challenge digest to a scalar. SetUniformBytes only errors on a
// wrong-length input, which cannot occur for a fixed 64-byte digest; we fail
// closed with this error rather than silently treating it as a non-match.
const errVerifyHRAM = "identity: verify: reduce challenge scalar"

// Structural verification-reject messages (SSOT). VerifyCanonical returns these
// as a non-nil error so an operator can distinguish a malformed-input /
// forgery-key rejection from an honest signature non-match (which is (false,
// nil)). They are EXPORTED so callers/tests reference the single definition.
const (
	// ErrVerifyBadPubKeyLen is returned for a public key that is not
	// ed25519.PublicKeySize bytes.
	ErrVerifyBadPubKeyLen = "identity: verify: public key must be ed25519.PublicKeySize bytes"
	// ErrVerifyBadSigLen is returned for a signature that is not
	// ed25519.SignatureSize bytes.
	ErrVerifyBadSigLen = "identity: verify: signature must be ed25519.SignatureSize bytes"
	// ErrVerifySmallOrderPubKey is returned for a small-order (universal-forgery)
	// or otherwise undecodable public key.
	ErrVerifySmallOrderPubKey = "identity: verify: public key is small-order or undecodable"
	// ErrVerifySmallOrderR is returned for a signature whose R component is
	// small-order or undecodable. ZIP-215 would accept these; cofactorless
	// strict verification (matching dalek verify_strict) rejects them.
	ErrVerifySmallOrderR = "identity: verify: signature R is small-order or undecodable"
	// ErrVerifyNonCanonicalR is returned for a signature whose R component is a
	// non-canonical point encoding (y >= p). filippo SetBytes silently reduces it
	// mod p, but dalek verify_strict compares the COMPRESSED bytes of the
	// recomputed R against sig[:32] — so a non-canonical R it rejects must be
	// rejected here too, or Go would accept what Rust rejects (a forgeable seam).
	ErrVerifyNonCanonicalR = "identity: verify: signature R is a non-canonical encoding (y >= p)"
	// ErrVerifyNonCanonicalS is returned for a signature whose S scalar is
	// non-canonical (S >= L, the group order).
	ErrVerifyNonCanonicalS = "identity: verify: signature S is non-canonical (>= L)"
)

// Canonicalize returns the RFC 8785 (JCS) canonical form of the supplied
// JSON. The canonical form sorts object keys lexicographically by their
// UTF-16 code units, emits the shortest round-trippable number form, and
// preserves string values as raw UTF-8 (e.g. ⭐ stays 0xE2 0xAD 0x90,
// never \u-escaped). The output is the exact byte string that callers must
// sign and verify, carried in CanonicalBytes so it cannot be confused with
// raw JSON.
func Canonicalize(jsonBytes []byte) (CanonicalBytes, error) {
	canonical, err := jcs.Transform(jsonBytes)
	if err != nil {
		return CanonicalBytes{}, fmt.Errorf("%s: %w", errCanonicalize, err)
	}
	return CanonicalBytes{b: canonical}, nil
}

// SignCanonical produces an ed25519 signature over the already-canonical
// bytes. It BYPASSES the Contract-B schema gate, so callers must already hold
// JCS-canonical, schema-valid bytes (registry/replay use only) — prefer
// SignEntry for raw JSON. It returns an error (never panics) on a nil or
// wrong-length private key. The CanonicalBytes parameter type makes it
// impossible to pass raw JSON into this path.
func SignCanonical(priv ed25519.PrivateKey, canonical CanonicalBytes) ([]byte, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%s", errBadPrivKeyLen)
	}
	return ed25519.Sign(priv, canonical.b), nil
}

// VerifyCanonical reports whether sig is a valid ed25519 signature by pub over
// the already-canonical bytes, using COFACTORLESS strict verification that
// matches the Rust trust-root verifier (ed25519-dalek verify_strict). It
// BYPASSES the Contract-B schema gate (registry/replay use only) — prefer
// VerifyEntry for raw JSON.
//
// It distinguishes two kinds of failure:
//   - STRUCTURAL rejects return (false, non-nil error): a wrong-length public
//     key or signature; a small-order / undecodable public key A (a
//     universal-forgery vector); a small-order / undecodable signature R (which
//     ZIP-215 would accept but verify_strict rejects); a non-canonical R point
//     encoding (y >= p, which dalek byte-compares and rejects); or a
//     non-canonical S scalar (S >= L). These mean the input/key is malformed or
//     hostile, not that an honest signature simply did not match.
//   - An honest signature non-match returns (false, nil).
//
// A valid match returns (true, nil). It never panics.
func VerifyCanonical(pub ed25519.PublicKey, canonical CanonicalBytes, sig []byte) (bool, error) {
	if len(pub) != ed25519.PublicKeySize {
		return false, fmt.Errorf("%s", ErrVerifyBadPubKeyLen)
	}
	if len(sig) != ed25519.SignatureSize {
		return false, fmt.Errorf("%s", ErrVerifyBadSigLen)
	}
	// Decode and reject a small-order (or undecodable) public key A: a
	// small-order key is a universal-forgery vector. ZIP-215 standardizes
	// small-order acceptance, so this explicit guard is what closes the hole.
	a, err := new(edwards25519.Point).SetBytes(pub)
	if err != nil || isSmallOrderPoint(a) {
		return false, fmt.Errorf("%s", ErrVerifySmallOrderPubKey)
	}
	// Decode and reject a small-order (or undecodable) R. verify_strict rejects
	// these; ZIP-215 would accept them — this is the Go<->Rust divergence guard.
	r, err := new(edwards25519.Point).SetBytes(sig[:32])
	if err != nil || isSmallOrderPoint(r) {
		return false, fmt.Errorf("%s", ErrVerifySmallOrderR)
	}
	// Reject a non-canonical R encoding (y >= p). SetBytes silently reduced it
	// mod p above; dalek verify_strict instead byte-compares the recomputed R
	// against sig[:32] and rejects, so we must reject too — else Go accepts an R
	// encoding Rust rejects (a Go<->Rust verdict divergence on the trust root).
	if subtle.ConstantTimeCompare(r.Bytes(), sig[:32]) != 1 {
		return false, fmt.Errorf("%s", ErrVerifyNonCanonicalR)
	}
	// Decode S as a CANONICAL scalar: SetCanonicalBytes errors if S >= L.
	s, err := edwards25519.NewScalar().SetCanonicalBytes(sig[32:])
	if err != nil {
		return false, fmt.Errorf("%s", ErrVerifyNonCanonicalS)
	}
	// Cofactorless check: R == [S]B - [k]A, with k = SHA512(R || A || M) mod L.
	hramInput := make([]byte, 0, 32+ed25519.PublicKeySize+len(canonical.b))
	hramInput = append(hramInput, sig[:32]...)
	hramInput = append(hramInput, pub...)
	hramInput = append(hramInput, canonical.b...)
	hram := sha512.Sum512(hramInput)
	k, err := edwards25519.NewScalar().SetUniformBytes(hram[:])
	if err != nil {
		// SetUniformBytes only errors on a wrong-length input, which cannot
		// happen for a fixed 64-byte SHA-512 digest. Fail CLOSED with a non-nil
		// error rather than (false, nil): an internal invariant breach is not an
		// honest signature non-match.
		return false, fmt.Errorf("%s: %w", errVerifyHRAM, err)
	}
	negK := edwards25519.NewScalar().Negate(k)
	rCheck := new(edwards25519.Point).VarTimeDoubleScalarBaseMult(negK, a, s)
	return rCheck.Equal(r) == 1, nil
}

// isSmallOrderPoint reports whether p is a small-order point (a member of the
// curve's 8-torsion subgroup). Such points must never appear as a verifying A or
// R: with the matching all-zero signature they forge acceptance for every
// message, and a small-order R is exactly what ZIP-215 accepts but verify_strict
// rejects. A point P is small-order iff [8]P is the identity (8 is the
// edwards25519 cofactor).
func isSmallOrderPoint(p *edwards25519.Point) bool {
	cofactorMultiple := new(edwards25519.Point).MultByCofactor(p)
	return cofactorMultiple.Equal(edwards25519.NewIdentityPoint()) == 1
}

// SignEntry is a VALIDATED entry point: it runs the Contract-B schema gate,
// canonicalizes the raw JSON entry, then signs the canonical bytes. It makes
// it impossible to sign a non-conformant or non-canonical entry. It returns an
// error for a schema violation, uncanonicalizable JSON, or a nil/wrong-length
// private key (never panics). The (sig, err) it returns is SignCanonical's,
// returned directly after the entry has been validated and canonicalized.
func SignEntry(priv ed25519.PrivateKey, jsonBytes []byte) ([]byte, error) {
	if err := ValidateSchema(jsonBytes); err != nil {
		return nil, err
	}
	canonical, err := Canonicalize(jsonBytes)
	if err != nil {
		return nil, err
	}
	return SignCanonical(priv, canonical)
}

// VerifyEntry is a VALIDATED entry point: it runs the Contract-B schema gate,
// canonicalizes the raw JSON entry, then verifies sig against the canonical
// bytes using cofactorless strict verification (matching dalek verify_strict).
// It returns an error when the entry
// violates the schema, cannot be canonicalized, or is structurally rejected by
// VerifyCanonical (malformed key/sig, small-order pubkey); a well-formed,
// conformant entry whose signature simply does not match returns (false, nil).
func VerifyEntry(pub ed25519.PublicKey, jsonBytes []byte, sig []byte) (bool, error) {
	if err := ValidateSchema(jsonBytes); err != nil {
		return false, err
	}
	canonical, err := Canonicalize(jsonBytes)
	if err != nil {
		return false, err
	}
	return VerifyCanonical(pub, canonical, sig)
}
