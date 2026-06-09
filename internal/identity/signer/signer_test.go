package signer

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	// AllowedUIDs is non-empty so New reaches loadPrivateKey (the empty-allowlist
	// guard runs FIRST and would otherwise mask the wrong-size-key check).
	_, err := New(Config{KeyPath: bad, SocketPath: filepath.Join(dir, "s.sock"), AllowedUIDs: []uint32{1000}})
	if err == nil {
		t.Fatal("expected error for wrong-size key file")
	}
	if err.Error() != errLoadKeyLen {
		t.Fatalf("expected errLoadKeyLen, got %v", err)
	}
}

func TestNewRejectsMissingKeyFile(t *testing.T) {
	dir := t.TempDir()
	// Non-empty allowlist so we exercise the missing-key path, not the allowlist guard.
	_, err := New(Config{KeyPath: filepath.Join(dir, "nope.key"), SocketPath: filepath.Join(dir, "s.sock"), AllowedUIDs: []uint32{1000}})
	if err == nil {
		t.Fatal("expected error for missing key file")
	}
}

func TestNewRejectsEmptyAllowlist(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	// Valid key + socket, but no AllowedUIDs → the deny-all guard must fail closed
	// at construction (covers the signer.New guard directly, not via the cmd guard).
	_, err := New(Config{KeyPath: keyPath, SocketPath: filepath.Join(t.TempDir(), "s.sock")})
	if err == nil {
		t.Fatal("expected error for empty AllowedUIDs")
	}
	if err.Error() != errEmptyAllowlist {
		t.Fatalf("expected errEmptyAllowlist, got %v", err)
	}
}

