package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 0.9.4's upgrade note wired persona hooks into knowledge projects. Merging a
// narrower profile afterwards only ADDED, so the corrected command left them
// running and status called the project healthy. A merge for a profile now
// removes our own groups that fall outside it; a group that also holds the
// project's own hooks is left for a person, and status names it.
const damagedKnowledgeSettings = `{"hooks": {
  "SessionStart": [
    {"matcher": "startup", "hooks": [{"type": "command", "command": "x/load-persona.sh"}]},
    {"matcher": "startup", "hooks": [{"type": "command", "command": "x/their-own.sh"}, {"type": "command", "command": "x/capture-episode.sh"}]}
  ],
  "SessionEnd": [{"hooks": [{"type": "command", "command": "x/capture-episode.sh"}]}],
  "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "x/vault-recall.sh"}]}]
}}`

func TestMerge_ForAProfileRemovesOurHooksOutsideIt(t *testing.T) {
	dir := t.TempDir()
	settings := writeSettings(t, dir, "settings.json", damagedKnowledgeSettings)

	res, err := MergeIntoSettingsForProfile(dir, "", nil, ProfileKnowledge, false, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{hookSessionStartScript, hookSessionEndScript}, res.Removed)

	raw, err := os.ReadFile(settings)
	require.NoError(t, err)
	s := string(raw)
	assert.NotContains(t, s, `"x/load-persona.sh"`, "ours alone, outside the profile: removed")
	assert.Contains(t, s, "their-own.sh", "a group with the project's own hook is kept")
	assert.Contains(t, s, "vault-recall.sh", "inside the profile: kept")
}

func TestMerge_AFullProfileRemovesNothing(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, "settings.json", damagedKnowledgeSettings)
	res, err := MergeIntoSettingsForProfile(dir, "", nil, ProfileFull, false, false)
	require.NoError(t, err)
	assert.Empty(t, res.Removed)
}

func TestStatus_NamesHooksWiredOutsideTheDeclaredProfile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "scripts"), 0o755))
	writeSettings(t, dir, "settings.json", damagedKnowledgeSettings)
	require.NoError(t, writeDeclaredProfileFor(dir, AgentClaude, ProfileKnowledge))

	report, err := Status(dir)
	require.NoError(t, err)
	outside := map[string]bool{}
	for _, e := range report.Events {
		if e.State == EventOutsideProfile {
			outside[e.Script] = true
		}
	}
	assert.True(t, outside[hookSessionStartScript], "a persona loader in a knowledge project is named")
	_, unwired := report.EventCounts()
	assert.Positive(t, unwired, "and it gates")
}
