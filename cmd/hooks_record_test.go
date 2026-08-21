package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHooksRecord_RejectsEventsOutsideTheAllowlist is the security half.
//
// Everything written through this command is later read as evidence about agent
// behaviour. #121 closed one route by which non-agent activity reached the usage
// log; a free-text recorder would reopen it from the CLI side, where any script
// or typo could mint rows under an existing event name.
func TestHooksRecord_RejectsEventsOutsideTheAllowlist(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	for _, bad := range []string{"ask", "search", "note_access", "context_pack", "whatever"} {
		var out bytes.Buffer
		c := hooksRecordCmd
		c.SetOut(&out)
		c.SetErr(&out)
		err := runHooksRecord(c, []string{bad})
		require.Error(t, err, "%q must not be writable from the CLI", bad)
		require.Contains(t, err.Error(), "unknown hook event")
	}
}

// TestHooksRecord_SucceedsWithoutAUsageLog pins the failure policy.
//
// A hook records that it fired as a side effect of doing its real job. If the
// log is off or unavailable, the recording is skipped and the hook still
// succeeds — failing a compaction in order to note that it happened would be a
// worse bug than the ambiguity this command removes.
func TestHooksRecord_SucceedsWithoutAUsageLog(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	saved := experimentSession
	experimentSession = nil
	t.Cleanup(func() { experimentSession = saved })

	var out bytes.Buffer
	c := hooksRecordCmd
	c.SetOut(&out)
	c.SetErr(&out)

	require.NoError(t, runHooksRecord(c, []string{"write_prompt"}))
	require.Contains(t, out.String(), "not recorded",
		"the difference between recorded and unavailable must be stated, not swallowed")
}
