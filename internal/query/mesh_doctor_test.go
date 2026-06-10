package query

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/anchor"
	"github.com/peiman/vaultmind/internal/identity/doctorclient"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/stretchr/testify/require"
)

// --- test doubles -----------------------------------------------------------

// stubDaemon implements MeshDaemonClient for the tier-2/tier-3 tests.
type stubDaemon struct {
	root        doctorclient.WellKnownRoot
	rootErr     error
	directory   []byte
	dirErr      error
	whoamiOK    bool
	whoamiAgent string
}

func (s *stubDaemon) FetchRoot(_ context.Context) (doctorclient.WellKnownRoot, error) {
	return s.root, s.rootErr
}
func (s *stubDaemon) FetchDirectory(_ context.Context) ([]byte, error) {
	return s.directory, s.dirErr
}
func (s *stubDaemon) Whoami(_ context.Context) (bool, string, error) {
	return s.whoamiOK, s.whoamiAgent, nil
}

// stubSigner implements MeshSigner. holds the private key so it can answer a
// proof-of-possession challenge — exactly the keyless route doctor must use
// (the CLI never reads the key file; the SIGNER holds it).
type stubSigner struct {
	priv    ed25519.PrivateKey
	signErr error
}

func (s *stubSigner) Sign(canonical []byte) ([]byte, error) {
	if s.signErr != nil {
		return nil, s.signErr
	}
	return ed25519.Sign(s.priv, canonical), nil
}

// --- fixtures ---------------------------------------------------------------

// lowEntropyKey deterministically derives an ed25519 keypair from a seed string
// (gitleaks-friendly: NewKeyFromSeed of a known-low-entropy seed, never a real
// secret).
func lowEntropyKey(t *testing.T, seed string) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	s := make([]byte, ed25519.SeedSize)
	copy(s, seed)
	priv := ed25519.NewKeyFromSeed(s)
	return priv.Public().(ed25519.PublicKey), priv
}

// buildSignedRegistry roots a one-binding registry under rootPriv and returns the
// distribution bytes + the network id derived from the root.
func buildSignedRegistry(t *testing.T, rootPub ed25519.PublicKey, rootPriv ed25519.PrivateKey, slug string, memberPub ed25519.PublicKey, now time.Time) ([]byte, string) {
	t.Helper()
	pk, err := registry.NewPublicKey(memberPub)
	require.NoError(t, err)
	reg := registry.Registry{
		Epoch:      1,
		ValidFrom:  now.Add(-time.Hour).Unix(),
		ValidUntil: now.Add(24 * time.Hour).Unix(),
		Agents: []registry.AgentBinding{{
			Slug:                    slug,
			DisplayName:             "Member",
			PubKey:                  pk,
			KeyEpoch:                1,
			ValidFrom:               now.Add(-time.Hour).Unix(),
			ValidUntil:              now.Add(24 * time.Hour).Unix(),
			AuthorizedOriginDaemons: []string{"daemon:local"},
		}},
	}
	env, err := registry.SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	raw, err := registry.MarshalDistribution(env)
	require.NoError(t, err)
	return raw, registry.NetworkID(rootPub)
}

func writeKeyFile(t *testing.T, path string, mode os.FileMode, size int) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, make([]byte, size), 0o600))
	require.NoError(t, os.Chmod(path, mode))
}

// --- TIER 1 -----------------------------------------------------------------

func TestMeshDoctor_Tier1_KeyPresentGoodModeAndSize(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "identity-signer.key")
	writeKeyFile(t, keyPath, 0o600, ed25519.PrivateKeySize)

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:    keyPath,
		SocketPath: filepath.Join(dir, "missing.sock"),
		Now:        time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, mi)
	require.True(t, mi.KeyPresent)
	require.True(t, mi.KeyModeOK)
	require.True(t, mi.KeySizeOK)
	// signer not running is INFO, not a hard error.
	require.False(t, mi.SignerReachable)
}

func TestMeshDoctor_Tier1_KeyAbsent(t *testing.T) {
	dir := t.TempDir()
	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:    filepath.Join(dir, "nope.key"),
		SocketPath: filepath.Join(dir, "missing.sock"),
		Now:        time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, mi)
	require.False(t, mi.KeyPresent)
	require.False(t, mi.KeyModeOK)
	require.False(t, mi.KeySizeOK)
}

