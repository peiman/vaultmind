package hooks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mergeView is a permissive decode of a settings payload's hooks, used to
// assert structure without coupling tests to the exact emitted bytes.
type mergeView struct {
	Hooks struct {
		SessionStart     []mergeGroup `json:"SessionStart"`
		UserPromptSubmit []mergeGroup `json:"UserPromptSubmit"`
		PreToolUse       []mergeGroup `json:"PreToolUse"`
		SessionEnd       []mergeGroup `json:"SessionEnd"`
	} `json:"hooks"`
}

type mergeGroup struct {
	Matcher string `json:"matcher,omitempty"`
	Hooks   []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	} `json:"hooks"`
}

func (g mergeGroup) commands() []string {
	out := make([]string, 0, len(g.Hooks))
	for _, h := range g.Hooks {
		out = append(out, h.Command)
	}
	return out
}

func allCommands(groups []mergeGroup) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g.commands()...)
	}
	return out
}

func TestMergeStanza_EmptyInput_WritesFullStanza(t *testing.T) {
	out, changed, err := MergeStanza(nil, "")
	require.NoError(t, err)
	assert.True(t, changed, "fresh file must report a change")

	var v mergeView
	require.NoError(t, json.Unmarshal(out, &v), "output must be valid JSON")
	sessionStart := strings.Join(allCommands(v.Hooks.SessionStart), "\n")
	assert.Contains(t, sessionStart, hookSessionStartScript)
	assert.Contains(t, sessionStart, hookHealthScript, "SessionStart must wire both persona and health (P0)")
	assert.Contains(t, strings.Join(allCommands(v.Hooks.UserPromptSubmit), "\n"), hookUserPromptSubmitScript)
	assert.Contains(t, strings.Join(allCommands(v.Hooks.PreToolUse), "\n"), hookPreToolUseScript)
	assert.Contains(t, strings.Join(allCommands(v.Hooks.SessionEnd), "\n"), hookSessionEndScript)
	assert.Equal(t, "Read", v.Hooks.PreToolUse[0].Matcher, "Read matcher must be preserved")
}

// SessionStart is the only event carrying two canonical groups (persona +
// health). Merging into a file that already hand-wires ONLY persona must ADD
// health without duplicating persona — the two-group additive path the health
// hook introduced (review finding I1). The print path (SettingsStanza) is
// tested separately; this is the in-place merge that `hooks install --merge`
// actually runs.
func TestMergeStanza_AddsHealthToSessionStartWithoutDuplicatingPersona(t *testing.T) {
	existing := []byte(`{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/load-persona.sh"}]}]}}`)
	out, changed, err := MergeStanza(existing, "")
	require.NoError(t, err)
	assert.True(t, changed, "health hook must be added")

	var v mergeView
	require.NoError(t, json.Unmarshal(out, &v))
	joined := strings.Join(allCommands(v.Hooks.SessionStart), "\n")
	assert.Contains(t, joined, hookHealthScript, "health must be added")
	assert.Contains(t, joined, hookSessionStartScript, "persona must be preserved")

	// Idempotency: a second merge changes nothing and never duplicates persona.
	out2, changed2, err := MergeStanza(out, "")
	require.NoError(t, err)
	assert.False(t, changed2, "re-merge must be a no-op")
	var v2 mergeView
	require.NoError(t, json.Unmarshal(out2, &v2))
	personaCount := strings.Count(strings.Join(allCommands(v2.Hooks.SessionStart), "\n"), hookSessionStartScript)
	assert.Equal(t, 1, personaCount, "persona must not be duplicated across merges")
}

func TestMergeStanza_PreservesForeignHooksAndKeys(t *testing.T) {
	existing := []byte(`{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [ { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/scripts/their-existing-hook.sh" } ] }
    ]
  },
  "permissions": { "allow": ["Bash(ls:*)"] }
}`)

	out, changed, err := MergeStanza(existing, "")
	require.NoError(t, err)
	assert.True(t, changed)

	// Their UserPromptSubmit hook must survive AND ours must be added.
	var v mergeView
	require.NoError(t, json.Unmarshal(out, &v))
	ups := strings.Join(allCommands(v.Hooks.UserPromptSubmit), "\n")
	assert.Contains(t, ups, "their-existing-hook.sh", "foreign hook must be preserved")
	assert.Contains(t, ups, hookUserPromptSubmitScript, "our recall hook must be appended")
	assert.Len(t, v.Hooks.UserPromptSubmit, 2, "both entries coexist")

	// The other three events get added.
	assert.NotEmpty(t, v.Hooks.SessionStart)
	assert.NotEmpty(t, v.Hooks.PreToolUse)
	assert.NotEmpty(t, v.Hooks.SessionEnd)

	// Unknown top-level keys must be preserved.
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &top))
	assert.Contains(t, top, "permissions", "permissions key must survive the merge")
}

