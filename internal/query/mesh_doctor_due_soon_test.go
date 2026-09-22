package query

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The 30-day window is kept on purpose (Peiman, 2026-09-22: a longer one is
// only a longer time to forget), so the warning has to arrive BEFORE the lapse,
// in every agent's doctor — not the day sends start being dropped.

func dueSoonInput(t *testing.T, age time.Duration) MeshDoctorInput {
	t.Helper()
	signed := time.Unix(1_700_000_000, 0)
	rootPub, rootPriv := lowEntropyKey(t, "due-soon-root")
	memberPub, memberPriv := lowEntropyKey(t, "due-soon-member")
	raw, _ := buildSignedRegistryWindow(t, rootPub, rootPriv, "agent:mira", memberPub,
		signed, signed.Add(365*24*time.Hour))
	return MeshDoctorInput{
		KeyPath:       filepath.Join(t.TempDir(), "k.key"),
		SocketPath:    filepath.Join(t.TempDir(), "missing.sock"),
		Slug:          "agent:mira",
		Now:           signed.Add(age),
		Signer:        &stubSigner{priv: memberPriv},
		RegistryBytes: raw,
		MaxStaleness:  30 * 24 * time.Hour,
	}
}

func TestMeshDoctor_WarnsInsideTheLastWeek(t *testing.T) {
	mi, err := BuildMeshIdentity(context.Background(), dueSoonInput(t, 25*24*time.Hour))
	require.NoError(t, err)
	assert.Contains(t, mi.Warnings, fmt.Sprintf(WarnMeshRegistryDueSoonFmt, 5),
		"five days before the hub drops signed posts is when someone must act")
	assert.NotContains(t, mi.Warnings, WarnMeshRegistryStale, "not lapsed yet")
}

func TestMeshDoctor_QuietOutsideTheLastWeek(t *testing.T) {
	mi, err := BuildMeshIdentity(context.Background(), dueSoonInput(t, 17*24*time.Hour))
	require.NoError(t, err)
	// Joined, not looped: an empty Warnings slice must still run the assertion.
	assert.NotContains(t, strings.Join(mi.Warnings, "\n"), "re-sign",
		"13 days left is not news; a daily warning is one nobody reads")
}

func TestMeshDoctor_LapsedIsStaleNotDueSoon(t *testing.T) {
	mi, err := BuildMeshIdentity(context.Background(), dueSoonInput(t, 31*24*time.Hour))
	require.NoError(t, err)
	assert.Contains(t, mi.Warnings, WarnMeshRegistryStale)
	assert.NotContains(t, strings.Join(mi.Warnings, "\n"), "day(s) left")
}