func TestMeshDoctor_Tier1_KeyBadMode(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "identity-signer.key")
	writeKeyFile(t, keyPath, 0o644, ed25519.PrivateKeySize)

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:    keyPath,
		SocketPath: filepath.Join(dir, "missing.sock"),
		Now:        time.Now(),
	})
	require.NoError(t, err)
	require.True(t, mi.KeyPresent)
	require.False(t, mi.KeyModeOK)
	require.True(t, mi.KeySizeOK)
	require.NotEmpty(t, mi.Warnings)
}

func TestMeshDoctor_Tier1_KeyBadSize(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "identity-signer.key")
	writeKeyFile(t, keyPath, 0o600, 10)

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:    keyPath,
		SocketPath: filepath.Join(dir, "missing.sock"),
		Now:        time.Now(),
	})
	require.NoError(t, err)
	require.True(t, mi.KeyPresent)
	require.False(t, mi.KeySizeOK)
}

func TestMeshDoctor_Tier1_SymlinkRejected(t *testing.T) {
	dir := t.TempDir()
	realKey := filepath.Join(dir, "real.key")
	writeKeyFile(t, realKey, 0o600, ed25519.PrivateKeySize)
	linkPath := filepath.Join(dir, "identity-signer.key")
	require.NoError(t, os.Symlink(realKey, linkPath))

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:    linkPath,
		SocketPath: filepath.Join(dir, "missing.sock"),
		Now:        time.Now(),
	})
	require.NoError(t, err)
	// Lstat sees the symlink, NOT a regular 0600 file → mode check fails.
	require.True(t, mi.KeyPresent)
	require.False(t, mi.KeyModeOK)
}

// keylessGuardKey is a path the test asserts is NEVER opened for read. The
// readGuard install replaces os file-open hooks; here we assert via a sentinel:
// BuildMeshIdentity must only Lstat the key, never read it. We prove it by
// putting unreadable (0000) content that would error on read but stat fine.
func TestMeshDoctor_Tier1_KeylessNeverReadsKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "identity-signer.key")
	// Write a 64-byte file then strip ALL read perms. A 0600 file is required for
	// KeyModeOK; here we use 0200 (write-only) so any os.ReadFile would FAIL —
	// if doctor tried to read it, the result would differ. Custody stat still
	// works (size+mode via Lstat).
	writeKeyFile(t, keyPath, 0o200, ed25519.PrivateKeySize)

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:    keyPath,
		SocketPath: filepath.Join(dir, "missing.sock"),
		Now:        time.Now(),
	})
	require.NoError(t, err)
	require.True(t, mi.KeyPresent)
	require.True(t, mi.KeySizeOK)
	// 0200 != 0600 → mode not OK, but NO read attempted (no error surfaced).
	require.False(t, mi.KeyModeOK)
}

// --- TIER 2 -----------------------------------------------------------------

func TestMeshDoctor_Tier2_PinnedResolvesSelfVerifyGreen(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, "doctor-root-seed-green")
	memberPub, memberPriv := lowEntropyKey(t, "doctor-member-seed-green")
	raw, nid := buildSignedRegistry(t, rootPub, rootPriv, "agent:mira", memberPub, now)

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "identity-signer.key")
	writeKeyFile(t, keyPath, 0o600, ed25519.PrivateKeySize)

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       keyPath,
		SocketPath:    filepath.Join(dir, "missing.sock"),
		PinnedRootPub: rootPub,
		NetworkID:     nid,
		RegistryBytes: raw,
		Slug:          "agent:mira",
		Now:           now,
		Signer:        &stubSigner{priv: memberPriv},
	})
	require.NoError(t, err)
	require.True(t, mi.Pinned)
	require.True(t, mi.Authenticated)
	require.True(t, mi.BindingResolves)
	require.True(t, mi.HoldsBindingKey)
	require.Equal(t, StatusMeshAuthenticated, mi.Status)
	require.Equal(t, nid, mi.NetworkID)
}

