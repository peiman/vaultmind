package registry

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/peiman/vaultmind/internal/identity"
)

// Domain-type and namespace constants (SSOT — referenced, never inlined).
const (
	// ErrPubKeyBadLen is returned by NewPublicKey for a key that is not
	// ed25519.PublicKeySize bytes.
	ErrPubKeyBadLen = "registry: public key must be ed25519.PublicKeySize bytes"
	// ErrPubKeySmallOrder is returned by NewPublicKey for a small-order
	// (universal-forgery) or undecodable public key.
	ErrPubKeySmallOrder = "registry: public key is small-order or undecodable"

	// GlobalPrefix is the reserved global-prefix slot for a network id (Door 5).
	// v1 ids carry this prefix so they can be made hierarchical later WITHOUT a
	// re-key. The id itself is derived from the keypair, never from a public git
	// remote (which is public + precomputable and enables traffic analysis).
	GlobalPrefix = "vmnet1:"

	// networkIDHashBytes is how many leading bytes of the pubkey hash form the
	// network id body. 16 bytes (128 bits) is collision-safe for the id space.
	networkIDHashBytes = 16
)

// PublicKey is a VALIDATED ed25519 public key: it is wrong-length-rejected and
// small-order-rejected AT CONSTRUCTION (NewPublicKey), so an invalid key is
// unrepresentable in the binding/verify path. Code that holds a PublicKey can
// rely on it being decodable and not a universal-forgery vector.
type PublicKey struct {
	key ed25519.PublicKey
}

// NewPublicKey validates b and returns a PublicKey. It rejects a wrong-length
// key and a small-order/undecodable key (the universal-forgery vector that
// stdlib ed25519.Verify leaves open) by routing through the slice-1 guard, so a
// hostile key cannot enter a binding. It never panics.
func NewPublicKey(b []byte) (PublicKey, error) {
	if len(b) != ed25519.PublicKeySize {
		return PublicKey{}, fmt.Errorf("%s", ErrPubKeyBadLen)
	}
	// Route a no-op verify through the slice-1 primitive purely to reuse its
	// small-order/undecodable guard: a wrong-length sig short-circuits before
	// the (cheap) signature check, so the ONLY way to reach (false, non-nil) for
	// a correct-length key is the small-order reject.
	_, err := identity.VerifyCanonical(b, identity.CanonicalBytesFromTrusted(nil), make([]byte, ed25519.SignatureSize))
	if err != nil && err.Error() == identity.ErrVerifySmallOrderPubKey {
		return PublicKey{}, fmt.Errorf("%s", ErrPubKeySmallOrder)
	}
	pk := make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(pk, b)
	return PublicKey{key: pk}, nil
}

// Bytes returns a defensive COPY of the raw 32-byte public key. A copy (not an
// alias to internal storage) preserves the validated-key invariant: a caller
// that mutates the returned slice cannot corrupt the PublicKey held in a
// binding.
func (p PublicKey) Bytes() ed25519.PublicKey {
	out := make(ed25519.PublicKey, len(p.key))
	copy(out, p.key)
	return out
}

// SignedEntry is PROOF that a raw entry passed the Contract-B schema gate, was
// canonicalized, and verified under a validated PublicKey. The ONLY constructor
// is NewSignedEntry, which runs the full gate; a SignedEntry value therefore
// cannot exist for an invalid/forged entry — the invalid state is
// unrepresentable.
type SignedEntry struct {
	pub       PublicKey
	canonical identity.CanonicalBytes
	sig       []byte
}

// NewSignedEntry runs the gate: it verifies sig over the SCHEMA-VALIDATED,
// canonicalized form of rawJSON under pub (via identity.VerifyEntry, which runs
// ValidateSchema + Canonicalize + ZIP-215 strict verify). It returns an error
// when the entry violates the schema, cannot be canonicalized, is structurally
// rejected, or simply does not verify. On success the returned SignedEntry is
// proof the entry is conformant and authentic.
func NewSignedEntry(pub PublicKey, rawJSON []byte, sig []byte) (SignedEntry, error) {
	ok, err := identity.VerifyEntry(pub.key, rawJSON, sig)
	if err != nil {
		return SignedEntry{}, err
	}
	if !ok {
		return SignedEntry{}, fmt.Errorf("%s", errEntrySigMismatch)
	}
	canonical, err := identity.Canonicalize(rawJSON)
	if err != nil {
		return SignedEntry{}, err
	}
	return SignedEntry{pub: pub, canonical: canonical, sig: sig}, nil
}

// Canonical returns the canonical bytes that were signed.
func (s SignedEntry) Canonical() identity.CanonicalBytes { return s.canonical }

// Slug returns the empty string: a SignedEntry is a transport-level proof, not
// a slug-bound binding. The accessor exists so callers can treat any signed
// payload uniformly; binding-level identity lives on AgentBinding.
func (s SignedEntry) Slug() string { return "" }

// errEntrySigMismatch is returned by NewSignedEntry when a well-formed,
// conformant entry's signature simply does not match the public key.
const errEntrySigMismatch = "registry: signed entry signature does not match public key"

// NetworkID derives a stable, keypair-bound network id from a public key. The
// id is SHA-256(pubkey) truncated and hex-encoded behind the reserved
// GlobalPrefix. It is derived from the AUTHENTICATOR (the keypair), never from a
// public git-remote fingerprint, so it rotates with a re-key and does not leak a
// precomputable fingerprint→slug mapping (Door 5).
func NetworkID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return GlobalPrefix + hex.EncodeToString(sum[:networkIDHashBytes])
}

// decodePubKey decodes a base64 (std, padded) ed25519 public key into a
// validated PublicKey. It is the bridge from the wire/JSON form (base64 string)
// to the validated domain type used in the verify path.
func decodePubKey(b64 string) (PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return PublicKey{}, fmt.Errorf("registry: decode pubkey base64: %w", err)
	}
	return NewPublicKey(raw)
}
