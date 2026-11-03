//go:build unix

package hookscripts

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lockedOutput struct {
	mu sync.Mutex
	b  []byte
}

func (o *lockedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.b = append(o.b, p...)
	return len(p), nil
}

func (o *lockedOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return string(append([]byte(nil), o.b...))
}

type vendorProbe struct {
	down   atomic.Bool
	hits   atomic.Int32
	server *httptest.Server
}

func newVendorProbe(t *testing.T) *vendorProbe {
	t.Helper()
	p := &vendorProbe{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		if p.down.Load() {
			h, ok := w.(http.Hijacker)
			if !ok {
				return
			}
			conn, _, err := h.Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	p.server = httptest.NewUnstartedServer(handler)
	p.server.Listener = listener
	p.server.Start()
	return p
}

func installVendorWatchIdentityStub(t *testing.T, bin string) {
	t.Helper()
	InstallStub(t, bin, "vaultmind", `
case "$*" in
 'identity paths')
  printf "VM_MESH_SLUG='mesh-watch-test'\nVM_MESH_SELF='agent:mesh-watch-test'\nVM_MESH_OPERATOR=''\nVM_MESH_DAEMON='%s'\nVM_MESH_REGISTRY_DAYS_LEFT=''\n" "$VM_TEST_DAEMON"
  for key in PID LISTEN DISARM HEARTBEAT LASTARM LASTWAKE CURSOR; do printf 'VM_MESH_%s=%q\n' "$key" "$VM_TEST_ROOT/$key"; done ;;
 'identity fetch-registry') exit 0 ;;
 *) exit 1 ;;
esac
`)
}

func vendorWatchEnv(bin, dir, daemon, probeURL string, ceiling, wait int) []string {
	return []string{
		"PATH=" + bin + ":" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + dir,
		"AGENT_CHAT_PROJECT_PATH=" + dir, "VM_TEST_ROOT=" + dir, "VM_TEST_DAEMON=" + daemon,
		"MESH_VENDOR_PROBE_URL=" + probeURL, "MESH_WAIT_SECS=" + strconv.Itoa(wait),
		"MESH_WATCH_MAX_WALL_SECS=" + strconv.Itoa(ceiling),
	}
}

func startVendorWatch(t *testing.T, d *watchDaemon, probeURL string, ceiling, wait int, extraEnv ...string) (*exec.Cmd, *lockedOutput, string) {
	t.Helper()
	server := httptest.NewServer(d)
	t.Cleanup(server.Close)
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	dir, bin := t.TempDir(), t.TempDir()
	script := filepath.Join(dir, "mesh-watch.sh")
	require.NoError(t, os.WriteFile(script, body, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "LISTEN"), []byte(`{"mode":"all","agents":{"mesh-watch-test":{"rooms":{"mesh":{"scope":"all"}}}}}`), 0o600))
	installVendorWatchIdentityStub(t, bin)
	cmd := exec.Command("bash", script)
	setWatchProcessGroup(cmd)
	cmd.WaitDelay = 3 * time.Second
	cmd.Env = vendorWatchEnv(bin, dir, server.URL, probeURL, ceiling, wait)
	cmd.Env = append(cmd.Env, extraEnv...)
	out := &lockedOutput{}
	cmd.Stdout, cmd.Stderr = out, out
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			killWatchProcessGroup(cmd.Process.Pid)
			_ = cmd.Wait()
		}
	})
	return cmd, out, dir
}

func waitForVendorHits(t *testing.T, p *vendorProbe) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for p.hits.Load() == 0 {
		require.True(t, time.Now().Before(deadline), "watcher did not probe the vendor API")
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(body)))
			if parseErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("watcher PID file %s was not populated", path)
	return 0
}

func waitForHeartbeatRefresh(t *testing.T, path string, before time.Time) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(before) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("heartbeat did not refresh after %s", before.Format(time.RFC3339Nano))
}

