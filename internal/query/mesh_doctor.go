package query

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io/fs"
	"net"
	"os"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/doctorclient"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// ── Mesh-doctor: Contract-B identity health (sec-review M1/M3/M4) ────────────
//
// NORTH STAR: a health check whose one job is "am I OK?" must NEVER paint GREEN
// over a compromised — or even misconfigured — substrate. GREEN
// (Authenticated=true) is reserved for what doctor can CRYPTOGRAPHICALLY PROVE:
// a registry verified against a PINNED root (M1) AND a keyless proof that the
// running signer holds the binding key (M3). Everything else is WARNING/INFO and
// Authenticated stays false (M4: top-level boolean + envelope warning).

// Tier-2 status values (SSOT). Each is a distinct, machine-readable verdict.
const (
	// StatusMeshAuthenticated: pinned root verified the registry AND the running
	// signer proved possession of the resolved binding key. The ONLY green state.
	StatusMeshAuthenticated = "authenticated"
	// StatusMeshSelfConsistentUnpinned: no pin; the daemon-advertised root
	// verified its own registry for self-consistency ONLY — NOT authenticated.
	StatusMeshSelfConsistentUnpinned = "self-consistent-unpinned"
	// StatusMeshNotEnrolled: pinned + registry verified, but the member slug has
	// no live binding (enroll-add pending).
	StatusMeshNotEnrolled = "not-enrolled"
	// StatusMeshKeyMismatch: pinned + binding resolved, but the running signer
	// does NOT hold the binding's key (wrong signer/key).
	StatusMeshKeyMismatch = "key-mismatch"
	// StatusMeshUnverifiable: pinned, but the registry failed to verify
	// (bad-sig/stale/rollback) or could not be fetched.
	StatusMeshUnverifiable = "unverifiable"
	// StatusMeshNoSignal: no mesh signal present (caller should not surface the
	// section at all — see HasSignal).
	StatusMeshNoSignal = "no-signal"
)

// Tier-3 daemon-mode values (SSOT). FACTUAL served-state labels only — they do
// NOT imply message-signature protection (enforcement is a no-op today).
const (
	// DaemonModePlaintext: the daemon serves no well-known root (404).
	DaemonModePlaintext = "plaintext"
	// DaemonModeAdvisoryConfigured: a well-known root is served (registry
	// configured). Advisory only — verdict-gating is a return-true no-op today.
	DaemonModeAdvisoryConfigured = "advisory-configured"
	// DaemonModeUnknown: the daemon was unreachable, so its mode is unknown.
	DaemonModeUnknown = ""
)

// Constants governing custody + freshness.
const (
	// keyFilePerm is the only acceptable key-file mode: owner read/write only.
	keyFilePerm os.FileMode = 0o600
	// heartbeatStaleAfter is the freshness window for the watcher heartbeat
	// (~6× the 15s poll). Older than this → the watcher may be present-but-dead.
	heartbeatStaleAfter = 90 * time.Second
	// doctorMaxStaleness bounds how stale a verified registry may be before
	// doctor treats it as unverifiable (a stale registry may hide a revocation).
	doctorMaxStaleness = 24 * time.Hour
	// selfVerifyNonceBytes is the random-challenge length for proof-of-possession.
	selfVerifyNonceBytes = 32
	// selfVerifyDomainTag domain-separates the proof-of-possession challenge so a
	// signed challenge can NEVER be mistaken for an envelope/registry signature.
	selfVerifyDomainTag = "vaultmind-doctor-selfverify-v1:"
	// signerProbeNetwork + signerProbeTimeout bound the on-demand signer dial.
	signerProbeNetwork = "unix"
	signerProbeTimeout = 1500 * time.Millisecond
)

