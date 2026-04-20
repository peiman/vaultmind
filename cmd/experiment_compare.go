package cmd

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/spf13/cobra"
)

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

// formatCompareResult moved to cmd/experiment_format.go.
