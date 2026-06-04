package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/spf13/cobra"
)

// runHooksUninstallCore is the core of `vaultmind hooks uninstall`. It strips
// VaultMind's hook entries from the project's settings file (and, with
// --remove-scripts, deletes the installed scripts), then emits a JSON envelope
// or a human-readable summary. Removal is surgical — only entries referencing
// our canonical scripts are touched (hooks.RemoveFromSettings).
func runHooksUninstallCore(cmd *cobra.Command, projectDir string, jsonOut, local, removeScripts bool) error {
	res, err := hooks.RemoveFromSettings(projectDir, local, removeScripts)

	w := cmd.OutOrStdout()
	if jsonOut {
		env := envelope.OK("hooks uninstall", res)
		if err != nil {
			env.Status = "error"
			env.Errors = append(env.Errors, envelope.Issue{
				Code:    "hooks_uninstall_error",
				Message: err.Error(),
			})
		}
		_ = json.NewEncoder(w).Encode(env)
		return err
	}

	if res != nil {
		_, _ = fmt.Fprintf(w, "Settings: %s\n", res.SettingsPath)
		if len(res.Removed) > 0 {
			_, _ = fmt.Fprintf(w, "\nRemoved %d VaultMind hook entries:\n", len(res.Removed))
			for _, name := range res.Removed {
				_, _ = fmt.Fprintf(w, "  - %s\n", name)
			}
		} else {
			_, _ = fmt.Fprintf(w, "\nNo VaultMind hook entries found — nothing to remove.\n")
		}
		if len(res.ScriptsDeleted) > 0 {
			_, _ = fmt.Fprintf(w, "\nDeleted %d script(s) from .claude/scripts/:\n", len(res.ScriptsDeleted))
			for _, name := range res.ScriptsDeleted {
				_, _ = fmt.Fprintf(w, "  - %s\n", name)
			}
		}
	}
	return err
}