func TestMeshDoctor_Tier2_PinnedResolvesSelfVerifyFailKeyMismatch(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, "doctor-root-seed-mismatch")
	memberPub, _ := lowEntropyKey(t, "doctor-member-seed-mismatch")
	_, wrongPriv := lowEntropyKey(t, "doctor-WRONG-signer-seed")
	raw, nid := buildSignedRegistry(t, rootPub, rootPriv, "agent:mira", memberPub, now)

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "identity-signer.key")
	writeKeyFile(t, keyPath, 0o600, ed25519.PrivateKeySize)

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       keyPath,
		SocketPath:    filepath.Join(dir, "missing.sock"),
		PinnedRootPub: rootPub,
		NetworkID:     nid,
		RegistryBytes: raw,
		Slug:          "agent:mira",
		Now:           now,
		Signer:        &stubSigner{priv: wrongPriv}, // signer holds the WRONG key
	})
	require.NoError(t, err)
	require.True(t, mi.Pinned)
	require.True(t, mi.BindingResolves)
	require.False(t, mi.HoldsBindingKey)
	require.False(t, mi.Authenticated)
	require.Equal(t, StatusMeshKeyMismatch, mi.Status)
}

func TestMeshDoctor_Tier2_PinnedSlugAbsentNotEnrolled(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, "doctor-root-seed-absent")
	memberPub, memberPriv := lowEntropyKey(t, "doctor-member-seed-absent")
	raw, nid := buildSignedRegistry(t, rootPub, rootPriv, "agent:someoneelse", memberPub, now)

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       filepath.Join(t.TempDir(), "k.key"),
		SocketPath:    filepath.Join(t.TempDir(), "missing.sock"),
		PinnedRootPub: rootPub,
		NetworkID:     nid,
		RegistryBytes: raw,
		Slug:          "agent:mira",
		Now:           now,
		Signer:        &stubSigner{priv: memberPriv},
	})
	require.NoError(t, err)
	require.True(t, mi.Pinned)
	require.False(t, mi.BindingResolves)
	require.False(t, mi.Authenticated)
	require.Equal(t, StatusMeshNotEnrolled, mi.Status)
	require.NotEmpty(t, mi.Warnings)
}

func TestMeshDoctor_Tier2_PinnedBadSigRed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, "doctor-root-seed-badsig")
	otherPub, _ := lowEntropyKey(t, "doctor-OTHER-root")
	memberPub, _ := lowEntropyKey(t, "doctor-member-seed-badsig")
	raw, _ := buildSignedRegistry(t, rootPub, rootPriv, "agent:mira", memberPub, now)

	// Pin a DIFFERENT root than the one that signed → VerifyAndLoad fails.
	otherNID := registry.NetworkID(otherPub)
	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       filepath.Join(t.TempDir(), "k.key"),
		SocketPath:    filepath.Join(t.TempDir(), "missing.sock"),
		PinnedRootPub: otherPub,
		NetworkID:     otherNID,
		RegistryBytes: raw,
		Slug:          "agent:mira",
		Now:           now,
	})
	require.NoError(t, err)
	require.True(t, mi.Pinned)
	require.False(t, mi.Authenticated)
	require.Equal(t, StatusMeshUnverifiable, mi.Status)
	require.NotEmpty(t, mi.Warnings)
}

func TestMeshDoctor_Tier2_PinnedStaleRed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, "doctor-root-seed-stale")
	memberPub, _ := lowEntropyKey(t, "doctor-member-seed-stale")
	raw, nid := buildSignedRegistry(t, rootPub, rootPriv, "agent:mira", memberPub, now)

	// Verify FAR in the future → registry valid_until passed → ErrStale.
	future := now.Add(72 * time.Hour)
	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       filepath.Join(t.TempDir(), "k.key"),
		SocketPath:    filepath.Join(t.TempDir(), "missing.sock"),
		PinnedRootPub: rootPub,
		NetworkID:     nid,
		RegistryBytes: raw,
		Slug:          "agent:mira",
		Now:           future,
	})
	require.NoError(t, err)
	require.False(t, mi.Authenticated)
	require.Equal(t, StatusMeshUnverifiable, mi.Status)
}

func TestMeshDoctor_Tier2_UnpinnedSelfConsistentWarn(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, "doctor-root-seed-unpinned")
	memberPub, memberPriv := lowEntropyKey(t, "doctor-member-seed-unpinned")
	raw, nid := buildSignedRegistry(t, rootPub, rootPriv, "agent:mira", memberPub, now)

	// NO pin. The daemon advertises its own root; doctor verifies for
	// self-consistency ONLY → authenticated=false, never green.
	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       filepath.Join(t.TempDir(), "k.key"),
		SocketPath:    filepath.Join(t.TempDir(), "missing.sock"),
		RegistryBytes: raw,
		Slug:          "agent:mira",
		Now:           now,
		Signer:        &stubSigner{priv: memberPriv},
		Daemon: &stubDaemon{
			root:      doctorclient.WellKnownRoot{RootPubKey: b64(rootPub), NetworkID: nid},
			directory: raw,
			whoamiOK:  true,
		},
	})
	require.NoError(t, err)
	require.False(t, mi.Pinned)
	require.False(t, mi.Authenticated)
	require.Equal(t, StatusMeshSelfConsistentUnpinned, mi.Status)
	require.NotEmpty(t, mi.Warnings)
}

