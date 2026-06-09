package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/identity/enrollment"
)

// b64Std is a local helper for base64-std encoding in this test file.
func b64Std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// curve25519TestKey returns a deterministic 32-byte (length-only-checked)
// transport pubkey value.
func curve25519TestKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = 0x11
	}
	return k
}

// testEnrollmentJSON is a valid signed-subset enrollment request whose pubkey is
// filled in per-test (the identity key is generated fresh so the emitted sig can
// be verified by VerifyEnrollment).
func testEnrollmentJSON(pubB64 string) string {
	return `{"alg_version":1,"created":2000000,"display_name":"Mira",` +
		`"key_epoch":1,"network_id":"vmnet1:0011223344556677889900aabbccddee",` +
		`"nonce":"YWJjZGVmZ2hpamtsbW5vcA==","pubkey":"` + pubB64 + `","slug":"mira",` +
		`"transport_pubkey":"` + b64Std(curve25519TestKey()) + `"}`
}

// TestSignEnrollmentProducesVerifyingResultKeylessly proves SignEnrollment signs
// the canonical signed subset through the SignerClient seam (no key file) and
// emits a {sig, pubkey} JSON whose sig verifies under VerifyEnrollment.
func TestSignEnrollmentProducesVerifyingResultKeylessly(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fake := &fakeSignerClient{priv: priv}

	var out bytes.Buffer
	in := testEnrollmentJSON(b64Std(pub))
	if err := SignEnrollment(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignEnrollment: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, out.String())
	}
	if res[enrollment.FieldPubKey] != b64Std(pub) {
		t.Fatalf("pubkey echo mismatch: %v", res[enrollment.FieldPubKey])
	}
	sig, err := base64.StdEncoding.DecodeString(res[enrollment.FieldSig].(string))
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}

	ok, verr := enrollment.VerifyEnrollment(enrollment.Fields{
		AlgVersion: 1, Created: 2000000, DisplayName: "Mira", KeyEpoch: 1,
		NetworkID: "vmnet1:0011223344556677889900aabbccddee",
		Nonce:     "YWJjZGVmZ2hpamtsbW5vcA==", PubKey: b64Std(pub), Slug: "mira",
		TransportPubKey: b64Std(curve25519TestKey()),
	}, sig)
	if verr != nil || !ok {
		t.Fatalf("emitted sig did not verify: ok=%v err=%v", ok, verr)
	}
}

// TestSignEnrollmentByteIdenticalToDomain proves the CLI wire-decode introduces
// no drift: the canonical bytes the signer was handed equal the domain
// CanonicalizeEnrollment output for the same request.
func TestSignEnrollmentByteIdenticalToDomain(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fake := &fakeSignerClient{priv: priv}

	var out bytes.Buffer
	in := testEnrollmentJSON(b64Std(pub))
	if err := SignEnrollment(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignEnrollment: %v", err)
	}

	canonical, err := enrollment.CanonicalizeEnrollment(enrollment.Fields{
		AlgVersion: 1, Created: 2000000, DisplayName: "Mira", KeyEpoch: 1,
		NetworkID: "vmnet1:0011223344556677889900aabbccddee",
		Nonce:     "YWJjZGVmZ2hpamtsbW5vcA==", PubKey: b64Std(pub), Slug: "mira",
		TransportPubKey: b64Std(curve25519TestKey()),
	})
	if err != nil {
		t.Fatalf("CanonicalizeEnrollment: %v", err)
	}
	if !bytes.Equal(canonical.Bytes(), fake.gotCanonical) {
		t.Fatalf("CLI canonical bytes drift from domain:\n cli=%q\n dom=%q",
			fake.gotCanonical, canonical.Bytes())
	}
}

