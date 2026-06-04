package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/fix"
	"github.com/spf13/cobra"
)

// runFrontmatterFixCore is the core of `vaultmind frontmatter fix`. Splits
// out from the wiring (frontmatter_fix.go) to keep the cmd file under the
// project's ≤30-line cap. Walks the vault, identifies missing vaultmind-
// owned fields, optionally applies. Emits JSON envelope or a human-
// readable summary depending on jsonOut.
func runFrontmatterFixCore(cmd *cobra.Command, vaultPath string, apply, jsonOut bool) error {
	res, err := fix.RunBackfill(fix.Config{
		VaultPath: vaultPath,
		Apply:     apply,
	})
	if err != nil {
		return fmt.Errorf("running fix: %w", err)
	}

	if jsonOut {
		env := envelope.OK("frontmatter fix", res)
		if len(res.Items) > 0 && !apply {
			env.Status = "warning"
		}
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	mode := "(dry-run)"
	if apply {
		mode = "(applied)"
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(),
		"Scanned %d files; %d need backfill %s\n",
		res.FilesScanned, len(res.Items), mode); err != nil {
		return err
	}
	for _, item := range res.Items {
		if item.Error != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(),
				"  [error] %s: %s\n", item.Path, item.Error); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(),
			"  %s: missing %v → %v\n",
			item.Path, item.MissingFields, item.ProposedValues); err != nil {
			return err
		}
	}
	if !apply && len(res.Items) > 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(),
			"\nDry-run only — re-run with --apply to write changes."); err != nil {
			return err
		}
	}
	return nil
}
