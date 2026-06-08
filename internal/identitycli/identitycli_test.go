package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/signer"
)

const testEntry = `{"agent":"mira","epoch":1}`

// fakeSignerClient signs locally with a held key and records the bytes it
// received. It lets the test prove SignEntry produces a verifying signature
// WITHOUT ever opening a key file — the only key here is reached through the
// SignerClient seam, not via any file read in identitycli.
type fakeSignerClient struct {
	priv         ed25519.PrivateKey
	gotCanonical []byte
	failErr      error
}

func (f *fakeSignerClient) Sign(canonicalBytes []byte) ([]byte, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	f.gotCanonical = append([]byte(nil), canonicalBytes...)
	return ed25519.Sign(f.priv, canonicalBytes), nil
}

type sentinelErr string

func (e sentinelErr) Error() string { return string(e) }

func shortSock(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "vmic")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func TestSignEntryProducesVerifyingSignatureKeylessly(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fake := &fakeSignerClient{priv: priv}

	var out bytes.Buffer
	if err := SignEntry(&out, fake, []byte(testEntry)); err != nil {
		t.Fatalf("SignEntry: %v", err)
	}

	wantCanonical, err := identity.Canonicalize([]byte(testEntry))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if !bytes.Equal(fake.gotCanonical, wantCanonical.Bytes()) {
		t.Fatalf("client got %q, want canonical %q", fake.gotCanonical, wantCanonical.Bytes())
	}

	sig := decodeSig(t, out.String())
	ok, err := identity.VerifyCanonical(pub, wantCanonical, sig)
	if err != nil || !ok {
		t.Fatalf("signature did not verify: ok=%v err=%v", ok, err)
	}
}

func TestSignEntryRejectsInvalidSchema(t *testing.T) {
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	// A bare array is not a Contract-B object -> schema reject; signer not called.
	if err := SignEntry(&out, fake, []byte(`[1,2,3]`)); err == nil {
		t.Fatal("expected schema rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for schema-invalid input")
	}
	if out.Len() != 0 {
		t.Fatalf("schema reject must print nothing, got %q", out.String())
	}
}

func TestSignEntryFailsClosedOnSignerError(t *testing.T) {
	fake := &fakeSignerClient{failErr: sentinelErr("signer unreachable")}
	var out bytes.Buffer
	if err := SignEntry(&out, fake, []byte(testEntry)); err == nil {
		t.Fatal("expected fail-closed error, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("fail-closed must print nothing, got %q", out.String())
	}
}

// TestSignEntryEndToEndKeyless boots a real signer and drives SignEntry through
// the real Client: the only process holding the key is the signer.
func TestSignEntryEndToEndKeyless(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	keyDir, err := os.MkdirTemp("", "vmkey")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(keyDir) })
	keyPath := filepath.Join(keyDir, "k.key")
	if err := signer.SealPrivateKey(keyPath, priv); err != nil {
		t.Fatalf("SealPrivateKey: %v", err)
	}

	sockPath := shortSock(t)
	s, err := signer.New(signer.Config{
		KeyPath:     keyPath,
		SocketPath:  sockPath,
		AllowedUIDs: []uint32{uint32(os.Getuid())},
	})
	if err != nil {
		t.Fatalf("signer.New: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go s.Serve()
	t.Cleanup(func() { _ = s.Close() })

	client := &signer.Client{SocketPath: sockPath}
	var out bytes.Buffer
	if err := SignEntry(&out, client, []byte(testEntry)); err != nil {
		t.Fatalf("SignEntry: %v", err)
	}

	wantCanonical, _ := identity.Canonicalize([]byte(testEntry))
	ok, err := identity.VerifyCanonical(pub, wantCanonical, decodeSig(t, out.String()))
	if err != nil || !ok {
		t.Fatalf("e2e signature did not verify: ok=%v err=%v", ok, err)
	}
}

func TestInitWritesSealed0600KeyAndNeverEmitsPrivate(t *testing.T) {
	keyDir, err := os.MkdirTemp("", "vminit")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(keyDir) })
	keyPath := filepath.Join(keyDir, "k.key")

	var out bytes.Buffer
	if err := Init(&out, keyPath); err != nil {
		t.Fatalf("Init: %v", err)
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

	outStr := out.String()
	if !strings.Contains(outStr, PubKeyLabel) {
		t.Fatalf("output missing public-key label: %q", outStr)
	}
	if strings.Contains(outStr, base64.StdEncoding.EncodeToString(keyBytes)) {
		t.Fatal("init leaked the private key (base64) to stdout")
	}
	if strings.Contains(outStr, base64.StdEncoding.EncodeToString(keyBytes[:ed25519.SeedSize])) {
		t.Fatal("init leaked the private key seed (base64) to stdout")
	}

	priv := ed25519.PrivateKey(keyBytes)
	wantPub := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	if !strings.Contains(outStr, wantPub) {
		t.Fatalf("printed public key does not match sealed key; out=%q", outStr)
	}
}

func TestInitRefusesOverwrite(t *testing.T) {
	keyDir, err := os.MkdirTemp("", "vminit2")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(keyDir) })
	keyPath := filepath.Join(keyDir, "k.key")

	var out bytes.Buffer
	if err := Init(&out, keyPath); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if err := Init(&out, keyPath); err == nil {
		t.Fatal("second Init must refuse to overwrite existing key")
	}
}

func decodeSig(t *testing.T, output string) []byte {
	t.Helper()
	line := strings.TrimSpace(output)
	if !strings.HasPrefix(line, SigLabel) {
		t.Fatalf("output %q missing prefix %q", line, SigLabel)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, SigLabel))
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	return sig
}
