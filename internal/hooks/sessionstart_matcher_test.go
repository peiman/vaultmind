package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SessionStart was wired on "startup" only. After a compaction or a /clear
// the context starts over, and nothing put the vault map, the health line or
// the persona back. Claude Code adds SessionStart output to the context on
// "clear" and "compact" too; "resume" restores the history, which already
// holds the original output, so it stays out.

type sessionStartFile struct {
	Hooks struct {
		SessionStart []struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"SessionStart"`
	} `json:"hooks"`
}

// sessionStartMatchers maps each of our SessionStart scripts to the matchers
// of the groups that run it.
func sessionStartMatchers(t *testing.T, raw []byte) map[string][]string {
	t.Helper()
	var f sessionStartFile
	require.NoError(t, json.Unmarshal(raw, &f))
	out := map[string][]string{}
	for _, g := range f.Hooks.SessionStart {
		for _, h := range g.Hooks {
			for _, s := range []string{hookSessionStartScript, hookHealthScript} {
				if commandReferencesScript(h.Command, s) {
					out[s] = append(out[s], g.Matcher)
				}
			}
		}
	}
	return out
}

func TestSessionStart_FiresOnStartupClearAndCompact(t *testing.T) {
	stanza, err := SettingsStanza("")
	require.NoError(t, err)
	got := sessionStartMatchers(t, []byte(stanza))
	for _, s := range []string{hookSessionStartScript, hookHealthScript} {
		assert.Equal(t, []string{"startup|clear|compact"}, got[s], s)
	}
}

// An install from before this change has each script in a group of its own
// under "startup". The merge upgrades it in place.
const startupOnlySettings = `{"hooks": {"SessionStart": [
  {"matcher": "startup", "hooks": [{"type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/load-persona.sh"}]},
  {"matcher": "startup", "hooks": [{"type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/vaultmind-health.sh"}]}
]}}`

func TestMerge_UpgradesAStartupOnlySessionStart(t *testing.T) {
	out, changed, err := MergeStanza([]byte(startupOnlySettings), "")
	require.NoError(t, err)
	assert.True(t, changed)
	got := sessionStartMatchers(t, out)
	for _, s := range []string{hookSessionStartScript, hookHealthScript} {
		assert.Equal(t, []string{sessionStartMatcher}, got[s], s)
	}
}

func TestStatus_NamesAStartupOnlySessionStartAsStale(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude", "scripts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "settings.json"), []byte(startupOnlySettings), 0o600))

	report, err := Status(project)
	require.NoError(t, err)
	state := eventStates(report)
	assert.Equal(t, EventStaleMatcher, state[hookSessionStartScript])
	assert.Equal(t, EventStaleMatcher, state[hookHealthScript])
}

// eventStates maps each SessionStart script in report to its state.
func eventStates(report StatusReport) map[string]EventState {
	out := map[string]EventState{}
	for _, e := range report.Events {
		if e.Event == "SessionStart" {
			out[e.Script] = e.State
		}
	}
	return out
}

// A project that wires a script under several SessionStart groups by hand
// has chosen its sources. Widening its "startup" group would run the script
// twice on compact (once there, once in its own compact group).
func TestMerge_LeavesAHandWiredMultiSourceSessionStartAlone(t *testing.T) {
	hand := []byte(`{"hooks": {"SessionStart": [
  {"matcher": "compact", "hooks": [{"type": "command", "command": "x/load-persona.sh"}]},
  {"matcher": "startup", "hooks": [{"type": "command", "command": "x/load-persona.sh"}]},
  {"matcher": "startup|clear|compact", "hooks": [{"type": "command", "command": "x/vaultmind-health.sh"}]}
]}}`)
	out, _, err := MergeStanza(hand, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"compact", "startup"}, sessionStartMatchers(t, out)[hookSessionStartScript])

	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude", "scripts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "settings.json"), out, 0o600))
	report, err := Status(project)
	require.NoError(t, err)
	assert.Equal(t, EventWired, eventStates(report)[hookSessionStartScript], "hand-wired sources are the project's choice")
}

// A project's own script whose name ends in ours is not ours: it must not
// count as a second group, or status would call a stale install healthy
// while the merge (which matches names exactly) goes on to upgrade it.
func TestStatus_AForeignScriptWithASimilarNameIsNotASecondGroup(t *testing.T) {
	settings := `{"hooks": {"SessionStart": [
  {"matcher": "startup", "hooks": [{"type": "command", "command": "x/load-persona.sh"}]},
  {"matcher": "startup", "hooks": [{"type": "command", "command": "x/my-load-persona.sh"}]}
]}}`
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude", "scripts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "settings.json"), []byte(settings), 0o600))

	report, err := Status(project)
	require.NoError(t, err)
	assert.Equal(t, EventStaleMatcher, eventStates(report)[hookSessionStartScript])
}

// The multi-group rule holds for reach too: in two groups by hand, it is left
// as the project wired it.
func TestMerge_LeavesReachInTwoHandWiredGroupsAlone(t *testing.T) {
	hand := []byte(`{"hooks": {"PreToolUse": [
  {"matcher": "Bash", "hooks": [{"type": "command", "command": "x/vault-reach.sh"}]},
  {"matcher": "Write", "hooks": [{"type": "command", "command": "x/vault-reach.sh"}]}
]}}`)
	out, _, err := MergeStanza(hand, "")
	require.NoError(t, err)
	var p preToolUse
	require.NoError(t, json.Unmarshal(out, &p))
	var matchers []string
	for _, g := range p.Hooks.PreToolUse {
		if len(g.Hooks) > 0 && commandReferencesScript(g.Hooks[0].Command, hookReachScript) {
			matchers = append(matchers, g.Matcher)
		}
	}
	assert.Equal(t, []string{"Bash", "Write"}, matchers)
}

// Codex keeps "startup": its SessionStart sources are not verified, and the
// matcher is part of the hash Codex approves. A re-run must change nothing.
func TestCodex_SessionStartStaysOnStartupAndReRunsChangeNothing(t *testing.T) {
	project := t.TempDir()
	first, err := MergeIntoCodexHooks(project, "", ProfileFull, false)
	require.NoError(t, err)
	require.True(t, first.Changed)
	for s, m := range sessionStartMatchers(t, []byte(first.Merged)) {
		assert.Equal(t, []string{"startup"}, m, s)
	}

	again, err := MergeIntoCodexHooks(project, "", ProfileFull, false)
	require.NoError(t, err)
	assert.False(t, again.Changed, "an up-to-date Codex file must not be rewritten")
}
