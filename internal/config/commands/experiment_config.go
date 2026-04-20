package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ExperimentReportMetadata defines metadata for the experiment report command.
var ExperimentReportMetadata = config.CommandMetadata{
	Use:          "report",
	Short:        "Show experiment metrics (Hit@K, MRR)",
	Long:         "Reads the local experiment database and computes Hit@K and MRR metrics for each variant in the specified experiment.",
	ConfigPrefix: "app.experimentreport",
	FlagOverrides: map[string]string{
		"app.experimentreport.experiment": "experiment",
		"app.experimentreport.json":       "json",
		"app.experimentreport.k":          "k",
	},
}

// ExperimentReportOptions returns config options for the experiment report command.
func ExperimentReportOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.experimentreport.experiment", DefaultValue: "", Description: "Experiment name to report on", Type: "string"},
		{Key: "app.experimentreport.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.experimentreport.k", DefaultValue: 10, Description: "K value for Hit@K metric", Type: "int"},
	}
}

// ExperimentSummaryMetadata defines metadata for the experiment summary command.
var ExperimentSummaryMetadata = config.CommandMetadata{
	Use:          "summary",
	Short:        "Memory usage overview: top recalled notes, session gap stats",
	Long:         "Reads the local experiment database and reports which notes have been recalled most, how many unique notes have been retrieved, and the distribution of gaps between sessions. Use this as the weekly readout on what your retrievals look like.",
	ConfigPrefix: "app.experimentsummary",
	FlagOverrides: map[string]string{
		"app.experimentsummary.json": "json",
		"app.experimentsummary.top":  "top",
	},
}

// ExperimentSummaryOptions returns config options for the experiment summary command.
func ExperimentSummaryOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.experimentsummary.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.experimentsummary.top", DefaultValue: 10, Description: "Number of top-recalled notes to show", Type: "int"},
	}
}

// ExperimentTraceMetadata defines metadata for the experiment trace command.
var ExperimentTraceMetadata = config.CommandMetadata{
	Use:          "trace",
	Short:        "Drill into a specific session or note's retrieval history",
	Long:         "Given --session <id>, reports that session's caller (agent + operator) and every retrieval it performed in order. Given --note <id>, reports every session that retrieved that note with caller attribution. Exactly one of --session or --note must be provided.",
	ConfigPrefix: "app.experimenttrace",
	FlagOverrides: map[string]string{
		"app.experimenttrace.session": "session",
		"app.experimenttrace.note":    "note",
		"app.experimenttrace.json":    "json",
	},
}

// ExperimentTraceOptions returns config options for the experiment trace command.
func ExperimentTraceOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.experimenttrace.session", DefaultValue: "", Description: "Session ID to trace", Type: "string"},
		{Key: "app.experimenttrace.note", DefaultValue: "", Description: "Note ID to trace across sessions", Type: "string"},
		{Key: "app.experimenttrace.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

// ExperimentCompareMetadata defines metadata for the experiment compare command.
var ExperimentCompareMetadata = config.CommandMetadata{
	Use:          "compare",
	Short:        "Compare shadow variants against the primary retrieval per event",
	Long:         "Reads recent ask/search/context-pack events that recorded shadow variants and computes pairwise rank-agreement metrics (Jaccard@K and Kendall's tau on shared items). Reports per-variant-pair means and event counts, optionally broken down per event. Requires no labeled gold truth — this surfaces where variants disagree, not which is correct.",
	ConfigPrefix: "app.experimentcompare",
	FlagOverrides: map[string]string{
		"app.experimentcompare.session":    "session",
		"app.experimentcompare.caller":     "caller",
		"app.experimentcompare.since":      "since",
		"app.experimentcompare.event_type": "event-type",
		"app.experimentcompare.k":          "k",
		"app.experimentcompare.per_event":  "per-event",
		"app.experimentcompare.json":       "json",
	},
}

// ExperimentCompareOptions returns config options for the experiment compare command.
func ExperimentCompareOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.experimentcompare.session", DefaultValue: "", Description: "Restrict to a single session ID", Type: "string"},
		{Key: "app.experimentcompare.caller", DefaultValue: "", Description: "Restrict to a single caller label", Type: "string"},
		{Key: "app.experimentcompare.since", DefaultValue: "", Description: "Only events at or after this RFC3339 timestamp", Type: "string"},
		{Key: "app.experimentcompare.event_type", DefaultValue: "", Description: "Restrict to one event type (ask|search|context_pack); empty = all three", Type: "string"},
		{Key: "app.experimentcompare.k", DefaultValue: 10, Description: "K value for Jaccard@K (and the cap on list length used for Kendall's tau)", Type: "int"},
		{Key: "app.experimentcompare.per_event", DefaultValue: false, Description: "Emit one row per event in addition to aggregates", Type: "bool"},
		{Key: "app.experimentcompare.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(ExperimentReportOptions)
	config.RegisterOptionsProvider(ExperimentSummaryOptions)
	config.RegisterOptionsProvider(ExperimentTraceOptions)
	config.RegisterOptionsProvider(ExperimentCompareOptions)
}
