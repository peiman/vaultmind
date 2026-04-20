package cmd

import (
	"encoding/json"
	"fmt"
	"io"

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

// formatUsageSummary renders a human-readable summary. Empty sections are
// silent rather than printing "0 notes" headers — a blank report beats a
// cluttered one.
func formatUsageSummary(s *experiment.UsageSummary, w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Sessions: %d    Retrieval events: %d    Unique notes recalled: %d\n",
		s.TotalSessions, s.RetrievalEventCount, s.UniqueNotesRecalled); err != nil {
		return err
	}

	if s.GapStats.Count > 0 {
		if _, err := fmt.Fprintf(w, "\nSession gaps (%d): median %s, p90 %s, max %s\n",
			s.GapStats.Count,
			formatGap(s.GapStats.MedianSeconds),
			formatGap(s.GapStats.P90Seconds),
			formatGap(s.GapStats.MaxSeconds)); err != nil {
			return err
		}
	}

	if len(s.TopNotes) > 0 {
		if _, err := fmt.Fprintf(w, "\nTop recalled notes:\n"); err != nil {
			return err
		}
		for _, n := range s.TopNotes {
			if _, err := fmt.Fprintf(w, "  %4d  %s  (last %s)\n",
				n.RetrievalCountTotal, n.NoteID, n.LastRetrievedTs); err != nil {
				return err
			}
		}
	}

	return nil
}

// formatGap renders a seconds count as "Ns" / "Nm" / "Nh" / "Nd" depending
// on magnitude. Compact output for the terminal; precise seconds are in JSON.
func formatGap(seconds int64) string {
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh", seconds/3600)
	default:
		return fmt.Sprintf("%dd", seconds/86400)
	}
}
