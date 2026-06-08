// Package registry implements Contract-B SLICE 3: the trust-root REGISTRY
// itself — a root-signed, epoched, freshness-bounded slug→pubkey binding set
// plus the consumer resolve/verify surface.
//
// It builds STRICTLY on the slice-1 primitives (identity.Canonicalize for JCS,
// identity.VerifyCanonical for the small-order-rejecting ZIP-215 verify, and
// identity.ValidateSchema via the SignedEntry gate) and does not reimplement
// canonicalization or verification.
//
// The trust model (ratified in docs/mesh/contractb-decisions.md):
//
//   - The registry is signed by an OFFLINE root key (dev: an in-process
//     keypair). SignRegistry is the offline-root operation; VerifyAndLoad is the
//     load-bearing consumer trust gate.
//   - ANTI-ROLLBACK: a monotonic epoch; the consumer persists the highest epoch
//     seen and refuses any registry at-or-below it.
//   - FRESHNESS / anti-freeze-eclipse: a registry past valid_until OR older than
//     maxStaleness FAILS CLOSED (a stale registry may hide a revocation, so it
//     must not be trusted).
//   - REVOCATION + ROTATION: a binding carries revoked_at; rotation is a new
//     {pubkey,key_epoch} tuple with the old tuple revoked. The ed25519 pubkey is
//     identity; resolve/verify default-deny anything revoked, expired, or
//     epoch-mismatched.
//
// SLICE-3 scope is the registry MECHANISM only. The agent-chat --registry
// distribution wiring and the WireGuard daemon keys are deferred to slice 4.
package registry

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// Trust-gate reject messages (SSOT — referenced from the verify path, asserted
// by callers/tests, never inlined).
const (
	// ErrRootSig is returned by VerifyAndLoad when root_sig does not verify over
	// the canonical registry bytes (forgery / tamper / wrong root key).
	ErrRootSig = "registry: root signature does not verify"
	// ErrRollback is returned by VerifyAndLoad when reg.epoch <= the persisted
	// highest-seen epoch (anti-rollback).
	ErrRollback = "registry: epoch is at or below the highest seen (anti-rollback)"
	// ErrStale is returned by VerifyAndLoad when the registry is past valid_until
	// or older than maxStaleness (freshness fails CLOSED).
	ErrStale = "registry: registry is stale or expired (fail closed)"
	// ErrUnknownSlug is returned by Resolve/VerifyMessage when no live binding
	// matches the slug.
	ErrUnknownSlug = "registry: no live binding for slug"
	// ErrRevoked is returned when a binding has revoked_at set.
	ErrRevoked = "registry: binding is revoked"
	// ErrExpiredBinding is returned when now is past a binding's valid_until.
	ErrExpiredBinding = "registry: binding is expired"
	// ErrKeyEpochMismatch is returned by VerifyMessage when the caller's
	// key_epoch does not match the resolved binding (default-deny).
	ErrKeyEpochMismatch = "registry: key_epoch does not match the live binding"
	// ErrBadRootKey is returned by SignRegistry for a nil/wrong-length root key.
	ErrBadRootKey = "registry: root private key must be ed25519.PrivateKeySize bytes"
	// ErrEpochRange is returned by SignRegistry/VerifyAndLoad when an epoch
	// (registry epoch or a binding key_epoch) is outside [1, MaxSafeEpoch]. JCS
	// (RFC 8785) renders numbers as IEEE-754 doubles, so an epoch above 2^53
	// silently rounds (cross-language parity break + epoch confusion) and an
	// epoch near MaxInt64 fails JSON unmarshal (DoS); a zero/negative epoch also
	// breaks the monotonic anti-rollback floor.
	ErrEpochRange = "registry: epoch out of range [1, 2^53]"
	// ErrNotYetValid is returned by Resolve/VerifyMessage when now is before a
	// binding's valid_from (a not-yet-active binding must not resolve or verify).
	ErrNotYetValid = "registry: binding is not yet valid"
	// ErrDuplicateBinding is returned by VerifyAndLoad when the registry contains
	// duplicate {slug,key_epoch} tuples, or more than one live (non-revoked)
	// binding for the same slug (shadowing / ambiguous-resolution defense).
	ErrDuplicateBinding = "registry: duplicate binding (slug/key_epoch or multiple live bindings per slug)"
	// errMarshalRegistry wraps a registry JSON-marshal failure.
	errMarshalRegistry = "registry: marshal registry"
	// errUnmarshalRegistry wraps a canonical-registry JSON-unmarshal failure.
	errUnmarshalRegistry = "registry: unmarshal registry"

	// MaxSafeEpoch is the inclusive upper bound for any epoch (registry epoch and
	// binding key_epoch). It is 2^53 — the largest integer JCS (which formats
	// numbers as IEEE-754 doubles) can render without precision loss, so epochs in
	// [1, MaxSafeEpoch] round-trip identically across languages and never DoS the
	// JSON unmarshal path. The floor of 1 gives the monotonic counter a positive
	// base (negative/zero rejected).
	MaxSafeEpoch = 1 << 53
)

