package cmd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/signer"
)

// signerTestKey mints an ed25519 keypair and seals the private key to a 0600
// file under a SHORT temp dir, returning the key path and the public key. Short
// dirs keep the sibling socket path under the ~104-char darwin UDS limit.
func signerTestKey(t *testing.T) (keyPath string, pub ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dir, err := os.MkdirTemp("", "vmsgn")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	keyPath = filepath.Join(dir, "k.key")
	if err := signer.SealPrivateKey(keyPath, priv); err != nil {
		t.Fatalf("SealPrivateKey: %v", err)
	}
	return keyPath, pub
}

// waitForSocket polls until path exists or the deadline passes; it lets a test
// wait for runIdentitySigner's goroutine to bind before dialing.
func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("socket %q was not bound within deadline", path)
}

// TestRunIdentitySignerRoundTripAndGracefulShutdown starts runIdentitySigner in
// a goroutine, proves a keyless client round-trip signature VERIFIES, then
// cancels the context and asserts the socket file is removed (graceful Close).
func TestRunIdentitySignerRoundTripAndGracefulShutdown(t *testing.T) {
	keyPath, pub := signerTestKey(t)
	sockDir, err := os.MkdirTemp("", "vmsgs")
	if err != nil {
		t.Fatalf("MkdirTemp sock: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "s.sock")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runIdentitySigner(ctx, io.Discard, keyPath, sockPath) }()

	waitForSocket(t, sockPath)

	const entry = `{"agent":"mira","epoch":1}`
	canonical, err := identity.Canonicalize([]byte(entry))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	client := &signer.Client{SocketPath: sockPath}
	sig, err := client.Sign(canonical.Bytes())
	if err != nil {
		t.Fatalf("client.Sign: %v", err)
	}
	ok, err := identity.VerifyCanonical(pub, canonical, sig)
	if err != nil || !ok {
		t.Fatalf("signer signature did not verify: ok=%v err=%v", ok, err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runIdentitySigner returned error on graceful shutdown: %v", err)
	}
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("socket not removed on shutdown: stat err = %v", err)
	}
}

// TestRunIdentitySignerMissingKeyFailsClosed asserts a missing key file makes
// runIdentitySigner return an error and leave no socket behind.
func TestRunIdentitySignerMissingKeyFailsClosed(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmsgnmk")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	keyPath := filepath.Join(dir, "missing.key")
	sockPath := filepath.Join(dir, "s.sock")

	err = runIdentitySigner(context.Background(), io.Discard, keyPath, sockPath)
	if err == nil {
		t.Fatal("expected fail-closed error for missing key file, got nil")
	}
	if _, statErr := os.Stat(sockPath); !os.IsNotExist(statErr) {
		t.Fatalf("missing-key path must not bind a socket: stat err = %v", statErr)
	}
}

// TestRunIdentitySignerEmptyAllowlistRefuses asserts the empty-allowlist guard
// fails closed: an empty uid allowlist must refuse to start (never deny-all
// silently). It drives the low-level builder so the guard is exercised directly.
func TestRunIdentitySignerEmptyAllowlistRefuses(t *testing.T) {
	keyPath, _ := signerTestKey(t)
	sockDir, err := os.MkdirTemp("", "vmsgea")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "s.sock")

	err = serveIdentitySigner(context.Background(), io.Discard, signer.Config{
		KeyPath:     keyPath,
		SocketPath:  sockPath,
		AllowedUIDs: nil,
	})
	if err == nil {
		t.Fatal("expected refusal for empty uid allowlist, got nil")
	}
	if _, statErr := os.Stat(sockPath); !os.IsNotExist(statErr) {
		t.Fatalf("empty-allowlist path must not bind a socket: stat err = %v", statErr)
	}
}

// TestIdentitySignerCommandFailsClosedOnMissingKey drives the cobra entrypoint
// through RootCmd with explicit flags pointed at a MISSING key. This exercises
// runIdentitySignerCmd + resolveSignerPaths + the signal-context wiring while
// failing closed BEFORE Serve blocks (a missing key cannot bind), so the test
// returns promptly and leaves no socket behind.
func TestIdentitySignerCommandFailsClosedOnMissingKey(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmsgncmd")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	keyPath := filepath.Join(dir, "missing.key")
	sockPath := filepath.Join(dir, "s.sock")

	_, _, err = runRootCmd(t, "identity", "signer",
		"--signer-key", keyPath, "--signer-socket", sockPath)
	if err == nil {
		t.Fatal("expected fail-closed error for missing key, got nil")
	}
	if _, statErr := os.Stat(sockPath); !os.IsNotExist(statErr) {
		t.Fatalf("fail-closed path must not bind a socket: stat err = %v", statErr)
	}
}

// errWriter is an io.Writer that always fails, used to drive the startup-line
// write-error fail-closed path in serveIdentitySigner.
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) { return 0, errAlwaysFails }

var errAlwaysFails = io.ErrClosedPipe

// TestResolveSignerPathsFallsBackToXDGDefaults drives resolveSignerPaths with no
// flags set so both empty-path branches resolve the XDG defaults — the default
// resolution branches the flagged command tests skip.
func TestResolveSignerPathsFallsBackToXDGDefaults(t *testing.T) {
	resetFlagsRecursive(identitySignerCmd)

	keyPath, sockPath, err := resolveSignerPaths(identitySignerCmd)
	if err != nil {
		t.Fatalf("resolveSignerPaths: %v", err)
	}
	if !strings.HasSuffix(keyPath, signerKeyFilename) {
		t.Fatalf("key path %q does not end in %q", keyPath, signerKeyFilename)
	}
	if !strings.HasSuffix(sockPath, signerSocketFilename) {
		t.Fatalf("socket path %q does not end in %q", sockPath, signerSocketFilename)
	}
}

// TestServeIdentitySignerStartupWriteErrorFailsClosed proves a failing output
// writer on the startup line makes serveIdentitySigner FAIL CLOSED (return an
// error and Close — socket removed), never leaving a half-started daemon.
func TestServeIdentitySignerStartupWriteErrorFailsClosed(t *testing.T) {
	keyPath, _ := signerTestKey(t)
	sockDir, err := os.MkdirTemp("", "vmsgsw")
	if err != nil {
		t.Fatalf("MkdirTemp sock: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "s.sock")

	err = serveIdentitySigner(context.Background(), errWriter{}, signer.Config{
		KeyPath:     keyPath,
		SocketPath:  sockPath,
		AllowedUIDs: []uint32{uint32(os.Getuid())},
	})
	if err == nil {
		t.Fatal("expected fail-closed error on startup-line write failure, got nil")
	}
	if _, statErr := os.Stat(sockPath); !os.IsNotExist(statErr) {
		t.Fatalf("startup-write-error path must remove the socket: stat err = %v", statErr)
	}
}

// TestIdentitySignerCommandRegistered is a thin smoke test: `identity signer
// --help` succeeds and shows both flags, mirroring the sign-* command tests.
func TestIdentitySignerCommandRegistered(t *testing.T) {
	out, _, err := runRootCmd(t, "identity", "signer", "--help")
	if err != nil {
		t.Fatalf("identity signer --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"signer", "--signer-key", "--signer-socket"} {
		if !strings.Contains(help, want) {
			t.Fatalf("identity signer --help missing %q\n%s", want, help)
		}
	}
}
