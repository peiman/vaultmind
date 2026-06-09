package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/identity/invite"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// newInviteRootKey mints a root keypair and returns its base64-std pubkey plus
// the derived network id.
func newInviteRootKey(t *testing.T) (pubB64 string, networkID string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return base64.StdEncoding.EncodeToString(pub), registry.NetworkID(pub)
}

// TestInvitePrintsThreeBlocksAndRoundTrips drives Invite end to end: it prints
// the token, url, and fingerprint blocks, the printed token Decodes back, and
// the printed fingerprint equals the network id.
func TestInvitePrintsThreeBlocksAndRoundTrips(t *testing.T) {
	pubB64, networkID := newInviteRootKey(t)
	const relay = "https://chat.acme.com"

	var out bytes.Buffer
	if err := Invite(&out, pubB64, relay); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	got := out.String()

	for _, label := range []string{InviteTokenLabel, InviteURLLabel, InviteFingerprintLabel} {
		if !strings.Contains(got, label) {
			t.Fatalf("output missing label %q:\n%s", label, got)
		}
	}
	if !strings.Contains(got, InviteConfirmNote) {
		t.Fatalf("output missing OOB-confirm note:\n%s", got)
	}
	if !strings.Contains(got, networkID) {
		t.Fatalf("output missing network id %q:\n%s", networkID, got)
	}

	token := extractAfter(t, got, InviteTokenLabel)
	dec, err := invite.Decode(token)
	if err != nil {
		t.Fatalf("Decode(printed token): %v\noutput=%s", err, got)
	}
	if dec.NetworkID != networkID || dec.Relay != relay || dec.RootPubKey != pubB64 {
		t.Fatalf("decoded invite mismatch: %+v", dec)
	}

	// The printed URL must also Decode (fragment round-trip through the command).
	url := extractAfter(t, got, InviteURLLabel)
	if _, err := invite.Decode(url); err != nil {
		t.Fatalf("Decode(printed url): %v", err)
	}
}

// extractAfter returns the trimmed remainder of the first line containing label,
// after the label text.
func extractAfter(t *testing.T, out, label string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, label); i >= 0 {
			return strings.TrimSpace(line[i+len(label):])
		}
	}
	t.Fatalf("label %q not found in output:\n%s", label, out)
	return ""
}

// TestInviteFailsClosedOnBadRootPubKey proves Invite rejects an invalid root key
// (bad base64, wrong length, small-order) and prints nothing.
func TestInviteFailsClosedOnBadRootPubKey(t *testing.T) {
	cases := map[string]string{
		"bad base64":   "!!!not-base64!!!",
		"wrong length": base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize-1)),
		"small order":  base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)),
	}
	for name, pubB64 := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := Invite(&out, pubB64, "https://relay"); err == nil {
				t.Fatalf("Invite(%q) = nil error, want reject", name)
			}
			if out.Len() != 0 {
				t.Fatalf("fail-closed Invite leaked output: %q", out.String())
			}
		})
	}
}

// TestInviteFailsClosedOnEmptyRelay proves an empty relay is rejected with no
// partial output.
func TestInviteFailsClosedOnEmptyRelay(t *testing.T) {
	pubB64, _ := newInviteRootKey(t)
	var out bytes.Buffer
	if err := Invite(&out, pubB64, ""); err == nil {
		t.Fatal("Invite with empty relay = nil error, want reject")
	}
	if out.Len() != 0 {
		t.Fatalf("fail-closed Invite leaked output: %q", out.String())
	}
}
