// Package registry implements Contract-B SLICE 3: the trust-root REGISTRY
// itself — a root-signed, epoched, freshness-bounded slug→pubkey binding set
// plus the consumer resolve/verify surface.
//
// It builds STRICTLY on the slice-1 primitives (identity.Canonicalize for JCS,
// identity.VerifyCanonical for the small-order-rejecting cofactorless strict verify, and
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
	//
	// TODO(contract-b hardening): defense-in-depth — consider MaxSafeEpoch = (1<<53)-1.
	// A compromised root could hand-craft a body with epoch 2^53+1, which JCS rounds
	// to 2^53 (in-range at this ceiling). Unreachable via the legitimate SignRegistry
	// mint gate (it rejects >2^53 pre-canonicalization), so this is belt-and-suspenders
	// against an already-compromised root, not a real attacker capability. With the
	// ceiling at 2^53-1, a 2^53+1 body rounds to 2^53 which is then above-ceiling and
	// rejected at load too. Alternative: a round-trip re-canonicalization equality
	// check in VerifyAndLoad (closes the whole JCS-normalized-field-smuggling class).
	MaxSafeEpoch = 1 << 53
)

// epochInRange reports whether e is within the JCS-safe, anti-rollback-floored
// range [1, MaxSafeEpoch].
func epochInRange(e int) bool { return e >= 1 && e <= MaxSafeEpoch }

// ClassAgent is the DEFAULT effective class of a binding whose Class is empty.
// An empty Class means "agent" so existing bindings (which set neither Class nor
// VouchAllowlist) authorize only the plain agent path; bridge/human authority
// must be explicitly GRANTED by setting Class. This is the SSOT for the default.
const ClassAgent = "agent"

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
	// Roles are the root-granted capabilities of this binding (bridge,
	// code_signing, god). Nil and empty both mean the binding has no roles, and
	// the canonical form then omits the roles key so a roles-free registry stays
	// byte-identical to one signed before the field existed. Order is
	// significant and is preserved. god is allowed only on a human binding;
	// bridge and code_signing only on an agent binding.
	Roles []Role
	// Custody is who holds the binding's private key. Empty and "shared" are the
	// default and are omitted on the wire. "isolated" is carried through.
	// Anything else is refused. Only the root sets it, by signing the registry.
	Custody string
	// Class is the authority class of this binding. An empty Class means agent
	// (see ClassAgent / EffectiveClass). A binding loaded from humans[] has
	// Class human; the class is not a field on the binding's wire object — the
	// array it was loaded from is the source of truth. VouchAllowlist is a
	// separate, in-memory authority channel and is not on the registry wire.
	Class string
	// VouchAllowlist is the set of principals (e.g. "human:siavoush") a bridge
	// binding may vouch for. An empty allowlist authorizes NO vouch (fail closed).
	VouchAllowlist []string
}

// EffectiveClass returns the binding's authority class, defaulting an empty
// Class to ClassAgent. It is the SSOT for the empty-Class-means-agent rule.
func (b AgentBinding) EffectiveClass() string {
	if b.Class == "" {
		return ClassAgent
	}
	return b.Class
}

// Registry is the root-signed binding set with a monotonic epoch and a freshness
// window.
type Registry struct {
	// Epoch is the monotonic anti-rollback counter.
	Epoch int
	// ValidFrom / ValidUntil bound registry freshness (unix seconds).
	ValidFrom  int64
	ValidUntil int64
	// Agents are the agent bindings. A human is not an agent: people live in Humans.
	Agents []AgentBinding
	// Humans are the person bindings. Nil and empty both mean no people, and the
	// canonical form then omits the humans key so an existing registry stays
	// byte-identical. A human binding's Class is human.
	Humans []AgentBinding
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
	AuthorizedOriginDaemons []string        `json:"authorized_origin_daemons"`
	Custody                 string          `json:"custody,omitempty"`
	DisplayName             string          `json:"display_name"`
	KeyEpoch                int             `json:"key_epoch"`
	PubKey                  string          `json:"pubkey"`
	RevokedAt               *int            `json:"revoked_at,omitempty"`
	Roles                   json.RawMessage `json:"roles,omitempty"`
	Slug                    string          `json:"slug"`
	ValidFrom               int64           `json:"valid_from"`
	ValidUntil              int64           `json:"valid_until"`
}

