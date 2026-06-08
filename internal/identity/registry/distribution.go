package registry

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Distribution-envelope field names and error strings (SSOT — referenced from
// the (de)serialization path and asserted by tests, never inlined).
const (
	// fieldDistRegistry is the envelope field carrying base64-std of the
	// JCS-canonical registry bytes — the EXACT bytes root_sig covers.
	fieldDistRegistry = "registry"
	// fieldDistRootSig is the envelope field carrying base64-std of the ed25519
	// root signature over the registry bytes.
	fieldDistRootSig = "root_sig"
	// fieldDistRootKeyEpoch is the envelope field carrying which root key signed.
	fieldDistRootKeyEpoch = "root_key_epoch"

	// ErrDistMarshal wraps a distribution-envelope marshal failure.
	ErrDistMarshal = "registry: marshal distribution envelope"
	// ErrDistUnmarshal wraps a distribution-envelope unmarshal failure (bad JSON
	// or an unknown field — strict, fail-closed decoding).
	ErrDistUnmarshal = "registry: parse distribution envelope"
	// ErrDistMissingRegistry is returned by ParseDistribution when the registry
	// field is absent or empty.
	ErrDistMissingRegistry = "registry: distribution envelope is missing the registry field"
	// ErrDistMissingRootSig is returned by ParseDistribution when the root_sig
	// field is absent or empty.
	ErrDistMissingRootSig = "registry: distribution envelope is missing the root_sig field"
	// ErrDistBadRegistryB64 is returned when the registry field is not valid
	// base64-std.
	ErrDistBadRegistryB64 = "registry: distribution registry field is not valid base64"
	// ErrDistBadRootSigB64 is returned when the root_sig field is not valid
	// base64-std.
	ErrDistBadRootSigB64 = "registry: distribution root_sig field is not valid base64"
	// ErrDistBadSigLen is returned when the decoded root_sig is not
	// ed25519.SignatureSize bytes (fail closed before the verify path ever sees a
	// malformed signature).
	ErrDistBadSigLen = "registry: distribution root_sig must be ed25519.SignatureSize bytes"
)

// MarshalDistribution serializes a SignedRegistry to the distribution-envelope
// JSON, keyed by the fieldDist* SSOT constants. The registry bytes are emitted
// base64-std VERBATIM — they are the exact bytes root_sig covers and must not be
// re-canonicalized or double-encoded.
func MarshalDistribution(env SignedRegistry) ([]byte, error) {
	out, err := json.Marshal(map[string]any{
		fieldDistRegistry:     base64.StdEncoding.EncodeToString(env.Registry),
		fieldDistRootSig:      base64.StdEncoding.EncodeToString(env.RootSig),
		fieldDistRootKeyEpoch: env.RootKeyEpoch,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrDistMarshal, err)
	}
	return out, nil
}

// ParseDistribution deserializes a distribution-envelope JSON into a
// SignedRegistry. It FAILS CLOSED on every malformed input — bad JSON, an
// unknown field, a missing/empty required field, bad base64, or a wrong-length
// signature — returning an error and a zero SignedRegistry (never a partial that
// could be mistaken for valid). It never panics.
//
// ParseDistribution is ONLY (de)serialization + structural fail-closed parsing;
// it does NOT verify the root signature, freshness, or anti-rollback. The
// consumer flow is ParseDistribution -> VerifyAndLoad, which performs the full
// slice-3 trust logic over the parsed bytes.
func ParseDistribution(data []byte) (SignedRegistry, error) {
	// Decode into a raw map so the strict unknown-field check and the
	// presence/type checks are keyed by the fieldDist* SSOT constants (no struct
	// tags to drift from the named constants).
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return SignedRegistry{}, fmt.Errorf("%s: %w", ErrDistUnmarshal, err)
	}
	// Reject any unknown field: a smuggled extra key must not be silently dropped.
	for k := range raw {
		if k != fieldDistRegistry && k != fieldDistRootSig && k != fieldDistRootKeyEpoch {
			return SignedRegistry{}, fmt.Errorf("%s: unknown field %q", ErrDistUnmarshal, k)
		}
	}

	regB64, err := decodeStringField(raw, fieldDistRegistry, ErrDistMissingRegistry)
	if err != nil {
		return SignedRegistry{}, err
	}
	sigB64, err := decodeStringField(raw, fieldDistRootSig, ErrDistMissingRootSig)
	if err != nil {
		return SignedRegistry{}, err
	}
	var rootKeyEpoch int
	if rkeRaw, ok := raw[fieldDistRootKeyEpoch]; ok {
		if err := json.Unmarshal(rkeRaw, &rootKeyEpoch); err != nil {
			return SignedRegistry{}, fmt.Errorf("%s: %s: %w", ErrDistUnmarshal, fieldDistRootKeyEpoch, err)
		}
	}

	regBytes, err := base64.StdEncoding.DecodeString(regB64)
	if err != nil {
		return SignedRegistry{}, fmt.Errorf("%s: %w", ErrDistBadRegistryB64, err)
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return SignedRegistry{}, fmt.Errorf("%s: %w", ErrDistBadRootSigB64, err)
	}
	if len(sig) != ed25519.SignatureSize {
		return SignedRegistry{}, fmt.Errorf("%s", ErrDistBadSigLen)
	}

	return SignedRegistry{
		Registry:     regBytes,
		RootSig:      sig,
		RootKeyEpoch: rootKeyEpoch,
	}, nil
}

// decodeStringField extracts a required, non-empty JSON string field from raw,
// returning missingErr (fail closed) when the field is absent or empty and
// ErrDistUnmarshal when it is present but not a JSON string.
func decodeStringField(raw map[string]json.RawMessage, field, missingErr string) (string, error) {
	rawVal, ok := raw[field]
	if !ok {
		return "", fmt.Errorf("%s", missingErr)
	}
	var s string
	if err := json.Unmarshal(rawVal, &s); err != nil {
		return "", fmt.Errorf("%s: %s: %w", ErrDistUnmarshal, field, err)
	}
	if s == "" {
		return "", fmt.Errorf("%s", missingErr)
	}
	return s, nil
}
