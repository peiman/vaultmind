//go:build unix

package hookscripts

import (
	"bytes"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A watcher that dies without running its cleanup (SIGKILL: the takeover's
// fallback after 5 s) left its stream loops running with no parent. They kept
// the session's output open and long-polled forever. On main they hung the
// takeover test for the whole package timeout (2026-10-31): since #371 a stream
// whose reads fail keeps retrying for the 10-minute outage window. A stream now
// ends as soon as its watcher is gone.
func TestMeshWatch_StreamsEndWhenTheirWatcherIsKilled(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	c, dir := startKillableWatcher(t, d)

	waitForArm(dir)
	waitForLongPoll(d)
	killAndRequireNoSurvivors(t, c, "its streams")
}

// Killed while the hub is down, the watcher is still retrying its first
// reading in a command substitution: a child that is not one of its jobs. That
// retry ran for the whole outage window (600 s), and an arm stopped mid-baseline
// left one behind, holding the output: the 9m22s hang on main.
func TestMeshWatch_BaselineRetryEndsWhenItsWatcherIsKilled(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	d.down.Store(true)
	c, _ := startKillableWatcher(t, d)

	deadline := time.Now().Add(20 * time.Second)
	for d.downHits.Load() == 0 {
		require.True(t, time.Now().Before(deadline), "the watcher never tried its baseline")
		time.Sleep(100 * time.Millisecond)
	}
	killAndRequireNoSurvivors(t, c, "its baseline retry")
}

func startKillableWatcher(t *testing.T, d *watchDaemon) (*exec.Cmd, string) {
	t.Helper()
	server := httptest.NewServer(d)
	t.Cleanup(server.Close)
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	dir, bin := t.TempDir(), t.TempDir()
	script := filepath.Join(dir, "mesh-watch.sh")
	require.NoError(t, os.WriteFile(script, body, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "LISTEN"), []byte(`{"mode":"all","agents":{"mesh-watch-test":{"rooms":{"mesh":{"scope":"all"},"other":{"scope":"all"}}}}}`), 0o600))
	InstallStub(t, bin, "vaultmind", `
case "$*" in
 'identity paths')
  printf "VM_MESH_SLUG='mesh-watch-test'\nVM_MESH_SELF='agent:mesh-watch-test'\nVM_MESH_OPERATOR=''\nVM_MESH_DAEMON='%s'\nVM_MESH_REGISTRY_DAYS_LEFT=''\n" "$VM_TEST_DAEMON"
  for key in PID LISTEN DISARM HEARTBEAT LASTARM LASTWAKE; do printf 'VM_MESH_%s=%q\n' "$key" "$VM_TEST_ROOT/$key"; done ;;
 'identity fetch-registry') exit 0 ;;
 *) exit 1 ;;
esac
`)
	var out bytes.Buffer
	c := exec.Command("bash", script)
	c.Env = []string{"PATH=" + bin + ":" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir(), "AGENT_CHAT_PROJECT_PATH=" + dir, "VM_TEST_ROOT=" + dir, "VM_TEST_DAEMON=" + server.URL, "MESH_WAIT_SECS=2", "MESH_WATCH_MAX_WALL_SECS=120"}
	c.Stdout, c.Stderr = &out, &out
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, c.Start())
	pgid := c.Process.Pid
	// Whatever the outcome, nothing from this watcher outlives the test.
	t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })

	return c, dir
}

// Wait returns only once every process holding the output has exited.
func killAndRequireNoSurvivors(t *testing.T, c *exec.Cmd, what string) {
	t.Helper()
	require.NoError(t, c.Process.Signal(syscall.SIGKILL))
	exited := make(chan struct{})
	go func() { _ = c.Wait(); close(exited) }()
	select {
	case <-exited:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s outlived the watcher: 20 s after SIGKILL its output was still held open", what)
	}
}
