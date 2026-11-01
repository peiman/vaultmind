package hookscripts

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// #371: the watcher ended after 10 failed overlap reads, ~150 s at the 15 s
// cadence, so any hub blip longer than that left the agent deaf until a manual
// re-arm (seen four times on 2026-10-29, once for 32 hours). It now retries
// through an outage and gives up only after MESH_OUTAGE_GIVEUP_SECS of
// continuous failure, saying what the last failure was.

func waitForArm(dir string) {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "LASTARM")); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitForLongPoll returns once the watcher's long poll reaches the daemon:
// baselines are done and the watcher is watching. LASTARM alone comes earlier.
func waitForLongPoll(d *watchDaemon) {
	deadline := time.After(20 * time.Second)
	for {
		select {
		case r := <-d.waits:
			if r.Path == "/chat_wait" {
				return
			}
		case <-deadline:
			return
		}
	}
}

func TestMeshWatch_ShortHubOutageDoesNotEndTheWatcher(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	out, _ := runWatchDaemon(t, d, 60, 2, func(dir string) {
		waitForArm(dir)
		waitForLongPoll(d)
		d.down.Store(true)
		time.Sleep(12 * time.Second) // 12 failed reads at a 1 s cadence: past the old count of 10
		d.down.Store(false)
		d.add(watchMsg("after-outage", "agent:other", int(time.Now().Unix()), "mesh"))
	}, "MESH_SWEEP_SECS=1")

	assert.NotContains(t, out, "WATCHER ERROR", "a short outage must not end the watcher: %s", out)
	assert.Contains(t, out, "WAKE: new mesh message from agent:other", "it recovers and wakes on the message after the outage")
}

func TestMeshWatch_PersistentOutageEndsLoudlyWithTheCause(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	start := time.Now()
	out, _ := runWatchDaemon(t, d, 60, 2, func(dir string) {
		waitForArm(dir)
		d.down.Store(true)
	}, "MESH_SWEEP_SECS=1", "MESH_OUTAGE_GIVEUP_SECS=4")

	assert.Contains(t, out, "WATCHER ERROR", "a persistent outage still ends the watcher loudly")
	assert.Contains(t, out, "unreachable for", "and says how long it has been down")
	assert.Contains(t, out, "503", "and what the last failure was, so a hub outage is told apart from a watcher fault")
	assert.Less(t, time.Since(start), 45*time.Second, "within the give-up window, not at the wall-clock ceiling")
}
