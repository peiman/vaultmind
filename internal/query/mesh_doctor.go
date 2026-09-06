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
	"strings"
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
	WarnMeshKeyMode            = "identity key file is not 0600 (custody mode is wrong)"
	WarnMeshKeySize            = "identity key file is not the expected ed25519 private-key size"
	WarnMeshUnpinned           = "registry self-consistent (daemon-advertised root), NOT authenticated — enroll persists a pin, or pass --mesh-root-pubkey"
	WarnMeshNotEnrolled        = "your slug is not in the network registry yet (enroll-add pending?)"
	WarnMeshKeyMismatch        = "your binding exists but the running signer does not hold its key (wrong signer/key?)"
	WarnMeshUnverifiable       = "the network registry did not verify against your pinned root (bad signature, stale, or rolled back)"
	WarnMeshNoRegistry         = "no registry available to verify (daemon unreachable and no --mesh-registry given)"
	WarnMeshEnforcementOff     = "message-signature enforcement is NOT YET active (advisory mode is a no-op today)"
	WarnMeshHeartbeatStale     = "wake-watcher heartbeat is stale — the watcher may be present-but-dead"
	WarnMeshFreshnessUnchecked = "registry freshness UNCHECKED — no registry to read (declare registry_path in agents.yaml); staleness is UNKNOWN, not OK"
	// WarnMeshRegistryUnreadable: a path WAS declared and could not be read.
	// A typo'd or rotated registry_path is a config error; degrading it to
	// silence gives the same output as never having declared one.
	WarnMeshRegistryUnreadable = "the declared registry file could not be read — check registry_path in agents.yaml"
	// WarnMeshSelfConsistencyFailed is the UNPINNED twin of Unverifiable. The
	// pinned wording blames "your pinned root", which on this path the
	// operator does not have — two warnings in one section, one saying you
	// have no pin and the other blaming it.
	WarnMeshSelfConsistencyFailed = "the registry did NOT verify against the daemon-advertised root — it is not even self-consistent (bad signature, stale, or rolled back)"
	// WarnMeshRegistryStale: the registry verifies against its root but is
	// older than the freshness bound. Named separately from Unverifiable
	// because the fix is different (re-sign and redeploy, not investigate a
	// bad signature) and because the SYMPTOM is invisible from inside: reads,
	// long-poll and the wake-watcher keep working while every signed send is
	// refused. That is how the 2026-09-06 mesh-wide mute stayed unnoticed for
	// a day with every liveness check green.
	// WarnMeshFreshnessUnchecked: no registry was available to read, so the
	// freshness question was never asked. Named because "disabled" and
	// "healthy" printed identically — the same reasoning that produced
	// WarnMeshHeartbeatUnresolved: a check that cannot run is not a pass.
	WarnMeshRegistryStale = "the registry is past its freshness bound — signed SENDS will be refused while reads keep working; re-sign and redeploy it"
	// WarnMeshHeartbeatUnresolved: the check could not run. Rendering silence
	// (or a filesystem verdict) here is how a seven-day-dead watcher reported
	// "not found" — the checker had no path and never looked.
	WarnMeshHeartbeatUnresolved = "watcher heartbeat path is UNRESOLVED (no agent slug) — watcher liveness is UNKNOWN, not OK"
	// WarnMeshWatcherNotRearmed: lastwake is newer than lastarm. The watcher
	// fired, the session was re-invoked, and nobody armed a successor — every
	// message since is landing in silence. This is the one check that fires on
	// the real 2026-08-16 death.
	WarnMeshWatcherNotRearmed = "the watcher woke and was never re-armed — you are not being woken for messages"
)

