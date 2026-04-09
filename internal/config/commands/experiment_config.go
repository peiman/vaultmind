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

func init() {
	config.RegisterOptionsProvider(ExperimentReportOptions)
}
