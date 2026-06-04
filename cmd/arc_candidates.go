package cmd

import (
	"encoding/json"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/distill"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/spf13/cobra"
)

var arcCandidatesCmd = MustNewCommand(commands.ArcCandidatesMetadata, runArcCandidates)

func init() {
	arcCmd.AddCommand(arcCandidatesCmd)
}

// runArcCandidates scans <vault>/episodes and prints the propose-only candidate
// report. It never writes arcs — the distill package has no arc writer.
func runArcCandidates(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppArcCandidatesVault)
	report, err := distill.ScanEpisodes(filepath.Join(vaultPath, "episodes"))
	if err != nil {
		return err
	}
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppArcCandidatesJson) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(envelope.OK("arc-candidates", report))
	}
	return distill.FormatReport(report, cmd.OutOrStdout())
}
