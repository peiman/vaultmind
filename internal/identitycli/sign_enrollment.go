package identitycli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/identity/enrollment"
)

// Enrollment-signing error strings (SSOT).
const (
	// ErrEnrollmentParse is returned by SignEnrollment when the enrollment JSON
	// cannot be decoded (bad JSON, unknown field, or trailing data — strict, fail
	// closed).
	ErrEnrollmentParse = "identitycli: parse enrollment JSON"
	// ErrEnrollmentMarshalResult wraps a marshal failure of the sign result.
	ErrEnrollmentMarshalResult = "identitycli: marshal sign-enrollment result"
)

// wireEnrollment is the JSON shape SignEnrollment reads from stdin/--file: the
// SIGNED-SUBSET fields only. transport_endpoint is a POINTER so absent-vs-present
// maps onto enrollment.Fields' optional-emit rule. The transport sig is NOT read
// here — it is the result of signing, stamped by the caller afterwards.
type wireEnrollment struct {
	// alg_version/created/key_epoch decode as int64 so the enrollment [0, 2^53]
	// gate is the single authority for range (and behaves identically on
	// 32/64-bit). A plain int would JSON-parse-error a >2^31 value on a 32-bit
	// build before the gate ever ran — the parity trap the signed contract forbids.
	AlgVersion        int64   `json:"alg_version"`
	Created           int64   `json:"created"`
	DisplayName       string  `json:"display_name"`
	KeyEpoch          int64   `json:"key_epoch"`
	NetworkID         string  `json:"network_id"`
	Nonce             string  `json:"nonce"`
	PubKey            string  `json:"pubkey"`
	Slug              string  `json:"slug"`
	TransportEndpoint *string `json:"transport_endpoint"`
	TransportPubKey   string  `json:"transport_pubkey"`
}

// SignEnrollment reads an agent-enrollment request (the signed-subset fields),
// enforces the Contract-B enrollment gates + canonicalizes via the enrollment
// package, signs the canonical bytes through the KEYLESS SignerClient, and writes
// {sig, pubkey} as JSON to out (pubkey is ECHOED from the request — it IS the
// signed proof-of-possession key). It is KEYLESS and FAILS CLOSED: any parse,
// gate, canonicalize, or signer error returns non-nil and prints nothing.
func SignEnrollment(out io.Writer, client enrollment.SignerClient, enrollmentJSON []byte) error {
	w, err := decodeWireEnrollment(enrollmentJSON)
	if err != nil {
		return err
	}
	res, err := enrollment.SignEnrollment(client, enrollment.Fields{
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
	})
	if err != nil {
		return fmt.Errorf("signing enrollment via signer: %w", err)
	}

	outJSON, err := json.Marshal(map[string]any{
		enrollment.FieldSig:    res.Sig,
		enrollment.FieldPubKey: w.PubKey,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", ErrEnrollmentMarshalResult, err)
	}
	_, werr := fmt.Fprintf(out, "%s\n", outJSON)
	return werr
}

// decodeWireEnrollment strictly decodes the enrollment-request JSON: unknown
// fields and trailing data are rejected (fail closed).
func decodeWireEnrollment(enrollmentJSON []byte) (wireEnrollment, error) {
	dec := json.NewDecoder(bytes.NewReader(enrollmentJSON))
	dec.DisallowUnknownFields()
	var w wireEnrollment
	if err := dec.Decode(&w); err != nil {
		return wireEnrollment{}, fmt.Errorf("%s: %w", ErrEnrollmentParse, err)
	}
	if dec.More() {
		return wireEnrollment{}, fmt.Errorf("%s: trailing data after JSON object", ErrEnrollmentParse)
	}
	return w, nil
}
