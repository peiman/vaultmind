package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
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

// waitForSocket waits for a listening socket, not just its directory entry:
// the path can exist briefly before the listener accepts connections.
func waitForSocket(t *testing.T, path string) {
	t.Helper()
	// Generous: the wait ends at the first successful dial, so the deadline
	// only matters when the signer never listens. 2 s was a guess that a
	// suite under a load average above 100 can exceed (#180).
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("unix", path, 50*time.Millisecond); err == nil {
			_ = conn.Close()
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
	defer cancel() // no goroutine leak if an assertion below fails before the explicit cancel
	done := make(chan error, 1)
	var out bytes.Buffer
	go func() { done <- runIdentitySigner(ctx, &out, keyPath, sockPath) }()

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

	// 'Prints only non-secret lines': the socket path appears; the raw key bytes
	// never do. Safe to read out after <-done (the goroutine's writes happen-before
	// the send on done).
	logged := out.String()
	if !strings.Contains(logged, sockPath) {
		t.Fatalf("startup line should name the socket path; got %q", logged)
	}
	keyBytes, rerr := os.ReadFile(keyPath)
	if rerr != nil {
		t.Fatalf("read key file: %v", rerr)
	}
	if bytes.Contains(out.Bytes(), keyBytes) {
		t.Fatal("daemon output leaked raw key bytes")
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

// startOnceSignerCmd runs serveIdentitySignerOnce in the background on a fresh
// key and socket, returning the socket path, the public key, the stderr buffer
// and the channel that receives its return value.
func startOnceSignerCmd(t *testing.T, ctx context.Context) (sockPath string, pub ed25519.PublicKey, errOut *bytes.Buffer, done chan error) {
	t.Helper()
	keyPath, pub := signerTestKey(t)
	sockDir, err := os.MkdirTemp("", "vmsgo")
	if err != nil {
		t.Fatalf("MkdirTemp sock: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath = filepath.Join(sockDir, "s.sock")
	errOut = &bytes.Buffer{}
	done = make(chan error, 1)
	cfg := currentUserSignerConfig(keyPath, sockPath)
	go func() { done <- serveIdentitySignerOnce(ctx, io.Discard, errOut, cfg) }()
	waitForSocket(t, sockPath)
	return sockPath, pub, errOut, done
}

// TestServeIdentitySignerOncePrintsHashOfSignedBytesAndExits signs one request,
// then checks the run ended cleanly, removed its socket, printed the sha256 of
// exactly the bytes that were signed, and refuses a second request.
func TestServeIdentitySignerOncePrintsHashOfSignedBytesAndExits(t *testing.T) {
	sockPath, pub, errOut, done := startOnceSignerCmd(t, context.Background())

	canonical, err := identity.Canonicalize([]byte(`{"agent":"mira","epoch":1}`))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	client := &signer.Client{SocketPath: sockPath}
	sig, err := client.Sign(canonical.Bytes())
	if err != nil {
		t.Fatalf("client.Sign: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveIdentitySignerOnce: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("once-mode signer did not exit after its signature")
	}

	sum := sha256.Sum256(canonical.Bytes())
	want := fmt.Sprintf("signed once: sha256=%s bytes=%d\n", hex.EncodeToString(sum[:]), len(canonical.Bytes()))
	if errOut.String() != want {
		t.Fatalf("stderr = %q, want %q", errOut.String(), want)
	}
	if ok, err := identity.VerifyCanonical(pub, canonical, sig); err != nil || !ok {
		t.Fatalf("signature does not verify over the hashed bytes: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("socket not removed: stat err = %v", err)
	}
	if _, err := client.Sign([]byte(`{"agent":"evil","epoch":1}`)); err == nil {
		t.Fatal("a second request was signed after the once-mode run")
	}
}

// TestServeIdentitySignerOnceSignalBeforeSigning stops a once-mode run with
// nothing signed: it exits cleanly, prints no hash, and removes the socket.
func TestServeIdentitySignerOnceSignalBeforeSigning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sockPath, _, errOut, done := startOnceSignerCmd(t, ctx)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serveIdentitySignerOnce after signal: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("printed %q although nothing was signed", errOut.String())
	}
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("socket not removed: stat err = %v", err)
	}
}

// TestServeIdentitySignerOnceGivesUpAfterFailures checks the failure bound: a
// run that only sees failed requests exits non-zero with nothing signed.
func TestServeIdentitySignerOnceGivesUpAfterFailures(t *testing.T) {
	sockPath, _, errOut, done := startOnceSignerCmd(t, context.Background())
	for i := 0; i < signer.OnceMaxFailures; i++ {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("Dial %d: %v", i, err)
		}
		_, _ = conn.Write([]byte{0xff, 0xff, 0xff, 0xff}) // oversize frame: refused
		_, _ = io.ReadAll(conn)
		_ = conn.Close()
	}
	if err := <-done; err == nil {
		t.Fatal("expected an error after only failed requests, got nil")
	}
	if errOut.Len() != 0 {
		t.Fatalf("printed %q although nothing was signed", errOut.String())
	}
}

// TestServeIdentitySignerOnceFailsClosedOnEmptyAllowlist keeps the startup
// guards in once mode.
func TestServeIdentitySignerOnceFailsClosedOnEmptyAllowlist(t *testing.T) {
	keyPath, _ := signerTestKey(t)
	err := serveIdentitySignerOnce(context.Background(), io.Discard, io.Discard, signer.Config{KeyPath: keyPath, SocketPath: "/nonexistent/s.sock"})
	if err == nil {
		t.Fatal("expected refusal for empty uid allowlist, got nil")
	}
}

// TestIdentitySignerHelpDocumentsOnce shows --once and its rules in help.
func TestIdentitySignerHelpDocumentsOnce(t *testing.T) {
	out, _, err := runRootCmd(t, "identity", "signer", "--help")
	if err != nil {
		t.Fatalf("identity signer --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"--once", "signed once: sha256=", "do not count"} {
		if !strings.Contains(help, want) {
			t.Fatalf("identity signer --help missing %q\n%s", want, help)
		}
	}
}

// TestIdentitySignerCommandOnceFailsClosedOnMissingKey drives --once through
// RootCmd so the flag wiring is exercised; a missing key fails before serving.
func TestIdentitySignerCommandOnceFailsClosedOnMissingKey(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmsgnoc")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	_, _, err = runRootCmd(t, "identity", "signer", "--once",
		"--signer-key", filepath.Join(dir, "missing.key"), "--signer-socket", filepath.Join(dir, "s.sock"))
	if err == nil {
		t.Fatal("expected fail-closed error for missing key, got nil")
	}
}

// TestServeIdentitySignerOnceSignalAfterSignature: a signal right after the
// signature must not lose the hash line, and the line appears exactly once.
func TestServeIdentitySignerOnceSignalAfterSignature(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sockPath, _, errOut, done := startOnceSignerCmd(t, ctx)
	msg := []byte(`{"agent":"mira","epoch":1}`)
	if _, err := (&signer.Client{SocketPath: sockPath}).Sign(msg); err != nil {
		t.Fatalf("client.Sign: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serveIdentitySignerOnce: %v", err)
	}
	sum := sha256.Sum256(msg)
	want := fmt.Sprintf("signed once: sha256=%s bytes=%d\n", hex.EncodeToString(sum[:]), len(msg))
	if errOut.String() != want {
		t.Fatalf("stderr = %q, want exactly %q", errOut.String(), want)
	}
}

// TestIdentitySignerCommandOnceReachesOnceMode drives `identity signer --once`
// through RootCmd with a real key: the flag must select once mode, so the run
// announces it, prints the hash, refuses a second request and exits 0.
func TestIdentitySignerCommandOnceReachesOnceMode(t *testing.T) {
	keyPath, pub := signerTestKey(t)
	sockDir, err := os.MkdirTemp("", "vmsgrc")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "s.sock")

	type result struct {
		out, errOut *bytes.Buffer
		err         error
	}
	done := make(chan result, 1)
	go func() {
		out, errOut, err := runRootCmd(t, "identity", "signer", "--once",
			"--signer-key", keyPath, "--signer-socket", sockPath)
		done <- result{out, errOut, err}
	}()
	waitForSocket(t, sockPath)

	canonical, err := identity.Canonicalize([]byte(`{"agent":"mira","epoch":1}`))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	client := &signer.Client{SocketPath: sockPath}
	sig, err := client.Sign(canonical.Bytes())
	if err != nil {
		t.Fatalf("client.Sign: %v", err)
	}
	var r result
	select {
	case r = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("identity signer --once did not exit after its signature")
	}
	if r.err != nil {
		t.Fatalf("identity signer --once: %v", r.err)
	}
	if !strings.Contains(r.out.String(), "(once mode: will sign ONE request)") {
		t.Fatalf("startup line does not announce once mode: %q", r.out.String())
	}
	sum := sha256.Sum256(canonical.Bytes())
	want := fmt.Sprintf("signed once: sha256=%s bytes=%d", hex.EncodeToString(sum[:]), len(canonical.Bytes()))
	if !strings.Contains(r.errOut.String(), want) {
		t.Fatalf("stderr %q does not contain %q", r.errOut.String(), want)
	}
	if ok, err := identity.VerifyCanonical(pub, canonical, sig); err != nil || !ok {
		t.Fatalf("signature does not verify: ok=%v err=%v", ok, err)
	}
	if _, err := client.Sign([]byte(`{"agent":"evil","epoch":1}`)); err == nil {
		t.Fatal("a second request was signed after identity signer --once")
	}
}