// Heartbeat states. Three distinct non-green diagnoses with three different
// fixes; collapsing any two is how the last one hid.
const (
	HeartbeatUnresolved = "unresolved" // no path — the check could not run
	HeartbeatAbsent     = "absent"     // path known; no file (watcher never armed)
	HeartbeatStale      = "stale"      // file present; mtime outside the window
	HeartbeatFresh      = "fresh"
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
	// RegistryUnread carries a DECLARED registry path that could not be read,
	// so the cmd layer's failure reaches the section instead of vanishing.
	RegistryUnread string
	Slug           string
	Signer         MeshSigner // keyless proof-of-possession

	// Tier-3 reachability.
	Daemon        MeshDaemonClient
	HeartbeatPath string
	// LastwakePath / LastarmPath feed the re-arm check; either empty disables it.
	LastwakePath string
	LastarmPath  string

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
	// WatcherHeartbeatState is one of the Heartbeat* constants. Fresh==true iff
	// State=="fresh" (both kept for back-compat readers of the JSON).
	WatcherHeartbeatState string `json:"watcher_heartbeat_state"`
	// WatcherHeartbeatPath is WHERE the check looked. Always carried: a verdict
	// about a file whose location is undisclosed cannot be audited, and an
	// undisclosed path is exactly how a wrong directory stayed wrong for months.
	WatcherHeartbeatPath string `json:"watcher_heartbeat_path,omitempty"`
	// WatcherRearmed is false when lastwake postdates lastarm (see checkRearm).
	WatcherRearmed bool `json:"watcher_rearmed"`

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

	// Freshness LAST and unconditionally: it is the one registry question that
	// needs neither a pin nor a reachable daemon, so it must not sit inside
	// either path's early returns — nor downstream of a function that returns
	// nil on its success path, which is how this check spent its first hours
	// doing nothing at all.
	checkRegistryFreshness(mi, in, registryBytes, in.Now)

	return mi, nil
}

// checkRegistryFreshness warns when the registry is past the freshness bound,
// reading the window straight out of the bytes.
//
// Signature verification needs a root key and (unpinned) a reachable daemon;
// the clock needs neither. Keeping them together made the staleness question
// unanswerable on exactly the deployment that needed it — a remote hub, where
// doctor's loopback-pinned probe can never reach — and the mesh went mute for
// a day behind checks that all reported green. Warnings dedupe by string, so
// the pinned/unpinned paths naming the same condition is harmless.
func checkRegistryFreshness(mi *DoctorMeshIdentity, in MeshDoctorInput, regBytes []byte, now time.Time) {
	// A declared path that could not be read is reported and then we CARRY ON:
	// returning here disabled the check on the registry actually in play when a
	// daemon was serving one — an early return switching off a check on a path
	// where a registry exists, which is the original defect wearing a new hat.
	if in.RegistryUnread != "" {
		mi.addWarning(WarnMeshRegistryUnreadable)
	}
	if len(regBytes) == 0 {
		if in.RegistryUnread == "" {
			// Only when nothing was declared. With an unreadable declared path
			// the operator already has a specific, more useful warning; adding
			// "no registry to read" alongside it counts one fact twice.
			mi.addWarning(WarnMeshFreshnessUnchecked)
		}
		return
	}
	env, err := registry.ParseDistribution(regBytes)
	if err != nil {
		mi.addWarning(unverifiableWarning(in))
		return
	}
	validFrom, validUntil, err := registry.Freshness(env)
	if err != nil {
		// ParseDistribution is STRUCTURAL — it base64-decodes the body and
		// never checks it is JSON — so a valid envelope wrapping garbage lands
		// here. The previous repair fixed the branch beside this one and left
		// this one mute.
		mi.addWarning(unverifiableWarning(in))
		return
	}
	if registry.IsStaleAt(validFrom, validUntil, now, doctorMaxStaleness) {
		mi.addWarning(WarnMeshRegistryStale)
	}
}