// epochInRange reports whether e is within the JCS-safe, anti-rollback-floored
// range [1, MaxSafeEpoch].
func epochInRange(e int) bool { return e >= 1 && e <= MaxSafeEpoch }

// AgentBinding binds a slug to a validated ed25519 public key for one key epoch,
// with a freshness window and an optional revocation timestamp. The ed25519
// pubkey is the identity; the slug is a label.
type AgentBinding struct {
	// Slug is the addressable agent label.
	Slug string
	// DisplayName is the human-facing name (may contain non-ASCII, e.g. "Mira ⭐").
	DisplayName string
	// PubKey is the validated ed25519 public key (small-order-rejected).
	PubKey PublicKey
	// KeyEpoch is the rotation generation of {pubkey,key_epoch}.
	KeyEpoch int
	// ValidFrom / ValidUntil bound the binding's validity (unix seconds).
	ValidFrom  int64
	ValidUntil int64
	// AuthorizedOriginDaemons lists the daemon ids permitted to originate for
	// this slug.
	AuthorizedOriginDaemons []string
	// RevokedAt is the unix-seconds revocation time, or nil if the binding is
	// live. A revoked binding never resolves or verifies.
	RevokedAt *int
}

// Registry is the root-signed binding set with a monotonic epoch and a freshness
// window.
type Registry struct {
	// Epoch is the monotonic anti-rollback counter.
	Epoch int
	// ValidFrom / ValidUntil bound registry freshness (unix seconds).
	ValidFrom  int64
	ValidUntil int64
	// Agents are the bindings.
	Agents []AgentBinding
}

// SignedRegistry is the distribution envelope: the JCS-canonical registry bytes,
// the ed25519 root signature over exactly those bytes, and the root key epoch.
type SignedRegistry struct {
	// Registry is the JCS-canonical encoding of the Registry — the EXACT bytes
	// that root_sig covers.
	Registry []byte
	// RootSig is the ed25519 signature by the offline root key over Registry.
	RootSig []byte
	// RootKeyEpoch identifies which root key signed (for root rotation).
	RootKeyEpoch int
}

// wireBinding is the JSON/JCS shape of a binding. Field names + ordering are the
// SSOT canonical form ratified in docs/mesh/contractb-decisions.md (snake_case,
// pubkey as base64). omitempty on revoked_at means a live binding emits no key
// (nil pointer == "live"), matching the decisions-doc canonical example.
type wireBinding struct {
	AuthorizedOriginDaemons []string `json:"authorized_origin_daemons"`
	DisplayName             string   `json:"display_name"`
	KeyEpoch                int      `json:"key_epoch"`
	PubKey                  string   `json:"pubkey"`
	RevokedAt               *int     `json:"revoked_at,omitempty"`
	Slug                    string   `json:"slug"`
	ValidFrom               int64    `json:"valid_from"`
	ValidUntil              int64    `json:"valid_until"`
}

// wireRegistry is the JSON/JCS shape of the registry. JCS sorts keys, so the Go
// field order here is for readability only; the canonical bytes are
// Canonicalize's output.
type wireRegistry struct {
	Agents     []wireBinding `json:"agents"`
	Epoch      int           `json:"epoch"`
	ValidFrom  int64         `json:"valid_from"`
	ValidUntil int64         `json:"valid_until"`
}

