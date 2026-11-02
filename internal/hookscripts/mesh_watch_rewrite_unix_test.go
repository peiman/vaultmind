//go:build unix

package hookscripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeshWatch_InPlaceRewriteDoesNotBreakARunningWatcher(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	out, _ := runWatchDaemon(t, d, 30, 1, func(dir string) {
		waitForArm(dir)
		waitForLongPoll(d)

		path := filepath.Join(dir, "mesh-watch.sh")
		before, err := os.Stat(path)
		require.NoError(t, err)
		body, ok := Get("mesh-watch.sh")
		require.True(t, ok)
		// The extra comments shift all later offsets, while the malformed tail
		// makes an old incrementally-read shell fail instead of merely exiting.
		rewritten := strings.Replace(string(body), "set -uo pipefail", strings.Repeat("# inserted while running\n", 2048)+"set -uo pipefail", 1)
		rewritten += "\nif [[\n"
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
		require.NoError(t, err)
		_, err = file.WriteString(rewritten)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		after, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, os.SameFile(before, after), "rewrite must preserve the watcher inode")

		d.add(watchMsg("rewrite", "agent:other", int(time.Now().Unix()), "mesh"))
	}, "MESH_VENDOR_PROBE_URL=off")

	assert.Contains(t, out, "WAKE: new mesh message")
	assert.NotContains(t, out, "syntax error")
}
