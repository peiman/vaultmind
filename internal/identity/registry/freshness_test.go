package registry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsStaleAt_FreshInsideBothBounds(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	require.False(t, IsStaleAt(now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix(), now, 24*time.Hour))
}

func TestIsStaleAt_AgedPastMaxStalenessWhileStillValid(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	// The live shape: valid_until a year out, age past the bound. This exact
	// combination muted the mesh on 2026-09-06 — validity said 2027, age said 31 days.
	require.True(t, IsStaleAt(now.Add(-31*24*time.Hour).Unix(), now.Add(365*24*time.Hour).Unix(), now, 30*24*time.Hour))
}

func TestIsStaleAt_ExpiredEvenWhenRecentlySigned(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	require.True(t, IsStaleAt(now.Add(-time.Minute).Unix(), now.Add(-time.Second).Unix(), now, 24*time.Hour))
}

func TestIsStaleAt_BoundIsInclusive(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	require.False(t, IsStaleAt(now.Add(-24*time.Hour).Unix(), now.Add(time.Hour).Unix(), now, 24*time.Hour),
		"exactly AT maxStaleness is honored, matching VerifyAndLoad")
	require.True(t, IsStaleAt(now.Add(-24*time.Hour-time.Second).Unix(), now.Add(time.Hour).Unix(), now, 24*time.Hour))
}

func TestIsStaleAt_FutureValidFromIsStaleNotPerpetuallyFresh(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	require.True(t, IsStaleAt(now.Add(time.Hour).Unix(), now.Add(48*time.Hour).Unix(), now, 24*time.Hour))
}

func TestFreshness_ReadsWindowWithoutAnyRootKey(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	reg := Registry{Epoch: 1, ValidFrom: now.Unix(), ValidUntil: now.Add(time.Hour).Unix()}
	canonical, err := canonicalBytes(reg)
	require.NoError(t, err)

	// No signature, no root: freshness must still answer.
	vf, vu, err := Freshness(SignedRegistry{Registry: canonical.Bytes()})
	require.NoError(t, err)
	require.Equal(t, now.Unix(), vf)
	require.Equal(t, now.Add(time.Hour).Unix(), vu)
}

// MUTATION-REVIEW FINDING: the doc comment promises "a local prediction and the
// daemon's verdict cannot disagree about the boundary", and nothing checked it —
// `>` → `>=` on valid_until survived. The premise of the whole helper is that it
// PREDICTS VerifyAndLoad; that agreement is now the test.
func TestIsStaleAt_AgreesWithVerifyAndLoadAcrossTheBoundaries(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x5a)
	base := time.Unix(1_700_000_000, 0)
	maxStaleness := 24 * time.Hour

	// valid_until INSIDE the staleness bound, deliberately: with valid_until at
	// 48h and maxStaleness at 24h the age check always bites first, so the
	// valid_until boundary is never actually exercised — a fixture that cannot
	// reach the line it claims to guard (mutation-verified: `>` → `>=` survived
	// that shape).
	reg := Registry{Epoch: 1, ValidFrom: base.Unix(), ValidUntil: base.Add(12 * time.Hour).Unix()}
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)

	for _, now := range []time.Time{
		base.Add(-time.Second),               // before valid_from
		base,                                 // exactly valid_from
		base.Add(12 * time.Hour),             // EXACTLY valid_until (inside the age bound, so this boundary is reachable)
		base.Add(12*time.Hour + time.Second), // one tick past valid_until
		base.Add(maxStaleness),               // exactly at the staleness bound
		base.Add(maxStaleness + time.Second), // one tick past it
	} {
		vf, vu, ferr := Freshness(env)
		require.NoError(t, ferr)
		predicted := IsStaleAt(vf, vu, now, maxStaleness)

		_, _, verr := VerifyAndLoad(rootPub, env, 0, now, maxStaleness)
		authoritative := verr != nil

		require.Equal(t, authoritative, predicted,
			"local prediction and the daemon's verdict must agree at %s", now.UTC())
	}
}