func TestMeshDoctor_Tier2_FetchesDirectoryFromDaemonWhenNoOfflineRegistry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, "doctor-root-seed-fetch")
	memberPub, memberPriv := lowEntropyKey(t, "doctor-member-seed-fetch")
	raw, nid := buildSignedRegistry(t, rootPub, rootPriv, "agent:mira", memberPub, now)

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "identity-signer.key")
	writeKeyFile(t, keyPath, 0o600, ed25519.PrivateKeySize)

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       keyPath,
		SocketPath:    filepath.Join(dir, "missing.sock"),
		PinnedRootPub: rootPub,
		NetworkID:     nid,
		Slug:          "agent:mira",
		Now:           now,
		Signer:        &stubSigner{priv: memberPriv},
		Daemon:        &stubDaemon{directory: raw, whoamiOK: true, root: doctorclient.WellKnownRoot{RootPubKey: b64(rootPub), NetworkID: nid}},
	})
	require.NoError(t, err)
	require.True(t, mi.Authenticated)
	require.Equal(t, StatusMeshAuthenticated, mi.Status)
	require.True(t, mi.DaemonReachable)
}

// --- TIER 3 -----------------------------------------------------------------

func TestMeshDoctor_Tier3_DaemonReachablePlaintext(t *testing.T) {
	now := time.Now()
	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:    filepath.Join(t.TempDir(), "k.key"),
		SocketPath: filepath.Join(t.TempDir(), "missing.sock"),
		Now:        now,
		Daemon: &stubDaemon{
			whoamiOK: true,
			rootErr:  doctorclient.ErrNotConfigured, // 404 → plaintext
		},
	})
	require.NoError(t, err)
	require.True(t, mi.DaemonReachable)
	require.Equal(t, DaemonModePlaintext, mi.DaemonMode)
	require.False(t, mi.EnforcementActive)
}

func TestMeshDoctor_Tier3_DaemonReachableAdvisoryConfigured(t *testing.T) {
	now := time.Now()
	rootPub, _ := lowEntropyKey(t, "doctor-tier3-root")
	nid := registry.NetworkID(rootPub)
	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:    filepath.Join(t.TempDir(), "k.key"),
		SocketPath: filepath.Join(t.TempDir(), "missing.sock"),
		Now:        now,
		Daemon: &stubDaemon{
			whoamiOK: true,
			root:     doctorclient.WellKnownRoot{RootPubKey: b64(rootPub), NetworkID: nid},
		},
	})
	require.NoError(t, err)
	require.True(t, mi.DaemonReachable)
	require.Equal(t, DaemonModeAdvisoryConfigured, mi.DaemonMode)
	require.False(t, mi.EnforcementActive, "enforcement is a no-op today; must never claim active")
}

func TestMeshDoctor_Tier3_DaemonUnreachable(t *testing.T) {
	now := time.Now()
	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:    filepath.Join(t.TempDir(), "k.key"),
		SocketPath: filepath.Join(t.TempDir(), "missing.sock"),
		Now:        now,
		Daemon:     &stubDaemon{whoamiOK: false},
	})
	require.NoError(t, err)
	require.False(t, mi.DaemonReachable)
}

func TestMeshDoctor_Tier3_HeartbeatFresh(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	hb := filepath.Join(dir, "mesh-watch.heartbeat")
	require.NoError(t, os.WriteFile(hb, []byte("x"), 0o600))
	require.NoError(t, os.Chtimes(hb, now.Add(-10*time.Second), now.Add(-10*time.Second)))

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       filepath.Join(dir, "k.key"),
		SocketPath:    filepath.Join(dir, "missing.sock"),
		HeartbeatPath: hb,
		Now:           now,
	})
	require.NoError(t, err)
	require.True(t, mi.WatcherHeartbeatFresh)
	require.LessOrEqual(t, mi.WatcherHeartbeatAge, 11)
}

