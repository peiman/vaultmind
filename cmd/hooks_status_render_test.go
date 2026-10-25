package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderHooksStatusEvents_RepairsAllFiles(t *testing.T) {
	for _, state := range []hooks.EventState{hooks.EventStaleMatcher, hooks.EventOutsideProfile} {
		for _, tc := range []struct {
			name  string
			files []string
			flags []string
		}{
			{"shared", []string{"settings.json"}, []string{""}},
			{"local", []string{"settings.local.json"}, []string{" --local"}},
			{"both", []string{"settings.json", "settings.local.json"}, []string{"", " --local"}},
		} {
			t.Run(string(state)+"/"+tc.name, func(t *testing.T) {
				report := hooks.StatusReport{ProjectDir: "/tmp/p", Profile: hooks.ProfileKnowledge,
					Events: []hooks.EventStatus{{Event: "PreToolUse", Script: "vault-reach.sh", State: state, Files: tc.files}}}
				var out bytes.Buffer
				require.NoError(t, renderHooksStatusEvents(&out, report))
				assert.Equal(t, 1, strings.Count(out.String(), "PreToolUse -> vault-reach.sh"), "one line per event")
				assert.Contains(t, out.String(), "("+strings.Join(tc.files, ", ")+")")
				base := "vaultmind hooks install /tmp/p"
				label := "old matcher"
				if state == hooks.EventOutsideProfile {
					base += " --profile knowledge"
					label = "outside profile"
				}
				assert.Contains(t, out.String(), label+" PreToolUse -> vault-reach.sh")
				var commands []string
				for _, flag := range tc.flags {
					commands = append(commands, base+" --merge"+flag)
				}
				assert.Contains(t, out.String(), strings.Join(commands, "; "))
				if len(tc.flags) == 1 && tc.flags[0] == "" {
					assert.NotContains(t, out.String(), "--local")
				}
			})
		}
	}
}

func TestRenderWiringHealth_GroupsNotices(t *testing.T) {
	for _, warning := range []bool{false, true} {
		t.Run(map[bool]string{false: "notes", true: "warnings"}[warning], func(t *testing.T) {
			message := "wired in both settings files; Claude Code runs it once — keep one"
			if warning {
				message = "Warning: hooks can fire twice; remove one file's wiring. Neither file is removed automatically."
			}
			report := hooks.StatusReport{Events: []hooks.EventStatus{
				{Event: "SessionStart", Script: "load-persona.sh", Files: []string{"settings.json", "settings.local.json"}},
				{Event: "SessionEnd", Script: "capture-episode.sh", Files: []string{"settings.json", "settings.local.json"}},
			}}
			for i := range report.Events {
				if warning {
					report.Events[i].Warnings = []string{message}
				} else {
					report.Events[i].Notes = []string{message}
				}
			}
			var out bytes.Buffer
			require.NoError(t, renderWiringHealth(&out, report))
			assert.Equal(t, "  "+message+"\n"+
				"    SessionStart -> load-persona.sh (settings.json, settings.local.json)\n"+
				"    SessionEnd -> capture-episode.sh (settings.json, settings.local.json)\n", out.String())
			for n := range 3 {
				require.Error(t, renderWiringHealth(&failAfterNWriter{ok: n}, report), "write %d", n)
			}
		})
	}
}

func TestHooksStatus_BothFilesNeedRepair(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	for _, outside := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale", true: "outside profile"}[outside], func(t *testing.T) {
			project := t.TempDir()
			_, _, err := runRootCmd(t, "hooks", "install", project, "--profile", "full", "--merge")
			require.NoError(t, err)
			path := filepath.Join(project, ".claude", "settings.json")
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			base := "vaultmind hooks install " + project
			if outside {
				require.NoError(t, hooks.WriteDeclaredProfile(project, hooks.ProfileKnowledge))
				base += " --profile knowledge"
			} else {
				old := strings.Replace(string(body), `"Bash|Edit|Write|MultiEdit"`, `"Bash"`, 1)
				require.NotEqual(t, string(body), old)
				body = []byte(old)
			}
			require.NoError(t, os.WriteFile(path, body, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "settings.local.json"), body, 0o600))
			out, _, err := runRootCmd(t, "hooks", "status", project)
			require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
			assert.Contains(t, out.String(), base+" --merge; "+base+" --merge --local")
		})
	}
}