// unverifiableWarning picks wording the operator can act on: blaming "your
// pinned root" when there is no pin sends them to fix something they do not
// have. The unpinned constant was added for exactly this and then not used
// here, which made it cosmetic.
func unverifiableWarning(in MeshDoctorInput) string {
	if len(in.PinnedRootPub) == 0 {
		return WarnMeshSelfConsistencyFailed
	}
	return WarnMeshUnverifiable
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
	checkRearm(mi, in.LastwakePath, in.LastarmPath)

	// An OFFLINE registry the operator handed us stands on its own. Returning
	// nil here discarded it the moment no daemon answered, which is the normal
	// state for a fleet whose daemon is remote (doctor's probe is loopback-
	// pinned by design) — so every registry check silently did nothing on the
	// deployments that most needed them.
	if in.Daemon == nil {
		return in.RegistryBytes
	}
	reachable, _, _ := in.Daemon.Whoami(ctx)
	mi.DaemonReachable = reachable
	if !reachable {
		return in.RegistryBytes
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
	// Returning the OPERATOR'S bytes rather than nil in the else-branch is the
	// point: a reachable daemon used to hand nil downstream whenever an offline
	// registry existed, which silently disabled the freshness check on exactly
	// the configuration it was written for — a fossil loopback daemon answers
	// whoami, serves no root, and thereby switched off its own alarm.
	if len(in.RegistryBytes) == 0 {
		if dir, derr := in.Daemon.FetchDirectory(ctx); derr == nil {
			return dir
		}
		return nil
	}
	return in.RegistryBytes
}

// checkHeartbeat reads the watcher heartbeat file's mtime and sets one of four
// states — FRESHNESS, not a process check.
//
//	unresolved  no path: the check COULD NOT RUN, and that is a warning — a
//	            check that cannot run is not a pass. This used to return
//	            silently, and the renderer printed a filesystem verdict
//	            ("not found") for a lookup that never touched the filesystem.
//	absent      path known, no file: INFO, not a warning — most members have
//	            never armed a watcher, and absence must not trip
//	            `jq '.status=="warning"'`.
//	stale       present but old — the watcher may be present-but-dead. Warning.
//	fresh       written within heartbeatStaleAfter.
//
// Stat, not Lstat: a heartbeat reached through a symlink should report the
// TARGET's mtime — the link's own mtime is noise that reads permanently stale.
// There is no security rationale for refusing symlinks here, unlike the
// key-custody check where that refusal is the point.
func checkHeartbeat(mi *DoctorMeshIdentity, heartbeatPath string, now time.Time) {
	mi.WatcherHeartbeatPath = heartbeatPath
	if heartbeatPath == "" {
		mi.WatcherHeartbeatState = HeartbeatUnresolved
		mi.addWarning(WarnMeshHeartbeatUnresolved)
		return
	}
	info, err := os.Stat(heartbeatPath)
	if err != nil {
		mi.WatcherHeartbeatState = HeartbeatAbsent
		return
	}
	age := now.Sub(info.ModTime())
	if age < 0 {
		// A FUTURE mtime is a liar — clock skew or a hand-written file — not a
		// healthy watcher. The old clamp rendered it fresh: the most trusted
		// state granted on the least trustworthy evidence.
		mi.WatcherHeartbeatAge = 0
		mi.WatcherHeartbeatState = HeartbeatStale
		mi.addWarning(WarnMeshHeartbeatStale)
		return
	}
	mi.WatcherHeartbeatAge = int(age.Seconds())
	if age <= heartbeatStaleAfter {
		mi.WatcherHeartbeatState = HeartbeatFresh
		mi.WatcherHeartbeatFresh = true
		return
	}
	mi.WatcherHeartbeatState = HeartbeatStale
	mi.addWarning(WarnMeshHeartbeatStale)
}

// checkRearm compares the watcher's lastwake and lastarm stamps.
//
// LASTWAKE records DETECTION — the watcher saw a message and exited so the
// harness could re-invoke the session. Only a subsequent arm records that
// anyone actually came back. wake newer than arm means the wake happened and
// no successor was armed: from that moment every message lands in silence,
// and the heartbeat says nothing because the watcher is GONE, not dead.
//
// Verified against the live files before this was written: the 2026-08-16
// death shows lastwake 23:29:22 with no later arm. This is the one check that
// fires on that incident.
//
// Either file absent ⇒ the pair cannot testify; stay silent (the heartbeat
// states carry the never-armed and unresolved diagnoses).
func checkRearm(mi *DoctorMeshIdentity, lastwakePath, lastarmPath string) {
	mi.WatcherRearmed = true // default: nothing testifies against it
	if lastwakePath == "" || lastarmPath == "" {
		return
	}
	wake, werr := os.Stat(lastwakePath)
	arm, aerr := os.Stat(lastarmPath)
	if werr != nil || aerr != nil {
		return
	}
	if wake.ModTime().After(arm.ModTime()) {
		mi.WatcherRearmed = false
		mi.addWarning(WarnMeshWatcherNotRearmed)
	}
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
	// The verdict is USED, not just computed. Discarding verr here made a stale
	// registry indistinguishable from a fresh one on the unpinned path — which
	// is the common path, since a pin requires enroll — and that silence is
	// what let a mesh-wide send mute run for a day behind green checks.
	_, _, verr := registry.VerifyAndLoad(advertised, mustParse(regBytes), 0, in.Now, doctorMaxStaleness)
	switch {
	case verr == nil:
		if mi.NetworkID == "" {
			mi.NetworkID = root.NetworkID
		}
	case strings.Contains(verr.Error(), registry.ErrStale):
		mi.addWarning(WarnMeshRegistryStale)
	default:
		mi.addWarning(WarnMeshSelfConsistencyFailed)
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
