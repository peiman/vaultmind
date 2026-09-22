package query

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity/doctorclient"
	"github.com/stretchr/testify/require"
)

// LIVE-OBSERVED (2026-09-22, dabir on Siavoush's box, reproduced here): doctor
// said the epoch-4 registry "did not verify ... (bad signature, stale, or rolled
// back)". The signature was valid and the registry was 17 days into a 30-day
// hub bound — sends were landing. The SIGNATURE checks still ran with the
// hardcoded 24h early-warning bound that 9c54e12 had already demoted to
// advisory on the freshness path, so every registry older than a day read as
// forged. An operator told "bad signature" goes looking for tampering or
// re-pins a root; both are wrong moves on a healthy mesh.
//
// The fixture is the live shape: signed 17 days ago, one-year window.

const liveRegistryAge = 17 * 24 * time.Hour

func agedRegistry(t *testing.T, seed string) (raw []byte, nid string, rootPub ed25519.PublicKey, memberPriv ed25519.PrivateKey, now time.Time) {
	t.Helper()
	signed := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, seed+"-root")
	memberPub, memberPriv := lowEntropyKey(t, seed+"-member")
	raw, nid = buildSignedRegistryWindow(t, rootPub, rootPriv, "agent:mira", memberPub,
		signed, signed.Add(365*24*time.Hour))
	return raw, nid, rootPub, memberPriv, signed.Add(liveRegistryAge)
}

func TestMeshDoctor_PinnedValidSignatureIsNotReportedForgedBecauseItIsOld(t *testing.T) {
	raw, nid, rootPub, memberPriv, now := agedRegistry(t, "sigage-pinned")

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       filepath.Join(t.TempDir(), "k.key"),
		SocketPath:    filepath.Join(t.TempDir(), "missing.sock"),
		PinnedRootPub: rootPub,
		NetworkID:     nid,
		RegistryBytes: raw,
		Slug:          "agent:mira",
		Now:           now,
		Signer:        &stubSigner{priv: memberPriv},
		MaxStaleness:  30 * 24 * time.Hour,
	})
	require.NoError(t, err)
	require.NotContains(t, mi.Warnings, WarnMeshUnverifiable,
		"a valid signature 17 days into a 30-day bound is not 'bad signature, stale, or rolled back'")
	require.Equal(t, StatusMeshAuthenticated, mi.Status)
}

// Dabir's box declares no bound at all. Undeclared must still not mean forged:
// the age belongs to the ADVISORY freshness line, never to the signature verdict.
func TestMeshDoctor_PinnedUndeclaredBoundAgeIsAdvisoryNotForgery(t *testing.T) {
	raw, nid, rootPub, memberPriv, now := agedRegistry(t, "sigage-undeclared")

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
	require.NotContains(t, mi.Warnings, WarnMeshUnverifiable)
	require.Equal(t, StatusMeshAuthenticated, mi.Status)
	require.Contains(t, mi.Warnings, WarnMeshRegistryAging,
		"with no declared bound the age is still worth an advisory")
}

func TestMeshDoctor_UnpinnedValidSignatureIsNotReportedInconsistentBecauseItIsOld(t *testing.T) {
	raw, nid, rootPub, memberPriv, now := agedRegistry(t, "sigage-unpinned")

	mi, err := BuildMeshIdentity(context.Background(), MeshDoctorInput{
		KeyPath:       filepath.Join(t.TempDir(), "k.key"),
		SocketPath:    filepath.Join(t.TempDir(), "missing.sock"),
		RegistryBytes: raw,
		Slug:          "agent:mira",
		Now:           now,
		Signer:        &stubSigner{priv: memberPriv},
		MaxStaleness:  30 * 24 * time.Hour,
		Daemon: &stubDaemon{
			root:      doctorclient.WellKnownRoot{RootPubKey: b64(rootPub), NetworkID: nid},
			directory: raw,
			whoamiOK:  true,
		},
	})
	require.NoError(t, err)
	require.NotContains(t, mi.Warnings, WarnMeshSelfConsistencyFailed)
	require.NotContains(t, mi.Warnings, WarnMeshRegistryStale,
		"17 days into a declared 30-day bound is not stale")
}
