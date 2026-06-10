package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/enrollment"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// enroll-add output-label / guidance constants (SSOT) — every printed string the
// admin enroll-add flow emits is a named constant here, never an inline literal.
const (
	// defaultEnrollAddValiditySeconds is the default registry+binding issuance
	// window when --validity-seconds is empty: one year.
	defaultEnrollAddValiditySeconds = int64(31_536_000)

	// enrollAddFirstEpoch is the epoch a FRESH (no --registry) registry emits on
	// its first binding: a fresh registry starts at epoch 0, and the first emit
	// bumps it to 1.
	enrollAddFreshEpoch = 0

	// EnrollAddAddedLabel prefixes the added binding's slug.
	EnrollAddAddedLabel = "added: "
	// EnrollAddPubKeyLabel prefixes the added binding's pubkey.
	EnrollAddPubKeyLabel = "pubkey: "
	// EnrollAddKeyEpochLabel prefixes the added binding's key_epoch.
	EnrollAddKeyEpochLabel = "key_epoch: "
	// EnrollAddEpochLabel prefixes the new registry epoch.
	EnrollAddEpochLabel = "new registry epoch: "
	// EnrollAddWindowLabel prefixes the issuance window (valid_from..valid_until).
	EnrollAddWindowLabel = "issuance window: "
	// EnrollAddNextStepNote tells the admin to root-sign the emitted registry.
	EnrollAddNextStepNote = "NEXT: run the ROOT signer and pipe this to `vaultmind identity sign-registry`."
	// EnrollAddTransportNote surfaces that transport fields are not yet carried
	// into the binding (WireGuard wiring is a later slice).
	EnrollAddTransportNote = "NOTE: transport_pubkey/transport_endpoint are NOT yet carried into the binding (WireGuard = later slice)."
)

// enroll-add error strings (SSOT). Each failure mode is a distinct typed reject.
const (
	// ErrEnrollAddEmptyRequest is returned when the request input is empty.
	ErrEnrollAddEmptyRequest = "identitycli: enrollment request is empty"
	// errEnrollAddReadRequest wraps a request-read failure.
	errEnrollAddReadRequest = "identitycli: read enrollment request"
	// errEnrollAddParseRequest wraps an UnmarshalWire failure (bad/strict JSON).
	errEnrollAddParseRequest = "identitycli: parse enrollment request"
	// errEnrollAddDecodeSig wraps a base64 decode failure of the request sig.
	errEnrollAddDecodeSig = "identitycli: decode enrollment request signature"
	// ErrEnrollAddPoP is returned when proof-of-possession fails: the request sig
	// does not verify under its own pubkey.
	ErrEnrollAddPoP = "identitycli: proof-of-possession failed: the request signature does not verify under its pubkey"
	// errEnrollAddBadRootPubKey wraps a bad --root-pubkey (base64 / invalid key).
	errEnrollAddBadRootPubKey = "identitycli: --root-pubkey must be base64-std of an ed25519 public key"
	// ErrEnrollAddNoNetwork is returned when neither --root-pubkey nor
	// --network-id is supplied (cannot resolve the admin network).
	ErrEnrollAddNoNetwork = "identitycli: at least one of --root-pubkey or --network-id is required to resolve the admin network"
	// ErrEnrollAddNetworkSpecifierDisagree is returned when --root-pubkey-derived
	// network and --network-id are both supplied but disagree.
	ErrEnrollAddNetworkSpecifierDisagree = "identitycli: --root-pubkey-derived network and --network-id disagree"
	// ErrEnrollAddCrossNetwork is returned when the request's network_id is not
	// the admin network (cross-network request refused).
	ErrEnrollAddCrossNetwork = "identitycli: cross-network request refused"
	// errEnrollAddBadValidity wraps a non-numeric --validity-seconds.
	errEnrollAddBadValidity = "identitycli: --validity-seconds must be an integer"
	// ErrEnrollAddSignedNeedsRootPubKey is returned when a signed-envelope
	// --registry is supplied without --root-pubkey (cannot integrity-verify).
	ErrEnrollAddSignedNeedsRootPubKey = "identitycli: a signed-envelope --registry requires --root-pubkey to integrity-verify it"
	// ErrEnrollAddUntrustedRegistry is returned when a signed-envelope --registry
	// root signature does not verify (refusing to mutate untrusted state).
	ErrEnrollAddUntrustedRegistry = "identitycli: current registry root signature invalid — refusing to mutate untrusted state"
	// ErrEnrollAddUnrecognizedRegistry is returned when the --registry input is
	// neither a signed envelope nor an unsigned wireRegistry.
	ErrEnrollAddUnrecognizedRegistry = "identitycli: unrecognized registry shape (expected an unsigned wireRegistry or a signed distribution envelope)"
	// ErrEnrollAddLiveSlugExists is returned when the slug already has a live
	// binding (rotation is out of scope for v1; revoke before re-adding).
	ErrEnrollAddLiveSlugExists = "identitycli: slug already has a live binding; revoke before re-adding"
	// ErrEnrollAddTupleExists is returned when the {slug,key_epoch} tuple already
	// exists in the current registry.
	ErrEnrollAddTupleExists = "identitycli: a binding with this {slug,key_epoch} already exists"
	// ErrEnrollAddEpochOverflow is returned when bumping the epoch would exceed
	// the JCS-safe ceiling (MaxSafeEpoch).
	ErrEnrollAddEpochOverflow = "identitycli: new registry epoch exceeds the JCS-safe ceiling"
	// errEnrollAddNotSignable wraps the final guarantee check: the emitted
	// registry must round-trip buildRegistry+canonicalize.
	errEnrollAddNotSignable = "identitycli: emitted registry failed the signable round-trip"
)

