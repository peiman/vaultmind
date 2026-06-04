package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ExperimentReportMetadata defines metadata for the experiment report command.
var ExperimentReportMetadata = config.CommandMetadata{
	Use:   "report",
	Short: "Measure retrieval quality: Hit@K and MRR per variant",
	Long: `Read the local experiment database and compute Hit@K and MRR for each
variant. These are absolute quality metrics that tell you how well a
retrieval variant performs when a labeled target exists.

METRICS

  Hit@K: Fraction of lookups where the target note appeared in the top K
         results (0.0 = never found, 1.0 = always in top K).
  MRR:   Mean Reciprocal Rank — average of 1/rank for each lookup where
         the target was found (rank 1 = 1.0, rank 2 = 0.5, rank 3 = 0.33).

WHEN TO USE

  Use "experiment report" when you want to know whether a variant is actually
  correct — not just whether two variants disagree. It requires events where
  a target note was labeled at retrieval time (e.g., spreading-activation
  lookups where the source note is known). Contrast with "experiment compare",
  which measures variant disagreement and needs no labeled ground truth.

FLAGS

  --experiment: restrict to a named experiment (string; default: all experiments).
  --k:          K value for Hit@K (int; default 10).
  --json:       emit JSON instead of a table (bool).

EXAMPLES

  vaultmind experiment report
      # Hit@K and MRR across all variants and experiments, K=10.

  vaultmind experiment report --k 5
      # Stricter threshold: target must appear in the top 5 to count.

  vaultmind experiment report --experiment spreading-activation-v2
      # Metrics only for the named experiment.

  vaultmind experiment report --experiment spreading-activation-v2 --json
      # Machine-readable output for scripting or logging.`,
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
	Use:   "summary",
	Short: "Memory usage overview: top recalled notes, session gap stats",
	Long: `Read the local experiment database and report which notes are recalled most
often, how many unique notes have been retrieved, and the distribution of
time gaps between sessions.

Use this as the weekly readout on whether recall is staying focused or
drifting. A small number of notes dominating the top list may signal
retrieval fixation; many unique notes with long gaps between sessions may
signal low vault engagement.

OUTPUT INCLUDES

  Top-N most-recalled notes (note id, title, recall count).
  Total unique notes retrieved across all sessions.
  Session gap distribution (min, median, max time between sessions).

FLAGS

  --top:  number of top-recalled notes to show (int; default 10).
  --json: emit JSON instead of a table (bool).

EXAMPLES

  vaultmind experiment summary
      # Overview with the top 10 recalled notes and session gap stats.

  vaultmind experiment summary --top 20
      # Widen to the top 20 notes to see if recall coverage is broad.

  vaultmind experiment summary --json
      # Machine-readable output for logging or scripting.`,
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
	Use:   "compare",
	Short: "Surface where retrieval variants disagree, without needing labeled ground truth",
	Long: `Read recent ask/search/context-pack events that recorded shadow variants and
compute pairwise rank-agreement metrics (Jaccard@K and Kendall's tau) for
each variant pair. Reports per-pair means and event counts, with an optional
per-event breakdown.

This command measures disagreement, not correctness. It answers: "do these
two variants tend to return different results?" without requiring any labeled
target notes. Contrast with "experiment report", which measures absolute
quality and requires labeled targets.

METRICS

  Jaccard@K:   Intersection over union of the top-K result sets from two
               variants (0 = no overlap, 1 = identical top-K). Use K to
               control how deep in the ranking you care about.
  Kendall tau: Rank correlation on items shared by both variants
               (-1 = fully reversed, 0 = uncorrelated, 1 = identical order).
               Undefined when fewer than 2 results are shared.

WHEN TO USE

  Use "experiment compare" when you have two or more retrieval variants
  (e.g., a new ranking model shadowing the primary) and want to measure how
  often they diverge, before investing in labeling ground truth for a full
  quality evaluation via "experiment report".

FLAGS

  --k:          top-K depth for Jaccard and the cap on list length for
                Kendall's tau (int; default 10).
  --per-event:  emit one row per event in addition to aggregates (bool).
  --since:      only include events at or after this RFC3339 timestamp (string).
  --session:    restrict to a single session ID (string).
  --caller:     restrict to a single caller label (string).
  --event-type: restrict to one event type: ask, search, or context_pack
                (string; default: all three).
  --json:       emit JSON instead of a table (bool).

EXAMPLES

  vaultmind experiment compare
      # Compare all variants across all recent events, K=10.

  vaultmind experiment compare --k 5
      # Stricter: only count a result as "in top K" if it is in the top 5.

  vaultmind experiment compare --since 2024-06-01T00:00:00Z
      # Only events on or after a cutoff date.

  vaultmind experiment compare --session abc123 --per-event
      # Restrict to one agent session and emit one row per event.

  vaultmind experiment compare --event-type ask --json
      # Only ask events, machine-readable output for scripting.`,
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
