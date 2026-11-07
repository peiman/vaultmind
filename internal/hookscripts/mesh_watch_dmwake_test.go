package hookscripts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The first room's long poll also carries the DM mailbox. A DM woken on that
// poll used to be announced as "new mesh message", so an agent read the mesh
// room, found nothing, and concluded there was nothing (workhorse, 2026-11-07).
// The wake line must name the scope it actually saw.
func TestMeshWatch_DMWakeNamesTheMailbox(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon()
	out, _ := runWatchDaemon(t, d, 8, 20, func(_ string) {
		waitForLongPoll(d)
		d.add(watchMsg("dm-1", "agent:cm", int(time.Now().Unix())+1, "dm"))
	})
	require.Contains(t, out, "WAKE: new DM message from agent:cm")
	require.Contains(t, out, "read your DM mailbox")
	require.NotContains(t, out, "new mesh message", "a DM must not be announced as a room message")
}