// TestSignEnrollmentRejectsGateViolation: a downgraded alg_version is rejected
// and the signer is never called; nothing is printed.
func TestSignEnrollmentRejectsGateViolation(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	bad := `{"alg_version":2,"created":1,"display_name":"Mira","key_epoch":1,` +
		`"network_id":"vmnet1:x","nonce":"abc","pubkey":"` + b64Std(pub) + `",` +
		`"slug":"mira","transport_pubkey":"` + b64Std(curve25519TestKey()) + `"}`
	if err := SignEnrollment(&out, fake, []byte(bad)); err == nil {
		t.Fatal("expected gate rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a gate-violating request")
	}
	if out.Len() != 0 {
		t.Fatalf("gate reject must print nothing, got %q", out.String())
	}
}

// TestSignEnrollmentRejectsUnknownField: a smuggled extra key fails closed.
func TestSignEnrollmentRejectsUnknownField(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	bad := testEnrollmentJSON(b64Std(pub))
	bad = bad[:len(bad)-1] + `,"evil":true}`
	if err := SignEnrollment(&out, fake, []byte(bad)); err == nil {
		t.Fatal("expected unknown-field rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("parse reject must print nothing, got %q", out.String())
	}
}

// TestSignEnrollmentRejectsTrailingData: trailing data after the JSON object is
// a strict parse reject.
func TestSignEnrollmentRejectsTrailingData(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	bad := testEnrollmentJSON(b64Std(pub)) + ` {"x":1}`
	if err := SignEnrollment(&out, fake, []byte(bad)); err == nil {
		t.Fatal("expected trailing-data rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("parse reject must print nothing, got %q", out.String())
	}
}

// TestSignEnrollmentFailsClosedOnSignerError: a signer error surfaces, no output.
func TestSignEnrollmentFailsClosedOnSignerError(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fake := &fakeSignerClient{failErr: sentinelErr("signer unreachable")}
	var out bytes.Buffer
	if err := SignEnrollment(&out, fake, []byte(testEnrollmentJSON(b64Std(pub)))); err == nil {
		t.Fatal("expected fail-closed error, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("fail-closed must print nothing, got %q", out.String())
	}
}

// TestSignEnrollmentWithTransportEndpoint exercises the optional
// transport_endpoint passing through the CLI into the signed bytes.
func TestSignEnrollmentWithTransportEndpoint(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fake := &fakeSignerClient{priv: priv}
	var out bytes.Buffer
	in := `{"alg_version":1,"created":2000000,"display_name":"Mira","key_epoch":1,` +
		`"network_id":"vmnet1:0011223344556677889900aabbccddee",` +
		`"nonce":"YWJjZGVmZ2hpamtsbW5vcA==","pubkey":"` + b64Std(pub) + `","slug":"mira",` +
		`"transport_endpoint":"203.0.113.7:51820",` +
		`"transport_pubkey":"` + b64Std(curve25519TestKey()) + `"}`
	if err := SignEnrollment(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignEnrollment: %v", err)
	}
	ep := "203.0.113.7:51820"
	ok, verr := enrollment.VerifyEnrollment(enrollment.Fields{
		AlgVersion: 1, Created: 2000000, DisplayName: "Mira", KeyEpoch: 1,
		NetworkID: "vmnet1:0011223344556677889900aabbccddee",
		Nonce:     "YWJjZGVmZ2hpamtsbW5vcA==", PubKey: b64Std(pub), Slug: "mira",
		TransportEndpoint: &ep, TransportPubKey: b64Std(curve25519TestKey()),
	}, decodeEnrollSig(t, out.String()))
	if verr != nil || !ok {
		t.Fatalf("emitted sig (with transport_endpoint) did not verify: ok=%v err=%v", ok, verr)
	}
}

// decodeEnrollSig extracts the base64 sig from the {sig,pubkey} JSON output.
func decodeEnrollSig(t *testing.T, output string) []byte {
	t.Helper()
	var res map[string]any
	if err := json.Unmarshal([]byte(output), &res); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, output)
	}
	sig, err := base64.StdEncoding.DecodeString(res[enrollment.FieldSig].(string))
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	return sig
}
