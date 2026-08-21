package hooks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateRecordable_IsAClosedSet pins the allowlist.
//
// The usage log is the evidence base for every retrieval measurement. #121
// closed one route by which non-agent activity reached it; a free-text event
// recorder would reopen the same hole from the CLI side, where any script or
// typo could inject rows that later read as agent behaviour.
func TestValidateRecordable_IsAClosedSet(t *testing.T) {
	require.NoError(t, ValidateRecordable("write_prompt"))

	for _, bad := range []string{"", "ask", "search", "note_access", "anything", "WRITE_PROMPT"} {
		require.Error(t, ValidateRecordable(bad), "%q must not be recordable from the CLI", bad)
	}
}

// TestRecordableNames_IsDeterministic — an error message or help string that
// reorders between runs cannot be asserted on, which is how a message drifts
// out of sync with the set it describes.
func TestRecordableNames_IsDeterministic(t *testing.T) {
	first := RecordableNames()
	for i := 0; i < 20; i++ {
		require.Equal(t, first, RecordableNames(), "run %d reordered", i)
	}
	require.Contains(t, first, "write_prompt")
}
