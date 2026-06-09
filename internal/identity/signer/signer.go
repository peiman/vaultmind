// Package signer implements the Contract-B DEV-INTERIM custody signer.
//
// The load-bearing property of real custody is that the ed25519 private key
// lives in a SEPARATE process from the CLI: the CLI is KEYLESS and asks the
// signer to sign canonical bytes over a 0600 Unix-domain socket. The signer
// authenticates the caller by deriving its UID via darwin LOCAL_PEERCRED and
// checking it against an allowlist, runs a policy hook, then signs with
// identity.SignCanonical. The private key NEVER leaves this process — it is
// never returned, logged, or exposed.
//
// DEV-INTERIM SCOPE: this is the ARCHITECTURE for real custody, not real
// isolation yet. Today the signer and the CLI run as the SAME UID, so a
// same-uid peer is not actually a different trust domain — the UID allowlist
// gates "which uids may sign" but cannot yet distinguish the keyless CLI from
// any other same-uid process.
//
// TODO(contract-b hardening): real isolation requires the signer to run under a
// DEDICATED SERVICE UID via launchd, in a sandbox, with the key wrapped by the
// Secure Enclave (SEP). Until those land, this provides the API surface and the
// process boundary but NOT a real privilege boundary.
package signer

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// Wire/identity error and protocol constants (SSOT).
const (
	// network is the Unix-domain socket network for both ends.
	network = "unix"

	// keyFilePerm is the required permission for the sealed private-key file.
	keyFilePerm os.FileMode = 0o600
	// socketPerm is the required permission for the listening socket.
	socketPerm os.FileMode = 0o600

	// maxRequestBytes bounds a single signing request so a hostile peer cannot
	// force unbounded allocation. Canonical Contract-B entries are tiny; 1 MiB
	// is far beyond any legitimate entry.
	maxRequestBytes = 1 << 20

	// respOK / respErr are the single-byte response status prefixes.
	respOK  byte = 0x01
	respErr byte = 0x00

	// errSealKeyLen is returned by SealPrivateKey for a wrong-length key.
	errSealKeyLen = "signer: seal: private key must be ed25519.PrivateKeySize bytes"
	// errLoadKeyLen is returned by load for a key file of the wrong size.
	errLoadKeyLen = "signer: load: key file is not ed25519.PrivateKeySize bytes"
	// errUIDNotAllowed is returned when the caller UID is not on the allowlist.
	errUIDNotAllowed = "signer: caller uid not allowed"
	// errPolicyRefused prefixes a policy-hook refusal.
	errPolicyRefused = "signer: policy refused"
	// errNotListening is returned when Serve is called before Listen.
	errNotListening = "signer: not listening (call Listen first)"
	// errEmptyAllowlist is returned when AllowedUIDs is empty. An empty allowlist
	// would deny EVERY caller silently (every request answered errUIDNotAllowed),
	// which is a fail-open-looking deny-all misconfiguration: the signer would
	// run and bind but sign nothing. Fail closed at construction instead.
	errEmptyAllowlist = "signer: AllowedUIDs must list at least one uid (refusing deny-all)"
	// errSignerAlreadyRunning is returned by Listen when a LIVE signer is already
	// answering on the socket path — refusing to start prevents silently
	// hijacking a running signer's socket (which would leave the first daemon
	// alive-but-blind and clients reaching an unexpected key).
	errSignerAlreadyRunning = "signer: already running at socket (refusing to start)"
	// errSocketPathNotSocket is returned by Listen when the socket path exists but
	// is NOT a socket — we never blind-delete an arbitrary file at a typo'd path.
	errSocketPathNotSocket = "signer: path exists and is not a socket (refusing to remove)"
	// errKeyFilePermissive is returned by loadPrivateKey when the key file is
	// group/world-accessible — a custody key whose whole point is confinement must
	// be 0600 (sshd-style strict modes); fail closed so a leaked-perm key is noticed.
	errKeyFilePermissive = "signer: key file is group/world-accessible (refusing to load)"

	// socketDialProbeTimeout bounds the liveness dial-probe in Listen so a
	// misbehaving socket can't wedge startup.
	socketDialProbeTimeout = 1500 * time.Millisecond
)

// PolicyFunc inspects the canonical bytes a caller asked to sign and returns a
// non-nil error to REFUSE signing. It is a seam: the default is permissive
// (signs everything) but the hook is always PRESENT so a future policy can
// refuse dangerous messages without an API change.
type PolicyFunc func(canonicalBytes []byte) error

