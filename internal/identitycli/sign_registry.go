package identitycli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/identity/registry"
)

// Registry-signing error strings (SSOT).
const (
	// ErrRegistryParse is returned by SignRegistry when the registry JSON cannot
	// be decoded (bad JSON, unknown field, or trailing data — strict, fail closed).
	ErrRegistryParse = "identitycli: parse registry JSON"
	// ErrRegistryBadPubKey is returned when a binding's pubkey is not valid
	// base64-std of a validatable ed25519 public key.
	ErrRegistryBadPubKey = "identitycli: binding pubkey must be base64-std of an ed25519 public key"
	// ErrRegistryMarshalDist wraps a distribution-envelope marshal failure.
	ErrRegistryMarshalDist = "identitycli: marshal distribution envelope"
	// ErrRegistryRevokedAtNegative is returned when a binding's revoked_at is
	// negative (a pre-epoch revocation timestamp is nonsensical — fail closed).
	ErrRegistryRevokedAtNegative = "identitycli: binding revoked_at must not be negative"
)

// wireBinding is the JSON shape SignRegistry reads for one agent binding. Field
// names match the registry package's canonical/distribution names (snake_case,
// pubkey base64-std). revoked_at is a pointer so absent == live. The signed
// integers (key_epoch, timestamps) decode as int64 and the epoch range gate runs
// on the int64 value BEFORE any int64->int narrowing (see epochInRangeI64), so
// the wire boundary is the single authority and a >2^31 value cannot truncate
// past the gate on a 32-bit build. key_epoch narrows to int; the validity
// timestamps stay int64; revoked_at maps to *int (64-bit-target-safe; widening
// tracked as hardening).
type wireBinding struct {
	AuthorizedOriginDaemons []string `json:"authorized_origin_daemons"`
	DisplayName             string   `json:"display_name"`
	KeyEpoch                int64    `json:"key_epoch"`
	PubKey                  string   `json:"pubkey"`
	RevokedAt               *int64   `json:"revoked_at,omitempty"`
	Slug                    string   `json:"slug"`
	ValidFrom               int64    `json:"valid_from"`
	ValidUntil              int64    `json:"valid_until"`
}

// wireRegistry is the JSON shape SignRegistry reads from stdin/--file: the
// UNSIGNED registry (NOT a SignedRegistry). epoch decodes as int64 for the same
// platform-parity reason as the binding integers.
type wireRegistry struct {
	Agents     []wireBinding `json:"agents"`
	Epoch      int64         `json:"epoch"`
	ValidFrom  int64         `json:"valid_from"`
	ValidUntil int64         `json:"valid_until"`
}

// SignRegistry reads an UNSIGNED registry (the registry.Registry shape), signs
// its JCS-canonical bytes through the KEYLESS SignerClient (the root key stays in
// the signer), assembles the distribution envelope via the registry package, and
// writes the envelope JSON to out. It is KEYLESS and FAILS CLOSED: any parse, bad
// pubkey, epoch-gate, signer, or marshal error returns non-nil and prints
// nothing (never a partial/unsigned result).
func SignRegistry(out io.Writer, client registry.RegistrySigner, registryJSON []byte) error {
	w, err := decodeWireRegistry(registryJSON)
	if err != nil {
		return err
	}
	reg, err := buildRegistry(w)
	if err != nil {
		return err
	}
	env, err := registry.SignRegistryWithSigner(client, reg)
	if err != nil {
		return fmt.Errorf("signing registry via signer: %w", err)
	}
	outJSON, err := registry.MarshalDistribution(env)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrRegistryMarshalDist, err)
	}
	_, werr := fmt.Fprintf(out, "%s\n", outJSON)
	return werr
}