// EnrollAddConfig carries everything the admin enroll-add flow needs. The
// registry input is passed as raw bytes (the cmd layer reads --registry from a
// file or leaves it nil for a fresh registry), and the request is read from the
// supplied reader (the cmd resolves file-vs-stdin). The clock is injectable for
// deterministic timestamps in tests.
type EnrollAddConfig struct {
	// RootPubKeyB64 is the base64-std root ed25519 pubkey. REQUIRED to
	// integrity-verify a signed-envelope --registry; also used to derive the
	// admin network id.
	RootPubKeyB64 string
	// NetworkID is an alternative admin-network specifier (vmnet1:…). At least one
	// of RootPubKeyB64 / NetworkID is required; when both are set they must agree.
	NetworkID string
	// RegistryInput is the current registry: an unsigned wireRegistry JSON OR a
	// signed distribution envelope (auto-detected). nil/empty => fresh registry.
	RegistryInput []byte
	// ValiditySeconds is the registry+binding issuance window in seconds (string
	// so the flag layer can stay on a plain string flag). Empty => one year.
	ValiditySeconds string
	// OriginDaemons is the comma-separated AuthorizedOriginDaemons list (parsed
	// here). Empty => no authorized origin daemons.
	OriginDaemons string

	// Now is the timestamp source (nil => time.Now). Injected for deterministic
	// issuance windows in tests.
	Now func() time.Time
}

