package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeOverlaps_EventScriptAndAgent(t *testing.T) {
	for _, tc := range []struct {
		name, event, command string
		agent                Agent
		overlap              bool
	}{
		{"same", "UserPromptSubmit", "bash /project/vault-recall.sh", AgentClaude, true},
		{"bare basename", "UserPromptSubmit", "bash vault-recall.sh", AgentClaude, true},
		{"printed basename", "UserPromptSubmit", "printf vault-recall.sh", AgentClaude, false},
		{"printed path", "UserPromptSubmit", "printf /project/vault-recall.sh", AgentClaude, false},
		{"script argument", "UserPromptSubmit", "bash other.sh vault-recall.sh", AgentClaude, false},
		{"script path argument", "UserPromptSubmit", "bash other.sh /project/vault-recall.sh", AgentClaude, false},
		{"generated env prefix", "UserPromptSubmit", "VAULTMIND_VAULT='/vault with spaces' bash '/project/vault-recall.sh'", AgentClaude, true},
		{"other event", "SessionStart", "bash /project/vault-recall.sh", AgentClaude, false},
		{"other script", "UserPromptSubmit", "bash /project/vault-reach.sh", AgentClaude, false},
		{"prefix", "UserPromptSubmit", "bash /project/my-vault-recall.sh", AgentClaude, false},
		{"suffix", "UserPromptSubmit", "bash /project/vault-recall.sh.bak", AgentClaude, false},
		{"other agent", "UserPromptSubmit", "bash /project/vault-recall.sh", AgentCodex, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, project := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
			write := func(path, event, command string) {
				t.Helper()
				b, err := json.Marshal(map[string]any{"hooks": map[string]any{event: []any{map[string]any{"hooks": []any{map[string]string{"command": command}}}}}})
				require.NoError(t, err)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
				require.NoError(t, os.WriteFile(path, b, 0o600))
			}
			write(settingsFilePath(home, false), "UserPromptSubmit", "bash /user/vault-recall.sh")
			path := settingsFilePath(project, true)
			if tc.agent == AgentCodex {
				path = CodexHooksPath(project)
			}
			write(path, tc.event, tc.command)
			out, err := ScopeOverlaps(project, home, nil, "")
			require.NoError(t, err)
			if tc.overlap {
				require.Len(t, out, 1)
				assert.Equal(t, []string{hookUserPromptSubmitScript}, out[0].Scripts)
				assert.Equal(t, path, out[0].ProjectSettings)
			} else {
				assert.Empty(t, out)
			}
		})
	}
}

func TestUserCommandProblem_Paths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quote's space", "vault-recall.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/bash\n"), 0o700))
	for _, tc := range []struct{ command, problem string }{
		{"bash " + singleQuote(path), ""},
		{`bash "` + path + `"`, ""},
		{`bash "$CLAUDE_PROJECT_DIR"/.claude/scripts/vault-recall.sh`, "must be absolute"},
		{"bash /missing/vault-recall.sh", "unavailable"},
		{"bash /missing/no-extension", "unavailable"},
		{"VAULTMIND_VAULT='/vault.sh' bash " + singleQuote(path), ""},
		{"printf vault-recall.sh", ""},
		{"printf /missing/vault-recall.sh", ""},
		{"bash " + singleQuote(path) + " /missing/vault-reach.sh", ""},
		{"zsh -f " + singleQuote(path), ""},
		{`bash -e "$CLAUDE_PROJECT_DIR"/.claude/scripts/vault-recall.sh`, "must be absolute"},
		{"bash " + singleQuote(dir), "not a regular file"},
		{"bash " + singleQuote(filepath.Join(dir, "directory.sh")), "unavailable"},
		{"bash relative/vault-recall.sh", "must be absolute"},
		{`bash 'unterminated/vault-recall.sh`, "cannot resolve"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			got := userCommandProblem(tc.command, AgentClaude)
			if tc.problem == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tc.problem)
			}
		})
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "directory.sh"), 0o750))
	assert.Contains(t, userCommandProblem("bash "+singleQuote(filepath.Join(dir, "directory.sh")), AgentClaude), "not a regular file")
}