func TestNewRejectsPermissiveKeyFile(t *testing.T) {
	// A valid-length ed25519 key written group/world-readable (0644) must be
	// refused — a custody key must be 0600 (sshd-style strict modes).
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dir := t.TempDir()
	loose := filepath.Join(dir, "loose.key")
	if err := os.WriteFile(loose, priv, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err = New(Config{KeyPath: loose, SocketPath: filepath.Join(dir, "s.sock"), AllowedUIDs: []uint32{1000}})
	if err == nil {
		t.Fatal("expected refusal for group/world-readable key file")
	}
	if !strings.Contains(err.Error(), errKeyFilePermissive) {
		t.Fatalf("expected errKeyFilePermissive, got %v", err)
	}
}

func TestListenRefusesWhenLiveSignerPresent(t *testing.T) {
	keyPath, pub := newKeyFile(t)
	// First signer is LIVE (startSigner registers cleanup + returns a client).
	client, sockPath := startSigner(t, keyPath, []uint32{uint32(os.Getuid())})

	// A second signer on the same socket must REFUSE — no silent hijack of a
	// running signer (which would leave the first alive-but-blind).
	s2, err := New(Config{KeyPath: keyPath, SocketPath: sockPath, AllowedUIDs: []uint32{uint32(os.Getuid())}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s2.Listen(); err == nil {
		_ = s2.Close()
		t.Fatal("second Listen must refuse a live signer's socket")
	} else if !strings.Contains(err.Error(), errSignerAlreadyRunning) {
		t.Fatalf("expected errSignerAlreadyRunning, got %v", err)
	}

	// The first signer must still sign — it was not hijacked.
	canonical, err := identity.Canonicalize([]byte(canonicalEntry))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	sig, err := client.Sign(canonical.Bytes())
	if err != nil {
		t.Fatalf("first signer should still sign after the refused second start: %v", err)
	}
	if ok, verr := identity.VerifyCanonical(pub, canonical, sig); !ok || verr != nil {
		t.Fatalf("first signer's signature did not verify: ok=%v err=%v", ok, verr)
	}
}

func TestListenReapsDeadSocket(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	sockPath := shortSocketPath(t)
	// Leave a DEAD socket file (file exists, type socket, no listener) by binding
	// then closing with unlink disabled — simulates a prior crashed run.
	addr, err := net.ResolveUnixAddr(network, sockPath)
	if err != nil {
		t.Fatalf("ResolveUnixAddr: %v", err)
	}
	ln, err := net.ListenUnix(network, addr)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	ln.SetUnlinkOnClose(false)
	_ = ln.Close()
	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("expected a leftover dead socket file: %v", err)
	}

	// Listen should dial-probe (dead → no listener), reap it, and bind cleanly.
	s, err := New(Config{KeyPath: keyPath, SocketPath: sockPath, AllowedUIDs: []uint32{uint32(os.Getuid())}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen should reap a dead socket and bind: %v", err)
	}
	_ = s.Close()
}

func TestListenRefusesNonSocketFile(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	dir := t.TempDir()
	notSock := filepath.Join(dir, "regular.file")
	if err := os.WriteFile(notSock, []byte("important operator data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := New(Config{KeyPath: keyPath, SocketPath: notSock, AllowedUIDs: []uint32{uint32(os.Getuid())}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Listen(); err == nil {
		_ = s.Close()
		t.Fatal("Listen must refuse a non-socket file path (never blind-delete it)")
	} else if !strings.Contains(err.Error(), errSocketPathNotSocket) {
		t.Fatalf("expected errSocketPathNotSocket, got %v", err)
	}
	// The operator's file must NOT have been deleted.
	if _, statErr := os.Stat(notSock); statErr != nil {
		t.Fatalf("non-socket file must be preserved, not deleted: %v", statErr)
	}
}

func TestWriteFrameRejectsOversizePayload(t *testing.T) {
	// A nil conn is fine: the size check fires before any write.
	if err := writeFrame(nil, make([]byte, maxRequestBytes+1)); err == nil {
		t.Fatal("expected error for oversize frame")
	}
}

// startRawServer stands up a raw Unix-domain listener whose single accepted
// connection is driven by handler. It returns the socket path. It exists so the
// client can be tested against DELIBERATELY MALFORMED server responses that a
// well-behaved signer would never produce.
func startRawServer(t *testing.T, handler func(net.Conn)) string {
	t.Helper()
	sockPath := shortSocketPath(t)
	ln, err := net.Listen(network, sockPath)
	if err != nil {
		t.Fatalf("raw listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		handler(conn)
	}()
	return sockPath
}

// assertSignFailsClosed asserts client.Sign returns a non-nil error AND never a
// nil-error-with-a-signature — the load-bearing fail-closed contract.
func assertSignFailsClosed(t *testing.T, sockPath string) {
	t.Helper()
	client := &Client{SocketPath: sockPath}
	sig, err := client.Sign([]byte(`{"agent":"mira","epoch":1}`))
	if err == nil {
		t.Fatalf("expected fail-closed error, got nil (sig=%x)", sig)
	}
	if sig != nil {
		t.Fatalf("fail-closed path returned a non-nil signature: %x", sig)
	}
}

// TestClientFailsClosedOnEmptySocketPath asserts a Client with no SocketPath
// fails closed (non-nil error, nil signature) rather than dialing an empty
// address — a misconfiguration must never silently produce no signature.
func TestClientFailsClosedOnEmptySocketPath(t *testing.T) {
	client := &Client{}
	sig, err := client.Sign([]byte(`{"agent":"mira","epoch":1}`))
	if err == nil {
		t.Fatalf("expected error for empty SocketPath, got nil (sig=%x)", sig)
	}
	if sig != nil {
		t.Fatalf("empty-SocketPath path returned a signature: %x", sig)
	}
}

// TestClientFailsClosedOnMalformedServer drives client.Sign against servers that
// violate the wire protocol. In every case Sign must FAIL CLOSED — these cover
// the read-status / readFrame / response-parse error branches in client.Sign.
func TestClientFailsClosedOnMalformedServer(t *testing.T) {
	tests := []struct {
		name    string
		handler func(net.Conn)
	}{
		{
			// Closes immediately after reading nothing: the client's read of the
			// 1-byte response status hits EOF.
			name:    "closes before any response",
			handler: func(conn net.Conn) { _ = conn.Close() },
		},
		{
			// Sends the status byte, then announces an oversize response frame
			// (> maxRequestBytes): readFrame must reject before allocating.
			name: "oversize response length prefix",
			handler: func(conn net.Conn) {
				_, _ = conn.Write([]byte{respOK})
				var hdr [frameHeaderLen]byte
				binary.BigEndian.PutUint32(hdr[:], maxRequestBytes+1)
				_, _ = conn.Write(hdr[:])
			},
		},
		{
			// Sends status + a header claiming N bytes, then closes early: the
			// client's readFrame payload read hits EOF (truncated frame).
			name: "announces bytes then closes early",
			handler: func(conn net.Conn) {
				_, _ = conn.Write([]byte{respOK})
				var hdr [frameHeaderLen]byte
				binary.BigEndian.PutUint32(hdr[:], 64)
				_, _ = conn.Write(hdr[:])
				_, _ = conn.Write([]byte("short"))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sockPath := startRawServer(t, tc.handler)
			assertSignFailsClosed(t, sockPath)
		})
	}
}

// TestSignerRejectsOversizeRequestOnLiveConn connects raw to a STARTED signer
// and writes a request header announcing 0xFFFFFFFF bytes. The signer's
// read-side readFrame must reject it (n > maxRequestBytes) and reply with a
// respErr frame — no OOM, no hang, no signature.
func TestSignerRejectsOversizeRequestOnLiveConn(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	_, sockPath := startSigner(t, keyPath, []uint32{uint32(os.Getuid())})

	conn, err := net.Dial(network, sockPath)
	if err != nil {
		t.Fatalf("dial signer: %v", err)
	}
	defer func() { _ = conn.Close() }()

	var hdr [frameHeaderLen]byte
	binary.BigEndian.PutUint32(hdr[:], 0xFFFFFFFF)
	if _, err := conn.Write(hdr[:]); err != nil {
		t.Fatalf("write oversize header: %v", err)
	}

	// Read the response: a respErr status byte, then a framed error message.
	status := make([]byte, 1)
	if _, err := io.ReadFull(conn, status); err != nil {
		t.Fatalf("read response status: %v", err)
	}
	if status[0] != respErr {
		t.Fatalf("status = %#x, want respErr (%#x)", status[0], respErr)
	}
	payload, err := readFrame(conn)
	if err != nil {
		t.Fatalf("read error frame: %v", err)
	}
	if !bytes.Contains(payload, []byte(errFrameTooLarge)) {
		t.Fatalf("error frame %q does not mention oversize rejection", string(payload))
	}
}

// TestPolicyHookReceivesCanonicalBytesAndPermissiveSigns asserts two contracts:
//  1. the bytes handed to the Policy hook are EXACTLY the canonical request
//     bytes (so a real policy inspects what is actually signed), and
//  2. a permissive policy (returns nil) still produces a verifying signature.
func TestPolicyHookReceivesCanonicalBytesAndPermissiveSigns(t *testing.T) {
	keyPath, pub := newKeyFile(t)
	cb := canonicalBytes(t)

	var seen []byte
	sockPath := shortSocketPath(t)
	s, err := New(Config{
		KeyPath:     keyPath,
		SocketPath:  sockPath,
		AllowedUIDs: []uint32{uint32(os.Getuid())},
		Policy: func(b []byte) error {
			seen = append([]byte(nil), b...)
			return nil // permissive: must still sign
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = s.Serve() }()
	t.Cleanup(func() { _ = s.Close() })

	client := &Client{SocketPath: sockPath}
	sig, err := client.Sign(cb.Bytes())
	if err != nil {
		t.Fatalf("client.Sign with permissive policy: %v", err)
	}

	if !bytes.Equal(seen, cb.Bytes()) {
		t.Fatalf("policy saw %q, want canonical request bytes %q", seen, cb.Bytes())
	}
	ok, err := identity.VerifyCanonical(pub, cb, sig)
	if err != nil || !ok {
		t.Fatalf("permissive-policy signature did not verify: ok=%v err=%v", ok, err)
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