// wireRegistry is the JSON/JCS shape of the registry. JCS sorts keys, so the Go
// field order here is for readability only; the canonical bytes are
// Canonicalize's output.
type wireRegistry struct {
	Agents []wireBinding `json:"agents"`
	// Humans is omitted when empty so a people-free registry stays byte-identical.
	Humans     []wireBinding `json:"humans,omitempty"`
	Epoch      int           `json:"epoch"`
	ValidFrom  int64         `json:"valid_from"`
	ValidUntil int64         `json:"valid_until"`
}

// toWire converts a Registry to its wire shape (base64 pubkeys). roles is
// omitted when empty (see MarshalRoles). humans is omitted when empty. custody
// is omitted when it is the shared default. A role that is not one of the
// three variants fails closed, as does a custody value other than shared or
// isolated.
func toWire(reg Registry) (wireRegistry, error) {
	agents, err := bindingsToWire(reg.Agents)
	if err != nil {
		return wireRegistry{}, err
	}
	humans, err := bindingsToWire(reg.Humans)
	if err != nil {
		return wireRegistry{}, err
	}
	return wireRegistry{
		Agents:     agents,
		Humans:     humans,
		Epoch:      reg.Epoch,
		ValidFrom:  reg.ValidFrom,
		ValidUntil: reg.ValidUntil,
	}, nil
}

// bindingsToWire encodes bindings. An empty input returns nil so the humans
// array is omitted. A bad custody value or a malformed role fails closed.
func bindingsToWire(in []AgentBinding) ([]wireBinding, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]wireBinding, len(in))
	for i, a := range in {
		roles, err := MarshalRoles(a.Roles)
		if err != nil {
			return nil, err
		}
		custody, err := NormalizeCustody(a.Slug, a.Custody)
		if err != nil {
			return nil, err
		}
		out[i] = wireBinding{
			AuthorizedOriginDaemons: a.AuthorizedOriginDaemons,
			Custody:                 custody,
			DisplayName:             a.DisplayName,
			KeyEpoch:                a.KeyEpoch,
			PubKey:                  base64.StdEncoding.EncodeToString(a.PubKey.Bytes()),
			RevokedAt:               a.RevokedAt,
			Roles:                   roles,
			Slug:                    a.Slug,
			ValidFrom:               a.ValidFrom,
			ValidUntil:              a.ValidUntil,
		}
	}
	return out, nil
}

// fromWire converts a decoded wire registry back to a Registry, re-validating
// every pubkey through NewPublicKey so a small-order/garbage key in the bytes
// cannot enter a binding (an invalid binding is unrepresentable). Bindings in
// humans[] are marked Class human. A bad custody value fails closed and names
// the slug.
func fromWire(w wireRegistry) (Registry, error) {
	agents, err := bindingsFromWire(w.Agents, "")
	if err != nil {
		return Registry{}, err
	}
	humans, err := bindingsFromWire(w.Humans, ClassHuman)
	if err != nil {
		return Registry{}, err
	}
	return Registry{
		Epoch:      w.Epoch,
		ValidFrom:  w.ValidFrom,
		ValidUntil: w.ValidUntil,
		Agents:     agents,
		Humans:     humans,
	}, nil
}

func bindingsFromWire(in []wireBinding, class string) ([]AgentBinding, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]AgentBinding, len(in))
	for i, a := range in {
		pk, err := decodePubKey(a.PubKey)
		if err != nil {
			return nil, fmt.Errorf("registry: binding %q: %w", a.Slug, err)
		}
		roles, err := ParseRoles(a.Roles)
		if err != nil {
			return nil, fmt.Errorf("registry: binding %q: %w", a.Slug, err)
		}
		custody, err := NormalizeCustody(a.Slug, a.Custody)
		if err != nil {
			return nil, err
		}
		out[i] = AgentBinding{
			Slug:                    a.Slug,
			DisplayName:             a.DisplayName,
			PubKey:                  pk,
			KeyEpoch:                a.KeyEpoch,
			ValidFrom:               a.ValidFrom,
			ValidUntil:              a.ValidUntil,
			AuthorizedOriginDaemons: a.AuthorizedOriginDaemons,
			RevokedAt:               a.RevokedAt,
			Roles:                   roles,
			Custody:                 custody,
			Class:                   class,
		}
	}
	return out, nil
}

