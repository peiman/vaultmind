package hookscripts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func catchupDaemon() *watchDaemon {
	ts := int(time.Now().Unix()) - 120
	return newWatchDaemon(watchMsg("history-room", "agent:history", ts, "mesh"), watchMsg("history-other", "agent:history", ts, "other"), watchMsg("history-dm", "agent:history", ts, "dm"))
}

func wakeAndSave(t *testing.T, d *watchDaemon) string {
	t.Helper()
	out, dir := runWatchDaemon(t, d, 8, 20, func(_ string) {
		waitForLongPoll(d)
		d.add(watchMsg("A", "agent:A", int(time.Now().Unix())-60, "mesh"))
	})
	require.Contains(t, out, "WAKE: new mesh message from agent:A")
	return dir
}

func TestMeshWatch_CatchupGapClosed(t *testing.T) {
	t.Parallel()
	for _, stream := range []string{"mesh", "other", "dm"} {
		t.Run(stream, func(t *testing.T) {
			d := catchupDaemon()
			dir := wakeAndSave(t, d)
			d.add(watchMsg("B", "agent:B", int(time.Now().Unix())-30, stream))
			started := time.Now()
			out, _ := runWatchDaemon(t, d, 7, 20, nil, "VM_TEST_ROOT="+dir)
			require.Contains(t, out, "message from agent:B", "a message between watchers must not become history")
			require.Less(t, time.Since(started), 10*time.Second, "catch-up must beat the 20s wait window")
		})
	}
}

