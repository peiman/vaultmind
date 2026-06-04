package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/initvault"
	"github.com/peiman/vaultmind/internal/telemetry"
	"github.com/spf13/cobra"
)

// runInitScaffold runs the scaffold-a-vault flow. Split out from
// init.go's wiring so the wiring stays under the ≤30-line cap (ADR-001).
// The flag-handling lane (--print-instructions) lives in init.go;
// this file is the create-files lane. When p.wireHooks is set, the
// Claude Code hooks are provisioned after the scaffold (init_wire.go).
func runInitScaffold(cmd *cobra.Command, path string, p initWireParams) error {
	res, err := initvault.Init(path)
	if err != nil {
		return err
	}
	if _, err := telemetry.EnsureFingerprint(res.VaultPath); err != nil {
		return fmt.Errorf("generate fingerprint: %w", err)
	}
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "✅ Vault scaffolded at %s (%d files)\n\n", res.VaultPath, res.FilesAdded)

	if p.wireHooks {
		if err := wireInitHooks(w, res.VaultPath, p); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(w, "Next steps:\n")
	_, _ = fmt.Fprintf(w, "  cd %s\n", res.VaultPath)
	_, _ = fmt.Fprintf(w, "  vaultmind index --vault .\n")
	_, _ = fmt.Fprintf(w, "  vaultmind index --embed --vault .\n")
	_, _ = fmt.Fprintf(w, "  vaultmind ask \"who am I\" --vault .\n\n")
	_, _ = fmt.Fprintf(w, "Edit identity/who-am-i.md and references/current-context.md to make it yours.\n\n")
	_, _ = fmt.Fprintf(w, "If this vault lives in a git repo, add to .gitignore (the index is a\nregenerable cache; the type registry is source):\n")
	_, _ = fmt.Fprintf(w, "  .vaultmind/index.db*\n")
	_, _ = fmt.Fprintf(w, "  !.vaultmind/config.yaml\n\n")
	if !p.wireHooks {
		_, _ = fmt.Fprintf(w, "To wire Claude Code now: re-run with --wire-hooks, or\n")
		_, _ = fmt.Fprintf(w, "for agent-led setup (interview, project read, migration): vaultmind init --print-instructions\n")
	}
	return nil
}