// canonicalBytes marshals a Registry to its JCS-canonical bytes via the slice-1
// Canonicalize primitive — the EXACT bytes the root signs and the consumer
// verifies.
func canonicalBytes(reg Registry) (identity.CanonicalBytes, error) {
	wire, err := toWire(reg)
	if err != nil {
		return identity.CanonicalBytes{}, err
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return identity.CanonicalBytes{}, fmt.Errorf("%s: %w", errMarshalRegistry, err)
	}
	return identity.Canonicalize(raw)
}

// RegistrySigner signs JCS-canonical registry bytes via the keyless custody
// signer. The root private key stays in the signer process; the caller (the
// CLI) never loads it. The real implementation is *signer.Client; tests inject a
// fake to prove the sign path never opens the key file.
type RegistrySigner interface {
	Sign(canonicalBytes []byte) ([]byte, error)
}

// canonicalizeForSign runs the shared pre-sign gate for BOTH SignRegistry and
// SignRegistryWithSigner: it refuses an out-of-range epoch (registry or any
// binding key_epoch) — JCS-unsafe / anti-rollback-floor violations that must
// never be minted — then JCS-canonicalizes reg to the EXACT bytes the root
// signs. The signing seam (raw key vs custody signer) is the ONLY thing that
// differs between the two callers, so the canonical bytes are byte-identical.
func canonicalizeForSign(reg Registry) (identity.CanonicalBytes, error) {
	if !epochInRange(reg.Epoch) {
		return identity.CanonicalBytes{}, fmt.Errorf("%s", ErrEpochRange)
	}
	if err := epochsInRange(reg.Agents); err != nil {
		return identity.CanonicalBytes{}, err
	}
	if err := epochsInRange(reg.Humans); err != nil {
		return identity.CanonicalBytes{}, err
	}
	// Role placement, custody, and slug uniqueness are refused here so a root
	// signature is never minted over a registry VerifyAndLoad would reject.
	if err := validateRegistryBindings(reg); err != nil {
		return identity.CanonicalBytes{}, err
	}
	canonical, err := canonicalBytes(reg)
	if err != nil {
		return identity.CanonicalBytes{}, err
	}
	// One document walk covers every string the root is about to sign
	// (display name, slug, daemons, vouches_for, platforms). Canonical bytes
	// have no duplicate keys; the unsigned sign-registry input is checked
	// before decode, where a duplicated key is still visible.
	if err := identity.ValidateRegistrySchema(canonical.Bytes()); err != nil {
		return identity.CanonicalBytes{}, err
	}
	return canonical, nil
}

