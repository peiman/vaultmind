package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Whitebox: the unhandled-event guard cannot be reached through the public API
// (canonicalHooks is the only caller and it is correct), and a guard nobody can
// exercise is a comment rather than a check.
//
// The failure it prevents was real: PreCompact was added to the wiring table
// before the struct had a field for it, and the stanza rendered
// `"PreCompact": null` — the hook listed as canonical and silently never wired.
func TestBuildHooksObject_UnhandledEventFailsLoudly(t *testing.T) {
	_, err := buildHooksObject([]canonicalHook{
		{Event: "SomeFutureEvent", Script: "future.sh", Group: hookGroup{}},
	})
	require.Error(t, err, "an unmapped event must not render as nothing")
	assert.Contains(t, err.Error(), "future.sh", "the error names the script")
	assert.Contains(t, err.Error(), "SomeFutureEvent", "and the event")
}

// Every event in the real table maps to a field. This is the positive half: if
// someone adds a canonical hook and wires it correctly, no error.
func TestBuildHooksObject_EveryCanonicalEventIsMapped(t *testing.T) {
	obj, err := buildHooksObject(canonicalHooks(""))
	require.NoError(t, err)

	assert.Len(t, obj.SessionStart, 2, "persona loader and health nudge")
	assert.Len(t, obj.UserPromptSubmit, 1)
	assert.Len(t, obj.PreToolUse, 2, "read-tracking and reach-pointers")
	assert.Len(t, obj.PreCompact, 1, "the write-path trigger")
	assert.Len(t, obj.SessionEnd, 1)
}
