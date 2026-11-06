//go:build unix

package hookscripts

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMeshWatch_SleepGapDoesNotCountAsOutage(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	const (
		giveup     = 10
		wait       = 1
		suspendGap = 10 // MESH_SUSPEND_GAP_SECS: keeps the test short; the default is derived
	)
	downAfterResume := 5 * time.Second
	stopFor := time.Duration(giveup+suspendGap+1) * time.Second
	out, _ := runWatchDaemon(t, d, giveup+suspendGap+60, wait, func(dir string) {
		waitForArm(dir)
		waitForLongPoll(d)
		d.down.Store(true)
		deadline := time.Now().Add(10 * time.Second)
		for d.downHits.Load() < 1 {
			if !time.Now().Before(deadline) {
				t.Fatalf("the watcher did not observe the outage")
			}
			time.Sleep(100 * time.Millisecond)
		}
		pidBytes, err := os.ReadFile(filepath.Join(dir, "PID"))
		if err != nil {
			t.Fatalf("read watcher pid: %v", err)
		}
		pid, err := strconv.Atoi(string(bytes.TrimSpace(pidBytes)))
		if err != nil {
			t.Fatalf("parse watcher pid: %v", err)
		}
		if err := stopWatchProcessGroup(pid); err != nil {
			t.Fatalf("stop watcher process group: %v", err)
		}
		time.Sleep(stopFor)
		if err := continueWatchProcessGroup(pid); err != nil {
			t.Fatalf("continue watcher process group: %v", err)
		}
		// Keep the hub down after resume, shorter than the give-up window: every
		// worker then fails a read (reads held across the stop fail without a new
		// request, so the hit count cannot be the signal). Counting the stop as
		// outage gives up here; restarting the clock at resume does not.
		time.Sleep(downAfterResume)
		d.down.Store(false)
		d.add(watchMsg("after-sleep", "agent:other", int(time.Now().Unix()), "mesh"))
	}, "MESH_SWEEP_SECS=1", "MESH_OUTAGE_GIVEUP_SECS="+strconv.Itoa(giveup), "MESH_SUSPEND_GAP_SECS="+strconv.Itoa(suspendGap))

	assert.NotContains(t, out, "WATCHER ERROR", "machine sleep is not a hub outage: %s", out)
	assert.Contains(t, out, "WAKE: new mesh message from agent:other", "the watcher recovers after resume")
}
