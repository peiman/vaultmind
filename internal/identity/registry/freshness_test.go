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
