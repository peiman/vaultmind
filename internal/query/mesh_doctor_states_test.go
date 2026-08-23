package query

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func touchAt(t *testing.T, path string, at time.Time) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	require.NoError(t, os.Chtimes(path, at, at))
}

// TestCheckHeartbeat_UnresolvedIsNotAbsent pins the distinction that hid a
// seven-day-dead watcher.
//
// With no path, the check CANNOT RUN — and a check that cannot run is not a
// pass. Doctor used to return silently here and the renderer printed
// "not found (wake-on-idle not confirmed)": a filesystem verdict for a lookup
// that never touched the filesystem. On darwin the default path was "" for
// every user (the config default was empty), so every macOS install reported
// the same string whether their watcher was alive, dead, or never armed.
func TestCheckHeartbeat_UnresolvedIsNotAbsent(t *testing.T) {
	mi := &DoctorMeshIdentity{}
	checkHeartbeat(mi, "", time.Now())

	require.Equal(t, HeartbeatUnresolved, mi.WatcherHeartbeatState)
	require.NotEmpty(t, mi.Warnings, "a check that cannot run must warn, not shrug")
	require.Contains(t, mi.Warnings[0], "UNRESOLVED")
}

// TestCheckHeartbeat_AbsentIsInfoNotWarning — most members have never armed a
// watcher; absence must not trip `jq '.status=="warning"'`. But it must be a
// NAMED state, distinguishable from unresolved in JSON and text.
func TestCheckHeartbeat_AbsentIsInfoNotWarning(t *testing.T) {
	mi := &DoctorMeshIdentity{}
	p := filepath.Join(t.TempDir(), "never-armed.heartbeat")
	checkHeartbeat(mi, p, time.Now())

	require.Equal(t, HeartbeatAbsent, mi.WatcherHeartbeatState)
	require.Equal(t, p, mi.WatcherHeartbeatPath, "the path must be carried so a reader can tell WHERE was checked")
	require.Empty(t, mi.Warnings)
}

// TestCheckHeartbeat_FreshAndStale — the two states that existed before, now
// with the state string and path carried alongside.
func TestCheckHeartbeat_FreshAndStale(t *testing.T) {
	now := time.Now()

	fresh := &DoctorMeshIdentity{}
	fp := filepath.Join(t.TempDir(), "hb")
	touchAt(t, fp, now.Add(-10*time.Second))
	checkHeartbeat(fresh, fp, now)
	require.Equal(t, HeartbeatFresh, fresh.WatcherHeartbeatState)
	require.True(t, fresh.WatcherHeartbeatFresh)

	stale := &DoctorMeshIdentity{}
	sp := filepath.Join(t.TempDir(), "hb")
	touchAt(t, sp, now.Add(-7*24*time.Hour))
	checkHeartbeat(stale, sp, now)
	require.Equal(t, HeartbeatStale, stale.WatcherHeartbeatState)
	require.False(t, stale.WatcherHeartbeatFresh)
	require.NotEmpty(t, stale.Warnings)
}

// TestCheckHeartbeat_FutureMtimeIsStaleNotFresh — a heartbeat from the future
// is a liar (clock skew, or a hand-written file), not a healthy watcher. The
// old clamp made it render FRESH, which is the most trusted state granted on
// the least trustworthy evidence.
func TestCheckHeartbeat_FutureMtimeIsStaleNotFresh(t *testing.T) {
	mi := &DoctorMeshIdentity{}
	p := filepath.Join(t.TempDir(), "hb")
	touchAt(t, p, time.Now().Add(2*time.Hour))
	checkHeartbeat(mi, p, time.Now())

	require.Equal(t, HeartbeatStale, mi.WatcherHeartbeatState)
	require.False(t, mi.WatcherHeartbeatFresh)
}

