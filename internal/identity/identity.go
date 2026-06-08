// Package identity implements the Contract-B trust-root signing core:
// RFC 8785 (JCS) canonicalization, ed25519 signing/verification over the
// canonical bytes, and the Contract-B schema-validation gate.
//
// The cardinal rule of this package is that signatures are ALWAYS computed
// over canonical bytes. Callers should prefer SignEntry/VerifyEntry: these
// are the VALIDATED entry points — they run the Contract-B schema gate
// (ValidateSchema) AND canonicalize before signing/verifying, so a caller
// cannot accidentally sign a non-conformant or non-canonical entry.
//
// SignCanonical/VerifyCanonical are the low-level primitives. They BYPASS
// the schema gate and assume the caller already holds JCS-canonical bytes
// (e.g. a registry/replay path verifying a frozen vector). Do not use them
// on untrusted raw JSON — prefer SignEntry/VerifyEntry there.
//
// Verification hardens the all-zero / small-subgroup pubkey-forgery hole that
// stdlib crypto/ed25519.Verify leaves open, in two layers:
//
//  1. An explicit small-order public-key reject. The forgery {A=0, R=0, S=0}
//     satisfies [S]B = R + [k]A for EVERY message, so any small-order pubkey
//     is a universal-forgery vector. We decode the pubkey and reject it when
//     [8]A is the identity (i.e. A is in the 8-torsion). NOTE: ZIP-215 by
//     itself does NOT reject small-order points — it standardizes their
//     acceptance for consensus determinism — so this explicit guard, not the
//     verify ruleset, is what actually closes the hole.
//  2. ZIP-215 strict verification (ed25519consensus) for the signature check,
//     which deterministically rejects non-canonical point/scalar encodings.
//
// Signing stays on stdlib crypto/ed25519 (ZIP-215 is a verify-only ruleset and
// accepts all honest RFC-8032 signatures, so the frozen vector still verifies).
package identity

import (
	"crypto/ed25519"
	"fmt"

	"filippo.io/edwards25519"
	"github.com/gowebpki/jcs"
	"github.com/hdevalence/ed25519consensus"
)

// errCanonicalize wraps a JCS transform failure with package context.
const errCanonicalize = "identity: canonicalize"

// errBadPrivKeyLen is returned by SignEntry when the private key is nil or the
// wrong length, instead of letting crypto/ed25519.Sign panic.
const errBadPrivKeyLen = "identity: sign: private key must be ed25519.PrivateKeySize bytes"

// Canonicalize returns the RFC 8785 (JCS) canonical form of the supplied
// JSON. The canonical form sorts object keys lexicographically by their
// UTF-16 code units, emits the shortest round-trippable number form, and
// preserves string values as raw UTF-8 (e.g. ⭐ stays 0xE2 0xAD 0x90,
// never \u-escaped). The output is the exact byte string that callers must
// sign and verify.
func Canonicalize(jsonBytes []byte) ([]byte, error) {
	canonical, err := jcs.Transform(jsonBytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errCanonicalize, err)
	}
	return canonical, nil
}

// SignCanonical produces an ed25519 signature over the already-canonical
// bytes. It BYPASSES the Contract-B schema gate, so callers must already hold
// JCS-canonical, schema-valid bytes (registry/replay use only) — prefer
// SignEntry for raw JSON. It fails closed (returns nil) on a nil or
// wrong-length private key rather than panicking.
func SignCanonical(priv ed25519.PrivateKey, canonical []byte) []byte {
	if len(priv) != ed25519.PrivateKeySize {
		return nil
	}
	return ed25519.Sign(priv, canonical)
}

// VerifyCanonical reports whether sig is a valid ed25519 signature by pub over
// the already-canonical bytes, using ZIP-215 strict verification. It BYPASSES
// the Contract-B schema gate (registry/replay use only) — prefer VerifyEntry
// for raw JSON. A malformed (wrong-length) signature or public key, or a
// small-order / non-canonical pubkey, returns false rather than panicking.
func VerifyCanonical(pub ed25519.PublicKey, canonical []byte, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	// Reject small-order (and undecodable) public keys first: a small-order key
	// is a universal-forgery vector that ed25519consensus would otherwise
	// accept. ZIP-215 standardizes small-order acceptance, so this explicit
	// guard is what closes the hole.
	if isSmallOrderPubkey(pub) {
		return false
	}
	// ZIP-215 (ed25519consensus) gives deterministic signature verification,
	// rejecting non-canonical point/scalar encodings.
	return ed25519consensus.Verify(pub, canonical, sig)
}

// isSmallOrderPubkey reports whether pub decodes to a small-order point (a
// member of the curve's 8-torsion subgroup), or fails to decode at all. Such
// keys must never verify: with the matching all-zero signature they forge
// acceptance for every message. A point A is small-order iff [8]A is the
// identity (8 is the edwards25519 cofactor).
func isSmallOrderPubkey(pub []byte) bool {
	p, err := new(edwards25519.Point).SetBytes(pub)
	if err != nil {
		// An undecodable encoding is not a usable key — fail closed.
		return true
	}
	cofactorMultiple := new(edwards25519.Point).MultByCofactor(p)
	return cofactorMultiple.Equal(edwards25519.NewIdentityPoint()) == 1
}

// SignEntry is a VALIDATED entry point: it runs the Contract-B schema gate,
// canonicalizes the raw JSON entry, then signs the canonical bytes. It makes
// it impossible to sign a non-conformant or non-canonical entry. It returns an
// error for a schema violation, uncanonicalizable JSON, or a nil/wrong-length
// private key (never panics).
func SignEntry(priv ed25519.PrivateKey, jsonBytes []byte) ([]byte, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%s", errBadPrivKeyLen)
	}
	if err := ValidateSchema(jsonBytes); err != nil {
		return nil, err
	}
	canonical, err := Canonicalize(jsonBytes)
	if err != nil {
		return nil, err
	}
	return SignCanonical(priv, canonical), nil
}

// VerifyEntry is a VALIDATED entry point: it runs the Contract-B schema gate,
// canonicalizes the raw JSON entry, then verifies sig against the canonical
// bytes using ZIP-215 strict verification. It returns an error when the entry
// violates the schema or cannot be canonicalized; a well-formed, conformant
// entry that simply fails verification returns (false, nil).
func VerifyEntry(pub ed25519.PublicKey, jsonBytes []byte, sig []byte) (bool, error) {
	if err := ValidateSchema(jsonBytes); err != nil {
		return false, err
	}
	canonical, err := Canonicalize(jsonBytes)
	if err != nil {
		return false, err
	}
	return VerifyCanonical(pub, canonical, sig), nil
}
