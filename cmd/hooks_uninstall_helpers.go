package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/spf13/cobra"
)

// runHooksUninstallCore is the core of `vaultmind hooks uninstall`. It strips
// VaultMind's hook entries from the project's settings file (and, with
// --remove-scripts, deletes the installed scripts), then emits a JSON envelope
// or a human-readable summary. Removal is surgical — only entries referencing
// our canonical scripts are touched (hooks.RemoveFromSettings).
func runHooksUninstallCore(cmd *cobra.Command, projectDir, agent string, jsonOut, local, removeScripts bool) error {
	res, scriptsDir, err := removeHooksFor(projectDir, agent, local, removeScripts)

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
			_, _ = fmt.Fprintf(w, "\nDeleted %d script(s) from %s/:\n", len(res.ScriptsDeleted), scriptsDir)
			for _, name := range res.ScriptsDeleted {
				_, _ = fmt.Fprintf(w, "  - %s\n", name)
			}
		}
	}
	return err
}

// removeHooksFor dispatches on --agent: Claude Code's settings file, or Codex's
// .codex/hooks.json. It returns the scripts folder (relative to the project)
// that --remove-scripts cleans, for the summary.
func removeHooksFor(projectDir, agent string, local, removeScripts bool) (*hooks.RemoveFileResult, string, error) {
	switch strings.TrimSpace(agent) {
	case "", hooksAgentClaude:
		res, err := hooks.RemoveFromSettings(projectDir, local, removeScripts)
		return res, filepath.Join(".claude", "scripts"), err
	case hooksAgentCodex:
		if local {
			return nil, "", fmt.Errorf("--local is a Claude Code settings option; Codex reads .codex/hooks.json")
		}
		res, err := hooks.RemoveFromCodexHooks(projectDir, removeScripts)
		return res, filepath.Join(".vaultmind", "scripts"), err
	default:
		return nil, "", fmt.Errorf("--agent %q: must be %q or %q", agent, hooksAgentClaude, hooksAgentCodex)
	}
}