func TestMeshWatch_CatchupNoRewake(t *testing.T) {
	t.Parallel()
	d := catchupDaemon()
	dir := wakeAndSave(t, d)
	raw, err := os.ReadFile(filepath.Join(dir, "CURSOR"))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"A"`, "save the cursor AFTER detecting the wake")
	out, _ := runWatchDaemon(t, d, 3, 20, nil, "VM_TEST_ROOT="+dir)
	require.NotContains(t, out, "WAKE:")
	require.Contains(t, out, "quiet heartbeat")
}

func TestMeshWatch_CatchupHeartbeatSaves(t *testing.T) {
	t.Parallel()
	d := catchupDaemon()
	out, dir := runWatchDaemon(t, d, 3, 20, nil)
	require.Contains(t, out, "quiet heartbeat")
	d.add(watchMsg("B", "agent:B", int(time.Now().Unix())-30, "mesh"))
	out, _ = runWatchDaemon(t, d, 7, 20, nil, "VM_TEST_ROOT="+dir)
	require.Contains(t, out, "message from agent:B")
}

func writeCatchupState(t *testing.T, savedAt int64, rooms map[string]any, dm any) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := json.Marshal(map[string]any{"saved_at": savedAt, "rooms": rooms, "dm": dm})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CURSOR"), raw, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "LISTEN"), []byte(`{"mode":"all","agents":{"mesh-watch-test":{"rooms":{"mesh":{"scope":"all"},"other":{"scope":"all"}}}}}`), 0o600))
	return dir
}

func TestMeshWatch_CatchupBaselineFallback(t *testing.T) {
	t.Parallel()
	ts := int(time.Now().Unix()) - 120
	cursor := func(n int) map[string]any { return map[string]any{"ts": n, "ids": []string{}} }
	for _, tc := range []struct {
		name   string
		age    int64
		roomTS int
		rooms  map[string]any
		env    []string
	}{
		{name: "stale", age: 21601, roomTS: ts},
		{name: "custom staleness", age: 60, roomTS: ts, env: []string{"MESH_CATCHUP_MAX_SECS=30"}},
		{name: "newer than hub", roomTS: ts + 300},
		{name: "new room", rooms: map[string]any{"other": cursor(ts + 300)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rooms := tc.rooms
			if rooms == nil {
				rooms = map[string]any{"mesh": cursor(tc.roomTS)}
			}
			dir := writeCatchupState(t, time.Now().Unix()-tc.age, rooms, cursor(ts+300))
			d := catchupDaemon()
			d.add(watchMsg("B", "agent:B", ts+60, "mesh"))
			env := append([]string{"VM_TEST_ROOT=" + dir}, tc.env...)
			out, _ := runWatchDaemon(t, d, 3, 20, nil, env...)
			require.NotContains(t, out, "WAKE:", "use live baseline for this stream")
			require.Contains(t, out, "quiet heartbeat")
		})
	}
}

func TestMeshWatch_CatchupCorruptIgnored(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"garbage", `{"saved_at":1,"rooms":[]}`, `{"saved_at":9999999999,"rooms":{"mesh":{"ts":"bad","ids":[]}}}`} {
		t.Run(body, func(t *testing.T) {
			dir := writeCatchupState(t, 0, nil, nil)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "CURSOR"), []byte(body), 0o600))
			d := catchupDaemon()
			d.add(watchMsg("B", "agent:B", int(time.Now().Unix())-30, "mesh"))
			out, _ := runWatchDaemon(t, d, 8, 20, func(_ string) {
				waitForLongPoll(d)
				d.add(watchMsg("C", "agent:C", int(time.Now().Unix()), "mesh"))
			}, "VM_TEST_ROOT="+dir)
			require.Contains(t, out, "message from agent:C")
			require.Equal(t, 1, strings.Count(out, "mesh-watch: ignoring saved cursor"))
			require.NotContains(t, out, "message from agent:B")
		})
	}
}

func TestMeshWatch_CatchupUnsetPath(t *testing.T) {
	t.Parallel()
	d := catchupDaemon()
	dir := wakeAndSave(t, d)
	d.add(watchMsg("B", "agent:B", int(time.Now().Unix())-30, "mesh"))
	out, _ := runWatchDaemon(t, d, 3, 20, nil, "VM_TEST_ROOT="+dir, "VM_TEST_NO_CURSOR=1")
	require.NotContains(t, out, "WAKE:")
	require.Contains(t, out, "quiet heartbeat")
}

func TestMeshWatch_CatchupStopSaves(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{"disarm", "error"} {
		t.Run(reason, func(t *testing.T) {
			d := catchupDaemon()
			out, dir := runWatchDaemon(t, d, 10, 1, func(dir string) {
				waitForLongPoll(d)
				if reason == "disarm" {
					_ = os.WriteFile(filepath.Join(dir, "DISARM"), nil, 0o600)
				} else {
					d.down.Store(true)
				}
			}, "MESH_OUTAGE_GIVEUP_SECS=0", "MESH_SWEEP_SECS=1")
			if reason == "disarm" {
				require.Contains(t, out, "DISARMED")
				require.NoError(t, os.Remove(filepath.Join(dir, "DISARM")))
			} else {
				require.Contains(t, out, "WATCHER ERROR:")
				d.down.Store(false)
			}
			d.add(watchMsg("B", "agent:B", int(time.Now().Unix())-30, "mesh"))
			out, _ = runWatchDaemon(t, d, 7, 20, nil, "VM_TEST_ROOT="+dir)
			require.Contains(t, out, "message from agent:B")
		})
	}
}

func TestMeshWatch_CatchupSameSecondAndSaveTime(t *testing.T) {
	t.Parallel()
	ts := int(time.Now().Unix()) - 60
	d := newWatchDaemon(watchMsg("A", "agent:A", ts, "mesh"))
	c := map[string]any{"ts": ts, "ids": []string{"A"}}
	dir := writeCatchupState(t, time.Now().Unix(), map[string]any{"mesh": c}, map[string]any{"ts": ts + 300, "ids": []string{}})
	old := time.Now().Add(-24 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "CURSOR"), old, old))
	d.add(watchMsg("B", "agent:B", ts, "mesh"))
	out, _ := runWatchDaemon(t, d, 7, 20, nil, "VM_TEST_ROOT="+dir)
	require.Contains(t, out, "message from agent:B", "same-second IDs matter, and save time is independent of mtime")
}

func TestMeshWatch_CatchupUnreadableIgnored(t *testing.T) {
	t.Parallel()
	dir := writeCatchupState(t, time.Now().Unix(), map[string]any{}, nil)
	require.NoError(t, os.Chmod(filepath.Join(dir, "CURSOR"), 0))
	d := catchupDaemon()
	out, _ := runWatchDaemon(t, d, 8, 20, func(_ string) {
		waitForLongPoll(d)
		d.add(watchMsg("C", "agent:C", int(time.Now().Unix()), "mesh"))
	}, "VM_TEST_ROOT="+dir)
	require.Contains(t, out, "message from agent:C")
	require.Equal(t, 1, strings.Count(out, "mesh-watch: ignoring saved cursor"))
}

func TestMeshWatch_CatchupZeroTimestamp(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon(watchMsg("A", "agent:A", 0, "mesh"))
	out, dir := runWatchDaemon(t, d, 3, 20, nil)
	require.Contains(t, out, "quiet heartbeat")
	b := watchMsg("B", "agent:B", 0, "mesh")
	b.ReceiveTS = int(time.Now().Unix())
	d.add(b)
	// Ensure this gap message's receive time is strictly before the new arm.
	time.Sleep(2 * time.Second)
	out, _ = runWatchDaemon(t, d, 7, 20, nil, "VM_TEST_ROOT="+dir)
	require.Contains(t, out, "message from agent:B")
}

func TestMeshWatch_CatchupBaselineErrorPreservesDM(t *testing.T) {
	t.Parallel()
	d := catchupDaemon()
	dir := wakeAndSave(t, d)
	d.add(watchMsg("B", "agent:B", int(time.Now().Unix())-30, "dm"))
	d.baselineDown.Store(true)
	out, _ := runWatchDaemon(t, d, 5, 1, nil, "VM_TEST_ROOT="+dir, "MESH_OUTAGE_GIVEUP_SECS=0")
	require.Contains(t, out, "WATCHER ERROR:")
	d.baselineDown.Store(false)
	out, _ = runWatchDaemon(t, d, 7, 20, nil, "VM_TEST_ROOT="+dir)
	require.Contains(t, out, "message from agent:B", "a failed arm must not replace an unobserved DM cursor with its live baseline")
}