// Warning strings (SSOT) — each drives both the human Warnings slice and an
// envelope.AddWarning in the cmd layer.
const (
	WarnMeshKeyMode          = "identity key file is not 0600 (custody mode is wrong)"
	WarnMeshKeySize          = "identity key file is not the expected ed25519 private-key size"
	WarnMeshUnpinned         = "registry self-consistent (daemon-advertised root), NOT authenticated — enroll persists a pin, or pass --mesh-root-pubkey"
	WarnMeshNotEnrolled      = "your slug is not in the network registry yet (enroll-add pending?)"
	WarnMeshKeyMismatch      = "your binding exists but the running signer does not hold its key (wrong signer/key?)"
	WarnMeshUnverifiable     = "the network registry did not verify against your pinned root (bad signature, stale, or rolled back)"
	WarnMeshNoRegistry       = "no registry available to verify (daemon unreachable and no --mesh-registry given)"
	WarnMeshEnforcementOff   = "message-signature enforcement is NOT YET active (advisory mode is a no-op today)"
	WarnMeshHeartbeatStale   = "wake-watcher heartbeat is stale — the watcher may be present-but-dead"
	WarnMeshHeartbeatMissing = "no wake-watcher heartbeat (wake-on-idle not confirmed)"
)

// MeshSigner is the keyless signing seam: doctor asks the SIGNER (over the UDS)
// to sign a challenge; it NEVER reads the private key file itself. signer.Client
// satisfies this.
type MeshSigner interface {
	Sign(canonicalBytes []byte) ([]byte, error)
}

// MeshDaemonClient is the loopback-pinned daemon-probe seam. doctorclient.Client
// satisfies this.
type MeshDaemonClient interface {
	FetchRoot(ctx context.Context) (doctorclient.WellKnownRoot, error)
	FetchDirectory(ctx context.Context) ([]byte, error)
	Whoami(ctx context.Context) (bool, string, error)
}

// MeshDoctorInput carries everything BuildMeshIdentity needs. The cmd layer
// resolves paths (xdg + flags + env), the anchor pin, the slug, and constructs
// the signer + loopback daemon client.
type MeshDoctorInput struct {
	// Tier-1 custody (STAT-ONLY — never read).
	KeyPath    string
	SocketPath string

	// Tier-2 authenticity.
	PinnedRootPub ed25519.PublicKey // nil ⇒ UNPINNED path (never green)
	NetworkID     string
	RegistryBytes []byte // offline registry override (--mesh-registry); else fetched
	Slug          string
	Signer        MeshSigner // keyless proof-of-possession

	// Tier-3 reachability.
	Daemon        MeshDaemonClient
	HeartbeatPath string

	// Clock seam.
	Now time.Time
}

// DoctorMeshIdentity is the JSON-serializable mesh-health section. It is a
// POINTER on DoctorResult (nil ⇒ absent from --json) so the section appears only
// when a mesh signal exists.
type DoctorMeshIdentity struct {
	// Tier 1 — identity custody (local, keyless).
	KeyPresent      bool `json:"key_present"`
	KeyModeOK       bool `json:"key_mode_ok"`
	KeySizeOK       bool `json:"key_size_ok"`
	SignerReachable bool `json:"signer_reachable"` // INFO — signer is on-demand

	// Tier 2 — binding resolves in the live mesh (authenticated).
	Pinned          bool   `json:"pinned"`
	Authenticated   bool   `json:"authenticated"` // M4 top-level boolean
	NetworkID       string `json:"network_id,omitempty"`
	BindingResolves bool   `json:"binding_resolves"`
	HoldsBindingKey bool   `json:"holds_binding_key"` // selfVerify proof-of-possession
	Status          string `json:"status"`

	// Tier 3 — chat reachability (honest labels).
	DaemonReachable       bool   `json:"daemon_reachable"`
	DaemonMode            string `json:"daemon_mode,omitempty"`
	EnforcementActive     bool   `json:"enforcement_active"` // always false today
	WatcherHeartbeatFresh bool   `json:"watcher_heartbeat_fresh"`
	WatcherHeartbeatAge   int    `json:"watcher_heartbeat_age_secs"`

	// Warnings each also drive an envelope.AddWarning in the cmd layer.
	Warnings []string `json:"warnings,omitempty"`
}

