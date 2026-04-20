package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/spf13/cobra"
)

var experimentSummaryCmd = MustNewCommand(commands.ExperimentSummaryMetadata, runExperimentSummary)

func init() {
	experimentCmd.AddCommand(experimentSummaryCmd)
	setupCommandConfig(experimentSummaryCmd)
}

func runExperimentSummary(cmd *cobra.Command, _ []string) error {
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppExperimentsummaryJson)
	top := getConfigValueWithFlags[int](cmd, "top", config.KeyAppExperimentsummaryTop)

	dbPath, err := xdg.DataFile("experiments.db")
	if err != nil {
		return fmt.Errorf("resolving experiment db path: %w", err)
	}
	expDB, err := experiment.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening experiment db: %w", err)
	}
	defer func() { _ = expDB.Close() }()

	summary, err := expDB.UsageSummary(top)
	if err != nil {
		return fmt.Errorf("computing usage summary: %w", err)
	}

	if jsonOut {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(envelope.OK("experiment summary", summary))
	}
	return formatUsageSummary(summary, cmd.OutOrStdout())
}

// formatUsageSummary, formatGap moved to cmd/experiment_format.go
// (consolidated with trace + compare formatters).
