package hooks

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #357: Claude Code cuts each SessionStart hook's output to a preview past
// ~10,000 characters, so the arc layer gets a hook of its own. load-arcs.sh is
// wired wherever load-persona.sh is: the same event and matcher, right after
// it, for the persona and full profiles, and for every agent that runs the
// persona loader.

func TestArcsHook_WiredRightAfterThePersonaLoader(t *testing.T) {
	chs := canonicalHooks("")
	var starts []string
	for _, ch := range chs {
		if ch.Event == "SessionStart" {
			starts = append(starts, ch.Script)
			if ch.Script == hookArcsScript {
				assert.Equal(t, sessionStartMatcher, ch.Group.Matcher)
			}
		}
	}
	i := slices.Index(starts, hookSessionStartScript)
	require.GreaterOrEqual(t, i, 0)
	require.Less(t, i+1, len(starts))
	assert.Equal(t, hookArcsScript, starts[i+1], "the arcs follow the persona loader")

	stanza, err := SettingsStanza("")
	require.NoError(t, err)
	assert.Contains(t, stanza, "load-arcs.sh")
}

func TestArcsHook_InThePersonaAndFullProfilesOnly(t *testing.T) {
	assert.Contains(t, ScriptsForProfile(ProfilePersona), hookArcsScript)
	assert.Contains(t, ScriptsForProfile(ProfileFull), hookArcsScript)
	assert.NotContains(t, ScriptsForProfile(ProfileKnowledge), hookArcsScript, "a knowledge vault has no identity to recite")
}

func TestArcsHook_WiredForEveryAgentThatLoadsThePersona(t *testing.T) {
	assert.Equal(t, codexScripts[hookSessionStartScript], codexScripts[hookArcsScript], "Codex")
	assert.Equal(t, cursorWiring[hookSessionStartScript], cursorWiring[hookArcsScript], "Cursor")
}
