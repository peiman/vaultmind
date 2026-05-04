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
// this file is the create-files lane.
func runInitScaffold(cmd *cobra.Command, path string) error {
	res, err := initvault.Init(path)
	if err != nil {
		return err
	}
	if _, err := telemetry.EnsureFingerprint(res.VaultPath); err != nil {
		return fmt.Errorf("generate fingerprint: %w", err)
	}
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "✅ Vault scaffolded at %s (%d files)\n\n", res.VaultPath, res.FilesAdded)
	_, _ = fmt.Fprintf(w, "Next steps:\n")
	_, _ = fmt.Fprintf(w, "  cd %s\n", res.VaultPath)
	_, _ = fmt.Fprintf(w, "  vaultmind index --vault .\n")
	_, _ = fmt.Fprintf(w, "  vaultmind index --embed --vault .\n")
	_, _ = fmt.Fprintf(w, "  vaultmind ask \"who am I\" --vault .\n\n")
	_, _ = fmt.Fprintf(w, "Edit identity/who-am-i.md and references/current-context.md to make it yours.\n\n")
	_, _ = fmt.Fprintf(w, "For agent-led setup (interview, project read, migration support, hooks),\n")
	_, _ = fmt.Fprintf(w, "run: vaultmind init --print-instructions\n")
	return nil
}
