package invite

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/identity/registry"
)

// newRootKey mints a fresh ed25519 root keypair and returns the base64-std public
// key plus the derived network id (the canonical inputs to an Invite).
func newRootKey(t *testing.T) (pubB64 string, networkID string, pub ed25519.PublicKey) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return base64.StdEncoding.EncodeToString(pub), registry.NetworkID(pub), pub
}

// validInvite returns a well-formed Invite for the given root key.
func validInvite(t *testing.T) Invite {
	t.Helper()
	pubB64, networkID, _ := newRootKey(t)
	return Invite{
		NetworkID:  networkID,
		Relay:      "https://chat.acme.com",
		RootPubKey: pubB64,
	}
}

// TestEncodeDecodeRoundTripToken proves a valid Invite encodes to a vmenroll1:
// token that Decode parses back to the identical struct.
func TestEncodeDecodeRoundTripToken(t *testing.T) {
	inv := validInvite(t)

	token, url, err := Encode(inv)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasPrefix(token, invitePrefix) {
		t.Fatalf("token %q missing prefix %q", token, invitePrefix)
	}
	if want := inv.Relay + relayEnrollPath + "#" + token; url != want {
		t.Fatalf("url = %q, want %q", url, want)
	}

	got, err := Decode(token)
	if err != nil {
		t.Fatalf("Decode(token): %v", err)
	}
	if got != inv {
		t.Fatalf("round-trip mismatch:\n got=%+v\nwant=%+v", got, inv)
	}
}

// TestDecodeFromURLFragment proves Decode accepts the full enroll URL and reads
// the token from its fragment (after the first '#').
func TestDecodeFromURLFragment(t *testing.T) {
	inv := validInvite(t)
	_, url, err := Encode(inv)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := Decode(url)
	if err != nil {
		t.Fatalf("Decode(url): %v", err)
	}
	if got != inv {
		t.Fatalf("url round-trip mismatch:\n got=%+v\nwant=%+v", got, inv)
	}
}

// TestFingerprintIsNetworkID proves the OOB fingerprint is exactly the network id.
func TestFingerprintIsNetworkID(t *testing.T) {
	inv := validInvite(t)
	if fp := Fingerprint(inv); fp != inv.NetworkID {
		t.Fatalf("Fingerprint = %q, want network_id %q", fp, inv.NetworkID)
	}
}

// encodeToken is a test helper that base64url-nopad-encodes a raw wire-JSON body
// behind the invite prefix, BYPASSING Encode's validation. It lets the reject
// tests forge tokens whose decoded payload is individually malformed.
func encodeToken(jsonBody string) string {
	return invitePrefix + base64.RawURLEncoding.EncodeToString([]byte(jsonBody))
}

// TestEncodeFailsClosedOnInvalidInvite proves Encode validates before emitting:
// an empty relay (and other invalid fields) returns an error and no token.
func TestEncodeFailsClosedOnInvalidInvite(t *testing.T) {
	pubB64, networkID, _ := newRootKey(t)
	cases := map[string]Invite{
		"empty relay":        {NetworkID: networkID, Relay: "", RootPubKey: pubB64},
		"empty pubkey":       {NetworkID: networkID, Relay: "https://r", RootPubKey: ""},
		"mismatch networkid": {NetworkID: "vmnet1:deadbeef", Relay: "https://r", RootPubKey: pubB64},
	}
	for name, inv := range cases {
		t.Run(name, func(t *testing.T) {
			token, url, err := Encode(inv)
			if err == nil {
				t.Fatalf("Encode(%+v) = nil error, want reject", inv)
			}
			if token != "" || url != "" {
				t.Fatalf("fail-closed Encode leaked token=%q url=%q", token, url)
			}
		})
	}
}