// toWire converts a Registry to its wire shape (base64 pubkeys).
func toWire(reg Registry) wireRegistry {
	agents := make([]wireBinding, len(reg.Agents))
	for i, a := range reg.Agents {
		agents[i] = wireBinding{
			AuthorizedOriginDaemons: a.AuthorizedOriginDaemons,
			DisplayName:             a.DisplayName,
			KeyEpoch:                a.KeyEpoch,
			PubKey:                  base64.StdEncoding.EncodeToString(a.PubKey.Bytes()),
			RevokedAt:               a.RevokedAt,
			Slug:                    a.Slug,
			ValidFrom:               a.ValidFrom,
			ValidUntil:              a.ValidUntil,
		}
	}
	return wireRegistry{
		Agents:     agents,
		Epoch:      reg.Epoch,
		ValidFrom:  reg.ValidFrom,
		ValidUntil: reg.ValidUntil,
	}
}

// fromWire converts a decoded wire registry back to a Registry, re-validating
// every pubkey through NewPublicKey so a small-order/garbage key in the bytes
// cannot enter a binding (an invalid binding is unrepresentable).
func fromWire(w wireRegistry) (Registry, error) {
	agents := make([]AgentBinding, len(w.Agents))
	for i, a := range w.Agents {
		pk, err := decodePubKey(a.PubKey)
		if err != nil {
			return Registry{}, err
		}
		agents[i] = AgentBinding{
			Slug:                    a.Slug,
			DisplayName:             a.DisplayName,
			PubKey:                  pk,
			KeyEpoch:                a.KeyEpoch,
			ValidFrom:               a.ValidFrom,
			ValidUntil:              a.ValidUntil,
			AuthorizedOriginDaemons: a.AuthorizedOriginDaemons,
			RevokedAt:               a.RevokedAt,
		}
	}
	return Registry{
		Epoch:      w.Epoch,
		ValidFrom:  w.ValidFrom,
		ValidUntil: w.ValidUntil,
		Agents:     agents,
	}, nil
}

// canonicalBytes marshals a Registry to its JCS-canonical bytes via the slice-1
// Canonicalize primitive — the EXACT bytes the root signs and the consumer
// verifies.
func canonicalBytes(reg Registry) (identity.CanonicalBytes, error) {
	raw, err := json.Marshal(toWire(reg))
	if err != nil {
		return identity.CanonicalBytes{}, fmt.Errorf("%s: %w", errMarshalRegistry, err)
	}
	return identity.Canonicalize(raw)
}

// SignRegistry is the OFFLINE-ROOT operation: it JCS-canonicalizes reg and
// ed25519-signs the canonical bytes with the root private key, returning the
// distribution envelope. It reuses the slice-1 SignCanonical primitive and
// returns an error (never panics) on a nil/wrong-length root key.
func SignRegistry(rootPriv ed25519.PrivateKey, reg Registry) (SignedRegistry, error) {
	if len(rootPriv) != ed25519.PrivateKeySize {
		return SignedRegistry{}, fmt.Errorf("%s", ErrBadRootKey)
	}
	// Refuse to sign an out-of-range epoch (registry or any binding key_epoch):
	// these are JCS-unsafe / break the anti-rollback floor, so they must never be
	// minted in the first place.
	if !epochInRange(reg.Epoch) {
		return SignedRegistry{}, fmt.Errorf("%s", ErrEpochRange)
	}
	for _, a := range reg.Agents {
		if !epochInRange(a.KeyEpoch) {
			return SignedRegistry{}, fmt.Errorf("%s", ErrEpochRange)
		}
	}
	canonical, err := canonicalBytes(reg)
	if err != nil {
		return SignedRegistry{}, err
	}
	sig, err := identity.SignCanonical(rootPriv, canonical)
	if err != nil {
		return SignedRegistry{}, err
	}
	return SignedRegistry{
		Registry:     canonical.Bytes(),
		RootSig:      sig,
		RootKeyEpoch: 0,
	}, nil
}