// Config configures a Signer. KeyPath and SocketPath are required.
type Config struct {
	// KeyPath is the 0600 sealed ed25519 private-key file.
	KeyPath string
	// SocketPath is the 0600 Unix-domain socket the signer listens on.
	SocketPath string
	// AllowedUIDs is the allowlist of caller UIDs permitted to request a
	// signature. A caller whose peer UID is not in this set is denied.
	AllowedUIDs []uint32
	// Policy is the refusal hook. Nil means permissive (sign everything).
	//
	// TODO(contract-b hardening): the nil/permissive default lets any
	// allowlisted caller get arbitrary bytes signed. This is an INTENTIONAL
	// same-uid dev-interim residual, NOT permanent — before the signer moves
	// behind a dedicated service UID, it MUST enforce identity.ValidateSchema
	// (+ optionally re-canonicalize) server-side, because at that point the
	// CLI/caller is across a trust boundary and can no longer be trusted to
	// have validated. Shipping without this under a dedicated-uid deployment
	// would make the signer an arbitrary-bytes signing oracle behind the new
	// privilege boundary.
	Policy PolicyFunc
}

// Signer holds the private key in-process and serves signing requests over a
// 0600 Unix-domain socket. The key is never returned, logged, or exposed.
type Signer struct {
	priv    ed25519.PrivateKey
	cfg     Config
	allowed map[uint32]struct{}

	mu sync.Mutex
	ln *net.UnixListener
}

// New loads the sealed private key and prepares a Signer. It does NOT yet bind
// the socket — call Listen, then Serve. New fails if the key file is missing or
// the wrong size.
func New(cfg Config) (*Signer, error) {
	if cfg.KeyPath == "" {
		return nil, errors.New("signer: KeyPath is required")
	}
	if cfg.SocketPath == "" {
		return nil, errors.New("signer: SocketPath is required")
	}
	if len(cfg.AllowedUIDs) == 0 {
		return nil, errors.New(errEmptyAllowlist)
	}
	priv, err := loadPrivateKey(cfg.KeyPath)
	if err != nil {
		return nil, err
	}
	allowed := make(map[uint32]struct{}, len(cfg.AllowedUIDs))
	for _, u := range cfg.AllowedUIDs {
		allowed[u] = struct{}{}
	}
	return &Signer{priv: priv, cfg: cfg, allowed: allowed}, nil
}

// reapStaleSocket makes SocketPath safe to bind. It FAILS CLOSED rather than
// blindly unlinking: if a LIVE signer is already answering there (dial-probe
// succeeds) it refuses to start (no silent socket hijack / alive-but-blind
// first daemon); if the path is a non-socket file it refuses (never deletes an
// arbitrary file at a typo'd --signer-socket); it removes ONLY a genuinely-dead
// socket left by a prior crash.
func reapStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil // nothing there — free to bind
	}
	if err != nil {
		return fmt.Errorf("signer: stat socket path: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s: %q", errSocketPathNotSocket, path)
	}
	// It IS a socket — is a live signer answering? A successful dial means yes.
	if conn, derr := net.DialTimeout(network, path, socketDialProbeTimeout); derr == nil {
		_ = conn.Close()
		return fmt.Errorf("%s: %q", errSignerAlreadyRunning, path)
	}
	// Dead socket (no listener / connection refused) — safe to reap.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("signer: remove stale socket: %w", err)
	}
	return nil
}

// Listen binds the 0600 Unix-domain socket after reaping only a genuinely-dead
// stale socket (see reapStaleSocket — a live signer at the path makes Listen
// fail closed). The socket is chmod'd to 0600 so only the owning uid can connect
// (the same-uid dev-interim boundary; see package doc).
func (s *Signer) Listen() error {
	// Reap only a genuinely-dead socket; refuse if a LIVE signer is already
	// listening, and never delete a non-socket file at a typo'd path.
	if err := reapStaleSocket(s.cfg.SocketPath); err != nil {
		return err
	}
	addr, err := net.ResolveUnixAddr(network, s.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("signer: resolve socket: %w", err)
	}
	ln, err := net.ListenUnix(network, addr)
	if err != nil {
		return fmt.Errorf("signer: listen: %w", err)
	}
	// TODO(contract-b hardening): under a dedicated service uid + launchd, the
	// socket should be owned by the service uid and group-readable only by the
	// CLI's uid; for now 0600 + same-uid is the interim boundary.
	if err := os.Chmod(s.cfg.SocketPath, socketPerm); err != nil {
		_ = ln.Close()
		return fmt.Errorf("signer: chmod socket: %w", err)
	}
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	return nil
}

// Serve accepts connections until the listener is closed. Each connection is a
// single sign request/response. Serve blocks; run it in a goroutine.
//
// TODO(contract-b hardening): connections are handled sequentially with no
// per-connection read/write deadline — a slow or stalled client blocks the
// entire signer until the connection closes. This is an availability issue
// only (single same-uid caller in the dev-interim model). In the hardening
// slice, set a per-connection deadline via conn.SetDeadline immediately after
// AcceptUnix, and consider handling connections in goroutines if concurrent
// callers become possible.
func (s *Signer) Serve() error {
	s.mu.Lock()
	ln := s.ln
	s.mu.Unlock()
	if ln == nil {
		return errors.New(errNotListening)
	}
	for {
		conn, err := ln.AcceptUnix()
		if err != nil {
			// A closed listener is a clean shutdown (Close was called); any
			// other accept error is surfaced.
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("signer: accept: %w", err)
		}
		s.handle(conn)
	}
}

