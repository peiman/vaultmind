package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// DoctorMetadata defines the metadata for the doctor command.
var DoctorMetadata = config.CommandMetadata{
	Use:          "doctor",
	Short:        "Vault health summary",
	Long:         "Run diagnostics on the vault index and report issues. By default prints summary counts plus per-link details (can be hundreds of lines on a noisy vault). Use --summary for counts only.",
	ConfigPrefix: "app.doctor",
	FlagOverrides: map[string]string{
		"app.doctor.vault":   "vault",
		"app.doctor.json":    "json",
		"app.doctor.summary": "summary",
	},
}

// DoctorOptions returns configuration options for the doctor command.
func DoctorOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.doctor.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.doctor.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.doctor.summary", DefaultValue: false, Description: "Print summary counts only (suppress per-link details)", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(DoctorOptions)
}
