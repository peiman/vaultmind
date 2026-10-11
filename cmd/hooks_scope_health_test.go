package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHooksScopeHealth_Overlap(t *testing.T) {
	for _, agent := range []string{"claude", "codex", "cursor"} {
		for _, first := range []string{"user", "project"} {
			t.Run(agent+"/"+first, func(t *testing.T) {
				home := userHooksHome(t)
				userSettings := filepath.Join(home, "."+agent, "hooks.json")
				if agent == "claude" {
					userSettings = filepath.Join(home, ".claude", "settings.json")
				}
				if agent == "codex" {
					codexHome := t.TempDir()
					t.Setenv("CODEX_HOME", codexHome)
					userSettings = filepath.Join(codexHome, "hooks.json")
				}
				project := t.TempDir()
				install := func(scope string, extra ...string) (string, error) {
					out, _, err := runRootCmd(t, append([]string{"hooks", "install", project, "--scope", scope, "--agent", agent, "--merge"}, extra...)...)
					return out.String(), err
				}
				_, err := install(first)
				require.NoError(t, err)
				second := "project"
				if first == "project" {
					second = "user"
				}
				preview, err := install(second, "--dry-run")
				require.NoError(t, err)
				assert.Contains(t, preview, "overlap")
				out, err := install(second)
				require.NoError(t, err)
				assert.Contains(t, out, "overlap")
				assert.Contains(t, out, "remove one scope's wiring")
				assert.Contains(t, out, "--only")
				for _, scope := range []string{"project", "user"} {
					out, _, err := runRootCmd(t, "hooks", "status", project, "--scope", scope)
					require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
					assert.Contains(t, out.String(), "overlap "+agent)
					if agent == "cursor" {
						assert.Contains(t, out.String(), "sessionStart -> vaultmind-health.sh")
						assert.Contains(t, out.String(), "postToolUse -> vault-code-map.sh, vault-reach.sh, vault-track-read.sh")
					} else {
						assert.Contains(t, out.String(), "UserPromptSubmit -> vault-recall.sh")
					}
					assert.Contains(t, out.String(), project)
					assert.Contains(t, out.String(), userSettings)
					out, _, err = runRootCmd(t, "hooks", "status", project, "--scope", scope, "--json")
					require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
					assert.Contains(t, out.String(), `"overlaps":[`)
				}
			})
		}
	}
}

func writeUserRecallCommand(t *testing.T, home, command string) string {
	t.Helper()
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &doc))
	var wiring map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(doc["hooks"], &wiring))
	wiring["UserPromptSubmit"], err = json.Marshal([]any{map[string]any{"hooks": []any{map[string]string{"type": "command", "command": command}}}})
	require.NoError(t, err)
	doc["hooks"], err = json.Marshal(wiring)
	require.NoError(t, err)
	data, err = json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestHooksScopeHealth_StaleGeneratedAndCustom(t *testing.T) {
	for _, command := range []string{`bash "$CLAUDE_PROJECT_DIR"/.claude/scripts/vault-recall.sh`, `bash '/missing/.vaultmind/scripts/vault-recall.sh'`, `bash /missing/scripts/vault-recall.sh --custom; echo keep-me`, `bash '/custom/scripts/vault-recall.sh'`} {
		t.Run(command, func(t *testing.T) {
			home := userHooksHome(t)
			_, _, err := runRootCmd(t, "hooks", "install", "--scope", "user", "--merge")
			require.NoError(t, err)
			path := writeUserRecallCommand(t, home, command)
			original, err := os.ReadFile(path)
			require.NoError(t, err)
			out, _, err := runRootCmd(t, "hooks", "status", "--scope", "user")
			require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
			assert.Contains(t, out.String(), "broken")
			assert.Contains(t, out.String(), command)
			assert.Contains(t, out.String(), "fix:")
			assert.Contains(t, out.String(), "0 unwired", "failure must be the wired path, not missing events")
			out, _, err = runRootCmd(t, "hooks", "install", "--scope", "user", "--merge", "--dry-run")
			custom := strings.Contains(command, "--custom") || strings.Contains(command, "/custom/")
			if custom {
				require.ErrorContains(t, err, "conflict")
				assert.Contains(t, out.String()+err.Error(), command)
			} else {
				require.NoError(t, err)
				assert.NotContains(t, out.String(), "$CLAUDE_PROJECT_DIR")
				assert.Contains(t, out.String(), ".vaultmind/scripts/vault-recall.sh")
			}
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, original, after, "preview writes nothing")
			_, _, err = runRootCmd(t, "hooks", "install", "--scope", "user", "--merge", "--force")
			if custom {
				require.ErrorContains(t, err, "conflict")
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, original, after, "even --force preserves custom commands")
			} else {
				require.NoError(t, err)
				out, _, err = runRootCmd(t, "hooks", "status", "--scope", "user")
				require.NoError(t, err, out.String())
			}
		})
	}
}

func TestHooksScopeHealth_OnlyAvoidsOverlap(t *testing.T) {
	userHooksHome(t)
	project := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", "--scope", "user", "--only", "vault-recall.sh", "--merge")
	require.NoError(t, err)
	out, _, err := runRootCmd(t, "hooks", "install", project, "--only", "vaultmind-health.sh", "--merge")
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "overlap")
	settings, err := os.ReadFile(filepath.Join(project, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(settings), "vault-recall.sh")
}

func TestHooksScopeHealth_ScriptMentionDoesNotOverlapOrBlockInstall(t *testing.T) {
	for _, command := range []string{"printf vault-recall.sh", "bash other.sh vault-recall.sh"} {
		t.Run(command, func(t *testing.T) {
			home := userHooksHome(t)
			project := t.TempDir()
			_, _, err := runRootCmd(t, "hooks", "install", project, "--merge")
			require.NoError(t, err)
			path := filepath.Join(home, ".claude", "settings.json")
			raw, err := json.Marshal(map[string]any{"hooks": map[string]any{"UserPromptSubmit": []any{map[string]any{"hooks": []any{map[string]string{"type": "command", "command": command}}}}}})
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
			require.NoError(t, os.WriteFile(path, raw, 0o600))
			out, _, err := runRootCmd(t, "hooks", "status", project)
			require.NoError(t, err, out.String())
			assert.NotContains(t, out.String(), "overlap claude")
			_, _, err = runRootCmd(t, "hooks", "install", "--scope", "user", "--merge")
			require.NoError(t, err)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(after), command)
		})
	}
}