// Close stops the listener and removes the socket file. The in-memory key is
// dropped when the process exits; Close does not attempt to scrub it (Go's GC
// gives no guarantee, and the dedicated-uid hardening is the real mitigation).
func (s *Signer) Close() error {
	s.mu.Lock()
	ln := s.ln
	s.ln = nil
	s.mu.Unlock()
	if ln == nil {
		return nil
	}
	// Go's net.UnixListener (created via ListenUnix) unlinks the socket file IT
	// created on Close. We rely on that and DO NOT blind-remove by path, which
	// could unlink a successor's live socket (and would mask this Close's error).
	return ln.Close()
}

// handle services one connection: authenticate the peer UID, read the request,
// run policy, sign, and write the response. Errors are reported to the caller
// as a respErr frame, never as a leaked key or a silent success.
//
// TODO(contract-b hardening): handle has no panic recovery — a panicking
// Policy hook would crash the signer process. Add a recover() at the top of
// this function in the hardening slice so a misbehaving hook is converted to
// an error response rather than a process crash.
func (s *Signer) handle(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()

	uid, err := peerUID(conn)
	if err != nil {
		writeErr(conn, fmt.Sprintf("signer: peer uid: %v", err))
		return
	}
	if _, ok := s.allowed[uid]; !ok {
		writeErr(conn, errUIDNotAllowed)
		return
	}

	req, err := readFrame(conn)
	if err != nil {
		writeErr(conn, fmt.Sprintf("signer: read request: %v", err))
		return
	}

	if s.cfg.Policy != nil {
		if perr := s.cfg.Policy(req); perr != nil {
			writeErr(conn, fmt.Sprintf("%s: %v", errPolicyRefused, perr))
			return
		}
	}

	// TODO(contract-b hardening): client-side-only validation is trusted ONLY
	// because the caller and signer run as the same UID today (same-uid
	// dev-interim residual — see the Policy TODO above). Once the signer moves
	// behind a dedicated service UID, "the CLI guarantees JCS-canonical bytes"
	// is no longer a valid trust claim: the CLI is across a privilege boundary
	// and must be treated as untrusted input. At that point, the signer MUST
	// re-validate via identity.ValidateSchema (and optionally re-canonicalize)
	// server-side before signing, or the Policy hook must enforce it.
	//
	// Sign over the received bytes. The key never leaves here.
	sig, err := identity.SignCanonical(s.priv, identity.CanonicalBytesFromTrusted(req))
	if err != nil {
		writeErr(conn, fmt.Sprintf("signer: sign: %v", err))
		return
	}
	writeOK(conn, sig)
}

// loadPrivateKey reads the sealed key file and returns the ed25519 private key.
// It enforces the exact key length so a truncated/corrupt file fails closed.
func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	// Strict modes (sshd-style): refuse a group/world-accessible key file. The
	// custody key's whole point is confinement; loading a 0644 key silently would
	// contradict that. The message names the path + mode only, never contents.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("signer: stat key file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s: %q has mode %#o", errKeyFilePermissive, path, info.Mode().Perm())
	}
	b, err := os.ReadFile(path) // #nosec G304 -- operator-configured sealed signer key path (signer Config), not runtime user input
	if err != nil {
		return nil, fmt.Errorf("signer: read key file: %w", err)
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, errors.New(errLoadKeyLen)
	}
	priv := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	copy(priv, b)
	return priv, nil
}

// SealPrivateKey writes priv to path with 0600 permissions, creating it
// exclusively (O_EXCL) so an existing key is never silently overwritten. It is
// the ONLY blessed way to persist a signer key. The private key is written
// raw; the SEP-wrap is deferred.
//
// TODO(contract-b hardening): wrap the key with the Secure Enclave (SEP) so it
// is non-exfiltratable at rest; today it is a raw 0600 file on disk.
//
// TODO(contract-b hardening): a mid-write error (e.g. disk full after partial
// write) leaves a partial key file at path. In the hardening slice, write to a
// temp file in the same directory, then os.Rename into place atomically so a
// failed write never produces a truncated key file.
func SealPrivateKey(path string, priv ed25519.PrivateKey) error {
	if len(priv) != ed25519.PrivateKeySize {
		return errors.New(errSealKeyLen)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, keyFilePerm) // #nosec G304 -- operator-configured sealed signer key path, created 0600 O_EXCL
	if err != nil {
		return fmt.Errorf("signer: create key file: %w", err)
	}
	if _, err := f.Write(priv); err != nil {
		_ = f.Close()
		return fmt.Errorf("signer: write key file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("signer: close key file: %w", err)
	}
	// Re-assert 0600 in case umask/OS widened the create mode.
	if err := os.Chmod(path, keyFilePerm); err != nil {
		return fmt.Errorf("signer: chmod key file: %w", err)
	}
	return nil
}
