package cmd

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/identitycli"
)

// TestIdentityInitCommandWiring drives `identity init` through RootCmd and
// asserts the thin command seals a 0600 key and prints only the public key.
func TestIdentityInitCommandWiring(t *testing.T) {
	keyDir, err := os.MkdirTemp("", "vmcmdinit")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(keyDir) })
	keyPath := filepath.Join(keyDir, "k.key")

	out, _, err := runRootCmd(t, "identity", "init", "--signer-key", keyPath)
	if err != nil {
		t.Fatalf("identity init: %v", err)
	}
	if !strings.Contains(out.String(), identitycli.PubKeyLabel) {
		t.Fatalf("output missing public-key label: %q", out.String())
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("Stat key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key perm = %o, want 0600", perm)
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("ReadFile key: %v", err)
	}
	if len(keyBytes) != ed25519.PrivateKeySize {
		t.Fatalf("sealed key len = %d, want %d", len(keyBytes), ed25519.PrivateKeySize)
	}
}

// TestIdentitySignCommandFailsClosedWhenSignerUnreachable drives `identity sign`
// through RootCmd pointed at a non-existent socket and asserts it fails closed.
func TestIdentitySignCommandFailsClosedWhenSignerUnreachable(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsign")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	entryPath := filepath.Join(dir, "entry.json")
	if err := os.WriteFile(entryPath, []byte(`{"agent":"mira","epoch":1}`), 0o600); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	sockPath := filepath.Join(dir, "nope.sock")

	_, _, err = runRootCmd(t, "identity", "sign", "--file", entryPath, "--signer-socket", sockPath)
	if err == nil {
		t.Fatal("expected fail-closed error when signer unreachable, got nil")
	}
}