// VerifyAndLoad is the load-bearing consumer trust gate. It FAILS CLOSED on
// every check, in order:
//
//  1. ROOT SIG: verify root_sig over the canonical registry bytes via
//     identity.VerifyCanonical (which rejects a small-order/non-canonical root
//     key and a non-canonical signature). A bad sig is rejected.
//  2. EPOCH RANGE: reject if reg.epoch or any binding key_epoch is outside
//     [1, MaxSafeEpoch] (JCS-safe + anti-rollback floor).
//  3. UNIQUENESS: reject duplicate {slug,key_epoch} tuples or more than one live
//     binding per slug.
//  4. ANTI-ROLLBACK: reject if reg.epoch <= persistedHighestEpoch.
//  5. FRESHNESS: reject (fail closed) if now is before reg.valid_from, past
//     reg.valid_until, OR (now - reg.valid_from) > maxStaleness.
//
// Boundary convention: valid_until and maxStaleness are INCLUSIVE — a registry
// is honored AT exactly now == valid_until and AT exactly staleness ==
// maxStaleness, and rejected one tick past either. valid_from is inclusive too
// (honored AT now == valid_from, rejected before it).
//
// On success it returns the verified Registry and newHighestEpoch =
// max(persistedHighestEpoch, reg.epoch) for the caller to persist.
func VerifyAndLoad(
	pinnedRootPub ed25519.PublicKey,
	env SignedRegistry,
	persistedHighestEpoch int,
	now time.Time,
	maxStaleness time.Duration,
) (Registry, int, error) {
	// 1. Root signature over the canonical bytes (small-order root key rejected
	// inside VerifyCanonical). Any structural error or a non-match is a reject.
	ok, err := identity.VerifyCanonical(
		pinnedRootPub,
		identity.CanonicalBytesFromTrusted(env.Registry),
		env.RootSig,
	)
	if err != nil {
		return Registry{}, persistedHighestEpoch, fmt.Errorf("%s: %w", ErrRootSig, err)
	}
	if !ok {
		return Registry{}, persistedHighestEpoch, fmt.Errorf("%s", ErrRootSig)
	}

	// Decode the now-authenticated bytes. Pubkeys are re-validated in fromWire.
	var w wireRegistry
	if err := json.Unmarshal(env.Registry, &w); err != nil {
		return Registry{}, persistedHighestEpoch, fmt.Errorf("%s: %w", errUnmarshalRegistry, err)
	}
	reg, err := fromWire(w)
	if err != nil {
		return Registry{}, persistedHighestEpoch, err
	}

	// 2. Epoch range — reject a JCS-unsafe / non-positive epoch (registry epoch
	// and every binding key_epoch). An out-of-range epoch in the authenticated
	// body means epoch confusion or a broken monotonic floor; fail closed.
	if !epochInRange(reg.Epoch) {
		return Registry{}, persistedHighestEpoch, fmt.Errorf("%s", ErrEpochRange)
	}
	for _, b := range reg.Agents {
		if !epochInRange(b.KeyEpoch) {
			return Registry{}, persistedHighestEpoch, fmt.Errorf("%s", ErrEpochRange)
		}
	}

	// 3. Uniqueness — reject duplicate {slug,key_epoch} tuples and more than one
	// live (non-revoked) binding per slug, so a shadow binding cannot mask the
	// intended one.
	if err := checkBindingUniqueness(reg.Agents); err != nil {
		return Registry{}, persistedHighestEpoch, err
	}

	// 4. Anti-rollback.
	if reg.Epoch <= persistedHighestEpoch {
		return Registry{}, persistedHighestEpoch, fmt.Errorf("%s", ErrRollback)
	}

	// 5. Freshness — fail closed. A stale registry may hide a revocation. A
	// registry whose valid_from is in the FUTURE is NOT fresh, it is suspicious:
	// without this guard a future valid_from yields a negative staleness that
	// always satisfies maxStaleness (perpetual freshness; defeats
	// revocation-withholding limits).
	if now.Unix() < reg.ValidFrom {
		return Registry{}, persistedHighestEpoch, fmt.Errorf("%s", ErrStale)
	}
	if now.Unix() > reg.ValidUntil {
		return Registry{}, persistedHighestEpoch, fmt.Errorf("%s", ErrStale)
	}
	if now.Sub(time.Unix(reg.ValidFrom, 0)) > maxStaleness {
		return Registry{}, persistedHighestEpoch, fmt.Errorf("%s", ErrStale)
	}

	newHighest := persistedHighestEpoch
	if reg.Epoch > newHighest {
		newHighest = reg.Epoch
	}
	return reg, newHighest, nil
}

