package hookscripts

import (
	"bytes"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #362/#363: arming takes over a running watcher by sending it TERM. The old
// watcher's TERM trap ran cleanup (killing its streams) and then RESUMED, so its
// main loop saw every stream dead and printed "WATCHER ERROR: all streams
// exited … re-arm", exit 3. The session obeyed and re-armed, which took over
// the newer watcher, which said the same: two watchers taking the pidfile from
// each other. A replaced watcher must say so, exit cleanly, and NOT ask for a
// re-arm, while its successor keeps watching.
func TestMeshWatch_ReplacedWatcherSaysSoAndDoesNotAskForReArm(t *testing.T) {
	server := httptest.NewServer(newWatchDaemon())
	defer server.Close()
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	dir, bin := t.TempDir(), t.TempDir()
	script := filepath.Join(dir, "mesh-watch.sh")
	require.NoError(t, os.WriteFile(script, body, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "LISTEN"), []byte(`{"mode":"all","agents":{"mesh-watch-test":{"rooms":{"mesh":{"scope":"all"}}}}}`), 0o600))
	InstallStub(t, bin, "vaultmind", `
case "$*" in
 'identity paths')
  printf "VM_MESH_SLUG='mesh-watch-test'\nVM_MESH_SELF='agent:mesh-watch-test'\nVM_MESH_OPERATOR=''\nVM_MESH_DAEMON='%s'\nVM_MESH_REGISTRY_DAYS_LEFT=''\n" "$VM_TEST_DAEMON"
  for key in PID LISTEN DISARM HEARTBEAT LASTARM LASTWAKE; do printf 'VM_MESH_%s=%q\n' "$key" "$VM_TEST_ROOT/$key"; done ;;
 'identity fetch-registry') exit 0 ;;
 *) exit 1 ;;
esac
`)
	env := []string{"PATH=" + bin + ":" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir(), "AGENT_CHAT_PROJECT_PATH=" + dir, "VM_TEST_ROOT=" + dir, "VM_TEST_DAEMON=" + server.URL, "MESH_WAIT_SECS=2", "MESH_WATCH_MAX_WALL_SECS=60"}
	start := func() (*exec.Cmd, *bytes.Buffer) {
		var out bytes.Buffer
		c := exec.Command("bash", script)
		c.Env, c.Stdout, c.Stderr = env, &out, &out
		require.NoError(t, c.Start())
		return c, &out
	}
	waitFor := func(cond func() bool, what string) {
		deadline := time.Now().Add(20 * time.Second)
		for !cond() {
			require.True(t, time.Now().Before(deadline), "timed out waiting for %s", what)
			time.Sleep(100 * time.Millisecond)
		}
	}
	pidIs := func(p int) bool {
		raw, err := os.ReadFile(filepath.Join(dir, "PID"))
		return err == nil && strings.TrimSpace(string(raw)) == strconv.Itoa(p)
	}

	a, aOut := start()
	waitFor(func() bool { return pidIs(a.Process.Pid) }, "the first watcher to arm")
	b, bOut := start()
	defer func() {
		_ = os.WriteFile(filepath.Join(dir, "DISARM"), nil, 0o600) // stop the successor
		_ = b.Wait()
	}()

	aErr := a.Wait()
	aText := aOut.String()
	assert.NoError(t, aErr, "a replaced watcher exits cleanly (was exit 3): %s", aText)
	assert.Contains(t, aText, "REPLACED", "it says it was replaced")
	assert.Contains(t, aText, strconv.Itoa(b.Process.Pid), "and by which watcher")
	assert.NotContains(t, aText, "WATCHER ERROR", "a takeover is not an error")
	assert.NotContains(t, strings.ToLower(aText), "re-arm\n", "and must not ask for the re-arm that would replace the successor")

	time.Sleep(time.Second)
	assert.Nil(t, b.ProcessState, "the successor keeps watching: %s", bOut.String())
	assert.True(t, pidIs(b.Process.Pid), "and owns the pidfile")
}