// TestDecodeRejectsEachFailure drives every fail-closed branch of Decode.
func TestDecodeRejectsEachFailure(t *testing.T) {
	pubB64, networkID, pub := newRootKey(t)

	// A token whose body is structurally fine, used to derive variants.
	goodBody := `{"network_id":"` + networkID + `","relay":"https://r","root_pubkey":"` + pubB64 + `"}`

	// smallOrderB64 is base64-std of the 32-byte all-zero key (a small-order key
	// registry.NewPublicKey rejects).
	smallOrderB64 := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	// shortKeyB64 is base64-std of a 31-byte key (wrong length).
	shortKeyB64 := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize-1))
	// A second, DIFFERENT root key whose network id will not match networkID.
	otherPubB64, _, _ := newRootKey(t)

	cases := map[string]struct {
		in      string
		wantErr string
	}{
		"missing prefix":      {in: "notvmenroll1:abc", wantErr: ErrBadPrefix},
		"bad base64":          {in: invitePrefix + "!!!not-base64!!!", wantErr: ErrBadBase64},
		"bad json":            {in: encodeToken(`{not json`), wantErr: ErrBadJSON},
		"unknown field":       {in: encodeToken(`{"network_id":"x","relay":"y","root_pubkey":"z","extra":1}`), wantErr: ErrBadJSON},
		"trailing data":       {in: encodeToken(goodBody + `{}`), wantErr: ErrBadJSON},
		"empty relay":         {in: encodeToken(`{"network_id":"` + networkID + `","relay":"","root_pubkey":"` + pubB64 + `"}`), wantErr: ErrEmptyRelay},
		"empty network_id":    {in: encodeToken(`{"network_id":"","relay":"https://r","root_pubkey":"` + pubB64 + `"}`), wantErr: ErrNetworkIDMismatch},
		"bad-base64 pubkey":   {in: encodeToken(`{"network_id":"` + networkID + `","relay":"https://r","root_pubkey":"!!!"}`), wantErr: ErrBadRootPubKey},
		"wrong-length pubkey": {in: encodeToken(`{"network_id":"` + registry.NetworkID(make([]byte, ed25519.PublicKeySize-1)) + `","relay":"https://r","root_pubkey":"` + shortKeyB64 + `"}`), wantErr: ErrBadRootPubKey},
		"small-order pubkey":  {in: encodeToken(`{"network_id":"` + registry.NetworkID(make([]byte, ed25519.PublicKeySize)) + `","relay":"https://r","root_pubkey":"` + smallOrderB64 + `"}`), wantErr: ErrBadRootPubKey},
		"tampered network_id": {in: encodeToken(`{"network_id":"` + registry.NetworkID(pub) + `","relay":"https://r","root_pubkey":"` + otherPubB64 + `"}`), wantErr: ErrNetworkIDMismatch},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(tc.in)
			if err == nil {
				t.Fatalf("Decode(%q) = nil error, want %q", tc.in, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Decode error = %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}

// TestNetworkIDIntegrityRejectsSubstitution is the critical security test: an
// invite whose network_id is honestly derived from one root key but whose
// root_pubkey is a DIFFERENT valid key must be rejected (a relay cannot swap in
// its own anchor under the victim network's id).
func TestNetworkIDIntegrityRejectsSubstitution(t *testing.T) {
	victimPubB64, victimNetID, _ := newRootKey(t)
	attackerPubB64, _, _ := newRootKey(t)

	// Both keys are individually valid; only the binding is wrong.
	body := `{"network_id":"` + victimNetID + `","relay":"https://evil","root_pubkey":"` + attackerPubB64 + `"}`
	if _, err := Decode(encodeToken(body)); err == nil {
		t.Fatal("Decode accepted a network_id that does not match its root_pubkey")
	}

	// Sanity: the victim's own key under its own id still decodes.
	good := `{"network_id":"` + victimNetID + `","relay":"https://acme","root_pubkey":"` + victimPubB64 + `"}`
	if _, err := Decode(encodeToken(good)); err != nil {
		t.Fatalf("Decode rejected the legitimate victim invite: %v", err)
	}
}