func TestScriptsInCommand_ExecutedScript(t *testing.T) {
	for _, tc := range []struct {
		command string
		agent   Agent
		script  string
	}{
		{"printf vault-recall.sh", AgentClaude, ""},
		{"printf '/project/vault-recall.sh'", AgentClaude, ""},
		{"bash other.sh vault-recall.sh", AgentClaude, ""},
		{"/project/other.sh /project/vault-recall.sh", AgentClaude, ""},
		{"vault-recall.sh", AgentClaude, ""},
		{"/project/vault-recall.sh ignored.sh", AgentClaude, hookUserPromptSubmitScript},
		{"./vault-recall.sh", AgentClaude, hookUserPromptSubmitScript},
		{"VAULTMIND_VAULT='/vault with spaces' bash '/project/vault-recall.sh'", AgentClaude, hookUserPromptSubmitScript},
		{"A=one B='two' /bin/bash -eu -- /project/vault-recall.sh", AgentClaude, hookUserPromptSubmitScript},
		{"sh -e /project/vault-recall.sh", AgentClaude, hookUserPromptSubmitScript},
		{"zsh -f /project/vault-recall.sh", AgentClaude, hookUserPromptSubmitScript},
		{"bash -o errexit /project/vault-recall.sh", AgentClaude, hookUserPromptSubmitScript},
		{"bash -O extglob /project/vault-recall.sh", AgentClaude, hookUserPromptSubmitScript},
		{"bash --rcfile /project/vault-reach.sh /project/vault-recall.sh", AgentClaude, hookUserPromptSubmitScript},
		{"bash -c 'printf vault-recall.sh' /project/vault-recall.sh", AgentClaude, ""},
		{"bash -s /project/vault-recall.sh", AgentClaude, ""},
		{"bash -n /project/vault-recall.sh", AgentClaude, ""},
		{"bash --unknown /project/vault-recall.sh", AgentClaude, ""},
		{"bash -o vault-recall.sh", AgentClaude, ""},
		{"bash", AgentClaude, ""},
		{"", AgentClaude, ""},
		{"A=one", AgentClaude, ""},
		{`bash 'unterminated/vault-recall.sh`, AgentClaude, ""},
		{`"$SHELL" /project/vault-recall.sh`, AgentClaude, ""},
		{`bash "$SCRIPT"/vault-recall.sh`, AgentClaude, ""},
		{"bash /project/*/vault-recall.sh", AgentClaude, ""},
		{`bash "/project/\vault-recall.sh"`, AgentClaude, ""},
		{`$CLAUDE_PROJECT_DIR/.claude/scripts/vault-recall.sh`, AgentClaude, hookUserPromptSubmitScript},
		{"bash # /project/vault-recall.sh", AgentClaude, ""},
		{"bash /project/vault-recall.sh &&", AgentClaude, ""},
		{"bash /project/vault-recall.sh |", AgentClaude, ""},
		{"bash /project/vault-recall.sh ;;", AgentClaude, ""},
		{"bash\n/project/vault-recall.sh", AgentClaude, ""},
		{`bash "$CLAUDE_PROJECT_DIR"/.claude/scripts/vault-recall.sh`, AgentClaude, hookUserPromptSubmitScript},
		{`export VAULTMIND_PROJECT_DIR="$PWD"; bash '/project/vault-recall.sh'`, AgentCodex, hookUserPromptSubmitScript},
		{"bash /project/vault-recall.sh --custom; echo keep", AgentClaude, hookUserPromptSubmitScript},
		{"bash /project/cursor-adapter.sh vaultmind-health.sh", AgentCursor, hookHealthScript},
		{"printf /project/cursor-adapter.sh vaultmind-health.sh", AgentCursor, ""},
		{"bash other.sh /project/cursor-adapter.sh vaultmind-health.sh", AgentCursor, ""},
	} {
		t.Run(tc.command, func(t *testing.T) {
			got := scriptsInCommand(tc.command, tc.agent)
			if tc.script == "" {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, []string{tc.script}, got)
			}
		})
	}
}

func TestUserMerge_UnrelatedScriptMentionPreserved(t *testing.T) {
	for _, agent := range []Agent{AgentClaude, AgentCodex, AgentCursor} {
		for _, command := range []string{"printf vault-recall.sh", "printf /missing/vault-recall.sh", "bash other.sh vault-recall.sh", "printf /missing/cursor-adapter.sh vaultmind-health.sh", "printf '/missing/.vaultmind/scripts/cursor-adapter.sh' vaultmind-health.sh"} {
			t.Run(string(agent)+"/"+command, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
				path := wiringPaths(home, ScopeUser, agent)[0]
				entry := map[string]any{"hooks": []any{map[string]string{"type": "command", "command": command}}}
				if agent == AgentCursor {
					entry = map[string]any{"command": command}
				}
				raw, err := json.Marshal(map[string]any{"hooks": map[string]any{"SessionStart": []any{entry}}})
				require.NoError(t, err)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
				require.NoError(t, os.WriteFile(path, raw, 0o600))
				if agent == AgentClaude {
					_, err = mergeIntoSettingsForScope(home, "", nil, ProfileKnowledge, false, false, ScopeUser)
				} else {
					_, err = MergeUserAgentHooks(home, "", nil, ProfileKnowledge, agent, false)
				}
				require.NoError(t, err)
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				commands, err := commandsIn(after, path, agent)
				require.NoError(t, err)
				assert.Contains(t, commands, wiredCommand{"SessionStart", command, path})
			})
		}
	}
}