func TestMeshDoctor_Tier3_HeartbeatStale(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	hb := filepath.Join(dir, "mesh-watch.heartbeat")
	require.NoError(t, os.WriteFile(hb, []byte("x"), 0o600))
	require.NoError(t, os.Chtimes(hb, now.Add(-300*time.Second), now.Add(-300*time.Second)))

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       filepath.Join(dir, "k.key"),
		SocketPath:    filepath.Join(dir, "missing.sock"),
		HeartbeatPath: hb,
		Now:           now,
	})
	require.NoError(t, err)
	require.False(t, mi.WatcherHeartbeatFresh)
	require.NotEmpty(t, mi.Warnings)
}

func TestMeshDoctor_Tier3_HeartbeatAbsent(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       filepath.Join(dir, "k.key"),
		SocketPath:    filepath.Join(dir, "missing.sock"),
		HeartbeatPath: filepath.Join(dir, "nope.heartbeat"),
		Now:           now,
	})
	require.NoError(t, err)
	require.False(t, mi.WatcherHeartbeatFresh)
	require.Zero(t, mi.WatcherHeartbeatAge, "an absent heartbeat has no age")
	// ABSENT is INFO, not a warning — it must not inflate the warning count for
	// the many members who have never run the watcher. Only STALE warns.
	for _, w := range mi.Warnings {
		require.NotContains(t, w, "heartbeat", "absent heartbeat must not add a warning")
	}
}

// --- presence gating + anchor auto-discovery --------------------------------

func TestMeshDoctor_AnchorAutoDiscoverPin(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, "doctor-anchor-root")
	memberPub, memberPriv := lowEntropyKey(t, "doctor-anchor-member")
	raw, nid := buildSignedRegistry(t, rootPub, rootPriv, "agent:mira", memberPub, now)

	// Persist an anchor and resolve the pin from it (no --mesh-root-pubkey).
	dir := t.TempDir()
	anchorPath := filepath.Join(dir, "network-roots.json")
	require.NoError(t, anchor.Upsert(anchorPath, anchor.NetworkAnchor{
		NetworkID:   nid,
		RootPubKey:  b64(rootPub),
		ConfirmedAt: now.Unix(),
	}))
	anchors, err := anchor.Load(anchorPath)
	require.NoError(t, err)
	a, ok := anchor.Find(anchors, nid)
	require.True(t, ok)
	pinnedPub, err := decodeAnchorPub(a)
	require.NoError(t, err)

	keyPath := filepath.Join(dir, "identity-signer.key")
	writeKeyFile(t, keyPath, 0o600, ed25519.PrivateKeySize)

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       keyPath,
		SocketPath:    filepath.Join(dir, "missing.sock"),
		PinnedRootPub: pinnedPub,
		NetworkID:     nid,
		RegistryBytes: raw,
		Slug:          "agent:mira",
		Now:           now,
		Signer:        &stubSigner{priv: memberPriv},
	})
	require.NoError(t, err)
	require.True(t, mi.Authenticated)
}

// helper: base64-std of a pubkey.
func b64(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub)
}

// decodeAnchorPub mirrors what cmd does to turn a stored anchor into a pinned
// ed25519 pubkey, exercised here to prove the round-trip.
func decodeAnchorPub(a anchor.NetworkAnchor) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(a.RootPubKey)
	if err != nil {
		return nil, err
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return nil, err
	}
	return pk.Bytes(), nil
}

// keyless invariant: verify selfVerify uses a domain-tagged challenge that
// VerifyCanonical accepts — sanity that the challenge path is wired through the
// identity primitive, not a bespoke verify.
func TestMeshDoctor_SelfVerifyUsesIdentityPrimitive(t *testing.T) {
	pub, priv := lowEntropyKey(t, "doctor-selfverify-prim")
	challenge := buildSelfVerifyChallenge([]byte("nonce-1234"))
	sig := ed25519.Sign(priv, challenge.Bytes())
	ok, err := identity.VerifyCanonical(pub, challenge, sig)
	require.NoError(t, err)
	require.True(t, ok)

	_, wrongPriv := lowEntropyKey(t, "doctor-selfverify-wrong")
	badSig := ed25519.Sign(wrongPriv, challenge.Bytes())
	ok, err = identity.VerifyCanonical(pub, challenge, badSig)
	require.NoError(t, err)
	require.False(t, ok)
}
