package signer

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/identity"
)

// canonicalEntry is a minimal Contract-B-conformant entry (top-level JSON
// object, ASCII keys, integer/string values) used across the signer tests.
const canonicalEntry = `{"agent":"mira","epoch":1}`

// newKeyFile mints an ed25519 keypair, seals the private key to a 0600 file in a
// temp dir, and returns the file path plus the public key for verification.
func newKeyFile(t *testing.T) (keyPath string, pub ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dir := t.TempDir()
	keyPath = filepath.Join(dir, "signer.key")
	if err := SealPrivateKey(keyPath, priv); err != nil {
		t.Fatalf("SealPrivateKey: %v", err)
	}
	return keyPath, pub
}

// shortSocketPath returns a socket path short enough to fit the ~104-char Unix
// socket path limit (t.TempDir() under /var/folders on darwin can exceed it).
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "vmsk")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

// startSigner boots a Signer on a temp UDS with the given uid allowlist and
// returns a connected client plus the socket path. It registers cleanup.
func startSigner(t *testing.T, keyPath string, allow []uint32) (*Client, string) {
	t.Helper()
	sockPath := shortSocketPath(t)
	s, err := New(Config{
		KeyPath:     keyPath,
		SocketPath:  sockPath,
		AllowedUIDs: allow,
	})
	if err != nil {
		t.Fatalf("New signer: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go s.Serve()
	t.Cleanup(func() { _ = s.Close() })
	return &Client{SocketPath: sockPath}, sockPath
}

func canonicalBytes(t *testing.T) identity.CanonicalBytes {
	t.Helper()
	cb, err := identity.Canonicalize([]byte(canonicalEntry))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	return cb
}

func TestSealPrivateKeyIs0600(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file perm = %o, want 0600", perm)
	}
}

func TestSignerSignsForAllowlistedCaller(t *testing.T) {
	keyPath, pub := newKeyFile(t)
	// Allow the current uid (the test process IS the caller).
	client, _ := startSigner(t, keyPath, []uint32{uint32(os.Getuid())})

	cb := canonicalBytes(t)
	sig, err := client.Sign(cb.Bytes())
	if err != nil {
		t.Fatalf("client.Sign: %v", err)
	}

	ok, err := identity.VerifyCanonical(pub, cb, sig)
	if err != nil {
		t.Fatalf("VerifyCanonical err: %v", err)
	}
	if !ok {
		t.Fatal("signature did not verify")
	}
}

func TestSignerDeniesNonAllowlistedCaller(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	// Allowlist a uid that is NOT the caller's (caller uid + 1 wraps but is fine
	// for a not-equal check; ensure it differs).
	other := uint32(os.Getuid()) + 1
	client, _ := startSigner(t, keyPath, []uint32{other})

	cb := canonicalBytes(t)
	_, err := client.Sign(cb.Bytes())
	if err == nil {
		t.Fatal("expected denial for non-allowlisted caller, got nil error")
	}
}

func TestSignerNeverReturnsKeyBytes(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	client, _ := startSigner(t, keyPath, []uint32{uint32(os.Getuid())})

	priv, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("ReadFile key: %v", err)
	}

	cb := canonicalBytes(t)
	sig, err := client.Sign(cb.Bytes())
	if err != nil {
		t.Fatalf("client.Sign: %v", err)
	}
	// The signature is ed25519.SignatureSize and must not contain the seed.
	if len(sig) != ed25519.SignatureSize {
		t.Fatalf("sig len = %d, want %d", len(sig), ed25519.SignatureSize)
	}
	if containsSub(sig, priv) {
		t.Fatal("signer response leaked private key bytes")
	}
	// The ed25519 seed (first 32 bytes) must also not appear in the response.
	if containsSub(sig, priv[:ed25519.SeedSize]) {
		t.Fatal("signer response leaked private key seed")
	}
}

func TestClientFailsClosedWhenSignerUnreachable(t *testing.T) {
	// Point the client at a socket path that does not exist.
	client := &Client{SocketPath: shortSocketPath(t)}
	cb := canonicalBytes(t)
	_, err := client.Sign(cb.Bytes())
	if err == nil {
		t.Fatal("expected fail-closed error when signer unreachable, got nil")
	}
}

func TestPolicyHookCanRefuse(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	sockPath := shortSocketPath(t)
	denied := errors.New("policy refused")
	s, err := New(Config{
		KeyPath:     keyPath,
		SocketPath:  sockPath,
		AllowedUIDs: []uint32{uint32(os.Getuid())},
		Policy: func(_ []byte) error {
			return denied
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go s.Serve()
	t.Cleanup(func() { _ = s.Close() })

	client := &Client{SocketPath: sockPath}
	cb := canonicalBytes(t)
	if _, err := client.Sign(cb.Bytes()); err == nil {
		t.Fatal("expected policy refusal, got nil error")
	}
}

func TestSocketIs0600(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	_, sockPath := startSigner(t, keyPath, []uint32{uint32(os.Getuid())})
	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("Stat sock: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket perm = %o, want 0600", perm)
	}
}

func TestSealPrivateKeyRejectsWrongLength(t *testing.T) {
	dir := t.TempDir()
	if err := SealPrivateKey(filepath.Join(dir, "k"), []byte("short")); err == nil {
		t.Fatal("expected error for wrong-length private key")
	}
}

func TestSealPrivateKeyRefusesOverwrite(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := SealPrivateKey(keyPath, priv); err == nil {
		t.Fatal("expected O_EXCL refusal to overwrite existing key")
	}
}

func TestNewRequiresKeyAndSocketPaths(t *testing.T) {
	if _, err := New(Config{SocketPath: "/tmp/s.sock"}); err == nil {
		t.Fatal("expected error for missing KeyPath")
	}
	keyPath, _ := newKeyFile(t)
	if _, err := New(Config{KeyPath: keyPath}); err == nil {
		t.Fatal("expected error for missing SocketPath")
	}
}

func TestNewRejectsBadKeyFile(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(bad, []byte("not-a-key"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := New(Config{KeyPath: bad, SocketPath: filepath.Join(dir, "s.sock")}); err == nil {
		t.Fatal("expected error for wrong-size key file")
	}
}

func TestNewRejectsMissingKeyFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := New(Config{KeyPath: filepath.Join(dir, "nope.key"), SocketPath: filepath.Join(dir, "s.sock")}); err == nil {
		t.Fatal("expected error for missing key file")
	}
}

func TestWriteFrameRejectsOversizePayload(t *testing.T) {
	// A nil conn is fine: the size check fires before any write.
	if err := writeFrame(nil, make([]byte, maxRequestBytes+1)); err == nil {
		t.Fatal("expected error for oversize frame")
	}
}

// containsSub reports whether sub appears as a contiguous subsequence of b.
func containsSub(b, sub []byte) bool {
	if len(sub) == 0 || len(sub) > len(b) {
		return false
	}
	for i := 0; i+len(sub) <= len(b); i++ {
		match := true
		for j := range sub {
			if b[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