func (c EnrollAddConfig) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// EnrollAdd is the Contract-B ADMIN counterpart to `identity enroll`. It reads a
// member's signed enrollment-request wire JSON, verifies proof-of-possession,
// checks the request is for the admin's OWN network, integrity-verifies the
// current registry (when signed), pre-checks uniqueness FAIL-CLOSED, appends the
// new binding with a bumped epoch + refreshed issuance window, and emits the
// UPDATED UNSIGNED registry (the input `sign-registry` root-signs). It does NOT
// sign — that is the established two-verb split.
//
// It FAILS CLOSED: any read/parse/PoP/network/integrity/uniqueness/epoch error
// returns non-nil and emits NOTHING (never a partial registry).
func EnrollAdd(out, errOut io.Writer, in io.Reader, cfg EnrollAddConfig) error {
	fields, err := readAndVerifyRequest(in)
	if err != nil {
		return err
	}

	adminNet, rootPub, err := resolveAdminNetwork(cfg)
	if err != nil {
		return err
	}
	if fields.NetworkID != adminNet {
		return fmt.Errorf("%s: req %q != network %q", ErrEnrollAddCrossNetwork, fields.NetworkID, adminNet)
	}

	validity, err := parseValiditySeconds(cfg.ValiditySeconds)
	if err != nil {
		return err
	}

	reg, err := loadCurrentRegistry(cfg.RegistryInput, rootPub)
	if err != nil {
		return err
	}

	if err := checkUniqueness(reg, fields.Slug, int(fields.KeyEpoch)); err != nil {
		return err
	}

	binding, err := buildNewBinding(fields, cfg, validity)
	if err != nil {
		return err
	}

	updated, err := appendAndBump(reg, binding, validity, cfg.now().Unix())
	if err != nil {
		return err
	}

	emitted, err := serializeSignable(updated)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(out, "%s\n", emitted); err != nil {
		return err
	}
	return writeEnrollAddGuidance(errOut, binding, updated.Epoch)
}

// readAndVerifyRequest reads the request bytes, strictly decodes them, and
// enforces proof-of-possession (the sig verifies under the request's own
// pubkey). A gate failure returns a non-nil error; an honest sig non-match
// returns ErrEnrollAddPoP.
func readAndVerifyRequest(in io.Reader) (enrollment.Fields, error) {
	reqBytes, err := io.ReadAll(in)
	if err != nil {
		return enrollment.Fields{}, fmt.Errorf("%s: %w", errEnrollAddReadRequest, err)
	}
	if len(bytes.TrimSpace(reqBytes)) == 0 {
		return enrollment.Fields{}, fmt.Errorf("%s", ErrEnrollAddEmptyRequest)
	}
	fields, sigB64, err := enrollment.UnmarshalWire(reqBytes)
	if err != nil {
		return enrollment.Fields{}, fmt.Errorf("%s: %w", errEnrollAddParseRequest, err)
	}
	sigRaw, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return enrollment.Fields{}, fmt.Errorf("%s: %w", errEnrollAddDecodeSig, err)
	}
	ok, err := enrollment.VerifyEnrollment(fields, sigRaw)
	if err != nil {
		// A pre-sign gate failure or malformed signature — re-surface the typed
		// gate error so the admin sees exactly which rule the request broke.
		return enrollment.Fields{}, err
	}
	if !ok {
		return enrollment.Fields{}, fmt.Errorf("%s", ErrEnrollAddPoP)
	}
	return fields, nil
}

// resolveAdminNetwork derives the admin network id from --root-pubkey and/or
// --network-id. It requires at least one specifier, decodes+validates the root
// pubkey when present, and requires the two specifiers to AGREE when both are
// supplied. It returns the admin network id and the decoded root pubkey (nil
// when only --network-id is given).
func resolveAdminNetwork(cfg EnrollAddConfig) (string, ed25519.PublicKey, error) {
	if cfg.RootPubKeyB64 == "" && cfg.NetworkID == "" {
		return "", nil, fmt.Errorf("%s", ErrEnrollAddNoNetwork)
	}
	var rootPub ed25519.PublicKey
	var derived string
	if cfg.RootPubKeyB64 != "" {
		pub, err := decodeEnrollAddRootPubKey(cfg.RootPubKeyB64)
		if err != nil {
			return "", nil, err
		}
		rootPub = pub
		derived = registry.NetworkID(pub)
	}
	switch {
	case derived != "" && cfg.NetworkID != "":
		if derived != cfg.NetworkID {
			return "", nil, fmt.Errorf("%s: %q != %q", ErrEnrollAddNetworkSpecifierDisagree, derived, cfg.NetworkID)
		}
		return derived, rootPub, nil
	case derived != "":
		return derived, rootPub, nil
	default:
		return cfg.NetworkID, rootPub, nil
	}
}

