package cmd

import (
	"fmt"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/spf13/cobra"
)

var exportCmd = MustNewCommand(commands.ExportMetadata, runExport)

func init() {
	MustAddToRoot(exportCmd)
}

func runExport(cmd *cobra.Command, _ []string) error {
	tier := getConfigValueWithFlags[string](cmd, "tier", config.KeyAppExportTier)
	if tier == "" {
		tier = getConfigValueWithFlags[string](cmd, "", config.KeyExperimentsTelemetry)
	}
	if tier == "" {
		return fmt.Errorf("no telemetry tier set — run vaultmind interactively once to opt in, or pass --tier")
	}

	expDB, err := openExperimentDB()
	if err != nil {
		return fmt.Errorf("open experiment db: %w", err)
	}
	defer func() { _ = expDB.Close() }()

	w := cmd.OutOrStdout()
	if path := getConfigValueWithFlags[string](cmd, "output", config.KeyAppExportOutput); path != "" {
		f, err := os.Create(path) // #nosec G304 -- path is from --output flag (operator-controlled)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer func() { _ = f.Close() }()
		w = f
	}

	return experiment.ExportToJSONL(expDB, tier, w)
}
