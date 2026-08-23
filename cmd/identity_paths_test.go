package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// writePathsRegistry gives the resolver a real agents.yaml and points the
// process at it, the way chat-mcp and the watcher actually run.
func writePathsRegistry(t *testing.T, slug, project string) {
	t.Helper()
	dir := t.TempDir()
	reg := filepath.Join(dir, "agents.yaml")
	require.NoError(t, os.WriteFile(reg, []byte(
		"agents:\n  - slug: \""+slug+"\"\n    project_path: \""+project+"\"\n"), 0o600))
	t.Setenv("AGENT_CHAT_REGISTRY", reg)
	t.Setenv("AGENT_CHAT_PROJECT_PATH", project)
}

// TestIdentityPaths_ShellOutputIsEvalable pins the bootstrap contract the
// canonical mesh watcher depends on: the script's FIRST statement is
// eval "$(vaultmind identity paths)", and everything it knows about
// identity comes from that. If this output stops being valid shell, every
// watcher fails to arm — loudly, which is the design, but only if the failure
// is real rather than a quoting bug here.
//
// So the test does not eyeball the format: it runs the output through bash and
// reads the variables back out.
func TestIdentityPaths_ShellOutputIsEvalable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("VAULTMIND_MESH_DIR", t.TempDir())
	writePathsRegistry(t, "mira", t.TempDir())
	t.Setenv("AGENT_CHAT_DAEMON_URL", "http://100.64.22.69:8080")

	var out bytes.Buffer
	c := identityPathsCmd
	c.SetOut(&out)
	c.SetErr(&out)
	require.NoError(t, runIdentityPaths(c, nil))

	script := out.String() + "\nprintf '%s|%s|%s' \"$VM_MESH_SLUG\" \"$VM_MESH_SELF\" \"$VM_MESH_HEARTBEAT\"\n"
	res, err := exec.Command("bash", "-c", script).Output()
	require.NoError(t, err, "the default output must be directly evalable by bash")

	parts := strings.Split(string(res), "|")
	require.Len(t, parts, 3)
	require.Equal(t, "mira", parts[0])
	require.Equal(t, "agent:mira", parts[1])
	require.Equal(t, "mesh-watch-mira.heartbeat", filepath.Base(parts[2]))
}

// TestIdentityPaths_JSONCarriesTheSameFacts — the machine-readable twin, used
// by tests and by `listen`. Same derivation, second renderer.
func TestIdentityPaths_JSONCarriesTheSameFacts(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	meshDir := t.TempDir()
	t.Setenv("VAULTMIND_MESH_DIR", meshDir)
	writePathsRegistry(t, "workhorse", t.TempDir())
	t.Setenv("AGENT_CHAT_DAEMON_URL", "")

	var out bytes.Buffer
	c := identityPathsCmd
	c.SetOut(&out)
	c.SetErr(&out)
	require.NoError(t, c.Flags().Set("json", "true"))
	t.Cleanup(func() { _ = c.Flags().Set("json", "false") })
	require.NoError(t, runIdentityPaths(c, nil))

	var env struct {
		Result struct {
			Slug      string `json:"slug"`
			Self      string `json:"self"`
			Dir       string `json:"dir"`
			Heartbeat string `json:"heartbeat"`
			Lastarm   string `json:"lastarm"`
			DaemonURL string `json:"daemon_url"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.Equal(t, "workhorse", env.Result.Slug)
	require.Equal(t, "agent:workhorse", env.Result.Self)
	require.Equal(t, meshDir, env.Result.Dir)
	require.Equal(t, "mesh-watch-workhorse.lastarm", filepath.Base(env.Result.Lastarm))
	require.NotEmpty(t, env.Result.DaemonURL, "daemon URL must fall back to the default, never empty")
}

// TestIdentityPaths_NoSlugRefusesLoudly — the watcher's bootstrap treats a
// non-zero exit as "do not arm". An empty-but-zero answer here would arm a
// watcher with degenerate paths ("mesh-watch-.heartbeat") that doctor would then
// dutifully report on. Refusal must be an error, not a shrug.
func TestIdentityPaths_NoSlugRefusesLoudly(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("VAULTMIND_MESH_DIR", t.TempDir())
	t.Setenv("AGENT_CHAT_REGISTRY", filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv("AGENT_CHAT_PROJECT_PATH", t.TempDir())

	var out bytes.Buffer
	c := identityPathsCmd
	c.SetOut(&out)
	c.SetErr(&out)
	err := runIdentityPaths(c, nil)
	require.Error(t, err, "no resolvable slug must be a hard error")
	require.Contains(t, err.Error(), "slug")
}

// TestIdentityPaths_RegistryDefaultsWithoutEnv closes the gap the first arming
// attempt hit: with AGENT_CHAT_REGISTRY unset, the resolver looked at "" and
// refused — in a shell where the registry sat exactly where meshpaths says it
// lives. "An env var is a thing you can forget" was this feature's own design
// argument, and the first version built the forgettable half anyway. The env
// stays as an override; the default is the mesh dir's agents.yaml.
func TestIdentityPaths_RegistryDefaultsWithoutEnv(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	meshDir := t.TempDir()
	t.Setenv("VAULTMIND_MESH_DIR", meshDir)
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(meshDir, "agents.yaml"), []byte(
		"agents:\n  - slug: \"mira\"\n    project_path: \""+project+"\"\n"), 0o600))
	t.Setenv("AGENT_CHAT_REGISTRY", "") // deliberately unset
	t.Setenv("AGENT_CHAT_PROJECT_PATH", project)

	var out bytes.Buffer
	c := identityPathsCmd
	c.SetOut(&out)
	c.SetErr(&out)
	require.NoError(t, runIdentityPaths(c, nil),
		"the registry at its default location must resolve without any env")
	require.Contains(t, out.String(), "VM_MESH_SLUG='mira'")
}