// SignRegistry is the OFFLINE-ROOT operation: it JCS-canonicalizes reg and
// ed25519-signs the canonical bytes with the root private key, returning the
// distribution envelope. It reuses the slice-1 SignCanonical primitive and
// returns an error (never panics) on a nil/wrong-length root key.
func SignRegistry(rootPriv ed25519.PrivateKey, reg Registry) (SignedRegistry, error) {
	if len(rootPriv) != ed25519.PrivateKeySize {
		return SignedRegistry{}, fmt.Errorf("%s", ErrBadRootKey)
	}
	canonical, err := canonicalizeForSign(reg)
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

// SignRegistryWithSigner is the KEYLESS parallel of SignRegistry: it runs the
// SAME epoch gate + JCS-canonicalization, then signs the canonical bytes via the
// custody signer (the root private key never enters this process) instead of a
// raw key. The result is byte-identical to SignRegistry over the same registry —
// only the signing seam differs. It FAILS CLOSED: any gate, canonicalize, or
// signer error returns a non-nil error and a zero SignedRegistry.
func SignRegistryWithSigner(client RegistrySigner, reg Registry) (SignedRegistry, error) {
	canonical, err := canonicalizeForSign(reg)
	if err != nil {
		return SignedRegistry{}, err
	}
	sig, err := client.Sign(canonical.Bytes())
	if err != nil {
		return SignedRegistry{}, fmt.Errorf("registry: sign via signer: %w", err)
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
//     [1, MaxSafeEpoch] (JCS-safe + anti-rollback floor). Agent and human
//     bindings are both checked.
//  3. BINDINGS: reject duplicate {slug,key_epoch} tuples or more than one live
//     binding per slug, in agents and in humans; the same slug in both arrays;
//     a bad custody value; god on an agent; bridge or code_signing on a human;
//     and more than one binding that holds god.
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

	// Decode the now-authenticated bytes. Pubkeys are re-validated in fromWire,
	// and roles is parsed there. A malformed role rejects the registry. Any
	// other unknown field in the signed bytes stays ignored, as it was before
	// roles existed. A non-NFC string anywhere in those bytes is then refused.
	reg, err := decodeAuthenticatedRegistry(env.Registry)
	if err != nil {
		return Registry{}, persistedHighestEpoch, err
	}

	// 2. Epoch range — reject a JCS-unsafe / non-positive epoch (registry epoch
	// and every binding key_epoch). An out-of-range epoch in the authenticated
	// body means epoch confusion or a broken monotonic floor; fail closed.
	if !epochInRange(reg.Epoch) {
		return Registry{}, persistedHighestEpoch, fmt.Errorf("%s", ErrEpochRange)
	}
	if err := epochsInRange(reg.Agents); err != nil {
		return Registry{}, persistedHighestEpoch, err
	}
	if err := epochsInRange(reg.Humans); err != nil {
		return Registry{}, persistedHighestEpoch, err
	}

	// 3. Bindings — uniqueness inside each array, slug disjointness across
	// them, custody, and where god / bridge / code_signing may sit. A shadow
	// binding or a second god cannot load.
	if err := validateRegistryBindings(reg); err != nil {
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

// decodeAuthenticatedRegistry decodes a registry whose root signature has
// already been checked. Role shape is parsed first, so a malformed role still
// returns ErrMalformedRole. The NFC walk then covers every string in the raw
// JSON, including a field the typed decode ignores. It uses
// identity.ValidateDocumentNFC, which merges duplicate keys and does not reject
// them: Rust's verifier does the same through serde_json::Value.
func decodeAuthenticatedRegistry(raw []byte) (Registry, error) {
	var w wireRegistry
	if err := json.Unmarshal(raw, &w); err != nil {
		return Registry{}, fmt.Errorf("%s: %w", errUnmarshalRegistry, err)
	}
	reg, err := fromWire(w)
	if err != nil {
		return Registry{}, err
	}
	if err := identity.ValidateDocumentNFC(raw); err != nil {
		return Registry{}, err
	}
	return reg, nil
}

// Resolve returns the LIVE binding for slug at now. It default-denies: a binding
// with revoked_at set, one whose valid_until has passed, or one whose valid_from
// is still in the future (not yet active), is skipped. When the slug exists only
// in a revoked form, ErrRevoked is returned so the reason is distinguishable
// from ErrUnknownSlug; otherwise an unknown/no-live-binding slug is rejected.
// When multiple bindings share a slug (rotation), the first live, in-window one
// is returned.
func Resolve(reg Registry, slug string, now time.Time) (AgentBinding, error) {
	// Agents only. A human slug is not an agent slug, even when the text matches.
	return resolveSlug(reg.Agents, slug, now)
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
			return fmt.Errorf("%s: %q", ErrDuplicateBinding, b.Slug)
		}
		seenTuple[tk] = struct{}{}
		if b.RevokedAt == nil {
			if _, dup := liveSlug[b.Slug]; dup {
				return fmt.Errorf("%s: %q", ErrDuplicateBinding, b.Slug)
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

// ResolveTuple exposes the shared {slug,key_epoch} default-deny resolution for
// callers (the envelope verify layer) that need the RESOLVED binding to enforce
// class/vouch AUTHORITY after a successful VerifyMessage. It applies the SAME
// revoked/expired/not-yet-valid/epoch-mismatch default-deny as VerifyMessage, so
// a binding ResolveTuple returns is exactly the one VerifyMessage verified under.
func ResolveTuple(reg Registry, slug string, keyEpoch int, now time.Time) (AgentBinding, error) {
	return resolveTuple(reg, slug, keyEpoch, now)
}

// resolveTuple resolves the live agent binding for the EXACT {slug,key_epoch}
// tuple at now. It does not look at humans. resolveTupleIn is the shared
// default-deny scan.
func resolveTuple(reg Registry, slug string, keyEpoch int, now time.Time) (AgentBinding, error) {
	return resolveTupleIn(reg.Agents, slug, keyEpoch, now)
}
