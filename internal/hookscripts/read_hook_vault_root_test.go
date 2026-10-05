package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The read hooks find a note's vault by walking up from the note. A Codex or
// Cursor project keeps its hook scripts in .vaultmind/scripts/, and the walk
// took that folder for a vault, so a note in a project inside a vault was
// looked up in the project. A vault has .vaultmind/config.yaml or index.db,
// the rule `vaultmind`'s own discovery uses.
func TestReadHooks_WalkPastAScriptsOnlyDotVaultmind(t *testing.T) {
	for _, script := range []string{"vault-track-read.sh", "vault-block-read.sh"} {
		t.Run(script, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "argv.log")
			stub := "#!/bin/bash\nprintf '%s\\n' \"$@\" > " + logPath + "\necho 'body'\nexit 0\n"
			h := newHookEnv(t, stub)

			vault := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(vault, ".vaultmind"), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(vault, ".vaultmind", "config.yaml"), []byte("types: {}\n"), 0o600))
			project := filepath.Join(vault, "project")
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".vaultmind", "scripts"), 0o750))
			note := filepath.Join(project, "vaultmind-notes", "a.md")
			require.NoError(t, os.MkdirAll(filepath.Dir(note), 0o750))
			require.NoError(t, os.WriteFile(note, []byte("# a\n"), 0o600))

			stdin, err := json.Marshal(map[string]any{"tool_name": "Read", "tool_input": map[string]string{"file_path": note}})
			require.NoError(t, err)
			// vault-block-read exits 2 when it blocks a Read; that is its job.
			cmd := exec.Command("/bin/bash", script) //nolint:gosec // a hook script of this package
			cmd.Env, cmd.Stdin = h.env(false), strings.NewReader(string(stdin))
			if err := cmd.Run(); err != nil {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)
				require.Equal(t, 2, exitErr.ExitCode(), "only a block may exit non-zero")
			}

			raw, err := os.ReadFile(logPath) //nolint:gosec // temp path owned by this test
			require.NoError(t, err, "the hook never looked the note up")
			args := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
			for i, a := range args {
				if a == "--vault" {
					require.Equal(t, vault, args[i+1], "the note is looked up in the vault, not the project")
					return
				}
			}
			t.Fatalf("no --vault in %v", args)
		})
	}
}