// addWarning appends w to the section's warnings (deduplicated by string).
func (m *DoctorMeshIdentity) addWarning(w string) {
	for _, existing := range m.Warnings {
		if existing == w {
			return
		}
	}
	m.Warnings = append(m.Warnings, w)
}

// BuildMeshIdentity runs the 3-tier Contract-B identity health check and returns
// the populated section. It NEVER reads the private key file (tier-1 is Lstat
// stat-only; the binding-key check is a keyless proof-of-possession via the
// signer). Authenticated is true ONLY on the pinned + resolves + selfVerify-pass
// path (M1+M3); every other state leaves Authenticated false (M4).
func BuildMeshIdentity(ctx context.Context, in MeshDoctorInput) (*DoctorMeshIdentity, error) {
	mi := &DoctorMeshIdentity{
		Status:            StatusMeshNoSignal,
		DaemonMode:        DaemonModeUnknown,
		NetworkID:         in.NetworkID,
		EnforcementActive: false,
	}

	checkKeyCustody(mi, in.KeyPath)
	mi.SignerReachable = signerReachable(in.SocketPath)

	// Tier 3 first (daemon reachability) so tier-2 can reuse a fetched registry.
	registryBytes := evaluateTier3(ctx, mi, in)

	// Tier 2 — authenticity.
	evaluateTier2(ctx, mi, in, registryBytes)

	return mi, nil
}

// checkKeyCustody fills tier-1 booleans from an Lstat of the key path — STAT
// ONLY, the file is never opened/read. Lstat (not Stat) means a SYMLINK at the
// key path is seen as a symlink, not a regular 0600 file, so its mode check
// fails (sshd strict-modes alignment).
func checkKeyCustody(mi *DoctorMeshIdentity, keyPath string) {
	info, err := os.Lstat(keyPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return // not-yet-initialised member: all tier-1 booleans stay false
		}
		// An unexpected stat error (permission on the parent dir, etc.) leaves
		// the booleans false; doctor never reads the key to recover.
		return
	}
	mi.KeyPresent = true
	// A regular file at exactly 0600 (a symlink's Mode() is ModeSymlink, so the
	// regular-file requirement + perm match both fail for a link).
	mi.KeyModeOK = info.Mode().IsRegular() && info.Mode().Perm() == keyFilePerm
	mi.KeySizeOK = info.Size() == int64(ed25519.PrivateKeySize)
	if !mi.KeyModeOK {
		mi.addWarning(WarnMeshKeyMode)
	}
	if !mi.KeySizeOK {
		mi.addWarning(WarnMeshKeySize)
	}
}