// decodeEnrollAddRootPubKey base64-std-decodes a root pubkey and validates it
// through registry.NewPublicKey (wrong-length / small-order rejected). It uses
// the enroll-add-specific typed error so the admin sees an enroll-add reject
// (distinct from the invite path's decodeRootPubKey).
func decodeEnrollAddRootPubKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errEnrollAddBadRootPubKey, err)
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errEnrollAddBadRootPubKey, err)
	}
	return pk.Bytes(), nil
}

// parseValiditySeconds parses the issuance-window seconds, defaulting to one year
// when empty. A non-numeric value fails closed.
func parseValiditySeconds(s string) (int64, error) {
	if s == "" {
		return defaultEnrollAddValiditySeconds, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", errEnrollAddBadValidity, err)
	}
	return v, nil
}

// loadCurrentRegistry resolves the current registry from the input bytes: a nil/
// empty input yields a FRESH empty registry (epoch 0). A signed distribution
// envelope is integrity-verified under rootPub (NOT VerifyAndLoad — an admin must
// be able to mutate a stale registry; only the root SIGNATURE must hold) and its
// inner JCS bytes used. An unsigned wireRegistry is used directly. Any other
// shape is refused.
func loadCurrentRegistry(input []byte, rootPub ed25519.PublicKey) (registry.Registry, error) {
	if len(bytes.TrimSpace(input)) == 0 {
		return registry.Registry{Epoch: enrollAddFreshEpoch}, nil
	}
	wireJSON, err := extractRegistryJSON(input, rootPub)
	if err != nil {
		return registry.Registry{}, err
	}
	w, err := decodeWireRegistry(wireJSON)
	if err != nil {
		return registry.Registry{}, err
	}
	// buildRegistry validates every existing binding + the epoch range — a
	// malformed current registry fails closed here.
	return buildRegistry(w)
}

// extractRegistryJSON auto-detects the registry input shape and returns the
// UNSIGNED wireRegistry JSON bytes. A signed envelope ({registry, root_sig, …})
// is integrity-verified and its inner bytes returned; an unsigned wireRegistry
// ({agents, epoch, …}) is returned verbatim.
func extractRegistryJSON(input []byte, rootPub ed25519.PublicKey) ([]byte, error) {
	shape, err := classifyRegistryShape(input)
	if err != nil {
		return nil, err
	}
	switch shape {
	case registryShapeSigned:
		return extractSignedRegistry(input, rootPub)
	case registryShapeUnsigned:
		return input, nil
	default:
		return nil, fmt.Errorf("%s", ErrEnrollAddUnrecognizedRegistry)
	}
}

type registryShape int

const (
	registryShapeUnknown registryShape = iota
	registryShapeSigned
	registryShapeUnsigned
)

// classifyRegistryShape inspects the top-level keys of the input JSON: a
// root_sig+registry pair is a signed envelope; an agents+epoch pair is an
// unsigned wireRegistry; anything else is unrecognized.
func classifyRegistryShape(input []byte) (registryShape, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(input, &probe); err != nil {
		return registryShapeUnknown, fmt.Errorf("%s: %w", ErrEnrollAddUnrecognizedRegistry, err)
	}
	_, hasRootSig := probe["root_sig"]
	_, hasRegistry := probe["registry"]
	if hasRootSig && hasRegistry {
		return registryShapeSigned, nil
	}
	_, hasAgents := probe["agents"]
	_, hasEpoch := probe["epoch"]
	if hasAgents && hasEpoch {
		return registryShapeUnsigned, nil
	}
	return registryShapeUnknown, nil
}

