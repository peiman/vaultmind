package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reach hook moved from matcher "Bash" to Bash|Edit|Write|MultiEdit so it
// can stand in front of the tools arcs are written with. Review (2026-09-25)
// found no existing install would ever get it: the merge skips any group that
// already references the script, and status only checked that the script name
// appeared — so every adopter stayed on "Bash" with status reporting healthy.

// A reach group as an existing project has it: our script under the old
// matcher, with a timeout of the project's own that must survive the upgrade.
const legacyReachSettings = `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Read", "hooks": [{"type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/scripts/vault-track-read.sh"}]},
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "VAULTMIND_VAULTS='a,b' $CLAUDE_PROJECT_DIR/.claude/scripts/vault-reach.sh", "timeout": 15}]}
    ]
  }
}`

type preToolUse struct {
	Hooks struct {
		PreToolUse []struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"PreToolUse"`
	} `json:"hooks"`
}

func reachGroup(t *testing.T, raw []byte) (matcher, command string, timeout int) {
	t.Helper()
	var p preToolUse
	require.NoError(t, json.Unmarshal(raw, &p))
	for _, g := range p.Hooks.PreToolUse {
		if len(g.Hooks) > 0 && commandReferencesScript(g.Hooks[0].Command, hookReachScript) {
			return g.Matcher, g.Hooks[0].Command, g.Hooks[0].Timeout
		}
	}
	t.Fatal("no reach group")
	return "", "", 0
}

func TestMerge_UpgradesTheReachMatcherAndKeepsTheProjectsOwnSettings(t *testing.T) {
	out, changed, err := MergeStanza([]byte(legacyReachSettings), "")
	require.NoError(t, err)
	assert.True(t, changed)

	matcher, command, timeout := reachGroup(t, out)
	assert.Equal(t, reachMatcher, matcher, "the old Bash-only matcher is upgraded")
	assert.Contains(t, command, "VAULTMIND_VAULTS='a,b'", "the project's own command is kept")
	assert.Equal(t, 15, timeout, "the project's own timeout is kept")
}

// A matcher the project set by hand is theirs; only the known old value is ours to change.
func TestMerge_LeavesAHandSetReachMatcherAlone(t *testing.T) {
	custom := []byte(`{"hooks": {"PreToolUse": [{"matcher": "Bash|Edit", "hooks": [{"type": "command", "command": "x/vault-reach.sh"}]}]}}`)
	out, _, err := MergeStanza(custom, "")
	require.NoError(t, err)
	matcher, _, _ := reachGroup(t, out)
	assert.Equal(t, "Bash|Edit", matcher)
}

func TestStatus_NamesAReachHookStillOnTheOldMatcher(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude", "scripts"), 0o755))
	settings := filepath.Join(project, ".claude", "settings.json")
	require.NoError(t, os.WriteFile(settings, []byte(legacyReachSettings), 0o600))

	report, err := Status(project)
	require.NoError(t, err)
	state := map[string]EventState{}
	for _, e := range report.Events {
		state[e.Event+"/"+e.Script] = e.State
	}
	assert.Equal(t, EventStaleMatcher, state["PreToolUse/"+hookReachScript],
		"a reach hook that cannot see Edit or Write is not healthy")
	_, unwired := report.EventCounts()
	assert.Positive(t, unwired, "a stale matcher gates like an unwired event")

	upgraded, _, err := MergeStanza([]byte(legacyReachSettings), "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settings, upgraded, 0o600))
	report, err = Status(project)
	require.NoError(t, err)
	for _, e := range report.Events {
		if e.Script == hookReachScript {
			assert.Equal(t, EventWired, e.State, "after the merge it is healthy")
		}
	}
}

// A group holding the project's own hook next to ours is hand-wired: widening
// its matcher would run their hook on every Edit and Write too. It is left
// alone, and status keeps naming it so a person can split it.
func TestMerge_LeavesAMixedReachGroupAloneAndStatusStillNamesIt(t *testing.T) {
	mixed := `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [
	  {"type": "command", "command": "x/my-guard.sh"},
	  {"type": "command", "command": "x/vault-reach.sh"}]}]}}`
	out, _, err := MergeStanza([]byte(mixed), "")
	require.NoError(t, err)
	var p preToolUse
	require.NoError(t, json.Unmarshal(out, &p))
	require.NotEmpty(t, p.Hooks.PreToolUse)
	assert.Equal(t, "Bash", p.Hooks.PreToolUse[0].Matcher, "the project's own hook keeps its matcher")

	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude", "scripts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "settings.json"), out, 0o600))
	report, err := Status(project)
	require.NoError(t, err)
	for _, e := range report.Events {
		if e.Script == hookReachScript {
			assert.Equal(t, EventStaleMatcher, e.State)
		}
	}
}