// decodeWireRegistry strictly decodes the unsigned-registry JSON: unknown fields
// and trailing data are rejected (fail closed).
func decodeWireRegistry(registryJSON []byte) (wireRegistry, error) {
	dec := json.NewDecoder(bytes.NewReader(registryJSON))
	dec.DisallowUnknownFields()
	var w wireRegistry
	if err := dec.Decode(&w); err != nil {
		return wireRegistry{}, fmt.Errorf("%s: %w", ErrRegistryParse, err)
	}
	if dec.More() {
		return wireRegistry{}, fmt.Errorf("%s: trailing data after JSON object", ErrRegistryParse)
	}
	return w, nil
}

// buildRegistry converts the decoded wire registry into a registry.Registry. It
// gates the registry epoch on the int64 wire value BEFORE narrowing (the wire
// boundary is the range authority), then builds each binding. Fails closed.
func buildRegistry(w wireRegistry) (registry.Registry, error) {
	if !epochInRangeI64(w.Epoch) {
		return registry.Registry{}, fmt.Errorf("%s", registry.ErrEpochRange)
	}
	agents := make([]registry.AgentBinding, len(w.Agents))
	for i, b := range w.Agents {
		ab, err := buildBinding(b)
		if err != nil {
			return registry.Registry{}, err
		}
		agents[i] = ab
	}
	return registry.Registry{
		Epoch:      int(w.Epoch),
		ValidFrom:  w.ValidFrom,
		ValidUntil: w.ValidUntil,
		Agents:     agents,
	}, nil
}

// buildBinding validates + converts one wire binding. The key_epoch range gate
// runs on the int64 value BEFORE narrowing; the pubkey is base64-std-decoded +
// validated; revoked_at is range-checked. Any failure fails closed.
func buildBinding(b wireBinding) (registry.AgentBinding, error) {
	if !epochInRangeI64(b.KeyEpoch) {
		return registry.AgentBinding{}, fmt.Errorf("%s", registry.ErrEpochRange)
	}
	if b.RevokedAt != nil && *b.RevokedAt < 0 {
		return registry.AgentBinding{}, fmt.Errorf("%s", ErrRegistryRevokedAtNegative)
	}
	pk, err := decodeBindingPubKey(b.PubKey)
	if err != nil {
		return registry.AgentBinding{}, err
	}
	return registry.AgentBinding{
		Slug:                    b.Slug,
		DisplayName:             b.DisplayName,
		PubKey:                  pk,
		KeyEpoch:                int(b.KeyEpoch),
		ValidFrom:               b.ValidFrom,
		ValidUntil:              b.ValidUntil,
		AuthorizedOriginDaemons: b.AuthorizedOriginDaemons,
		RevokedAt:               revokedAtPtr(b.RevokedAt),
	}, nil
}

// epochInRangeI64 mirrors registry's epoch gate on the int64 wire value, so the
// range [1, MaxSafeEpoch] is enforced BEFORE any int64->int narrowing — a value
// above 2^31 cannot truncate past the gate on a 32-bit build.
func epochInRangeI64(e int64) bool { return e >= 1 && e <= registry.MaxSafeEpoch }

// decodeBindingPubKey base64-std-decodes a binding pubkey and routes it through
// registry.NewPublicKey, so a wrong-length or small-order key cannot enter a
// binding. Both the base64 and the validation failures fail closed.
func decodeBindingPubKey(b64 string) (registry.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return registry.PublicKey{}, fmt.Errorf("%s: %w", ErrRegistryBadPubKey, err)
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return registry.PublicKey{}, fmt.Errorf("%s: %w", ErrRegistryBadPubKey, err)
	}
	return pk, nil
}

// revokedAtPtr narrows the int64 wire revoked_at to the registry's *int. A nil
// wire pointer (absent revoked_at) stays nil (== live). Negativity is gated in
// buildBinding before this runs. The int64->int narrowing is safe on 64-bit
// targets (the only build targets); widening registry.RevokedAt to *int64 is
// tracked as Contract-B hardening.
func revokedAtPtr(v *int64) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}
