package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var experimentReportCmd = MustNewCommand(commands.ExperimentReportMetadata, runExperimentReport)

func init() {
	experimentCmd.AddCommand(experimentReportCmd)
	setupCommandConfig(experimentReportCmd)
}

func runExperimentReport(cmd *cobra.Command, _ []string) error {
	expName := getConfigValueWithFlags[string](cmd, "experiment", config.KeyAppExperimentreportExperiment)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppExperimentreportJson)
	k := getConfigValueWithFlags[int](cmd, "k", config.KeyAppExperimentreportK)

	dbPath, err := xdg.DataFile("experiments.db")
	if err != nil {
		return fmt.Errorf("resolving experiment db path: %w", err)
	}
	expDB, err := experiment.Open(dbPath)
	if err != nil {
		if jsonOut {
			env := envelope.Error("experiment report", "db_error", err.Error(), "")
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		}
		return fmt.Errorf("opening experiment db: %w", err)
	}
	defer func() { _ = expDB.Close() }()

	var variants []string
	if expName != "" {
		expMap := viper.GetStringMap("experiments")
		exps := experiment.ParseExperiments(expMap)
		def, ok := exps[expName]
		if !ok {
			msg := fmt.Sprintf("experiment %q not found in config", expName)
			if jsonOut {
				env := envelope.Error("experiment report", "not_found", msg, "")
				return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
			}
			return errors.New(msg)
		}
		variants = def.AllVariants()
	} else {
		variants, err = expDB.DistinctVariants()
		if err != nil {
			return fmt.Errorf("querying variants: %w", err)
		}
	}

	if len(variants) == 0 {
		if jsonOut {
			env := envelope.OK("experiment report", map[string]any{"message": "no experiment data"})
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No experiment data found.")
		return err
	}

	report, err := expDB.Report(variants, k)
	if err != nil {
		return fmt.Errorf("computing report: %w", err)
	}

	if jsonOut {
		env := envelope.OK("experiment report", report)
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	return formatExperimentReport(report, expName, cmd.OutOrStdout())
}

func formatExperimentReport(report *experiment.ReportResult, expName string, w io.Writer) error {
	title := "all experiments"
	if expName != "" {
		title = expName
	}
	if _, err := fmt.Fprintf(w, "Experiment: %s (%d sessions, %d events, %d outcomes)\n\n",
		title, report.SessionCount, report.EventCount, report.OutcomeCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-25s Hit@%-5d MRR     Events\n",
		"Variant", report.K); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("-", 60)); err != nil {
		return err
	}
	for variant, metrics := range report.Variants {
		if _, err := fmt.Fprintf(w, "%-25s %-9.2f %-7.2f %d\n",
			variant, metrics.HitAtK, metrics.MRR, metrics.EventCount); err != nil {
			return err
		}
	}
	return nil
}