func waitForHeartbeat(t *testing.T, path string) time.Time {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil {
			return info.ModTime()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("heartbeat was not created")
	return time.Time{}
}

func waitForWatch(t *testing.T, cmd *exec.Cmd, timeout time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		killWatchProcessGroup(cmd.Process.Pid)
		t.Fatalf("watcher did not exit within %s", timeout)
		return nil
	}
}

func TestMeshWatch_VendorHoldReleasesAfterProbeRecovery(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	probe := newVendorProbe(t)
	t.Cleanup(probe.server.Close)
	probe.down.Store(true)
	cmd, out, dir := startVendorWatch(t, d, probe.server.URL, 30, 1)
	waitForArm(dir)
	waitForLongPoll(d)
	heartbeat := waitForHeartbeat(t, filepath.Join(dir, "HEARTBEAT"))
	d.add(watchMsg("vendor-hold", "agent:other", int(time.Now().Unix()), "mesh"))
	waitForVendorHits(t, probe)
	time.Sleep(1200 * time.Millisecond)
	assert.NotContains(t, out.String(), "WAKE: new mesh message", "the wake must remain held while the probe is down")
	waitForHeartbeatRefresh(t, filepath.Join(dir, "HEARTBEAT"), heartbeat)
	probe.down.Store(false)
	require.NoError(t, waitForWatch(t, cmd, 8*time.Second))
	text := out.String()
	wake, note := strings.Index(text, "WAKE: new mesh message"), strings.Index(text, "NOTE: held this wake ")
	assert.GreaterOrEqual(t, wake, 0, "the wake is printed after recovery")
	assert.Greater(t, note, wake, "the hold note follows the unchanged wake message")
	assert.Contains(t, text, "vendor API ("+probe.server.URL+") was unreachable")
}

func TestMeshWatch_OrphanedWatcherStopsVendorHold(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	probe := newVendorProbe(t)
	t.Cleanup(probe.server.Close)
	probe.down.Store(true)
	server := httptest.NewServer(d)
	t.Cleanup(server.Close)
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	dir, bin := t.TempDir(), t.TempDir()
	script := filepath.Join(dir, "mesh-watch.sh")
	outFile := filepath.Join(dir, "watch.out")
	require.NoError(t, os.WriteFile(script, body, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "LISTEN"), []byte(`{"mode":"all","agents":{"mesh-watch-test":{"rooms":{"mesh":{"scope":"all"}}}}}`), 0o600))
	installVendorWatchIdentityStub(t, bin)
	cmd := exec.Command("sh", "-c", `bash "$1" >"$2" 2>&1 & echo $!; read _`, "sh", script, outFile)
	cmd.Env = vendorWatchEnv(bin, dir, server.URL, probe.server.URL, 80, 1)
	setWatchProcessGroup(cmd)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = &lockedOutput{}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		killWatchProcessGroup(cmd.Process.Pid)
		_ = cmd.Wait()
	})
	watcherPIDText, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	watcherPID, err := strconv.Atoi(strings.TrimSpace(watcherPIDText))
	require.NoError(t, err)

	waitForArm(dir)
	waitForLongPoll(d)
	d.add(watchMsg("vendor-orphan", "agent:other", int(time.Now().Unix()), "mesh"))
	waitForVendorHits(t, probe)
	require.NoError(t, stdin.Close())

	deadline := time.Now().Add(10 * time.Second)
	gone := false
	for time.Now().Before(deadline) {
		if err := syscall.Kill(watcherPID, 0); err != nil {
			require.ErrorIs(t, err, syscall.ESRCH)
			gone = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, gone, "orphaned watcher still running")
	text, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(text), "WAKE: new mesh message from agent:other")
	assert.Contains(t, string(text), "NOTE: stopped holding this wake: the watcher's session is gone")
}

