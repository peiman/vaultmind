package hookscripts_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reach hook informs; it must never decide. In Claude Code a PreToolUse
// "permissionDecision": "allow" lets the tool run without the permission
// prompt the user's settings would show, so a hook meant to add context was
// auto-approving every git commit, git push, gh pr merge and vault write it
// fired on. With no decision the context still reaches the model and the
// normal permission flow applies.
func TestReachHook_NeverMakesAPermissionDecision(t *testing.T) {
	h := identityEnv(t)
	arc := filepath.Join(h.projectDir, "vaultmind-identity", "arcs", "a.md")

	for name, payload := range map[string]string{
		"push":        bashPayload("git push origin main"),
		"merge":       bashPayload("gh pr merge 12 --merge"),
		"commit":      bashPayload(`git commit -m "fix: something"`),
		"vault write": filePayload("Write", arc),
	} {
		out, _ := runHookScript(t, "vault-reach.sh", h.env(false), payload)
		require.NotEmpty(t, out, "%s: the hook must fire, or this test proves nothing", name)

		var got struct {
			HookSpecificOutput map[string]any `json:"hookSpecificOutput"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &got), "%s: output must be JSON: %s", name, out)
		assert.NotContains(t, got.HookSpecificOutput, "permissionDecision",
			"%s: an informing hook must leave the permission decision to the user's settings", name)
		assert.NotEmpty(t, got.HookSpecificOutput["additionalContext"], "%s: the context is still delivered", name)
	}
}
