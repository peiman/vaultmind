package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStatus_ReportsCanonicalEventsThatAreNotWired closes the same gap this
// command was built to close, one layer up.
//
// hooks status exists because doctor reported DRIFT but nothing reported
// ABSENCE: a hook the binary ships and a project never installed rendered as
// nothing, which looks exactly like health. It then compared script CONTENTS
// only — so a canonical event wired to no script at all stayed invisible for
// precisely the same reason.
//
// Not hypothetical. An adopter had three read-side hooks wired and SessionEnd
// absent; capture-episode.sh was present on disk and had produced 13 episodes,
// so every content-based check passed while the write half was switched off. The
// same project ran --pointers-only for a whole release cycle behind this class
// of blind spot.
//
// The canonical event→script map already lives in settings.go, where install
// uses it. Status simply never read it.
func TestStatus_ReportsCanonicalEventsThatAreNotWired(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude", "scripts"), 0o755))

	// The adopter's real shape, plus one case that is load-bearing for the
	// matcher itself: SessionStart is PRESENT in settings but runs somebody
	// else's script. Without that row this test cannot fail — for an event with
	// no entries the loop body never executes, so the script comparison is never
	// reached and a matcher that always says "wired" survives untouched.
	// Verified by mutation: strings.Contains(h.Command, "") passed the earlier
	// version of this test.
	settings := `{
	  "hooks": {
	    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/scripts/vault-recall.sh"}]}],
	    "SessionStart": [{"hooks": [{"type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/scripts/some-other-tool.sh"}]}]
	  }
	}`
	require.NoError(t, os.WriteFile(
		filepath.Join(project, ".claude", "settings.json"), []byte(settings), 0o600))

	report, err := Status(project)
	require.NoError(t, err)

	byEvent := map[string]EventState{}
	for _, e := range report.Events {
		byEvent[e.Event+"/"+e.Script] = e.State
	}
	require.NotEmpty(t, report.Events, "status must report event wiring, not only script contents")

	require.Equal(t, EventWired, byEvent["UserPromptSubmit/vault-recall.sh"],
		"this one IS wired")
	require.Equal(t, EventUnwired, byEvent["SessionEnd/capture-episode.sh"],
		"the write half is switched off and must be named")
	require.Equal(t, EventUnwired, byEvent["PreCompact/precompact-preserve.sh"],
		"the other write-path trigger, likewise")
	require.Equal(t, EventUnwired, byEvent["SessionStart/load-persona.sh"],
		"the event is wired, but to another tool's script — that is not our hook running")

	_, unwired := report.EventCounts()
	require.Positive(t, unwired, "an unwired canonical event must be counted, so it can gate")
}
