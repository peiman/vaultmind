package hookscripts_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthHook_PluginReportsOriginalPATHVersion(t *testing.T) {
	for _, tc := range []struct {
		name, version, marker string
		other, alias, vault   bool
		emptyEntry            bool
		warn                  bool
	}{
		{name: "older binary before setup", version: "v0.10.16", marker: "plugin", other: true, warn: true},
		{name: "older binary with vault", version: "v0.10.16", marker: "plugin", other: true, vault: true, warn: true},
		{name: "empty PATH entry", version: "v0.10.16", marker: "plugin", other: true, emptyEntry: true, warn: true},
		{name: "newer binary", version: "v99.0.0", marker: "plugin", other: true, warn: true},
		{name: "same version", version: "v0.10.17", marker: "plugin", other: true},
		{name: "same version without v", version: "0.10.17", marker: "plugin", other: true},
		{name: "project hook", version: "v0.10.16", other: true, vault: true},
		{name: "no other binary", marker: "plugin"},
		{name: "wrapper symlink", marker: "plugin", alias: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "plugin with spaces")
			bin := filepath.Join(root, "bin")
			require.NoError(t, os.MkdirAll(bin, 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, ".claude-plugin/plugin.json"), []byte(`{"version":"0.10.17"}`), 0o644))
			require.NoError(t, os.Symlink(filepath.Join(pluginRoot(t), "bin/vaultmind"), filepath.Join(bin, "vaultmind")))
			data := t.TempDir()
			cache := filepath.Join(data, "0.10.17")
			require.NoError(t, os.MkdirAll(cache, 0o755))
			hookscripts.InstallStub(t, cache, "vaultmind", "#!/bin/bash\nif [ \"$1\" = doctor ]; then echo "+shellSingleQuote(healthyTier)+"; fi\n")
			originalBin := t.TempDir()
			candidate := filepath.Join(originalBin, "vaultmind")
			if tc.other {
				hookscripts.InstallStub(t, originalBin, "vaultmind", fmt.Sprintf("#!/bin/bash\nif [ \"$1\" = --version ]; then echo 'vaultmind version %s, commit abc, built today'; fi\n", tc.version))
			} else if tc.alias {
				require.NoError(t, os.Symlink(filepath.Join(bin, "vaultmind"), candidate))
			}
			// The hook prepends bin; Claude may also have appended it. A later
			// matching binary must not hide the first original PATH candidate.
			laterBin := t.TempDir()
			if tc.other {
				hookscripts.InstallStub(t, laterBin, "vaultmind", "#!/bin/bash\necho 'vaultmind version v0.10.17'\n")
			}
			originalEntry := originalBin
			if tc.emptyEntry {
				originalEntry = ""
				physicalBin, err := filepath.EvalSymlinks(originalBin)
				require.NoError(t, err)
				candidate = filepath.Join(physicalBin, "vaultmind")
			}
			path := strings.Join([]string{bin, originalEntry, bin, laterBin, healthHookPATH}, ":")
			project := t.TempDir()
			// Use the wrapper's cached binary without probing or downloading.
			env := []string{"CLAUDE_PLUGIN_ROOT=" + root, "CLAUDE_PLUGIN_DATA=" + data, "VAULTMIND_PLUGIN_PROBE=1", "VAULTMIND_HOOK_SOURCE=" + tc.marker, "HOME=" + t.TempDir()}
			if tc.vault {
				env = append(env, "VAULTMIND_VAULT="+project)
			}
			script, err := filepath.Abs("vaultmind-health.sh")
			require.NoError(t, err)
			cmd := exec.Command("bash", script)
			cmd.Dir = originalBin
			cmd.Env = append([]string{"PATH=" + path, "CLAUDE_PROJECT_DIR=" + project}, env...)
			cmd.Stdin = strings.NewReader(`{"session_id":"test"}`)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			stdout, err := cmd.Output()
			require.NoError(t, err, stderr.String())
			assert.Empty(t, stderr.String())
			out := string(stdout)
			if tc.warn {
				want := fmt.Sprintf("VaultMind: your PATH has vaultmind %s at %s; this plugin is 0.10.17. The agent's commands run the first one on PATH — upgrade or remove it (or run \"%s/vaultmind\" explicitly).", strings.TrimPrefix(tc.version, "v"), candidate, bin)
				assert.Contains(t, strings.Split(strings.TrimSpace(out), "\n"), want)
				assert.Equal(t, 1, strings.Count(out, "your PATH has vaultmind"))
			} else {
				assert.NotContains(t, out, "your PATH has vaultmind")
			}
			if tc.vault {
				assert.Contains(t, out, "full BGE-M3 hybrid", "health still uses the plugin binary")
			}
		})
	}
}

func TestPluginSetupChecksOwnManifestVersion(t *testing.T) {
	skill := string(pluginRead(t, "skills/vaultmind-setup/SKILL.md"))
	_, block, ok := strings.Cut(skill, "```bash\n")
	require.True(t, ok)
	block, _, ok = strings.Cut(block, "```")
	require.True(t, ok)
	assert.Contains(t, block, "vaultmind --version")
	assert.Contains(t, block, "${CLAUDE_PLUGIN_ROOT}/.claude-plugin/plugin.json")
	assert.NotContains(t, block, "vaultmind init", "verify the version before any setup")
	assert.Contains(t, skill, "stop before `init`")
	for _, version := range []string{pluginVersion(t), "99.0.0"} {
		t.Run(version, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "own plugin")
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, ".claude-plugin/plugin.json"), []byte(fmt.Sprintf(`{"version":%q}`, version)), 0o644))
			tools := t.TempDir()
			hookscripts.InstallStub(t, tools, "vaultmind", "#!/bin/bash\necho 'vaultmind version v"+pluginVersion(t)+", commit abc'\n")
			// Skill substitution happens before Bash; plugin path variables
			// are deliberately absent from the Bash environment.
			command := strings.ReplaceAll(block, "${CLAUDE_PLUGIN_ROOT}", root)
			cmd := exec.Command("bash", "-c", command)
			cmd.Env = []string{"PATH=" + tools + ":" + healthHookPATH}
			out, err := cmd.CombinedOutput()
			if version == pluginVersion(t) {
				require.NoError(t, err, string(out))
			} else {
				require.Error(t, err, "a different manifest version must stop setup")
				assert.Contains(t, string(out), version)
				assert.Contains(t, string(out), pluginVersion(t))
				assert.Contains(t, string(out), "upgrade or remove")
			}
		})
	}
}