// Resolve returns the LIVE binding for slug at now. It default-denies: a binding
// with revoked_at set, one whose valid_until has passed, or one whose valid_from
// is still in the future (not yet active), is skipped. When the slug exists only
// in a revoked form, ErrRevoked is returned so the reason is distinguishable
// from ErrUnknownSlug; otherwise an unknown/no-live-binding slug is rejected.
// When multiple bindings share a slug (rotation), the first live, in-window one
// is returned.
func Resolve(reg Registry, slug string, now time.Time) (AgentBinding, error) {
	sawRevoked := false
	for _, b := range reg.Agents {
		if b.Slug != slug {
			continue
		}
		if b.RevokedAt != nil {
			sawRevoked = true
			continue
		}
		if now.Unix() < b.ValidFrom {
			continue
		}
		if now.Unix() > b.ValidUntil {
			continue
		}
		return b, nil
	}
	// Distinguish "the slug exists but every binding for it is revoked" from
	// "no such slug" so callers can alert specifically on revocation probing.
	if sawRevoked {
		return AgentBinding{}, fmt.Errorf("%s: %q", ErrRevoked, slug)
	}
	return AgentBinding{}, fmt.Errorf("%s: %q", ErrUnknownSlug, slug)
}

// checkBindingUniqueness rejects a binding set that contains duplicate
// {slug,key_epoch} tuples or more than one live (non-revoked) binding for the
// same slug. Either condition lets a shadow binding mask the intended one, so a
// conforming registry must not ship them.
func checkBindingUniqueness(agents []AgentBinding) error {
	type tuple struct {
		slug  string
		epoch int
	}
	seenTuple := make(map[tuple]struct{}, len(agents))
	liveSlug := make(map[string]struct{}, len(agents))
	for _, b := range agents {
		tk := tuple{slug: b.Slug, epoch: b.KeyEpoch}
		if _, dup := seenTuple[tk]; dup {
			return fmt.Errorf("%s", ErrDuplicateBinding)
		}
		seenTuple[tk] = struct{}{}
		if b.RevokedAt == nil {
			if _, dup := liveSlug[b.Slug]; dup {
				return fmt.Errorf("%s", ErrDuplicateBinding)
			}
			liveSlug[b.Slug] = struct{}{}
		}
	}
	return nil
}

// VerifyMessage resolves the live binding for slug, requires keyEpoch to match
// the binding's key_epoch (default-deny on mismatch), then verifies sig over the
// canonical bytes under the binding's validated pubkey via the slice-1
// VerifyCanonical. A keyed slug with a revoked/expired/epoch-mismatched binding
// is rejected. An honest non-match returns (false, nil); a structural problem
// returns (false, error).
func VerifyMessage(
	reg Registry,
	slug string,
	keyEpoch int,
	canonical identity.CanonicalBytes,
	sig []byte,
	now time.Time,
) (bool, error) {
	b, err := resolveTuple(reg, slug, keyEpoch, now)
	if err != nil {
		return false, err
	}
	return identity.VerifyCanonical(b.PubKey.Bytes(), canonical, sig)
}

// resolveTuple resolves the live binding for the EXACT {slug,key_epoch} tuple at
// now, default-denying a revoked, expired, or epoch-mismatched binding. It is
// the shared default-deny path behind VerifyMessage.
func resolveTuple(reg Registry, slug string, keyEpoch int, now time.Time) (AgentBinding, error) {
	for _, b := range reg.Agents {
		if b.Slug != slug || b.KeyEpoch != keyEpoch {
			continue
		}
		if b.RevokedAt != nil {
			return AgentBinding{}, fmt.Errorf("%s: %q", ErrRevoked, slug)
		}
		if now.Unix() < b.ValidFrom {
			return AgentBinding{}, fmt.Errorf("%s: %q", ErrNotYetValid, slug)
		}
		if now.Unix() > b.ValidUntil {
			return AgentBinding{}, fmt.Errorf("%s: %q", ErrExpiredBinding, slug)
		}
		return b, nil
	}
	// No tuple matched. Distinguish "slug exists but epoch differs" (mismatch)
	// from "no such slug" for a clearer default-deny signal.
	for _, b := range reg.Agents {
		if b.Slug == slug {
			return AgentBinding{}, fmt.Errorf("%s: %q", ErrKeyEpochMismatch, slug)
		}
	}
	return AgentBinding{}, fmt.Errorf("%s: %q", ErrUnknownSlug, slug)
}