// extractSignedRegistry parses the distribution envelope and INTEGRITY-verifies
// root_sig over the registry bytes via the identity-package canonical-verify
// primitive (NOT VerifyAndLoad — its freshness/rollback gates wrongly block an
// admin mutating a stale registry). It returns the inner JCS bytes only when the
// root signature holds. Requires rootPub.
func extractSignedRegistry(input []byte, rootPub ed25519.PublicKey) ([]byte, error) {
	if rootPub == nil {
		return nil, fmt.Errorf("%s", ErrEnrollAddSignedNeedsRootPubKey)
	}
	env, err := registry.ParseDistribution(input)
	if err != nil {
		return nil, err
	}
	ok, err := identity.VerifyCanonical(
		rootPub,
		identity.CanonicalBytesFromTrusted(env.Registry),
		env.RootSig,
	)
	if err != nil || !ok {
		return nil, fmt.Errorf("%s", ErrEnrollAddUntrustedRegistry)
	}
	return env.Registry, nil
}

// checkUniqueness FAIL-CLOSED pre-checks the new binding against the current
// registry: it refuses a {slug,key_epoch} tuple that already exists, and refuses
// a slug that already has a LIVE (non-revoked) binding (rotation is out of scope
// for v1). A bad append would poison the WHOLE registry (the consumer rejects the
// entire document), so this gate runs BEFORE any append.
func checkUniqueness(reg registry.Registry, slug string, keyEpoch int) error {
	for _, b := range reg.Agents {
		if b.Slug == slug && b.KeyEpoch == keyEpoch {
			return fmt.Errorf("%s: %q@%d", ErrEnrollAddTupleExists, slug, keyEpoch)
		}
		if b.Slug == slug && b.RevokedAt == nil {
			return fmt.Errorf("%s: %q", ErrEnrollAddLiveSlugExists, slug)
		}
	}
	return nil
}

// buildNewBinding constructs the new AgentBinding from the verified request: the
// validated pubkey, the request's key_epoch, a fresh issuance window (now ..
// now+validity), the parsed authorized-origin-daemons, and a nil revoked_at
// (live). Transport fields are deliberately NOT carried (WireGuard = later
// slice).
func buildNewBinding(fields enrollment.Fields, cfg EnrollAddConfig, validity int64) (registry.AgentBinding, error) {
	raw, err := base64.StdEncoding.DecodeString(fields.PubKey)
	if err != nil {
		return registry.AgentBinding{}, fmt.Errorf("%s: %w", ErrRegistryBadPubKey, err)
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return registry.AgentBinding{}, fmt.Errorf("%s: %w", ErrRegistryBadPubKey, err)
	}
	now := cfg.now().Unix()
	return registry.AgentBinding{
		Slug:                    fields.Slug,
		DisplayName:             fields.DisplayName,
		PubKey:                  pk,
		KeyEpoch:                int(fields.KeyEpoch),
		ValidFrom:               now,
		ValidUntil:              now + validity,
		AuthorizedOriginDaemons: parseOriginDaemons(cfg.OriginDaemons),
		RevokedAt:               nil,
	}, nil
}