func TestGeneratedUserCommand_PreservesVaultAndRejectsCustom(t *testing.T) {
	home := t.TempDir()
	for _, agent := range []Agent{AgentClaude, AgentCodex, AgentCursor} {
		t.Run(string(agent), func(t *testing.T) {
			var command string
			if agent == AgentCursor {
				command = cursorHooks("/old", "/vault's space", []string{"/one", "/two"}, ProfileKnowledge)[0].Hook.Command
			} else if agent == AgentCodex {
				command = codexHooks("/old", "/vault's space", []string{"/one", "/two"}, ProfileKnowledge)[1].Group.Hooks[0].Command
			} else {
				command = canonicalHooksFor("/vault's space", []string{"/one", "/two"})[2].Group.Hooks[0].Command
			}
			out, ok := generatedUserCommand(command, home, agent)
			require.True(t, ok, command)
			assert.Contains(t, out, singleQuote("/vault's space"))
			assert.Contains(t, out, filepath.Join(home, ".vaultmind", "scripts"))
			assert.NotContains(t, out, "export VAULTMIND_PROJECT_DIR='/old'")
			for _, custom := range []string{command + " --custom", "echo keep; " + command, command + "; echo keep", "env CUSTOM=1 " + command} {
				_, ok := generatedUserCommand(custom, home, agent)
				assert.False(t, ok, custom)
			}
			_, ok = generatedUserCommand("bash '/custom/scripts/vault-recall.sh'", home, agent)
			assert.False(t, ok, "a custom script location is not generated wiring")
		})
	}
}

func TestUserCursorCustomAdapterCommand_Preserved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	command := "bash " + singleQuote(filepath.Join(home, ".vaultmind", "scripts", cursorAdapterScript)) + " custom.sh"
	raw, err := json.Marshal(map[string]any{"version": 1, "hooks": map[string]any{"sessionStart": []any{cursorHookEntry{Command: command}}}})
	require.NoError(t, err)
	path := CursorHooksPath(home)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	_, err = MergeUserAgentHooks(home, "", nil, ProfileKnowledge, AgentCursor, false)
	require.ErrorContains(t, err, "custom command conflict")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, raw, after)
}

func TestUserCursorMerge_ScriptMentionPreservedWithOnly(t *testing.T) {
	home := t.TempDir()
	command := "printf " + singleQuote(filepath.Join(home, ".vaultmind", "scripts", cursorAdapterScript)) + " " + hookHealthScript
	raw, err := json.Marshal(map[string]any{"hooks": map[string]any{"sessionStart": []any{cursorHookEntry{Command: command}}}})
	require.NoError(t, err)
	path := CursorHooksPath(home)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	_, err = MergeSelectedAgentHooks(home, "", nil, ProfileKnowledge, AgentCursor, ScopeUser, false, []string{hookHealthScript})
	require.NoError(t, err)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	commands, err := commandsIn(after, path, AgentCursor)
	require.NoError(t, err)
	assert.Contains(t, commands, wiredCommand{"sessionStart", command, path})
}

func TestScopeOverlaps_CursorQuotedAdapterAndFileAliases(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	user := `{"hooks":{"sessionStart":[{"command":"bash \"/user/.vaultmind/scripts/cursor-adapter.sh\" vaultmind-health.sh"}]}}`
	local := `{"hooks":{"sessionStart":[{"command":"bash '/project/.vaultmind/scripts/cursor-adapter.sh' vaultmind-health.sh"}]}}`
	for path, content := range map[string]string{CursorHooksPath(home): user, CursorHooksPath(project): local} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	out, err := ScopeOverlaps(project, home, nil, AgentCursor)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, []string{hookHealthScript}, out[0].Scripts)
	t.Chdir(home)
	out, err = ScopeOverlaps(".", home, nil, AgentCursor)
	require.NoError(t, err)
	assert.Empty(t, out, "one file must not overlap itself through a relative alias")
}

func TestUserCursorMerge_IdempotentFormatting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	stanza, err := UserAgentStanza(home, "", nil, ProfileKnowledge, AgentCursor)
	require.NoError(t, err)
	var value any
	require.NoError(t, json.Unmarshal([]byte(stanza), &value))
	compact, err := json.Marshal(value)
	require.NoError(t, err)
	path := CursorHooksPath(home)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, compact, 0o600))
	result, err := MergeUserAgentHooks(home, "", nil, ProfileKnowledge, AgentCursor, false)
	require.NoError(t, err)
	assert.False(t, result.Changed)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, compact, after)
}
