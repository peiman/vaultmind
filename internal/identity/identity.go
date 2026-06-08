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
//     which deterministically rejects non-canonical SIGNATURE point/scalar
//     encodings.
//
// Signing stays on stdlib crypto/ed25519 (ZIP-215 is a verify-only ruleset and
// accepts all honest RFC-8032 signatures, so the frozen vector still verifies).
//
// TODO(contract-b registry slice): introduce a SignedEntry domain type bundling
// {entry, pubkey, sig} and a validating PublicKey constructor; both are deferred
// to the registry slice so this slice stays the signing/validation core only.
package identity

import (
	"crypto/ed25519"
	"fmt"

	"filippo.io/edwards25519"
	"github.com/gowebpki/jcs"
	"github.com/hdevalence/ed25519consensus"
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
// the already-canonical bytes, using ZIP-215 strict verification. It BYPASSES
// the Contract-B schema gate (registry/replay use only) — prefer VerifyEntry
// for raw JSON.
//
// It distinguishes two kinds of failure:
//   - STRUCTURAL rejects return (false, non-nil error): a wrong-length public
//     key or signature, or a small-order / undecodable public key (a
//     universal-forgery vector). These mean the input/key is malformed or
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
	// Reject small-order (and undecodable) public keys first: a small-order key
	// is a universal-forgery vector that ed25519consensus would otherwise
	// accept. ZIP-215 standardizes small-order acceptance, so this explicit
	// guard is what closes the hole.
	if isSmallOrderPubkey(pub) {
		return false, fmt.Errorf("%s", ErrVerifySmallOrderPubKey)
	}
	// ZIP-215 (ed25519consensus) gives deterministic signature verification,
	// rejecting non-canonical SIGNATURE point/scalar encodings. An honest
	// non-match is reported as (false, nil).
	return ed25519consensus.Verify(pub, canonical.b, sig), nil
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
// bytes using ZIP-215 strict verification. It returns an error when the entry
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
