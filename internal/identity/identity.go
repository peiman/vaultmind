// Package identity implements the Contract-B trust-root signing core:
// RFC 8785 (JCS) canonicalization, ed25519 signing/verification over the
// canonical bytes, and the Contract-B schema-validation gate.
//
// The cardinal rule of this package is that signatures are ALWAYS computed
// over canonical bytes. Callers should prefer SignEntry/VerifyEntry, which
// canonicalize first so a caller cannot accidentally sign raw,
// non-canonical JSON. The low-level Sign/Verify exist for callers that
// already hold canonical bytes (e.g. a verifier replaying a frozen vector).
package identity

import (
	"crypto/ed25519"
	"fmt"

	"github.com/gowebpki/jcs"
)

// errCanonicalize wraps a JCS transform failure with package context.
const errCanonicalize = "identity: canonicalize"

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

// Sign produces an ed25519 signature over the already-canonical bytes.
// The caller is responsible for ensuring canonical is the JCS canonical
// form; prefer SignEntry when starting from raw JSON.
func Sign(priv ed25519.PrivateKey, canonical []byte) []byte {
	return ed25519.Sign(priv, canonical)
}

// Verify reports whether sig is a valid ed25519 signature by pub over the
// already-canonical bytes. A malformed (wrong-length) signature or public
// key returns false rather than panicking.
func Verify(pub ed25519.PublicKey, canonical []byte, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, canonical, sig)
}

// SignEntry canonicalizes the raw JSON entry and signs the canonical bytes.
// This is the safe entry point: it makes it impossible to sign a
// non-canonical representation of an entry.
func SignEntry(priv ed25519.PrivateKey, jsonBytes []byte) ([]byte, error) {
	canonical, err := Canonicalize(jsonBytes)
	if err != nil {
		return nil, err
	}
	return Sign(priv, canonical), nil
}

// VerifyEntry canonicalizes the raw JSON entry and verifies sig against the
// canonical bytes. It returns an error only when the entry cannot be
// canonicalized (malformed JSON); a well-formed entry that simply fails
// verification returns (false, nil).
func VerifyEntry(pub ed25519.PublicKey, jsonBytes []byte, sig []byte) (bool, error) {
	canonical, err := Canonicalize(jsonBytes)
	if err != nil {
		return false, err
	}
	return Verify(pub, canonical, sig), nil
}