func TestMergeStanza_PreservesTopLevelKeyOrder(t *testing.T) {
	existing := []byte(`{
  "permissions": { "allow": ["Bash(ls:*)"] },
  "hooks": { "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "x/their.sh" } ] } ] },
  "disabledMcpjsonServers": ["rlm"]
}`)
	out, _, err := MergeStanza(existing, "")
	require.NoError(t, err)

	// Order of the three top-level keys must match the input order.
	permIdx := strings.Index(string(out), `"permissions"`)
	hooksIdx := strings.Index(string(out), `"hooks"`)
	mcpIdx := strings.Index(string(out), `"disabledMcpjsonServers"`)
	assert.True(t, permIdx < hooksIdx && hooksIdx < mcpIdx,
		"top-level key order must be preserved (permissions < hooks < disabledMcpjsonServers)")
}

func TestMergeStanza_Idempotent(t *testing.T) {
	once, changed1, err := MergeStanza(nil, "")
	require.NoError(t, err)
	assert.True(t, changed1)

	twice, changed2, err := MergeStanza(once, "")
	require.NoError(t, err)
	assert.False(t, changed2, "second merge must report no change")
	assert.Equal(t, string(once), string(twice), "second merge must be byte-identical")
}

func TestMergeStanza_DoesNotDuplicateManuallyWiredHook(t *testing.T) {
	// A project that already wired our recall hook by hand must not get a
	// duplicate, but should still get the other three events.
	existing := []byte(`{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [ { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/vault-recall.sh" } ] }
    ]
  }
}`)
	out, changed, err := MergeStanza(existing, "")
	require.NoError(t, err)
	assert.True(t, changed, "missing events were added")

	var v mergeView
	require.NoError(t, json.Unmarshal(out, &v))
	assert.Len(t, v.Hooks.UserPromptSubmit, 1, "recall hook must not be duplicated")
	assert.NotEmpty(t, v.Hooks.SessionStart, "missing SessionStart must be added")
}

func TestMergeStanza_BakesVaultPath(t *testing.T) {
	out, _, err := MergeStanza(nil, "/home/me/their-vault")
	require.NoError(t, err)
	assert.Contains(t, string(out), "VAULTMIND_VAULT='/home/me/their-vault'",
		"vault path must be baked into the merged commands")
}

func TestMergeStanza_MalformedJSON_ErrorsWithoutOutput(t *testing.T) {
	out, changed, err := MergeStanza([]byte(`{ "hooks": [unbalanced`), "")
	require.Error(t, err, "malformed settings must error rather than corrupt the file")
	assert.Nil(t, out, "no output on error — caller must not write")
	assert.False(t, changed)
}

func TestMergeStanza_HooksNotAnObject_Errors(t *testing.T) {
	out, changed, err := MergeStanza([]byte(`{"hooks": "not-an-object"}`), "")
	require.Error(t, err, "a non-object hooks value must error, not be silently clobbered")
	assert.Nil(t, out)
	assert.False(t, changed)
}

func TestMergeStanza_TrailingContent_Errors(t *testing.T) {
	out, changed, err := MergeStanza([]byte(`{"permissions": {}} TRAILING GARBAGE`), "")
	require.Error(t, err, "trailing content must error so re-emit never drops bytes")
	assert.Nil(t, out)
	assert.False(t, changed)
}

func TestMergeStanza_DuplicateTopLevelKey_Errors(t *testing.T) {
	out, _, err := MergeStanza([]byte(`{"permissions": {"a": 1}, "permissions": {"b": 2}}`), "")
	require.Error(t, err, "duplicate keys must error rather than silently drop one value")
	assert.Nil(t, out)
}

// TestMergeStanza_DoesNotMatchSimilarlyNamedUserScript proves the boundary
// match: a user's my-vault-recall.sh (prefix collision) must NOT be mistaken
// for our vault-recall.sh, so our hook is still installed alongside it.
func TestMergeStanza_DoesNotMatchSimilarlyNamedUserScript(t *testing.T) {
	existing := []byte(`{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [ { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/my-vault-recall.sh" } ] }
    ]
  }
}`)
	out, changed, err := MergeStanza(existing, "")
	require.NoError(t, err)
	assert.True(t, changed)

	var v mergeView
	require.NoError(t, json.Unmarshal(out, &v))
	assert.Len(t, v.Hooks.UserPromptSubmit, 2,
		"our recall hook must be added alongside the user's similarly-named one")
	ups := strings.Join(allCommands(v.Hooks.UserPromptSubmit), "\n")
	assert.Contains(t, ups, "my-vault-recall.sh", "user's script preserved")
	assert.Contains(t, ups, "/vault-recall.sh", "our script added")
}