func TestMeshWatch_WrappedWatcherStopsVendorHoldWhenSessionGoes(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	probe := newVendorProbe(t)
	t.Cleanup(probe.server.Close)
	probe.down.Store(true)
	server := httptest.NewServer(d)
	t.Cleanup(server.Close)
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	dir, bin := t.TempDir(), t.TempDir()
	script := filepath.Join(dir, "mesh-watch.sh")
	outFile := filepath.Join(dir, "watch.out")
	require.NoError(t, os.WriteFile(script, body, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "LISTEN"), []byte(`{"mode":"all","agents":{"mesh-watch-test":{"rooms":{"mesh":{"scope":"all"}}}}}`), 0o600))
	installVendorWatchIdentityStub(t, bin)
	// The outer shell is the session. Its wrapper child deliberately runs a
	// non-exec shell so that the watcher stays one level below the session.
	cmd := exec.Command("sh", "-c", `sh -c 'bash "$1" >"$2" 2>&1; :' wrapper "$1" "$2" & read _`, "sh", script, outFile)
	cmd.Env = vendorWatchEnv(bin, dir, server.URL, probe.server.URL, 80, 1)
	setWatchProcessGroup(cmd)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	cmd.Stdout = &lockedOutput{}
	cmd.Stderr = cmd.Stdout
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		killWatchProcessGroup(cmd.Process.Pid)
		_ = cmd.Wait()
	})

	pidPath := filepath.Join(dir, "PID")
	waitForArm(dir)
	waitForLongPoll(d)
	d.add(watchMsg("vendor-wrapped-orphan", "agent:other", int(time.Now().Unix()), "mesh"))
	waitForVendorHits(t, probe)
	watcherPID := waitForPIDFile(t, pidPath)
	require.NoError(t, stdin.Close())

	deadline := time.Now().Add(10 * time.Second)
	gone := false
	for time.Now().Before(deadline) {
		if err := syscall.Kill(watcherPID, 0); err != nil {
			require.ErrorIs(t, err, syscall.ESRCH)
			gone = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, gone, "wrapped watcher still running")

	text, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(text), "WAKE: new mesh message from agent:other")
	assert.Contains(t, string(text), "NOTE: stopped holding this wake: the watcher's session is gone")
}

func TestMeshWatch_VendorProbeOffDoesNotHold(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	out, _ := runWatchDaemon(t, d, 12, 1, func(dir string) {
		waitForArm(dir)
		waitForLongPoll(d)
		d.add(watchMsg("vendor-off", "agent:other", int(time.Now().Unix()), "mesh"))
	}, "MESH_VENDOR_PROBE_URL=off")
	assert.Contains(t, out, "WAKE: new mesh message")
	assert.NotContains(t, out, "NOTE: held this wake")
}

// A disarm during the hold ends it, but a captured wake still wins over the
// sentinel, as it did before the hold existed: the message is never dropped.
func TestMeshWatch_DisarmDuringVendorHoldStillPrintsTheWake(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	probe := newVendorProbe(t)
	t.Cleanup(probe.server.Close)
	probe.down.Store(true)
	cmd, out, dir := startVendorWatch(t, d, probe.server.URL, 30, 1)
	waitForArm(dir)
	waitForLongPoll(d)
	d.add(watchMsg("vendor-disarm", "agent:other", int(time.Now().Unix()), "mesh"))
	waitForVendorHits(t, probe)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "DISARM"), nil, 0o600))
	require.NoError(t, waitForWatch(t, cmd, 5*time.Second))
	text := out.String()
	assert.Contains(t, text, "WAKE: new mesh message from agent:other")
	assert.NotContains(t, text, "DISARMED (sentinel present)")
}

// Without a captured wake, a disarm during the hold prints DISARMED: here the
// quiet-heartbeat RE-ARM is what was held.
func TestMeshWatch_DisarmDuringHeldReArmPrintsDisarmed(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	probe := newVendorProbe(t)
	t.Cleanup(probe.server.Close)
	probe.down.Store(true)
	cmd, out, dir := startVendorWatch(t, d, probe.server.URL, 2, 1)
	waitForArm(dir)
	waitForVendorHits(t, probe)
	time.Sleep(500 * time.Millisecond)
	assert.NotContains(t, out.String(), "RE-ARM:", "the quiet heartbeat is held while the probe is down")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "DISARM"), nil, 0o600))
	require.NoError(t, waitForWatch(t, cmd, 5*time.Second))
	text := out.String()
	assert.Contains(t, text, "DISARMED (sentinel present) — not re-arming.")
	assert.NotContains(t, text, "RE-ARM:")
}

