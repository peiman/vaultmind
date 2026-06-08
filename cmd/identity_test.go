package cmd

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/envelope"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/signer"
	"github.com/peiman/vaultmind/internal/identitycli"
)

// startCmdSigner boots a real signer on a short UDS allowlisting the test uid
// and returns the socket path plus the public key for verification. It is the
// fixture the cmd-level sign tests use to drive `identity sign` end to end.
func startCmdSigner(t *testing.T) (sockPath string, pub ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	// Short dirs: a t.TempDir() under /var/folders can exceed the ~104-char UDS
	// path limit on darwin.
	keyDir, err := os.MkdirTemp("", "vmck")
	if err != nil {
		t.Fatalf("MkdirTemp key: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(keyDir) })
	keyPath := filepath.Join(keyDir, "k.key")
	if err := signer.SealPrivateKey(keyPath, priv); err != nil {
		t.Fatalf("SealPrivateKey: %v", err)
	}
	sockDir, err := os.MkdirTemp("", "vmcs")
	if err != nil {
		t.Fatalf("MkdirTemp sock: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath = filepath.Join(sockDir, "s.sock")
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
	go func() { _ = s.Serve() }()
	t.Cleanup(func() { _ = s.Close() })
	return sockPath, pub
}

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

// TestIdentitySignCommandViaStdin drives `identity sign` reading the entry from
// STDIN (the default agent flow — no --file) against a live signer and asserts a
// VERIFYING signature is printed. This covers readIdentityEntryJSON's stdin
// branch, which only --file tests previously exercised.
func TestIdentitySignCommandViaStdin(t *testing.T) {
	sockPath, pub := startCmdSigner(t)
	const entry = `{"agent":"mira","epoch":1}`

	RootCmd.SetIn(strings.NewReader(entry))
	defer RootCmd.SetIn(os.Stdin)

	out, _, err := runRootCmd(t, "identity", "sign", "--signer-socket", sockPath)
	if err != nil {
		t.Fatalf("identity sign via stdin: %v", err)
	}

	line := strings.TrimSpace(out.String())
	if !strings.HasPrefix(line, identitycli.SigLabel) {
		t.Fatalf("output %q missing signature label %q", line, identitycli.SigLabel)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, identitycli.SigLabel))
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	canonical, err := identity.Canonicalize([]byte(entry))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	ok, err := identity.VerifyCanonical(pub, canonical, sig)
	if err != nil || !ok {
		t.Fatalf("stdin-signed signature did not verify: ok=%v err=%v", ok, err)
	}
}

// TestIdentitySignCommandFileNotFound asserts `identity sign --file <missing>`
// FAILS CLOSED with a clear error and prints no signature, rather than silently
// falling back to stdin or emitting an empty result.
func TestIdentitySignCommandFileNotFound(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsignnf")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	missing := filepath.Join(dir, "does-not-exist.json")

	// No --signer-socket: this also exercises runIdentitySign's default-socket
	// resolution branch (socket is resolved before the entry is read).
	out, _, err := runRootCmd(t, "identity", "sign", "--file", missing)
	if err == nil {
		t.Fatal("expected error for nonexistent --file, got nil")
	}
	if strings.Contains(out.String(), identitycli.SigLabel) {
		t.Fatalf("file-not-found path must not print a signature, got %q", out.String())
	}
}

// TestIdentitySignEnvelopeCommandViaStdin drives `identity sign-envelope` reading
// the envelope from STDIN against a live signer, then proves the emitted sig
// verifies under a registry binding via VerifyEnvelope (the daemon's check).
func TestIdentitySignEnvelopeCommandViaStdin(t *testing.T) {
	sockPath, pub := startCmdSigner(t)
	const env = `{"alg_version":1,"body":"hello","from_agent":"mira","key_epoch":1,"nonce":"YWJjZGVmZ2hpamtsbW5vcA==","room":"dev","seq":7,"ts":2000000}`

	RootCmd.SetIn(strings.NewReader(env))
	defer RootCmd.SetIn(os.Stdin)

	out, _, err := runRootCmd(t, "identity", "sign-envelope",
		"--signer-socket", sockPath, "--from-pubkey", base64.StdEncoding.EncodeToString(pub))
	if err != nil {
		t.Fatalf("identity sign-envelope via stdin: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, out.String())
	}
	sig, err := base64.StdEncoding.DecodeString(res[envelope.FieldSig].(string))
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}

	pk, err := registry.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	reg := registry.Registry{
		Epoch: 1, ValidFrom: 0, ValidUntil: 1 << 40,
		Agents: []registry.AgentBinding{{
			Slug: "mira", DisplayName: "Mira", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 0, ValidUntil: 1 << 40,
		}},
	}
	room := "dev"
	ok, verr := envelope.VerifyEnvelope(reg, envelope.Fields{
		AlgVersion: 1, Body: "hello", FromAgent: "mira", KeyEpoch: 1,
		Nonce: "YWJjZGVmZ2hpamtsbW5vcA==", Room: &room, Seq: 7, TS: 2000000,
	}, sig, time.Unix(2000000, 0))
	if verr != nil || !ok {
		t.Fatalf("cmd-signed envelope did not verify: ok=%v err=%v", ok, verr)
	}
}

// TestIdentitySignEnvelopeCommandFailsClosedWhenSignerUnreachable drives the
// command at a non-existent socket and asserts it fails closed (no output).
func TestIdentitySignEnvelopeCommandFailsClosedWhenSignerUnreachable(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsignenv")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	envPath := filepath.Join(dir, "env.json")
	const env = `{"alg_version":1,"body":"hello","from_agent":"mira","key_epoch":1,"nonce":"abc","room":"dev","seq":1,"ts":1}`
	if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
	sockPath := filepath.Join(dir, "nope.sock")

	out, _, err := runRootCmd(t, "identity", "sign-envelope", "--file", envPath, "--signer-socket", sockPath)
	if err == nil {
		t.Fatal("expected fail-closed error when signer unreachable, got nil")
	}
	if strings.Contains(out.String(), envelope.FieldSig) {
		t.Fatalf("fail-closed path must not print a result, got %q", out.String())
	}
}

// TestIdentitySignEnvelopeCommandFileNotFound asserts the --file branch fails
// closed (and exercises runIdentitySignEnvelope's default-socket resolution,
// since no --signer-socket is passed) when the envelope file is missing.
func TestIdentitySignEnvelopeCommandFileNotFound(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsignenvnf")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	missing := filepath.Join(dir, "does-not-exist.json")

	out, _, err := runRootCmd(t, "identity", "sign-envelope", "--file", missing)
	if err == nil {
		t.Fatal("expected error for nonexistent --file, got nil")
	}
	if strings.Contains(out.String(), envelope.FieldSig) {
		t.Fatalf("file-not-found path must not print a result, got %q", out.String())
	}
}

// TestDefaultSignerPathsResolveUnderXDG asserts the XDG path helpers return
// non-empty, distinct paths ending in the SSOT filenames. This covers the
// happy path of defaultSignerKeyPath / defaultSignerSocketPath.
func TestDefaultSignerPathsResolveUnderXDG(t *testing.T) {
	keyPath, err := defaultSignerKeyPath()
	if err != nil {
		t.Fatalf("defaultSignerKeyPath: %v", err)
	}
	if !strings.HasSuffix(keyPath, signerKeyFilename) {
		t.Fatalf("key path %q does not end in %q", keyPath, signerKeyFilename)
	}

	sockPath, err := defaultSignerSocketPath()
	if err != nil {
		t.Fatalf("defaultSignerSocketPath: %v", err)
	}
	if !strings.HasSuffix(sockPath, signerSocketFilename) {
		t.Fatalf("socket path %q does not end in %q", sockPath, signerSocketFilename)
	}

	if keyPath == sockPath {
		t.Fatal("key and socket default paths must differ")
	}
}
