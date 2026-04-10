package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ExperimentsOptions returns config options for the experiments subsystem.
func ExperimentsOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "experiments", DefaultValue: map[string]any{}, Description: "Top-level experiment definitions map", Type: "map"},
		{Key: "experiments.telemetry", DefaultValue: "anonymous", Description: "Telemetry level: anonymous, full, off", Type: "string"},
		{Key: "experiments.outcome_window_sessions", DefaultValue: 2, Description: "Sessions to look back for outcome linkage", Type: "int"},
	}
}

func init() {
	config.RegisterOptionsProvider(ExperimentsOptions)
}