func TestMeshWatch_SentinelBeforeVendorHoldPreservesWake(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	probe := newVendorProbe(t)
	t.Cleanup(probe.server.Close)
	probe.down.Store(true)
	cmd, out, dir := startVendorWatch(t, d, probe.server.URL, 30, 1)
	waitForArm(dir)
	waitForLongPoll(d)
	d.add(watchMsg("vendor-pre-disarm", "agent:other", int(time.Now().Unix()), "mesh"))
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "LASTWAKE")); err == nil {
			break
		}
		require.True(t, time.Now().Before(deadline), "message was not delivered before the sentinel was created")
		time.Sleep(20 * time.Millisecond)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "DISARM"), nil, 0o600))
	require.NoError(t, waitForWatch(t, cmd, 3*time.Second))
	text := out.String()
	assert.Contains(t, text, "WAKE: new mesh message from agent:other")
	assert.NotContains(t, text, "DISARMED (sentinel present) — not re-arming.")
}

func TestMeshWatch_TermDuringVendorHoldPrintsStopped(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	probe := newVendorProbe(t)
	t.Cleanup(probe.server.Close)
	probe.down.Store(true)
	cmd, out, dir := startVendorWatch(t, d, probe.server.URL, 30, 1)
	waitForArm(dir)
	waitForLongPoll(d)
	d.add(watchMsg("vendor-term", "agent:other", int(time.Now().Unix()), "mesh"))
	waitForVendorHits(t, probe)
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	require.NoError(t, waitForWatch(t, cmd, 3*time.Second))
	text := out.String()
	assert.Contains(t, text, "STOPPED: mesh-watch was stopped")
	assert.NotContains(t, text, "NOTE: held this wake")
}

// An empty MESH_VENDOR_PROBE_URL falls back to the default, which follows the
// agent's own route: ANTHROPIC_BASE_URL when set.
func TestMeshWatch_VendorProbeDefaultsToAnthropicBaseURL(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	probe := newVendorProbe(t)
	t.Cleanup(probe.server.Close)
	probe.down.Store(true)
	_, out, dir := startVendorWatch(t, d, "", 30, 1, "ANTHROPIC_BASE_URL="+probe.server.URL)
	waitForArm(dir)
	waitForLongPoll(d)
	d.add(watchMsg("vendor-base-url", "agent:other", int(time.Now().Unix()), "mesh"))
	waitForVendorHits(t, probe)
	time.Sleep(1200 * time.Millisecond)
	assert.NotContains(t, out.String(), "WAKE: new mesh message", "the default probe is the agent's ANTHROPIC_BASE_URL")
}

// Under Bedrock the watcher cannot know the agent's endpoint, so it must not
// hold on a probe the agent never uses.
func TestMeshWatch_VendorProbeOffByDefaultUnderBedrock(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	probe := newVendorProbe(t)
	t.Cleanup(probe.server.Close)
	probe.down.Store(true)
	cmd, out, dir := startVendorWatch(t, d, "", 30, 1, "ANTHROPIC_BASE_URL="+probe.server.URL, "CLAUDE_CODE_USE_BEDROCK=1")
	waitForArm(dir)
	waitForLongPoll(d)
	d.add(watchMsg("vendor-bedrock", "agent:other", int(time.Now().Unix()), "mesh"))
	require.NoError(t, waitForWatch(t, cmd, 8*time.Second))
	assert.Contains(t, out.String(), "WAKE: new mesh message from agent:other")
	assert.NotContains(t, out.String(), "NOTE: held this wake")
	assert.Zero(t, probe.hits.Load(), "no probe under Bedrock")
}
