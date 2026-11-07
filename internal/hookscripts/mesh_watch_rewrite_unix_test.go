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
	// MESH_WATCH_MAX_WALL_SECS. Idle, startup plus the one-second poll is a
	// few seconds. A loaded task check instead reported the quiet heartbeat
	// at the old 30s ceiling: the message was added on the first poll of
	// either room, so the mesh baseline could record it as history and the
	// watcher then sat out the ceiling. 120s is more than ten times that
	// idle run. Rewrite only once the mesh poll is up — the script has been
	// parsed by then — and the minute after the 60s poll wait is for the rewrite.
	out, _ := runWatchDaemon(t, d, 120, 1, func(dir string) {
		requireStreamLongPoll(t, d, "mesh", 60*time.Second)

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

		// Stamp on receipt, strictly after the arm. The empty-baseline cursor is
		// the arm second, and a same-second ts is history.
		d.add(watchMsg("rewrite", "agent:other", int(time.Now().Unix())+1, "mesh"))
	}, "MESH_VENDOR_PROBE_URL=off")

	assert.Contains(t, out, "WAKE: new mesh message")
	assert.NotContains(t, out, "syntax error")
}
