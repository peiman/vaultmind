package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/spf13/cobra"
)

// formatTau renders a *float64 tau as "%.3f" or "  nan" when undefined (nil).
func formatTau(t *float64) string {
	if t == nil {
		return "  nan"
	}
	return fmt.Sprintf("%.3f", *t)
}

var experimentCompareCmd = MustNewCommand(commands.ExperimentCompareMetadata, runExperimentCompare)

func init() {
	experimentCmd.AddCommand(experimentCompareCmd)
	setupCommandConfig(experimentCompareCmd)
}

type compareResult struct {
	Aggregates []experiment.AggregateRow `json:"aggregates"`
	PerEvent   []perEventRow             `json:"per_event,omitempty"`
}

type perEventRow struct {
	EventID        string   `json:"event_id"`
	PrimaryVariant string   `json:"primary_variant"`
	ShadowVariant  string   `json:"shadow_variant"`
	JaccardAtK     float64  `json:"jaccard_at_k"`
	KendallTau     *float64 `json:"kendall_tau"` // nil when shared_items < 2 (tau undefined)
	SharedItems    int      `json:"shared_items"`
}

func runExperimentCompare(cmd *cobra.Command, _ []string) error {
	session := getConfigValueWithFlags[string](cmd, "session", config.KeyAppExperimentcompareSession)
	caller := getConfigValueWithFlags[string](cmd, "caller", config.KeyAppExperimentcompareCaller)
	since := getConfigValueWithFlags[string](cmd, "since", config.KeyAppExperimentcompareSince)
	eventType := getConfigValueWithFlags[string](cmd, "event-type", config.KeyAppExperimentcompareEventType)
	k := getConfigValueWithFlags[int](cmd, "k", config.KeyAppExperimentcompareK)
	perEvent := getConfigValueWithFlags[bool](cmd, "per-event", config.KeyAppExperimentcomparePerEvent)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppExperimentcompareJson)

	dbPath, err := xdg.DataFile("experiments.db")
	if err != nil {
		return fmt.Errorf("resolving experiment db path: %w", err)
	}
	db, err := experiment.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening experiment db: %w", err)
	}
	defer func() { _ = db.Close() }()

	filter := experiment.ComparableEventFilter{
		SessionID:    session,
		Caller:       caller,
		SinceRFC3339: since,
	}
	if eventType != "" {
		filter.EventTypes = []string{eventType}
	}

	events, err := db.LoadComparableEvents(filter)
	if err != nil {
		return fmt.Errorf("loading comparable events: %w", err)
	}

	result := compareResult{Aggregates: experiment.AggregateComparisons(events, k)}
	if perEvent {
		for _, ev := range events {
			for _, p := range ev.Pairs {
				tau, shared := experiment.KendallTauShared(p.PrimaryList, p.ShadowList)
				row := perEventRow{
					EventID:        ev.EventID,
					PrimaryVariant: p.PrimaryVariant,
					ShadowVariant:  p.ShadowVariant,
					JaccardAtK:     experiment.JaccardAtK(p.PrimaryList, p.ShadowList, k),
					SharedItems:    shared,
				}
				if shared >= 2 && !math.IsNaN(tau) {
					row.KendallTau = &tau
				}
				result.PerEvent = append(result.PerEvent, row)
			}
		}
	}

	if jsonOut {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(envelope.OK("experiment compare", result))
	}
	return formatCompareResult(cmd.OutOrStdout(), result, k, perEvent)
}

func formatCompareResult(w io.Writer, r compareResult, k int, perEvent bool) error {
	if len(r.Aggregates) == 0 {
		_, err := fmt.Fprintln(w, "No comparable events found. Shadow variants may be disabled or no ask/search/context-pack events have been recorded.")
		return err
	}
	if _, err := fmt.Fprintf(w, "Variant disagreement (K=%d)\n\n", k); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  %-18s  %-18s  %6s  %11s  %11s  %9s\n",
		"primary", "shadow", "events", "meanJaccard", "meanKendall", "kendallN"); err != nil {
		return err
	}
	for _, a := range r.Aggregates {
		if _, err := fmt.Fprintf(w, "  %-18s  %-18s  %6d  %11.3f  %11s  %9d\n",
			a.PrimaryVariant, a.ShadowVariant, a.EventCount,
			a.MeanJaccardAtK, formatTau(a.MeanKendallTau), a.KendallEventCount); err != nil {
			return err
		}
	}
	if perEvent && len(r.PerEvent) > 0 {
		if _, err := fmt.Fprintln(w, "\nPer-event:"); err != nil {
			return err
		}
		for _, pe := range r.PerEvent {
			if _, err := fmt.Fprintf(w, "  %s  %s->%s  jaccard=%.3f  tau=%s  shared=%d\n",
				pe.EventID, pe.PrimaryVariant, pe.ShadowVariant,
				pe.JaccardAtK, formatTau(pe.KendallTau), pe.SharedItems); err != nil {
				return err
			}
		}
	}
	return nil
}