// parseOriginDaemons splits a comma-separated list into trimmed, non-empty
// daemon ids. An empty input yields an empty (non-nil for a clean JSON array)
// slice.
func parseOriginDaemons(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// appendAndBump appends binding to reg, bumps the epoch (rejecting an overflow
// past MaxSafeEpoch), and sets a FRESH registry issuance window (now ..
// now+validity). Existing bindings keep their own windows.
func appendAndBump(reg registry.Registry, binding registry.AgentBinding, validity, now int64) (registry.Registry, error) {
	newEpoch := reg.Epoch + 1
	if int64(newEpoch) > int64(registry.MaxSafeEpoch) {
		return registry.Registry{}, fmt.Errorf("%s", ErrEnrollAddEpochOverflow)
	}
	agents := append(append([]registry.AgentBinding(nil), reg.Agents...), binding)
	return registry.Registry{
		Epoch:      newEpoch,
		ValidFrom:  now,
		ValidUntil: now + validity,
		Agents:     agents,
	}, nil
}

// serializeSignable re-serializes the updated registry to its UNSIGNED
// wireRegistry JSON and VALIDATES that the result round-trips
// buildRegistry+canonicalizes — proving the emitted document is guaranteed
// signable by `sign-registry` before it is written out.
func serializeSignable(reg registry.Registry) ([]byte, error) {
	wire := toWireRegistry(reg)
	emitted, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errEnrollAddNotSignable, err)
	}
	// Re-decode + re-build + canonicalize as the sign path would, so an
	// un-signable result is caught HERE, not at the signer.
	w, err := decodeWireRegistry(emitted)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errEnrollAddNotSignable, err)
	}
	built, err := buildRegistry(w)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errEnrollAddNotSignable, err)
	}
	// Run the EXACT sign-path canonicalization (epoch gate + JCS) over an
	// ephemeral throwaway root key: a real signer is not in scope here, but the
	// canonicalization is byte-identical, so a canonicalize-time failure surfaces
	// HERE rather than at the downstream signer. The signature is discarded.
	if _, err := registry.SignRegistry(ephemeralSignKey(), built); err != nil {
		return nil, fmt.Errorf("%s: %w", errEnrollAddNotSignable, err)
	}
	return emitted, nil
}

// ephemeralSignKey returns a deterministic, throwaway ed25519 private key used
// ONLY to exercise the registry sign-path canonicalization in serializeSignable.
// It never signs a distribution that leaves this process, so a fixed seed is
// safe (and keeps the canonicalization check allocation-free of crypto/rand).
func ephemeralSignKey() ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
}

// toWireRegistry converts a registry.Registry into the wireRegistry shape that
// sign-registry reads (the SAME struct decodeWireRegistry/buildRegistry use).
func toWireRegistry(reg registry.Registry) wireRegistry {
	agents := make([]wireBinding, len(reg.Agents))
	for i, a := range reg.Agents {
		agents[i] = wireBinding{
			AuthorizedOriginDaemons: a.AuthorizedOriginDaemons,
			DisplayName:             a.DisplayName,
			KeyEpoch:                int64(a.KeyEpoch),
			PubKey:                  base64.StdEncoding.EncodeToString(a.PubKey.Bytes()),
			RevokedAt:               revokedAtI64(a.RevokedAt),
			Slug:                    a.Slug,
			ValidFrom:               a.ValidFrom,
			ValidUntil:              a.ValidUntil,
		}
	}
	return wireRegistry{
		Agents:     agents,
		Epoch:      int64(reg.Epoch),
		ValidFrom:  reg.ValidFrom,
		ValidUntil: reg.ValidUntil,
	}
}

// revokedAtI64 widens the registry's *int revoked_at back to the wire *int64,
// preserving nil (== live).
func revokedAtI64(v *int) *int64 {
	if v == nil {
		return nil
	}
	n := int64(*v)
	return &n
}

// writeEnrollAddGuidance prints the human-readable summary to errOut: the added
// slug/pubkey/key_epoch, the new registry epoch + issuance window, the NEXT step
// (root-sign), and the transport-fields-not-carried note.
func writeEnrollAddGuidance(errOut io.Writer, b registry.AgentBinding, newEpoch int) error {
	_, err := fmt.Fprintf(errOut, "%s%s\n%s%s\n%s%d\n%s%d\n%s%d..%d\n%s\n%s\n",
		EnrollAddAddedLabel, b.Slug,
		EnrollAddPubKeyLabel, base64.StdEncoding.EncodeToString(b.PubKey.Bytes()),
		EnrollAddKeyEpochLabel, b.KeyEpoch,
		EnrollAddEpochLabel, newEpoch,
		EnrollAddWindowLabel, b.ValidFrom, b.ValidUntil,
		EnrollAddNextStepNote,
		EnrollAddTransportNote,
	)
	return err
}