// signerReachable dial-probes the signer socket (the reapStaleSocket technique:
// a successful dial means a live signer answers). Signer-not-running is INFO,
// never red — the signer runs on-demand. It NEVER reads the key file.
func signerReachable(socketPath string) bool {
	if socketPath == "" {
		return false
	}
	conn, err := net.DialTimeout(signerProbeNetwork, socketPath, signerProbeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// evaluateTier3 probes the daemon (loopback-pinned) for reachability + factual
// served mode, and checks the watcher heartbeat freshness. It returns any
// registry bytes fetched from the daemon (for tier-2 reuse) — empty when the
// caller supplied an offline registry or the daemon is unreachable/plaintext.
func evaluateTier3(ctx context.Context, mi *DoctorMeshIdentity, in MeshDoctorInput) []byte {
	checkHeartbeat(mi, in.HeartbeatPath, in.Now)

	if in.Daemon == nil {
		return nil
	}
	reachable, _, _ := in.Daemon.Whoami(ctx)
	mi.DaemonReachable = reachable
	if !reachable {
		return nil
	}

	// Factual served mode from the well-known root presence (200 vs 404).
	root, err := in.Daemon.FetchRoot(ctx)
	switch {
	case errors.Is(err, doctorclient.ErrNotConfigured):
		mi.DaemonMode = DaemonModePlaintext
	case err == nil && root.RootPubKey != "":
		mi.DaemonMode = DaemonModeAdvisoryConfigured
		// Advisory/enforcing NEVER implies protection today — say so.
		mi.addWarning(WarnMeshEnforcementOff)
	default:
		mi.DaemonMode = DaemonModeUnknown
	}

	// Reuse a daemon-served registry for tier-2 when no offline registry given.
	if len(in.RegistryBytes) == 0 {
		if dir, derr := in.Daemon.FetchDirectory(ctx); derr == nil {
			return dir
		}
	}
	return nil
}

// checkHeartbeat reads the watcher heartbeat file's mtime and sets freshness —
// FRESHNESS, not a process check. Absent ⇒ not confirmed; stale ⇒ warning.
func checkHeartbeat(mi *DoctorMeshIdentity, heartbeatPath string, now time.Time) {
	if heartbeatPath == "" {
		return
	}
	info, err := os.Lstat(heartbeatPath)
	if err != nil {
		mi.addWarning(WarnMeshHeartbeatMissing)
		return
	}
	age := now.Sub(info.ModTime())
	if age < 0 {
		age = 0
	}
	mi.WatcherHeartbeatAge = int(age.Seconds())
	if age <= heartbeatStaleAfter {
		mi.WatcherHeartbeatFresh = true
		return
	}
	mi.addWarning(WarnMeshHeartbeatStale)
}

// evaluateTier2 runs the authenticity check. registryBytes is the daemon-fetched
// registry (when no offline override). The UNPINNED path (no pin) verifies only
// for self-consistency and NEVER goes green (M1).
func evaluateTier2(ctx context.Context, mi *DoctorMeshIdentity, in MeshDoctorInput, registryBytes []byte) {
	regBytes := in.RegistryBytes
	if len(regBytes) == 0 {
		regBytes = registryBytes
	}

	if len(in.PinnedRootPub) == 0 {
		evaluateUnpinned(ctx, mi, in, regBytes)
		return
	}
	mi.Pinned = true
	evaluatePinned(mi, in, regBytes)
}

// evaluateUnpinned (M1): with NO pin, verify the registry against the
// DAEMON-ADVERTISED root for self-consistency ONLY. Authenticated stays false;
// status is self-consistent-unpinned with a loud warning. Never green.
func evaluateUnpinned(ctx context.Context, mi *DoctorMeshIdentity, in MeshDoctorInput, regBytes []byte) {
	mi.Status = StatusMeshSelfConsistentUnpinned
	mi.addWarning(WarnMeshUnpinned)

	if in.Daemon == nil || len(regBytes) == 0 {
		return
	}
	root, err := in.Daemon.FetchRoot(ctx)
	if err != nil || root.RootPubKey == "" {
		return
	}
	advertised, err := base64.StdEncoding.DecodeString(root.RootPubKey)
	if err != nil {
		return
	}
	// Self-consistency check ONLY — this proves the daemon's registry was signed
	// by the daemon's own advertised root. It authenticates NOTHING (a malicious
	// daemon serves a matched {evil_root, evil_registry}). Authenticated remains
	// false; we record NetworkID for display only.
	if _, _, verr := registry.VerifyAndLoad(advertised, mustParse(regBytes), 0, in.Now, doctorMaxStaleness); verr == nil {
		if mi.NetworkID == "" {
			mi.NetworkID = root.NetworkID
		}
	}
}

// evaluatePinned runs the authenticated path: VerifyAndLoad against the PINNED
// root, Resolve the slug, then a keyless selfVerify of the binding key. Green
// only on the full pass.
func evaluatePinned(mi *DoctorMeshIdentity, in MeshDoctorInput, regBytes []byte) {
	if len(regBytes) == 0 {
		mi.Status = StatusMeshUnverifiable
		mi.addWarning(WarnMeshNoRegistry)
		return
	}
	env, err := registry.ParseDistribution(regBytes)
	if err != nil {
		mi.Status = StatusMeshUnverifiable
		mi.addWarning(WarnMeshUnverifiable)
		return
	}
	reg, _, err := registry.VerifyAndLoad(in.PinnedRootPub, env, 0, in.Now, doctorMaxStaleness)
	if err != nil {
		mi.Status = StatusMeshUnverifiable
		mi.addWarning(WarnMeshUnverifiable)
		return
	}
	binding, err := registry.Resolve(reg, in.Slug, in.Now)
	if err != nil {
		mi.Status = StatusMeshNotEnrolled
		mi.addWarning(WarnMeshNotEnrolled)
		return
	}
	mi.BindingResolves = true

	// M3 keyless proof-of-possession: ask the signer to sign a fresh
	// domain-tagged random challenge, verify against the RESOLVED BINDING pubkey.
	// The private key file is NEVER read.
	if selfVerifyBindingKey(in.Signer, binding.PubKey.Bytes()) {
		mi.HoldsBindingKey = true
		mi.Authenticated = true
		mi.Status = StatusMeshAuthenticated
		return
	}
	mi.Status = StatusMeshKeyMismatch
	mi.addWarning(WarnMeshKeyMismatch)
}

// selfVerifyBindingKey performs the keyless proof-of-possession: it builds a
// fresh random domain-tagged challenge, asks the signer to sign it, and verifies
// the signature against bindingPub. A nil signer, a signer error, or a non-match
// all return false (fail closed). NO private-key file access occurs anywhere.
func selfVerifyBindingKey(signer MeshSigner, bindingPub ed25519.PublicKey) bool {
	if signer == nil {
		return false
	}
	nonce := make([]byte, selfVerifyNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return false
	}
	challenge := buildSelfVerifyChallenge(nonce)
	sig, err := signer.Sign(challenge.Bytes())
	if err != nil {
		return false
	}
	ok, err := identity.VerifyCanonical(bindingPub, challenge, sig)
	return err == nil && ok
}

// buildSelfVerifyChallenge domain-separates a random nonce so the signed bytes
// can NEVER be replayed as an envelope/registry signature. The result is fed to
// the signer (Sign over .Bytes()) and to VerifyCanonical.
func buildSelfVerifyChallenge(nonce []byte) identity.CanonicalBytes {
	tagged := make([]byte, 0, len(selfVerifyDomainTag)+len(nonce))
	tagged = append(tagged, []byte(selfVerifyDomainTag)...)
	tagged = append(tagged, nonce...)
	return identity.CanonicalBytesFromTrusted(tagged)
}

// mustParse parses regBytes, returning a zero envelope on failure (the caller's
// VerifyAndLoad then rejects it — fail closed). Used only on the unpinned
// self-consistency path where a parse failure simply yields "no self-consistency
// shown".
func mustParse(regBytes []byte) registry.SignedRegistry {
	env, err := registry.ParseDistribution(regBytes)
	if err != nil {
		return registry.SignedRegistry{}
	}
	return env
}

// HasSignal reports whether any mesh signal exists, so the cmd layer can decide
// whether to attach the section at all (nil ⇒ absent from --json).
func (m *DoctorMeshIdentity) HasSignal() bool {
	return m != nil && m.Status != StatusMeshNoSignal
}

// MeshSurfacedCounts returns the error/warning counts the mesh section
// contributes to the doctor rollup. Per M4 + doctor convention, mesh issues are
// WARNINGS (exit stays 0): every section Warnings entry counts as one warning.
func MeshSurfacedCounts(m *DoctorMeshIdentity) (errs, warns int) {
	if m == nil {
		return 0, 0
	}
	return 0, len(m.Warnings)
}
