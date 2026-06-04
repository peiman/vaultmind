package hooks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveStanza_RemovesOnlyOurEntries(t *testing.T) {
	// A project with its own UserPromptSubmit hook, plus our four wired in.
	base := []byte(`{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [ { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/scripts/their-existing-hook.sh" } ] }
    ]
  },
  "permissions": { "allow": ["Bash(ls:*)"] }
}`)
	merged, _, err := MergeStanza(base, "")
	require.NoError(t, err)

	out, removed, err := RemoveStanza(merged)
	require.NoError(t, err)

	// Their hook survives; ours are gone.
	assert.Contains(t, string(out), "their-existing-hook.sh", "foreign hook must survive uninstall")
	for _, name := range []string{hookSessionStartScript, hookUserPromptSubmitScript, hookPreToolUseScript, hookSessionEndScript} {
		assert.NotContains(t, string(out), name, "our script %q must be removed", name)
	}
	// The emptied SessionStart/PreToolUse/SessionEnd arrays must be dropped.
	var v mergeView
	require.NoError(t, json.Unmarshal(out, &v))
	assert.Len(t, v.Hooks.UserPromptSubmit, 1, "only the foreign entry remains")
	assert.Empty(t, v.Hooks.SessionStart)
	assert.Empty(t, v.Hooks.PreToolUse)
	assert.Empty(t, v.Hooks.SessionEnd)

	// Reported removals name our canonical scripts (both SessionStart entries
	// — persona loader and health nudge — plus recall/track/episode).
	assert.ElementsMatch(t,
		[]string{hookSessionStartScript, hookHealthScript, hookUserPromptSubmitScript, hookPreToolUseScript, hookSessionEndScript},
		removed)

	// Foreign top-level key preserved.
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &top))
	assert.Contains(t, top, "permissions")
}

func TestRemoveStanza_DropsEmptyHooksObject(t *testing.T) {
	// Settings whose only hooks are ours → the hooks key disappears entirely,
	// other keys survive.
	merged, _, err := MergeStanza([]byte(`{ "permissions": { "allow": [] } }`), "")
	require.NoError(t, err)

	out, _, err := RemoveStanza(merged)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &top))
	assert.NotContains(t, top, "hooks", "an emptied hooks object must be dropped")
	assert.Contains(t, top, "permissions", "unrelated keys survive")
}

func TestRemoveStanza_Idempotent(t *testing.T) {
	merged, _, err := MergeStanza(nil, "")
	require.NoError(t, err)

	once, removed1, err := RemoveStanza(merged)
	require.NoError(t, err)
	assert.NotEmpty(t, removed1)

	twice, removed2, err := RemoveStanza(once)
	require.NoError(t, err)
	assert.Empty(t, removed2, "second uninstall removes nothing")
	assert.Equal(t, string(once), string(twice), "second uninstall is byte-identical")
}

func TestRemoveStanza_NoHooksKey_IsNoOp(t *testing.T) {
	in := []byte(`{ "permissions": { "allow": ["Bash(ls:*)"] } }`)
	out, removed, err := RemoveStanza(in)
	require.NoError(t, err)
	assert.Empty(t, removed)
	assert.Equal(t, string(in), string(out), "no hooks → input returned verbatim")
}

func TestRemoveStanza_PreservesForeignTopLevelOrder(t *testing.T) {
	merged, _, err := MergeStanza([]byte(`{
  "permissions": { "allow": [] },
  "hooks": { "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "x/their.sh" } ] } ] },
  "disabledMcpjsonServers": ["rlm"]
}`), "")
	require.NoError(t, err)

	out, _, err := RemoveStanza(merged)
	require.NoError(t, err)

	permIdx := strings.Index(string(out), `"permissions"`)
	hooksIdx := strings.Index(string(out), `"hooks"`)
	mcpIdx := strings.Index(string(out), `"disabledMcpjsonServers"`)
	require.True(t, permIdx >= 0 && hooksIdx >= 0 && mcpIdx >= 0)
	assert.True(t, permIdx < hooksIdx && hooksIdx < mcpIdx, "top-level order preserved through uninstall")
}

func TestRemoveStanza_MalformedJSON_Errors(t *testing.T) {
	out, removed, err := RemoveStanza([]byte(`{ "hooks": [unbalanced`))
	require.Error(t, err)
	assert.Nil(t, out)
	assert.Empty(t, removed)
}

// TestRemoveStanza_DoesNotRemoveSimilarlyNamedUserScript proves the boundary
// match on the destructive path: a user's my-vault-recall.sh must survive
// uninstall — removing it would be silent data loss.
func TestRemoveStanza_DoesNotRemoveSimilarlyNamedUserScript(t *testing.T) {
	in := []byte(`{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [ { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/my-vault-recall.sh" } ] }
    ]
  }
}`)
	out, removed, err := RemoveStanza(in)
	require.NoError(t, err)
	assert.Empty(t, removed, "no VaultMind script matched")
	assert.Equal(t, string(in), string(out), "user's similarly-named hook must be untouched")
}

// TestRemoveStanza_PreservesMalformedGroupEntries proves a non-decodable entry
// in an event array is kept (we never remove what we can't understand).
func TestRemoveStanza_PreservesMalformedGroupEntries(t *testing.T) {
	in := []byte(`{"hooks": {"SessionStart": [42, {"hooks": [{"type": "command", "command": "bash /x/.claude/scripts/load-persona.sh"}]}]}}`)
	out, removed, err := RemoveStanza(in)
	require.NoError(t, err)
	assert.Equal(t, []string{hookSessionStartScript}, removed)
	assert.Contains(t, string(out), "42", "non-decodable entry must be preserved")
}