// TestCheckHeartbeat_SymlinkReportsTargetMtime — Stat, not Lstat. A heartbeat
// reached through a symlink used to report the LINK's mtime and read
// permanently stale. There is no security rationale for Lstat here (unlike the
// key-custody check, where refusing symlinks is the point).
func TestCheckHeartbeat_SymlinkReportsTargetMtime(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	real := filepath.Join(dir, "real.heartbeat")
	touchAt(t, real, now.Add(-5*time.Second)) // re-stamped stale below
	link := filepath.Join(dir, "link.heartbeat")
	require.NoError(t, os.Symlink(real, link))
	// os.Lchtimes does not exist in this Go version; the link's own mtime is
	// whatever Symlink stamped (now). The discriminating direction still holds:
	// under Lstat the age would be ~0 via the LINK regardless of the target, so
	// invert the fixture — make the TARGET stale and the link fresh, and require
	// the verdict to follow the TARGET.
	require.NoError(t, os.Chtimes(real, now.Add(-48*time.Hour), now.Add(-48*time.Hour)))

	// The check time must sit AFTER the link's own creation mtime, else the
	// link's mtime is "in the future" and the future-mtime rule ALSO reports
	// stale — making Stat and Lstat indistinguishable. (Found by mutation: the
	// first version of this fixture passed identically under both.) Anchor the
	// clock to the link's actual mtime, a few seconds ahead: under Lstat the
	// link then reads FRESH (~5s old), under Stat the target reads STALE (48h).
	linkInfo, err := os.Lstat(link)
	require.NoError(t, err)
	checkTime := linkInfo.ModTime().Add(5 * time.Second)

	mi := &DoctorMeshIdentity{}
	checkHeartbeat(mi, link, checkTime)
	require.Equal(t, HeartbeatStale, mi.WatcherHeartbeatState,
		"the TARGET's mtime is the watcher's liveness; under Lstat the fresh link would mask a stale target")
}

// TestCheckRearm_WokeAndNeverRearmed is the check that would have caught the
// real seven-day death, verified against the live files before this was built:
// lastwake mtime 2026-08-16 23:29 with no subsequent arm. LASTWAKE records
// DETECTION; only a new arm records that anyone came back. wake > arm ⇒ the
// watcher fired, the session was (supposedly) re-invoked, and nobody re-armed —
// every message since then is landing in silence.
func TestCheckRearm_WokeAndNeverRearmed(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	arm := filepath.Join(dir, "la")
	wake := filepath.Join(dir, "lw")
	touchAt(t, arm, now.Add(-3*time.Hour))
	touchAt(t, wake, now.Add(-1*time.Hour)) // woke AFTER the last arm

	mi := &DoctorMeshIdentity{}
	checkRearm(mi, wake, arm)

	require.False(t, mi.WatcherRearmed)
	found := false
	for _, w := range mi.Warnings {
		if w == WarnMeshWatcherNotRearmed {
			found = true
		}
	}
	require.True(t, found, "woke-after-arm must warn: %v", mi.Warnings)
}

// TestCheckRearm_QuietStates — armed-after-wake is healthy; either file absent
// means the pair cannot testify and must stay silent (the heartbeat states
// carry those diagnoses).
func TestCheckRearm_QuietStates(t *testing.T) {
	now := time.Now()

	healthy := &DoctorMeshIdentity{}
	d1 := t.TempDir()
	touchAt(t, filepath.Join(d1, "lw"), now.Add(-2*time.Hour))
	touchAt(t, filepath.Join(d1, "la"), now.Add(-1*time.Hour)) // re-armed after waking
	checkRearm(healthy, filepath.Join(d1, "lw"), filepath.Join(d1, "la"))
	require.True(t, healthy.WatcherRearmed)
	require.Empty(t, healthy.Warnings)

	silent := &DoctorMeshIdentity{}
	checkRearm(silent, filepath.Join(t.TempDir(), "no-lw"), filepath.Join(t.TempDir(), "no-la"))
	require.Empty(t, silent.Warnings, "absent files cannot testify either way")
}
