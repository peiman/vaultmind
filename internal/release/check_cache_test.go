package release

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReadCache_NegativeAnswersExpireSooner pins the asymmetry that makes this
// feature able to do its job.
//
// The two answers do not age the same way. "v0.7.0 is available" stays true
// however long it sits — nothing untags a release. "You are on the latest" stops
// being true the moment someone tags, and the cache had no way to know that had
// happened.
//
// Measured, not hypothesised: on 2026-08-21 the cache was written at 08:46:02
// with latest=v0.6.0, and v0.7.0 was tagged at 08:49:34 — three and a half
// minutes later. Under one 24h TTL for both answers, doctor would have gone on
// reporting "current" until the following morning. A staleness window inside the
// feature whose only purpose is to end silent staleness.
//
// A short negative TTL costs at most one proxy request per hour per machine —
// a GET for a version number, which is what `go install …@latest` does anyway.
func TestReadCache_NegativeAnswersExpireSooner(t *testing.T) {
	write := func(t *testing.T, info Info) string {
		t.Helper()
		dir := t.TempDir()
		body, err := json.Marshal(info)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(cachePath(dir), body, 0o600))
		return dir
	}

	twoHoursAgo := time.Now().UTC().Add(-2 * time.Hour)

	t.Run("a stale 'you are current' is not reused", func(t *testing.T) {
		dir := write(t, Info{
			Current: "v0.6.0", Latest: "v0.6.0", Newer: false, CheckedAt: twoHoursAgo,
		})
		_, ok := readCache(dir)
		require.False(t, ok, "a release may have been tagged since; go ask again")
	})

	t.Run("a positive answer survives the same age", func(t *testing.T) {
		dir := write(t, Info{
			Current: "v0.6.0", Latest: "v0.7.0", Newer: true, CheckedAt: twoHoursAgo,
		})
		got, ok := readCache(dir)
		require.True(t, ok, "nothing untags a release; this answer is still true")
		require.Equal(t, "v0.7.0", got.Latest)
	})

	t.Run("a fresh negative is still reused", func(t *testing.T) {
		dir := write(t, Info{
			Current: "v0.6.0", Latest: "v0.6.0", Newer: false,
			CheckedAt: time.Now().UTC().Add(-1 * time.Minute),
		})
		_, ok := readCache(dir)
		require.True(t, ok, "back-to-back doctor runs must not each hit the network")
	})

	t.Run("both expire at the outer bound", func(t *testing.T) {
		for _, newer := range []bool{true, false} {
			dir := write(t, Info{
				Current: "v0.6.0", Latest: "v0.7.0", Newer: newer,
				CheckedAt: time.Now().UTC().Add(-25 * time.Hour),
			})
			_, ok := readCache(dir)
			require.False(t, ok, "newer=%v: past cacheTTL nothing is reused", newer)
		}
	})
}
