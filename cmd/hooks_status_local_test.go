package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHooksStatus_LocalInstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	for _, profile := range []hooks.Profile{hooks.ProfileFull, hooks.ProfileKnowledge, hooks.ProfilePersona} {
		t.Run(string(profile), func(t *testing.T) {
			project := t.TempDir()
			_, _, err := runRootCmd(t, "hooks", "install", project, "--profile", string(profile), "--merge", "--local")
			require.NoError(t, err)
			out, _, err := runRootCmd(t, "hooks", "status", project, "--json")
			require.NoError(t, err, out.String())
			var env struct {
				Result struct {
					Events []struct {
						Event, Script, State string
						Files                []string
					}
				}
			}
			require.NoError(t, json.Unmarshal(out.Bytes(), &env))
			require.Len(t, env.Result.Events, len(hooks.EventScriptsForProfile(profile)))
			for _, e := range env.Result.Events {
				assert.Equal(t, "wired", e.State, "%s -> %s", e.Event, e.Script)
				assert.Equal(t, []string{"settings.local.json"}, e.Files)
			}
			out, _, err = runRootCmd(t, "hooks", "status", project)
			require.NoError(t, err, out.String())
			assert.Contains(t, out.String(), "settings.local.json")
		})
	}
}

func TestHooksStatus_BothSettingsWarn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	project := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", project, "--merge", "--local")
	require.NoError(t, err)
	body, err := os.ReadFile(filepath.Join(project, ".claude", "settings.local.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "settings.json"), body, 0o600))
	out, _, err := runRootCmd(t, "hooks", "status", project)
	require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten, "overlapping hooks fail status like scope overlaps")
	assert.Contains(t, out.String(), "hooks can fire twice")
	assert.Contains(t, out.String(), "settings.json, settings.local.json")
	assert.Contains(t, out.String(), "0 unwired")
}

func TestHooksStatus_MalformedSettingsErrorSymmetrically(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	for _, name := range []string{"settings.json", "settings.local.json"} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude"), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", name), []byte(`{"hooks":`), 0o600))
			out, _, err := runRootCmd(t, "hooks", "status", project, "--json")
			require.Error(t, err)
			assert.Contains(t, out.String(), `"status":"error"`)
			assert.Contains(t, out.String(), `"code":"status_failed"`)
			assert.Contains(t, out.String(), name)
		})
	}
}

func TestHooksStatus_LocalStaleMatcherFix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	project := t.TempDir()
	_, _, err := runRootCmd(t, "hooks", "install", project, "--merge", "--local")
	require.NoError(t, err)
	path := filepath.Join(project, ".claude", "settings.local.json")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	old := strings.Replace(string(body), `"Bash|Edit|Write|MultiEdit"`, `"Bash"`, 1)
	require.NotEqual(t, string(body), old)
	require.NoError(t, os.WriteFile(path, []byte(old), 0o600))
	out, _, err := runRootCmd(t, "hooks", "status", project)
	require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
	assert.Contains(t, out.String(), "old matcher PreToolUse -> vault-reach.sh")
	assert.Contains(t, out.String(), "settings.local.json")
	assert.Contains(t, out.String(), "vaultmind hooks install "+project+" --merge --local")
}
