package enrollment

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// errUnmarshalWire wraps a wire enrollment-request decode failure (bad JSON, an
// unknown field, or trailing data — strict, fail-closed decoding).
const errUnmarshalWire = "enrollment: parse wire request"

// UnmarshalWire is the exact INVERSE of MarshalWire: it strictly decodes a wire
// enrollment-request JSON into the signed-subset Fields plus the SEPARATED
// transport sig (base64-std string). The sig is NOT part of the signed subset,
// so it is returned alongside Fields rather than modeled into it.
//
// It FAILS CLOSED: an unknown field (DisallowUnknownFields) or trailing data
// after the JSON object (dec.More) is rejected, so a smuggled extra key or a
// trailing-object smuggling vector cannot slip through. The numeric fields
// (alg_version, created, key_epoch) decode as int64 so the typed range gate in
// CanonicalizeEnrollment — not a 32-bit JSON parse error — is the single,
// platform-independent authority (Go<->Rust parity). transport_endpoint decodes
// to a *string so absent (nil) is distinguishable from an empty value; an absent
// endpoint stays nil (absent != null).
//
// It does NOT verify the sig or run the pre-sign gates: the caller verifies
// proof-of-possession by handing Fields + the decoded sig to VerifyEnrollment.
func UnmarshalWire(data []byte) (Fields, string, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var w wireEnrollmentRequest
	if err := dec.Decode(&w); err != nil {
		return Fields{}, "", fmt.Errorf("%s: %w", errUnmarshalWire, err)
	}
	if dec.More() {
		return Fields{}, "", fmt.Errorf("%s: trailing data after JSON object", errUnmarshalWire)
	}
	fields := Fields{
		AlgVersion:        w.AlgVersion,
		Created:           w.Created,
		DisplayName:       w.DisplayName,
		KeyEpoch:          w.KeyEpoch,
		NetworkID:         w.NetworkID,
		Nonce:             w.Nonce,
		PubKey:            w.PubKey,
		Slug:              w.Slug,
		TransportEndpoint: w.TransportEndpoint,
		TransportPubKey:   w.TransportPubKey,
	}
	return fields, w.Sig, nil
}
